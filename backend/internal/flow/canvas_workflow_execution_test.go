package flow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

// The approval/model policy is an application-owned interface, not a fabricated
// production grant. Storage, membership checks, leases and billing remain real.
type workflowTestAuthority struct {
	approved bool
	revoked  bool
}

func (a *workflowTestAuthority) Authorize(_ context.Context, _ *gorm.DB, access CanvasWorkflowAccess) (CapabilityInput, error) {
	if !a.approved {
		return CapabilityInput{}, ErrApprovalRequired
	}
	if a.revoked {
		return CapabilityInput{}, ErrForbidden
	}
	if access.Frozen != nil {
		return CapabilityInput{}, nil
	}
	return CapabilityInput{Tools: []map[string]any{{"type": "function", "function": map[string]any{"name": "intel_collect_once", "parameters": map[string]any{"type": "object"}}}}, ModelRoute: map[string]any{"model": "test-model"}, PolicyHash: "approved-policy-v1"}, nil
}

func workflowExecutionSetup(t *testing.T) (CanvasWorkflowExecutions, CanvasWorkflowAdmission, *workflowTestAuthority) {
	t.Helper()
	db := testDB(t)
	if err := db.AutoMigrate(&model.CanvasWorkflowExecution{}); err != nil {
		t.Fatal(err)
	}
	fund(t, Billing{DB: db}, "u_alice", 10000)
	// Explicit account provisioning; admission must not synthesize a payer/quota.
	if _, err := (Billing{DB: db}).Account(db, model.PersonalTenantID("u_alice")); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CanvasProject{ID: "canvas-a", UserID: "u_alice", Revision: 7, PayloadJSON: `{"nodes":[{"id":"node-a","type":"skill"}],"connections":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CanvasProject{ID: "canvas-b", UserID: "u_bob", Revision: 7, PayloadJSON: `{"nodes":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	rev := int64(7)
	in := CanvasWorkflowAdmission{UserID: "u_alice", TenantID: model.PersonalTenantID("u_alice"), WorkspaceID: model.PersonalWorkspaceID("u_alice"), CanvasID: "canvas-a", ExecutionKey: "request-1", ExpectedRevision: &rev, BudgetMicro: 1000}
	auth := &workflowTestAuthority{approved: true}
	svc := CanvasWorkflowExecutions{DB: db, Authority: auth, Now: func() time.Time { return time.Unix(1800000000, 0).UTC() }}
	return svc, in, auth
}

func TestT12AdmissionRejectsWithoutSideEffects(t *testing.T) {
	for _, name := range []string{"tenant", "workspace", "canvas", "owner", "viewer", "revision", "missing_revision", "missing_authority", "unapproved", "revoked", "budget", "missing_account"} {
		t.Run(name, func(t *testing.T) {
			svc, in, auth := workflowExecutionSetup(t)
			want := http.StatusForbidden
			switch name {
			case "tenant":
				in.TenantID = model.PersonalTenantID("u_bob")
			case "workspace":
				in.WorkspaceID = model.PersonalWorkspaceID("u_bob")
			case "canvas":
				in.CanvasID = "canvas-b"
			case "owner":
				in.UserID = "u_bob"
			case "viewer":
				svc.DB.Model(&model.WorkspaceMembership{}).Where("workspace_id = ?", in.WorkspaceID).Update("role", "viewer")
			case "revision":
				*in.ExpectedRevision = 6
				want = http.StatusConflict
			case "missing_revision":
				in.ExpectedRevision = nil
				want = http.StatusBadRequest
			case "missing_authority":
				svc.Authority = nil
			case "unapproved":
				auth.approved = false
			case "revoked":
				auth.revoked = true
			case "budget":
				in.BudgetMicro = 20000
				want = http.StatusPaymentRequired
			case "missing_account":
				svc.DB.Where("tenant_id = ?", in.TenantID).Delete(&model.FlowBillingAccount{})
			}
			_, err := svc.Admit(context.Background(), in)
			if err == nil || CanvasWorkflowHTTPStatus(err) != want {
				t.Fatalf("want HTTP %d, got %v (%d)", want, err, CanvasWorkflowHTTPStatus(err))
			}
			for _, m := range []any{&model.CanvasWorkflowExecution{}, &model.FlowRunCapability{}, &model.FlowBudgetReservation{}, &model.FlowOperationProjection{}} {
				var count int64
				if err := svc.DB.Model(m).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("denied admission wrote %T: %d", m, count)
				}
			}
		})
	}
}

func TestT12AdmissionReplayFreezesAndDoesNotReserveAgain(t *testing.T) {
	svc, in, auth := workflowExecutionSetup(t)
	first, err := svc.Admit(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := RunCapability(svc.DB, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.PriceVersion == "" || snap.ModelRouteHash == "" || first.ReservationID == "" {
		t.Fatalf("missing frozen capability/reservation: %+v", first)
	}
	// Successful canvas writes may advance revision; replay must return the old run.
	svc.DB.Model(&model.CanvasProject{}).Where("id = ?", in.CanvasID).Update("revision", 8)
	again, err := svc.Admit(context.Background(), in)
	if err != nil || again.ID != first.ID || again.CheckpointJSON != first.CheckpointJSON {
		t.Fatalf("replay %+v %v", again, err)
	}
	changed := in
	changed.SaveReport = true
	if _, err = svc.Admit(context.Background(), changed); CanvasWorkflowHTTPStatus(err) != 409 {
		t.Fatalf("same key different args: %v", err)
	}
	var count int64
	svc.DB.Model(&model.FlowBudgetReservation{}).Count(&count)
	if count != 1 {
		t.Fatalf("reserved %d times", count)
	}
	var agents int64
	svc.DB.Model(&model.CloudAgentExecution{}).Count(&agents)
	if agents != 0 {
		t.Fatal("canvas state leaked into agent recovery table")
	}
	auth.revoked = true
	if _, err = svc.Admit(context.Background(), in); !errors.Is(err, ErrForbidden) {
		t.Fatalf("replay bypasses revocation: %v", err)
	}
}

func TestT12CheckpointPersistsNonModelResultAndRejectsUnknownNode(t *testing.T) {
	svc, in, _ := workflowExecutionSetup(t)
	ctx := context.Background()
	r, e := svc.Admit(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	token, e := svc.Acquire(ctx, in.UserID, r.ID, "worker")
	if e != nil {
		t.Fatal(e)
	}
	cp := CanvasWorkflowCheckpoint{Stage: "saving_result", Nodes: map[string]CanvasWorkflowNodeCheckpoint{"node-a": {Status: "failed", Error: "source unavailable", Result: json.RawMessage(`{"partial":true}`)}}}
	saved, e := svc.Checkpoint(ctx, in.UserID, r.ID, "worker", token, r.Revision, cp)
	if e != nil {
		t.Fatalf("non-model checkpoint: %v", e)
	}
	cp.Nodes["foreign-node"] = CanvasWorkflowNodeCheckpoint{Status: "succeeded"}
	if _, e = svc.Checkpoint(ctx, in.UserID, r.ID, "worker", token, saved.Revision, cp); !errors.Is(e, ErrForbidden) {
		t.Fatalf("unknown node accepted: %v", e)
	}
}

func TestT12ConcurrentAdmissionAndCanvasKeyIsolation(t *testing.T) {
	svc, in, _ := workflowExecutionSetup(t)
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, e := svc.Admit(context.Background(), in); ids <- r.ID; errs <- e }()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatal("concurrent duplicate")
		}
	}
	var count int64
	svc.DB.Model(&model.FlowBudgetReservation{}).Count(&count)
	if count != 1 {
		t.Fatalf("concurrent holds: %d", count)
	}
	if err := svc.DB.Create(&model.CanvasProject{ID: "canvas-c", UserID: in.UserID, Revision: 7, PayloadJSON: `{"nodes":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	in.CanvasID = "canvas-c"
	other, e := svc.Admit(context.Background(), in)
	if e != nil || other.ID == first {
		t.Fatalf("canvas key collision %v", e)
	}
}

func TestT12CheckpointSettlementAndLeaseExpiry(t *testing.T) {
	svc, in, _ := workflowExecutionSetup(t)
	ctx := context.Background()
	run, e := svc.Admit(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	token, e := svc.Acquire(ctx, in.UserID, run.ID, "worker")
	if e != nil {
		t.Fatal(e)
	}
	input, e := svc.PrepareModelNode(ctx, in.UserID, run.ID, "worker", token, "node-a", "test-model", ModelTaskInput{User: "input", BudgetMicro: 500, MaxTokens: 32, Context: `{"prompt_hash":"p1"}`})
	if e != nil {
		t.Fatal(e)
	}
	var envelope struct {
		Payload  string         `json:"payload"`
		Workflow map[string]any `json:"canvas_workflow"`
	}
	if e = json.Unmarshal([]byte(input.Context), &envelope); e != nil || envelope.Payload != `{"prompt_hash":"p1"}` || envelope.Workflow["capability_snapshot_id"] != run.CapabilitySnapshotID {
		t.Fatalf("context lost: %s", input.Context)
	}
	task, e := (ModelTasks{DB: svc.DB}).Enqueue(input)
	if e != nil {
		t.Fatal(e)
	}
	current, e := svc.Get(ctx, in.UserID, run.ID)
	if e != nil {
		t.Fatal(e)
	}
	cp, e := DecodeCanvasWorkflowCheckpoint(current)
	if e != nil {
		t.Fatal(e)
	}
	n := cp.Nodes["node-a"]
	n.TaskID = task.ID
	n.Status = "succeeded"
	n.Result = json.RawMessage(`{"value":1}`)
	cp.Nodes["node-a"] = n
	cp.Stage = "succeeded"
	cp.Result = json.RawMessage(`{"ok":true}`)
	cp.ReportID = "report-reference"
	if _, e = svc.Checkpoint(ctx, in.UserID, run.ID, "worker", token, current.Revision, cp); !errors.Is(e, ErrBudgetState) {
		t.Fatalf("unresolved task budget must block terminal checkpoint: %v", e)
	}
	if _, e = (Billing{DB: svc.DB}).Settle(task.ReservationID, 200); e != nil {
		t.Fatal(e)
	}
	done, e := svc.Checkpoint(ctx, in.UserID, run.ID, "worker", token, current.Revision, cp)
	if e != nil {
		t.Fatal(e)
	}
	again, e := svc.Admit(ctx, in)
	if e != nil || again.Status != "succeeded" || again.CheckpointJSON != done.CheckpointJSON {
		t.Fatalf("terminal replay: %+v %v", again, e)
	}
	var count int64
	svc.DB.Model(&model.CreditLedgerEntry{}).Where("type = ?", model.CreditLedgerConsume).Count(&count)
	if count != 1 {
		t.Fatalf("consume count %d", count)
	}
	var acc model.CreditAccount
	if e = svc.DB.First(&acc, "user_id = ?", in.UserID).Error; e != nil {
		t.Fatal(e)
	}
	if acc.AvailableMicrocredits != 9800 || acc.ReservedMicrocredits != 0 {
		t.Fatalf("balance %+v", acc)
	}
	// Expiration alone fences writes, even before a replacement worker acquires.
	in.ExecutionKey = "second"
	r, e := svc.Admit(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	tok, e := svc.Acquire(ctx, in.UserID, r.ID, "old")
	if e != nil {
		t.Fatal(e)
	}
	now := svc.now()
	svc.Now = func() time.Time { return now.Add(61 * time.Second) }
	if _, e = svc.PrepareModelNode(ctx, in.UserID, r.ID, "old", tok, "node-a", "test-model", ModelTaskInput{User: "input", BudgetMicro: 100, MaxTokens: 32}); !errors.Is(e, ErrLeaseHeld) {
		t.Fatalf("expired lease accepted: %v", e)
	}
	fresh, e := svc.Acquire(ctx, in.UserID, r.ID, "new")
	if e != nil || fresh <= tok {
		t.Fatalf("fencing token not advanced: %d %v", fresh, e)
	}
	svc.DB.Model(&model.Membership{}).Where("tenant_id = ? AND user_id = ?", in.TenantID, in.UserID).Update("status", "removed")
	if _, e = svc.PrepareModelNode(ctx, in.UserID, r.ID, "new", fresh, "node-a", "test-model", ModelTaskInput{User: "input", BudgetMicro: 100, MaxTokens: 32}); !errors.Is(e, ErrForbidden) {
		t.Fatalf("membership revocation ignored: %v", e)
	}
}

func TestT12CloseFailedRunReleasesQueuedAndKeepsUnknownForReconciliation(t *testing.T) {
	for _, scenario := range []string{"queued_released", "unknown_kept", "terminal_noop"} {
		t.Run(scenario, func(t *testing.T) {
			svc, in, _ := workflowExecutionSetup(t)
			ctx := context.Background()
			run, e := svc.Admit(ctx, in)
			if e != nil {
				t.Fatal(e)
			}
			token, e := svc.Acquire(ctx, in.UserID, run.ID, "worker")
			if e != nil {
				t.Fatal(e)
			}
			input, e := svc.PrepareModelNode(ctx, in.UserID, run.ID, "worker", token, "node-a", "test-model", ModelTaskInput{User: "input", BudgetMicro: 400, MaxTokens: 32})
			if e != nil {
				t.Fatal(e)
			}
			task, e := (ModelTasks{DB: svc.DB}).Enqueue(input)
			if e != nil {
				t.Fatal(e)
			}
			if scenario == "unknown_kept" {
				if e = (Billing{DB: svc.DB}).MarkUnknown(task.ReservationID, "model:"+task.IdemKey, "outcome unknown"); e != nil {
					t.Fatal(e)
				}
				if e = svc.DB.Model(&model.FlowModelTask{}).Where("id = ?", task.ID).Update("status", StatusReconciliation).Error; e != nil {
					t.Fatal(e)
				}
			}
			// Fenced cleanup: a foreign owner or stale token must not touch the run.
			if e = svc.CloseFailedRun(ctx, in.UserID, run.ID, "foreign", token); !errors.Is(e, ErrLeaseHeld) {
				t.Fatalf("foreign lease accepted: %v", e)
			}
			if e = svc.CloseFailedRun(ctx, in.UserID, run.ID, "worker", token+7); !errors.Is(e, ErrLeaseHeld) {
				t.Fatalf("stale token accepted: %v", e)
			}
			if e = svc.CloseFailedRun(ctx, in.UserID, run.ID, "worker", token); e != nil {
				t.Fatal(e)
			}
			stored, e := svc.Get(ctx, in.UserID, run.ID)
			if e != nil {
				t.Fatal(e)
			}
			var storedTask model.FlowModelTask
			if e = svc.DB.First(&storedTask, "id = ?", task.ID).Error; e != nil {
				t.Fatal(e)
			}
			var child model.FlowBudgetReservation
			if e = svc.DB.First(&child, "id = ?", task.ReservationID).Error; e != nil {
				t.Fatal(e)
			}
			var root model.FlowBudgetReservation
			if e = svc.DB.First(&root, "id = ?", run.ReservationID).Error; e != nil {
				t.Fatal(e)
			}
			switch scenario {
			case "queued_released":
				if stored.Status != "failed" || storedTask.Status != "failed" || child.Status != "released" || root.Status != "settled" {
					t.Fatalf("queued cleanup incomplete: run=%s task=%s child=%s root=%s", stored.Status, storedTask.Status, child.Status, root.Status)
				}
				var acc model.CreditAccount
				if e = svc.DB.First(&acc, "user_id = ?", in.UserID).Error; e != nil {
					t.Fatal(e)
				}
				if acc.ReservedMicrocredits != 0 || acc.AvailableMicrocredits != 10000 {
					t.Fatalf("held credits not returned: %+v", acc)
				}
			case "unknown_kept":
				if stored.Status != "waiting_external" || storedTask.Status != StatusReconciliation || child.Status != StatusReconciliation || root.Status != "reserved" {
					t.Fatalf("uncertain outcome discarded: run=%s task=%s child=%s root=%s", stored.Status, storedTask.Status, child.Status, root.Status)
				}
			case "terminal_noop":
				if stored.Status != "failed" {
					t.Fatalf("first close failed: %s", stored.Status)
				}
				if e = svc.CloseFailedRun(ctx, in.UserID, run.ID, "worker", token); e != nil {
					t.Fatal(e)
				}
				again, e2 := svc.Get(ctx, in.UserID, run.ID)
				if e2 != nil {
					t.Fatal(e2)
				}
				if again.Revision != stored.Revision {
					t.Fatalf("terminal close mutated run: %d %d", again.Revision, stored.Revision)
				}
			}
		})
	}
}

func TestT12CheckpointLeaseFenceAndModelIdentity(t *testing.T) {
	svc, in, auth := workflowExecutionSetup(t)
	run, err := svc.Admit(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	token, err := svc.Acquire(context.Background(), in.UserID, run.ID, "worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Acquire(context.Background(), in.UserID, run.ID, "worker-b"); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("double owner: %v", err)
	}
	input, err := svc.PrepareModelNode(context.Background(), in.UserID, run.ID, "worker-a", token, "node-a", "test-model", ModelTaskInput{System: "system", User: "input", BudgetMicro: 500, MaxTokens: 32})
	if err != nil {
		t.Fatal(err)
	}
	if input.ParentResID != run.ReservationID || input.TenantID != in.TenantID || input.Key == "" {
		t.Fatalf("unbound model request: %+v", input)
	}
	// Structural integration only: enqueue uses real ModelTasks + Billing; no GW call.
	task, err := (ModelTasks{DB: svc.DB}).Enqueue(input)
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := svc.PrepareModelNode(context.Background(), in.UserID, run.ID, "worker-a", token, "node-a", "test-model", ModelTaskInput{System: "system", User: "input", BudgetMicro: 500, MaxTokens: 32})
	if err != nil || repeat.Key != input.Key {
		t.Fatalf("unstable key: %+v %v", repeat, err)
	}
	duplicate, err := (ModelTasks{DB: svc.DB}).Enqueue(repeat)
	if err != nil || duplicate.ID != task.ID {
		t.Fatalf("duplicate task: %+v %v", duplicate, err)
	}
	if _, err = svc.PrepareModelNode(context.Background(), in.UserID, run.ID, "worker-a", token, "node-a", "test-model", ModelTaskInput{User: "changed", BudgetMicro: 500, MaxTokens: 32}); CanvasWorkflowHTTPStatus(err) != 409 {
		t.Fatalf("node key conflict: %v", err)
	}
	state := CanvasWorkflowCheckpoint{Stage: "saving_report", Nodes: map[string]CanvasWorkflowNodeCheckpoint{"node-a": {TaskID: task.ID, TaskKey: input.Key, RequestHash: hashJSON(struct {
		Model string
		Input ModelTaskInput
	}{"test-model", input}), Status: "queued"}}}
	// Use the persisted node hash; caller cannot replace the admitted request identity.
	current, err := svc.Get(context.Background(), in.UserID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := DecodeCanvasWorkflowCheckpoint(current)
	if err != nil {
		t.Fatal(err)
	}
	n := cp.Nodes["node-a"]
	n.TaskID = task.ID
	n.Status = "queued"
	state.Nodes["node-a"] = n
	saved, err := svc.Checkpoint(context.Background(), in.UserID, run.ID, "worker-a", token, current.Revision, state)
	if err != nil {
		t.Fatal(err)
	}
	restarted := svc
	got, err := restarted.Get(context.Background(), in.UserID, run.ID)
	if err != nil || got.CheckpointJSON != saved.CheckpointJSON {
		t.Fatalf("checkpoint lost: %+v %v", got, err)
	}
	if _, err = svc.Checkpoint(context.Background(), in.UserID, run.ID, "worker-b", token, saved.Revision, state); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("unfenced checkpoint: %v", err)
	}
	if _, err = svc.Checkpoint(context.Background(), in.UserID, run.ID, "worker-a", token, current.Revision, state); CanvasWorkflowHTTPStatus(err) != 409 {
		t.Fatalf("stale checkpoint: %v", err)
	}
	auth.revoked = true
	if _, err = svc.Acquire(context.Background(), in.UserID, run.ID, "worker-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoked run acquired: %v", err)
	}
}
