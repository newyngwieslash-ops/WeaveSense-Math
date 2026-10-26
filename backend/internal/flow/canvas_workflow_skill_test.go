package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

type canvasSkillTestAuthority struct {
	route   map[string]any
	skill   SkillRef
	revoked bool
}

func (a *canvasSkillTestAuthority) Authorize(_ context.Context, _ *gorm.DB, access CanvasWorkflowAccess) (CapabilityInput, error) {
	if a.revoked {
		return CapabilityInput{}, ErrForbidden
	}
	return CapabilityInput{Skills: []SkillRef{a.skill}, ModelRoute: a.route, PolicyHash: "local-approved-test"}, nil
}

func canvasSkillFixture(t *testing.T, response string, status int, statusURL ...string) (Tools, CanvasWorkflowSkillRequest, *atomic.Int32) {
	t.Helper()
	tools, engine, _, ss := setupP3(t)
	fund(t, Billing{DB: tools.DB}, "u_alice", 5_000_000)
	engine.evidence = []ResolvedEvidence{
		{EvidenceID: "fixture-monitor", Revision: 2, DocumentID: "fixture-document", SourceID: "monitor"},
		{EvidenceID: "fixture-factor", Revision: 1, DocumentID: "fixture-document", SourceID: "factor"},
		{EvidenceID: "not-in-input", Revision: 1, DocumentID: "fixture-document", SourceID: "other"},
	}
	skill, _ := FindSkill("regional-wastewater-modeling", "")
	if _, err := InstallSkill(tools.DB, ss.Scope, "u_alice", skill.ID, skill.Version); err != nil {
		t.Fatal(err)
	}
	calls := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/chat/completions" || r.Header.Get("Idempotency-Key") == "" || r.Header.Get("X-Flow-Reservation-Id") == "" {
			t.Error("missing managed gateway metadata")
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": response}}}, "usage": map[string]any{"total_tokens": 100}})
	}))
	t.Cleanup(srv.Close)
	tools.Model = &OpenAICompatGateway{URL: srv.URL, Model: "local-test", MicroPer1KTok: 1000, HTTP: srv.Client()}
	if err := tools.DB.AutoMigrate(&model.CanvasWorkflowExecution{}); err != nil {
		t.Fatal(err)
	}
	if _, err := (Billing{DB: tools.DB}).Account(tools.DB, ss.Scope.TenantID); err != nil {
		t.Fatal(err)
	}
	if err := tools.DB.Create(&model.CanvasProject{ID: "skill-canvas", UserID: "u_alice", Revision: 1, PayloadJSON: `{"nodes":[{"id":"skill-node","type":"skill"}],"connections":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	authority := &canvasSkillTestAuthority{skill: skill.Ref(), route: map[string]any{"flow_model_gateway": map[string]any{"model": "local-test", "url_hash": canvasSkillDigest([]byte(srv.URL)), "pricing_hash": hashJSON(int64(1000))}}}
	route := authority.route["flow_model_gateway"].(map[string]any)
	route["micro_per_1k_tokens"] = int64(1000)
	if len(statusURL) > 0 {
		route["status_url_hash"] = canvasSkillDigest([]byte(statusURL[0]))
	}
	executions := &CanvasWorkflowExecutions{DB: tools.DB, Authority: authority}
	rev := int64(1)
	run, err := executions.Admit(context.Background(), CanvasWorkflowAdmission{UserID: "u_alice", TenantID: ss.Scope.TenantID, WorkspaceID: ss.Scope.WorkspaceID, CanvasID: "skill-canvas", ExecutionKey: "skill-run", ExpectedRevision: &rev, BudgetMicro: 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	token, err := executions.Acquire(context.Background(), "u_alice", run.ID, "skill-worker")
	if err != nil {
		t.Fatal(err)
	}
	req := CanvasWorkflowSkillRequest{Executions: executions, WorkerID: "skill-worker", FencingToken: token, ModelID: "local-test", UserID: "u_alice", RunID: run.ID, NodeID: "skill-node",
		ApprovedSkill: skill.Ref(), Input: substanceInput(t, skill.ID), PreviousOutput: map[string]any{"old": "not input"}, MaterialScope: MaterialScope{DocumentIDs: []string{"fixture-document"}}, BudgetMicro: 100_000, MaxTokens: 4096}
	return tools, req, calls
}

func TestCanvasWorkflowSkillRejectsInvalidOutputsWithoutRerun(t *testing.T) {
	valid, _ := json.Marshal(substanceOutput(t, "regional-wastewater-modeling"))
	cases := map[string]string{"invalid_json": "{", "missing_fields": `{"title":"missing"}`, "nonfinite": strings.Replace(string(valid), `:23`, `:1e999`, 1), "fake_reference": strings.ReplaceAll(string(valid), "fixture-monitor", "fabricated"), "visible_but_not_input": strings.ReplaceAll(string(valid), "fixture-factor", "not-in-input"), "null": "null", "trailing": string(valid) + ` {}`, "duplicate_key": strings.Replace(string(valid), `"title":`, `"title":"hidden","title":`, 1)}
	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			tools, req, calls := canvasSkillFixture(t, response, 200)
			first, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
			if !errors.Is(err, ErrCanvasSkillOutput) || first.Output != nil || first.Status != "output_invalid" || first.TaskStatus != "succeeded" {
				t.Fatalf("%+v %v", first, err)
			}
			second, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
			if err == nil || second.TaskID != first.TaskID || calls.Load() != 1 {
				t.Fatalf("reran invalid paid output %+v calls=%d err=%v", second, calls.Load(), err)
			}
		})
	}
}

func TestCanvasWorkflowSkillAdmissionAndDispatchGates(t *testing.T) {
	raw, _ := json.Marshal(substanceOutput(t, "regional-wastewater-modeling"))
	for _, name := range []string{"input_schema", "input_nan", "fake_input", "wrong_revision", "unapproved_skill", "revoked", "route", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			tools, req, calls := canvasSkillFixture(t, string(raw), 200)
			ctx := context.Background()
			switch name {
			case "input_schema":
				delete(req.Input.(map[string]any), "monitoring_records")
			case "input_nan":
				req.Input.(map[string]any)["not_finite"] = math.NaN()
			case "fake_input":
				req.Input.(map[string]any)["monitoring_records"].([]any)[0].(map[string]any)["citations"] = []any{map[string]any{"evidence_id": "fake", "revision": 1}}
			case "wrong_revision":
				req.Input.(map[string]any)["monitoring_records"].([]any)[0].(map[string]any)["citations"] = []any{map[string]any{"evidence_id": "fixture-monitor", "revision": 1}}
			case "unapproved_skill":
				req.ApprovedSkill.Hash = "different"
			case "revoked":
				req.Executions.Authority.(*canvasSkillTestAuthority).revoked = true
			case "route":
				tools.Model.(*OpenAICompatGateway).Model = "other"
			case "cancelled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			result, err := tools.ExecuteCanvasWorkflowSkill(ctx, req)
			if err == nil || result.Output != nil || result.Status == "succeeded" || calls.Load() != 0 {
				t.Fatalf("%+v calls=%d err=%v", result, calls.Load(), err)
			}
		})
	}
}

func TestCanvasWorkflowSkillUnknownFailedAndTimeout(t *testing.T) {
	for _, code := range []int{400, 503} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			tools, req, calls := canvasSkillFixture(t, "", code)
			result, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
			if err == nil || result.Output != nil || result.Status == "succeeded" {
				t.Fatalf("%+v %v", result, err)
			}
			again, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
			if err == nil || again.TaskID != result.TaskID || calls.Load() != 1 {
				t.Fatalf("retry %+v %v calls=%d", again, err, calls.Load())
			}
		})
	}
	t.Run("timeout", func(t *testing.T) {
		tools, req, _ := canvasSkillFixture(t, "", 200)
		// A real local HTTP call times out after dispatch; its outcome is unknown.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(100 * time.Millisecond) }))
		defer srv.Close()
		gw := tools.Model.(*OpenAICompatGateway)
		gw.HTTP = &http.Client{Timeout: 10 * time.Millisecond, Transport: canvasSkillRedirectTransport{target: srv.URL, base: srv.Client().Transport}}
		result, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
		if !errors.Is(err, ErrModelUnknownOutcome) || result.Status != StatusReconciliation || result.Output != nil {
			t.Fatalf("%+v %v", result, err)
		}
	})
}

type canvasSkillRedirectTransport struct {
	target string
	base   http.RoundTripper
}

func (tr canvasSkillRedirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	target, _ := http.NewRequestWithContext(r.Context(), r.Method, tr.target+r.URL.Path, r.Body)
	clone.URL = target.URL
	if tr.base == nil {
		return http.DefaultTransport.RoundTrip(clone)
	}
	return tr.base.RoundTrip(clone)
}

func TestCanvasWorkflowSkillWorkerRechecksPersistedAuthority(t *testing.T) {
	for _, name := range []string{"missing_authority", "revoked", "uninstalled", "route_drift", "tampered_request", "cancelled_run"} {
		t.Run(name, func(t *testing.T) {
			tools, req, calls := canvasSkillFixture(t, "", 503)
			result, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
			if !errors.Is(err, ErrModelUnknownOutcome) {
				t.Fatal(err)
			}
			tasks := ModelTasks{DB: tools.DB, GW: tools.Model, CanvasExecutions: req.Executions}
			if err := tools.DB.Model(&model.FlowModelTask{}).Where("id = ?", result.TaskID).Updates(map[string]any{"status": "queued"}).Error; err != nil {
				t.Fatal(err)
			}
			switch name {
			case "missing_authority":
				tasks.CanvasExecutions = nil
			case "revoked":
				req.Executions.Authority.(*canvasSkillTestAuthority).revoked = true
			case "uninstalled":
				tools.DB.Where("skill_id = ?", req.ApprovedSkill.ID).Delete(&model.FlowSkillInstall{})
			case "route_drift":
				tools.Model.(*OpenAICompatGateway).URL = "http://127.0.0.1:1"
			case "tampered_request":
				tools.DB.Model(&model.FlowModelTask{}).Where("id = ?", result.TaskID).Update("request", `{"system":"injected","user":"injected","max_tokens":1}`)
			case "cancelled_run":
				tools.DB.Model(&model.CanvasWorkflowExecution{}).Where("id = ?", req.RunID).Update("status", "cancelled")
			}
			if _, err := tasks.RunTask(context.Background(), result.TaskID); err == nil {
				t.Fatal("worker accepted invalid persisted task")
			}
			if calls.Load() != 1 {
				t.Fatalf("dispatched after revocation: %d", calls.Load())
			}
		})
	}
}

func TestCanvasReviewSensitivePayloadNeverPersists(t *testing.T) {
	for _, scenario := range []string{"prior", "result"} {
		t.Run(scenario, func(t *testing.T) {
			output := substanceOutput(t, "regional-wastewater-modeling")
			if scenario == "result" { output["apiKey"] = "review-secret" }
			raw, _ := json.Marshal(output)
			tools, req, calls := canvasSkillFixture(t, string(raw), 200)
			if scenario == "prior" { req.PreviousOutput = map[string]any{"apiKey": "review-secret"} }
			_, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
			if err == nil { t.Fatal("sensitive payload accepted") }
			var tasks []model.FlowModelTask
			if err := tools.DB.Find(&tasks).Error; err != nil { t.Fatal(err) }
			for _, task := range tasks {
				if strings.Contains(task.Request+task.Result+task.Context+task.LastError, "review-secret") { t.Fatal("secret persisted") }
			}
			if scenario == "prior" && calls.Load() != 0 { t.Fatal("sensitive input sent") }
			if scenario == "result" {
				_, _ = tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
				if calls.Load() != 1 { t.Fatal("paid rejected output retried") }
			}
		})
	}
}

func TestCanvasReviewBudgetRejectsBeforeHTTP(t *testing.T) {
	raw, _ := json.Marshal(substanceOutput(t, "regional-wastewater-modeling"))
	tools, req, calls := canvasSkillFixture(t, string(raw), 200)
	req.BudgetMicro = 1
	_, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
	var count int64
	tools.DB.Model(&model.FlowModelTask{}).Count(&count)
	if !errors.Is(err, ErrBudgetInsufficient) || calls.Load() != 0 || count != 0 {
		t.Fatalf("underfunded request dispatched: err=%v calls=%d tasks=%d", err, calls.Load(), count)
	}
}

func TestCanvasReviewReconcileFrozenIdentityPriceAndRevocation(t *testing.T) {
	for _, scenario := range []string{"frozen_price_revoked", "wrong_endpoint", "missing_endpoint", "not_found_revoked", "not_found_authorized"} {
		t.Run(scenario, func(t *testing.T) {
			var queries atomic.Int32
			answer := "succeeded"
			if strings.HasPrefix(scenario, "not_found") || scenario == "wrong_endpoint" { answer = "not_found" }
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				queries.Add(1)
				if r.URL.Query().Get("idempotency_key") == "" { t.Error("missing task key") }
				_ = json.NewEncoder(w).Encode(map[string]any{"status": answer, "total_tokens": 100, "content": `{"late":true}`})
			}))
			defer srv.Close()
			frozenURL := srv.URL
			if scenario == "wrong_endpoint" { frozenURL += "/original" }
			if scenario == "missing_endpoint" { frozenURL = "" }
			tools, req, calls := canvasSkillFixture(t, "", 503, frozenURL)
			result, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
			if !errors.Is(err, ErrModelUnknownOutcome) { t.Fatal(err) }
			if scenario == "frozen_price_revoked" || scenario == "not_found_revoked" { req.Executions.Authority.(*canvasSkillTestAuthority).revoked = true }
			tasks := ModelTasks{DB: tools.DB, GW: tools.Model, CanvasExecutions: req.Executions, Status: ModelStatus{URL: srv.URL, MicroPer1KTok: 2000, HTTP: srv.Client()}}
			if _, err := tasks.Reconcile(context.Background(), 0); err != nil { t.Fatal(err) }
			task, err := tasks.Get(result.TaskID)
			if err != nil { t.Fatal(err) }
			var reservation model.FlowBudgetReservation
			if err := tools.DB.First(&reservation, "id = ?", task.ReservationID).Error; err != nil { t.Fatal(err) }
			switch scenario {
			case "frozen_price_revoked":
				if task.Status != "succeeded" || task.CostMicro != 100 || reservation.ConsumedMicro != 100 { t.Fatalf("not frozen: task=%+v reservation=%+v", task, reservation) }
			case "wrong_endpoint", "missing_endpoint":
				if queries.Load() != 0 || task.Status != StatusReconciliation { t.Fatalf("queried unrelated endpoint: queries=%d status=%s", queries.Load(), task.Status) }
			case "not_found_revoked":
				if task.Status == StatusRetryWait || task.Status == "queued" { t.Fatalf("revoked task requeued: %s", task.Status) }
			case "not_found_authorized":
				if task.Status != StatusRetryWait || reservation.Status != "reserved" { t.Fatalf("authorized retry lost: %s/%s", task.Status, reservation.Status) }
			}
			if calls.Load() != 1 { t.Fatalf("reconciliation dispatched model: %d", calls.Load()) }
		})
	}
}

func TestCanvasReviewDeniedQueueDoesNotStarveOrdinaryTask(t *testing.T) {
	tools, req, calls := canvasSkillFixture(t, "", 503)
	m := ModelTasks{DB: tools.DB, GW: tools.Model}
	old := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 25; i++ {
		task := model.FlowModelTask{ID: fmt.Sprintf("denied-%02d", i), TenantID: model.PersonalTenantID(req.UserID), IdemKey: fmt.Sprintf("cwn_denied_%d", i), Purpose: "canvas_workflow_node", Status: "queued", NextAttemptAt: old}
		if err := tools.DB.Create(&task).Error; err != nil { t.Fatal(err) }
	}
	task, err := m.Enqueue(ModelTaskInput{TenantID: model.PersonalTenantID(req.UserID), Key: "ordinary", Purpose: "brief", User: "safe", MaxTokens: 8, BudgetMicro: 1000})
	if err != nil { t.Fatal(err) }
	if _, err = m.RunDue(context.Background()); err != nil { t.Fatal(err) }
	stored, err := m.Get(task.ID)
	if err != nil { t.Fatal(err) }
	if calls.Load() != 1 || stored.Status != StatusReconciliation { t.Fatalf("ordinary task starved: calls=%d status=%s", calls.Load(), stored.Status) }
}

func TestCanvasWorkflowSkillCompleteOutputAndDuplicate(t *testing.T) {
	want := substanceOutput(t, "regional-wastewater-modeling")
	raw, _ := json.Marshal(want)
	tools, req, calls := canvasSkillFixture(t, string(raw), http.StatusOK)
	first, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
	if err != nil || first.Status != "succeeded" || first.TaskStatus != "succeeded" || first.TaskID == "" || !reflect.DeepEqual(first.Output, want) {
		t.Fatalf("result=%+v err=%v", first, err)
	}
	runCheckpoint, err := req.Executions.Get(context.Background(), req.UserID, req.RunID)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := DecodeCanvasWorkflowCheckpoint(runCheckpoint)
	if err != nil {
		t.Fatal(err)
	}
	outputJSON, _ := json.Marshal(first.Output)
	cp.Stage = "running"
	cp.Nodes[req.NodeID] = CanvasWorkflowNodeCheckpoint{TaskID: first.TaskID, TaskKey: first.TaskKey, RequestHash: first.RequestHash, PromptHash: first.PromptHash, Status: first.Status, Result: outputJSON}
	if _, err := req.Executions.Checkpoint(context.Background(), req.UserID, req.RunID, req.WorkerID, req.FencingToken, runCheckpoint.Revision, cp); err != nil {
		t.Fatal(err)
	}
	// New adapter/service values resume solely from durable run/task records.
	restarted := *req.Executions
	req.Executions = &restarted
	req.TaskID = first.TaskID
	second, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req)
	if err != nil || second.TaskID != first.TaskID || second.PromptHash != first.PromptHash || calls.Load() != 1 {
		t.Fatalf("duplicate result=%+v calls=%d err=%v", second, calls.Load(), err)
	}
	var task model.FlowModelTask
	if err := tools.DB.First(&task, "id = ?", first.TaskID).Error; err != nil {
		t.Fatal(err)
	}
	var stored struct {
		User string `json:"user"`
	}
	if err := json.Unmarshal([]byte(task.Request), &stored); err != nil {
		t.Fatal(err)
	}
	var prompt map[string]any
	if err := json.Unmarshal([]byte(stored.User), &prompt); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prompt["input"], req.Input) || !reflect.DeepEqual(prompt["previous_output"], req.PreviousOutput) {
		t.Fatal("input and previous output not separated")
	}
	var reservations []model.FlowBudgetReservation
	run, err := req.Executions.Get(context.Background(), req.UserID, req.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tools.DB.Where("parent_id = ?", run.ReservationID).Find(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if len(reservations) != 1 || reservations[0].ConsumedMicro != 100 || reservations[0].Status != "settled" {
		t.Fatalf("duplicate charged: %+v", reservations)
	}
	changed := req
	changed.PreviousOutput = map[string]any{"changed": true}
	if _, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), changed); !errors.Is(err, ErrCanvasWorkflowConflict) || calls.Load() != 1 {
		t.Fatalf("same node different prompt accepted: %v", err)
	}
	if err := tools.DB.Where("skill_id = ?", req.ApprovedSkill.ID).Delete(&model.FlowSkillInstall{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := tools.ExecuteCanvasWorkflowSkill(context.Background(), req); !errors.Is(err, ErrSkillNotFound) || calls.Load() != 1 {
		t.Fatalf("run pin bypassed uninstall: %v", err)
	}
}
