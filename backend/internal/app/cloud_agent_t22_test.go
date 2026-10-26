package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"yingce/backend/internal/database"
	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// t22Setup provisions the full recurring loop: a runnable canvas, the frozen
// model gateway route, the managed runner, and one engine server serving BOTH
// the model gateway and the monitor endpoints used by plan prepare/verify.
func t22Setup(t *testing.T) (*Service, *gorm.DB, *httptest.Server, flow.MonitorCanvasApprovedPlan, string) {
	t.Helper()
	flow.RegisterMonitorCanvasRecurringTool((&Service{}).MonitorCanvasRecurringTool)
	t.Cleanup(func() { flow.RegisterMonitorCanvasRecurringTool(nil); flow.RegisterMonitorCanvasApprovalVerifier(nil) })
	s, db, _, _ := creationTestService(t)
	flow.RegisterMonitorCanvasApprovalVerifier(s.VerifyCloudAgentPlanApproval)
	if err := database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.CanvasWorkflowExecution{}, &model.CanvasMonitorRunUsage{}, &model.FlowCommandOutbox{}, &model.FlowEventInbox{}); err != nil {
		t.Fatal(err)
	}
	var planConfig string = "null"
	monSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "{}"}}}, "usage": map[string]any{"total_tokens": 1}})
		case r.URL.Path == "/v1/monitors":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []any{map[string]any{"id": "mon22", "tenant_id": model.PersonalTenantID("user"), "workspace_id": model.PersonalWorkspaceID("user"), "source_ids": []string{"src22"}, "interval_seconds": 3600, "timezone": "UTC", "budget_micro_per_window": 100, "version": 5, "canvas_auto_run_grant": json.RawMessage(planConfig)}}})
		case r.URL.Path == "/v1/monitors/mon22/revisions":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []any{map[string]any{"version": 1, "spec": map[string]any{}}}})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(monSrv.Close)
	gateway := &flow.OpenAICompatGateway{URL: monSrv.URL, Model: "t22-model", MicroPer1KTok: 1000, HTTP: monSrv.Client()}
	s.ConfigureFlowModel(gateway)
	engine := flow.EngineClient{Cfg: flow.Config{EngineURL: monSrv.URL, SigningKey: []byte("t22-signing-key-0123456789abcdef"), Issuer: "flow-platform", Audience: "loom-engine", TTL: time.Minute}, HTTP: monSrv.Client()}
	s.ConfigureFlowEngine(engine)
	if _, err := flow.ResolveScope(db, "user", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := (flow.Billing{DB: db}).Account(db, model.PersonalTenantID("user")); err != nil {
		t.Fatal(err)
	}
	doc := `{"nodes":[{"id":"a","type":"data-source","metadata":{"dataSource":{"format":"json","rawText":"[{\"v\":1}]"},"content":"old"}},{"id":"b","type":"chart","metadata":{"content":"stale"}}],"connections":[{"id":"e","fromNodeId":"a","toNodeId":"b"}]}`
	if err := db.Create(&model.CanvasProject{ID: "cv22", UserID: "user", Revision: 1, PayloadJSON: doc}).Error; err != nil {
		t.Fatal(err)
	}
	runner := flow.CanvasWorkflowManagedRunner{Tools: flow.Tools{DB: db, Model: gateway}, WorkerID: "t22-runner", Timeout: 15 * time.Second}
	s.ConfigureCanvasWorkflowRunner(runner.Run)
	// Freeze the recurring plan from the real prepare path.
	args := map[string]any{"purpose": flow.MonitorCanvasRecurringPurpose, "monitor_id": "mon22", "canvas_id": "cv22", "total_budget_micro": 300, "max_runs": 2, "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
	scope, err := flow.ResolveScope(db, "user", "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := flow.PrepareCanvasMonitorApproval(context.Background(), db, engine, scope, "user", args)
	if err != nil {
		t.Fatal(err)
	}
	var plan flow.MonitorCanvasApprovedPlan
	if err = json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	planConfig = string(raw)
	// Persist the approved long-term decision the verifier reads (T21 chain).
	agentID := "ag_t22_" + strings.ToLower(strings.ReplaceAll(time.Now().Format("150405.000"), ".", ""))
	approvalID := "apr_t22"
	req := CloudAgentRequest{CanvasID: "cv22", Prompt: "周期画布运行授权", PermissionMode: "request_approval", IdempotencyKey: agentID, ContextScope: []string{"canvas"}}
	req.Budget.MaxCredits = 1
	profile := cloudAgentProfileSnapshot{Revision: agentProfileRevision(nil), Hash: agentProfileHash("")}
	canonical, policy, err := compileCloudAgentPolicies(req, defaultCanvasAppearance().AgentName, nil, "", profile)
	if err != nil {
		t.Fatal(err)
	}
	call := cloudAgentCall{ID: "call-t22"}
	call.Function.Name = "intel_apply_monitor"
	call.Function.Arguments = `{"monitor_id":"mon22","canvas_id":"cv22"}`
	state := cloudAgentRuntime{Request: req, Policy: policy, Profile: profile, Canonical: canonicalAgentRequest{SystemPrompt: canonical}, Decisions: map[string]string{approvalID: "approve"}, Events: []CloudAgentEvent{}, ManualApproval: true, TaskIDs: []string{agentID}, Calls: []cloudAgentCall{call}, WorkflowApprovals: map[string]cloudAgentApproval{approvalID: {ID: approvalID, Call: call, CallHash: cloudAgentApprovalCallHash(call), Purpose: flow.MonitorCanvasRecurringPurpose, Decision: "approve", ServerPlan: append(json.RawMessage(nil), raw...)}}}
	root := model.CloudAgentExecution{ID: agentID, UserID: "user", CanvasID: "cv22", Status: "completed", Revision: 1, CheckpointVersion: 2, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err = db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	if err = cloudAgentSave(&root, &state); err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&model.CloudAgentExecution{}).Where("id = ?", agentID).Updates(map[string]any{"state_json": root.StateJSON}).Error; err != nil {
		t.Fatal(err)
	}
	// The sealed grant carries the persisted approval references (T21 contract).
	plan.ExecutionID, plan.ApprovalID = agentID, approvalID
	return s, db, monSrv, plan, agentID
}

func t22CycleEvent(eventID, scheduledAt string, grant json.RawMessage) flow.Event {
	payload, _ := json.Marshal(map[string]any{"monitor_id": "mon22", "canvas_id": "cv22", "cycle_complete": true, "operation_id": "op22", "canvas_auto_run_grant": grant, "window": map[string]any{"scheduled_at": scheduledAt, "from": "2026-10-01T00:00:00Z", "to": "2026-10-02T00:00:00Z", "timezone": "UTC"}})
	return flow.Event{EventID: eventID, OperationID: "op22", TenantID: model.PersonalTenantID("user"), WorkspaceID: model.PersonalWorkspaceID("user"), Type: "collection.completed", Payload: payload}
}

func TestT22RecurringCanvasAutoRunLoop(t *testing.T) {
	s, db, _, plan, agentID := t22Setup(t)
	grant, _ := json.Marshal(plan)
	dispatcher := flow.CommandDispatcher{DB: db, Local: map[string]func(context.Context, model.FlowCommandOutbox) (int, []byte, error){flow.MonitorCanvasRunCommand: s.ExecuteMonitorCanvasRun}}
	dispatchDue := func() int {
		n, err := dispatcher.DispatchDue(context.Background(), 10)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	// Cycle 1: intake → local dispatch → verify → consume → admit → managed run.
	if ok, err := flow.ConsumeMonitorCanvasEvent(db, t22CycleEvent("evt-c1", "2026-10-02T00:00:00Z", grant)); err != nil || !ok {
		t.Fatalf("cycle1 intake: %v %v", ok, err)
	}
	if dispatchDue() != 1 {
		t.Fatal("cycle1 not dispatched")
	}
	deadline := time.Now().Add(20 * time.Second)
	var first model.CanvasWorkflowExecution
	for time.Now().Before(deadline) {
		db.Where("canvas_id = ?", "cv22").Order("created_at").First(&first)
		if first.Status == "succeeded" || first.Status == "failed" || first.Status == "waiting_external" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if first.Status != "succeeded" {
		t.Fatalf("cycle1 managed run: %s", first.Status)
	}
	var usage model.CanvasMonitorRunUsage
	if err := db.First(&usage, "monitor_id = ?", "mon22").Error; err != nil {
		t.Fatal(err)
	}
	if usage.RunsUsed != 1 || usage.MicroUsed != plan.BudgetMicroPerWindow {
		t.Fatalf("usage after cycle1: %+v", usage)
	}
	// Duplicate delivery of the same event: no second command, no second run.
	if ok, err := flow.ConsumeMonitorCanvasEvent(db, t22CycleEvent("evt-c1", "2026-10-02T00:00:00Z", grant)); err != nil || ok {
		t.Fatalf("duplicate cycle re-enqueued: %v %v", ok, err)
	}
	// Cycle 2: the managed write-back (computed/derived) must not change the
	// plan config hash; the recurring grant stays valid for the next cycle.
	if ok, err := flow.ConsumeMonitorCanvasEvent(db, t22CycleEvent("evt-c2", "2026-10-03T00:00:00Z", grant)); err != nil || !ok {
		t.Fatalf("cycle2 intake: %v %v", ok, err)
	}
	if dispatchDue() != 1 {
		t.Fatal("cycle2 not dispatched")
	}
	deadline = time.Now().Add(20 * time.Second)
	var second model.CanvasWorkflowExecution
	for time.Now().Before(deadline) {
		db.Where("canvas_id = ?", "cv22").Order("created_at DESC").First(&second)
		if second.ID != first.ID && (second.Status == "succeeded" || second.Status == "failed" || second.Status == "waiting_external") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if second.ID == first.ID || second.Status != "succeeded" {
		t.Fatalf("cycle2 managed run: %+v", second)
	}
	if err := db.First(&usage, "monitor_id = ?", "mon22").Error; err != nil {
		t.Fatal(err)
	}
	if usage.RunsUsed != 2 {
		t.Fatalf("usage after cycle2: %+v", usage)
	}
	// Cycle 3 exceeds max_runs=2: terminal skip, no execution, no retry loop.
	if ok, err := flow.ConsumeMonitorCanvasEvent(db, t22CycleEvent("evt-c3", "2026-10-04T00:00:00Z", grant)); err != nil || !ok {
		t.Fatalf("cycle3 intake: %v %v", ok, err)
	}
	if dispatchDue() != 1 {
		t.Fatal("cycle3 not dispatched")
	}
	var executions int64
	db.Model(&model.CanvasWorkflowExecution{}).Where("canvas_id = ?", "cv22").Count(&executions)
	if executions != 2 {
		t.Fatalf("exhausted plan created executions: %d", executions)
	}
	var cmd model.FlowCommandOutbox
	if err := db.Where("command = ?", flow.MonitorCanvasRunCommand).Order("created_at DESC").First(&cmd).Error; err != nil {
		t.Fatal(err)
	}
	if cmd.Status != "dispatched" || !strings.Contains(cmd.ResponseBody, "monitor_plan_exhausted") {
		t.Fatalf("exhausted command not terminal: %+v", cmd)
	}
	// The recurring authority rejects a single-run admission carrying the plan.
	var current model.CanvasProject
	if err := db.First(&current, "id = ?", "cv22").Error; err != nil {
		t.Fatal(err)
	}
	_, err := s.CanvasWorkflowExecutions().Admit(context.Background(), flow.CanvasWorkflowAdmission{UserID: "user", TenantID: model.PersonalTenantID("user"), WorkspaceID: model.PersonalWorkspaceID("user"), CanvasID: "cv22", ExecutionKey: "forged", ExpectedRevision: &current.Revision, BudgetMicro: plan.BudgetMicroPerWindow, AuthorityBinding: grant})
	if err == nil || !(errors.Is(err, flow.ErrForbidden) || errors.Is(err, flow.ErrCanvasWorkflowConflict) || errors.Is(err, flow.ErrApprovalRequired)) {
		t.Fatalf("forged single-run admission with recurring binding accepted: %v", err)
	}
	_ = agentID
}
