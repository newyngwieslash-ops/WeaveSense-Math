package flow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

type recoveryClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *recoveryClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *recoveryClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// recoverySetup admits one managed canvas run on a controllable clock.
func recoverySetup(t *testing.T) (CanvasWorkflowExecutions, CanvasWorkflowAdmission, *workflowTestAuthority, *recoveryClock, model.CanvasWorkflowExecution) {
	t.Helper()
	svc, in, auth := workflowExecutionSetup(t)
	clock := &recoveryClock{t: time.Unix(1800000000, 0).UTC()}
	svc.Now = clock.now
	run, err := svc.Admit(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return svc, in, auth, clock, run
}

func recoveryRun(t *testing.T, svc CanvasWorkflowExecutions, id string) (model.CanvasWorkflowExecution, CanvasWorkflowCheckpoint) {
	t.Helper()
	var run model.CanvasWorkflowExecution
	if err := svc.DB.First(&run, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	cp, err := DecodeCanvasWorkflowCheckpoint(run)
	if err != nil {
		t.Fatal(err)
	}
	return run, cp
}

func recoveryLeaseCount(t *testing.T, db *gorm.DB, id string) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.FlowRunLease{}).Where("run_id = ?", id).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// startedRun acquires the run and dispatches one model node, then "crashes":
// the worker never closes the run or releases its lease.
func startedRun(t *testing.T, svc CanvasWorkflowExecutions, in CanvasWorkflowAdmission, run model.CanvasWorkflowExecution, unknown bool) model.FlowModelTask {
	t.Helper()
	ctx := context.Background()
	token, err := svc.Acquire(ctx, in.UserID, run.ID, "crashed-worker")
	if err != nil {
		t.Fatal(err)
	}
	input, err := svc.PrepareModelNode(ctx, in.UserID, run.ID, "crashed-worker", token, "node-a", "test-model", ModelTaskInput{User: "input", BudgetMicro: 400, MaxTokens: 32})
	if err != nil {
		t.Fatal(err)
	}
	task, err := (ModelTasks{DB: svc.DB}).Enqueue(input)
	if err != nil {
		t.Fatal(err)
	}
	if unknown {
		if err = (Billing{DB: svc.DB}).MarkUnknown(task.ReservationID, "model:"+task.IdemKey, "outcome unknown"); err != nil {
			t.Fatal(err)
		}
		if err = svc.DB.Model(&model.FlowModelTask{}).Where("id = ?", task.ID).Update("status", StatusReconciliation).Error; err != nil {
			t.Fatal(err)
		}
	}
	return task
}

func TestCanvasWorkflowErrorCode(t *testing.T) {
	for err, want := range map[error]string{
		nil:                    "",
		ErrModelUnknownOutcome: "unknown_outcome",
		fmt.Errorf("x: %w", context.DeadlineExceeded): "timeout",
		context.Canceled:              "cancelled",
		ErrLeaseHeld:                  "lease_lost",
		ErrCanvasWorkflowConflict:     "conflict",
		ErrBudgetInsufficient:         "budget",
		ErrForbidden:                  "forbidden",
		ErrApprovalRequired:           "forbidden",
		ErrNotFound:                   "not_found",
		ErrInvalid:                    "invalid",
		errors.New("engine exploded"): "internal",
	} {
		if got := CanvasWorkflowErrorCode(err); got != want {
			t.Fatalf("%v: got %q want %q", err, got, want)
		}
	}
}

// Error persisted: a runner error before the run took its lease used to be
// dropped, leaving the run admitted forever. Settle logs it and closes the run
// with the error's code.
func TestCanvasRecoverySettlePersistsRunnerError(t *testing.T) {
	svc, _, _, _, run := recoverySetup(t)
	rec := CanvasWorkflowRecovery{Executions: svc}
	rec.Settle(context.Background(), run, fmt.Errorf("get: %w", ErrForbidden))
	got, cp := recoveryRun(t, svc, run.ID)
	if got.Status != "failed" || cp.ErrorCode != "forbidden" || cp.Error == "" {
		t.Fatalf("runner error not persisted: status=%s cp=%+v", got.Status, cp)
	}
	var root model.FlowBudgetReservation
	if err := svc.DB.First(&root, "id = ?", run.ReservationID).Error; err != nil {
		t.Fatal(err)
	}
	if root.Status != "settled" && root.Status != "released" {
		t.Fatalf("root hold kept after failure: %s", root.Status)
	}
	if recoveryLeaseCount(t, svc.DB, run.ID) != 0 {
		t.Fatal("settle left its lease behind")
	}
	// Repeating is a no-op: the run is terminal.
	rec.Settle(context.Background(), run, errors.New("again"))
	again, _ := recoveryRun(t, svc, run.ID)
	if again.Revision != got.Revision {
		t.Fatalf("terminal run mutated: %d -> %d", got.Revision, again.Revision)
	}
}

// A lost lease means another worker owns the run; Settle must not touch it.
func TestCanvasRecoverySettleLeavesLiveOwnerAlone(t *testing.T) {
	svc, in, _, _, run := recoverySetup(t)
	if _, err := svc.Acquire(context.Background(), in.UserID, run.ID, "live-worker"); err != nil {
		t.Fatal(err)
	}
	rec := CanvasWorkflowRecovery{Executions: svc}
	rec.Settle(context.Background(), run, ErrLeaseHeld)
	rec.Settle(context.Background(), run, errors.New("boom"))
	got, _ := recoveryRun(t, svc, run.ID)
	if got.Status != "admitted" || got.Revision != run.Revision {
		t.Fatalf("live run touched: %s rev %d", got.Status, got.Revision)
	}
}

// Stuck run swept + idempotency: an admitted run that dispatched nothing is
// requeued once, and an immediate second sweep does not requeue it again.
func TestCanvasRecoveryRequeuesUntouchedRunOnce(t *testing.T) {
	svc, _, _, clock, run := recoverySetup(t)
	calls := make(chan string, 4)
	rec := CanvasWorkflowRecovery{Executions: svc, StaleAfter: 10 * time.Minute, Run: func(_ context.Context, _ CanvasWorkflowExecutions, r model.CanvasWorkflowExecution) error {
		calls <- r.ID
		return nil
	}}
	rep, err := rec.Sweep(context.Background())
	if err != nil || len(rep.Requeued)+len(rep.Failed)+len(rep.Reconcile) != 0 {
		t.Fatalf("fresh run swept: %+v %v", rep, err)
	}
	clock.add(11 * time.Minute)
	rep, err = rec.Sweep(context.Background())
	if err != nil || len(rep.Requeued) != 1 || rep.Requeued[0] != run.ID {
		t.Fatalf("stale untouched run not requeued: %+v %v", rep, err)
	}
	select {
	case id := <-calls:
		if id != run.ID {
			t.Fatalf("requeued %s", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runner not called")
	}
	got, cp := recoveryRun(t, svc, run.ID)
	if got.Status != "admitted" || cp.Recovery != 1 {
		t.Fatalf("requeue not recorded: %s %+v", got.Status, cp)
	}
	if recoveryLeaseCount(t, svc.DB, run.ID) != 0 {
		t.Fatal("sweeper kept the lease of a requeued run")
	}
	rep, err = rec.Sweep(context.Background())
	if err != nil || len(rep.Requeued) != 0 {
		t.Fatalf("second sweep requeued again: %+v %v", rep, err)
	}
	select {
	case <-calls:
		t.Fatal("runner called twice")
	case <-time.After(100 * time.Millisecond):
	}
}

// A run that keeps getting stuck before it starts is failed after MaxRequeue.
func TestCanvasRecoveryFailsAfterRequeueBudget(t *testing.T) {
	svc, _, _, clock, run := recoverySetup(t)
	done := make(chan struct{}, 4)
	rec := CanvasWorkflowRecovery{Executions: svc, StaleAfter: time.Minute, MaxRequeue: 1, Run: func(context.Context, CanvasWorkflowExecutions, model.CanvasWorkflowExecution) error {
		done <- struct{}{}
		return nil
	}}
	clock.add(2 * time.Minute)
	if rep, err := rec.Sweep(context.Background()); err != nil || len(rep.Requeued) != 1 {
		t.Fatalf("first sweep: %+v %v", rep, err)
	}
	<-done
	clock.add(2 * time.Minute)
	rep, err := rec.Sweep(context.Background())
	if err != nil || len(rep.Failed) != 1 {
		t.Fatalf("exhausted run not failed: %+v %v", rep, err)
	}
	got, cp := recoveryRun(t, svc, run.ID)
	if got.Status != "failed" || cp.ErrorCode != CanvasRecoveryExhausted {
		t.Fatalf("got %s %+v", got.Status, cp)
	}
}

// Authority revoked while the run waited: closed as forbidden, never requeued.
func TestCanvasRecoveryDoesNotRequeueRevokedRun(t *testing.T) {
	svc, _, auth, clock, run := recoverySetup(t)
	auth.revoked = true
	rec := CanvasWorkflowRecovery{Executions: svc, Run: func(context.Context, CanvasWorkflowExecutions, model.CanvasWorkflowExecution) error {
		t.Error("revoked run requeued")
		return nil
	}}
	clock.add(time.Hour)
	if rep, err := rec.Sweep(context.Background()); err != nil || len(rep.Failed) != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	got, cp := recoveryRun(t, svc, run.ID)
	if got.Status != "failed" || cp.ErrorCode != "forbidden" {
		t.Fatalf("got %s %+v", got.Status, cp)
	}
}

// Restart recovery: a worker crashed mid-run. After its lease expires, a new
// process's sweeper (fresh struct, same database) closes the run: queued model
// work is failed and its hold released, never re-run; a call whose outcome is
// unknown keeps its hold and the run parks in waiting_external.
func TestCanvasRecoveryAfterRestartNeverRerunsPaidWork(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprintf("unknown=%v", unknown), func(t *testing.T) {
			svc, in, _, clock, run := recoverySetup(t)
			task := startedRun(t, svc, in, run, unknown)
			ran := false
			rec := CanvasWorkflowRecovery{Executions: svc, StaleAfter: 10 * time.Minute, Run: func(context.Context, CanvasWorkflowExecutions, model.CanvasWorkflowExecution) error {
				ran = true
				return nil
			}}
			// Lease still live (inside TTL) and run fresh: nothing to do.
			clock.add(30 * time.Second)
			if rep, err := rec.Sweep(context.Background()); err != nil || len(rep.Failed)+len(rep.Reconcile)+len(rep.Requeued) != 0 {
				t.Fatalf("live run swept: %+v %v", rep, err)
			}
			clock.add(15 * time.Minute)
			rep, err := rec.Sweep(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			got, cp := recoveryRun(t, svc, run.ID)
			var storedTask model.FlowModelTask
			if err = svc.DB.First(&storedTask, "id = ?", task.ID).Error; err != nil {
				t.Fatal(err)
			}
			var child model.FlowBudgetReservation
			if err = svc.DB.First(&child, "id = ?", task.ReservationID).Error; err != nil {
				t.Fatal(err)
			}
			if unknown {
				if len(rep.Reconcile) != 1 || got.Status != "waiting_external" || storedTask.Status != StatusReconciliation || child.Status != StatusReconciliation {
					t.Fatalf("unknown outcome not parked: rep=%+v run=%s task=%s child=%s", rep, got.Status, storedTask.Status, child.Status)
				}
			} else {
				if len(rep.Failed) != 1 || got.Status != "failed" || storedTask.Status != "failed" || child.Status != "released" {
					t.Fatalf("queued work not closed: rep=%+v run=%s task=%s child=%s", rep, got.Status, storedTask.Status, child.Status)
				}
			}
			if cp.ErrorCode != CanvasRecoveryWorkerLost || ran {
				t.Fatalf("code=%q reran=%v", cp.ErrorCode, ran)
			}
			if recoveryLeaseCount(t, svc.DB, run.ID) != 0 {
				t.Fatal("sweeper lease left behind")
			}
			// Idempotent: sweeping again changes nothing.
			clock.add(time.Hour)
			if rep, err = rec.Sweep(context.Background()); err != nil || len(rep.Failed)+len(rep.Reconcile)+len(rep.Requeued) != 0 {
				t.Fatalf("second sweep acted: %+v %v", rep, err)
			}
			again, _ := recoveryRun(t, svc, run.ID)
			if again.Revision != got.Revision {
				t.Fatalf("run mutated by second sweep: %d -> %d", got.Revision, again.Revision)
			}
		})
	}
}

// A run whose worker is alive (renewing its lease) is never closed, however
// old its last checkpoint is.
func TestCanvasRecoverySkipsRunWithLiveLease(t *testing.T) {
	svc, in, _, clock, run := recoverySetup(t)
	startedRun(t, svc, in, run, false)
	clock.add(time.Hour)
	if _, err := (RunLeases{DB: svc.DB, Now: svc.Now}).Acquire(run.ID, "crashed-worker"); err != nil {
		t.Fatal(err) // the owner renews (same owner re-acquire)
	}
	rep, err := (CanvasWorkflowRecovery{Executions: svc}).Sweep(context.Background())
	if err != nil || len(rep.Skipped) != 1 || len(rep.Failed)+len(rep.Reconcile) != 0 {
		t.Fatalf("live run swept: %+v %v", rep, err)
	}
}

// PG with FORCE RLS as the non-superuser app role: discovery runs in system
// context and finds every tenant's stale run, and each run is closed inside a
// transaction scoped to its own tenant, which cannot see other tenants' runs.
func TestPGCanvasRecoveryRespectsTenantRLS(t *testing.T) {
	db := pgTestDB(t)
	ctx := context.Background()
	clock := &recoveryClock{t: time.Unix(1800000000, 0).UTC()}
	svc := CanvasWorkflowExecutions{DB: db, Authority: &workflowTestAuthority{approved: true}, Now: clock.now}
	ins := map[string]CanvasWorkflowAdmission{}
	runs := map[string]model.CanvasWorkflowExecution{}
	for _, u := range []string{"u_alice", "u_bob"} {
		if err := db.Create(&model.User{ID: u, Username: u}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveScope(db, u, ""); err != nil {
			t.Fatal(err)
		}
		fund(t, Billing{DB: db}, u, 10000)
		if _, err := (Billing{DB: db}).Account(db, model.PersonalTenantID(u)); err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.CanvasProject{ID: "canvas-" + u, UserID: u, Revision: 7, PayloadJSON: `{"nodes":[{"id":"node-a","type":"skill"}],"connections":[]}`}).Error; err != nil {
			t.Fatal(err)
		}
		rev := int64(7)
		in := CanvasWorkflowAdmission{UserID: u, TenantID: model.PersonalTenantID(u), WorkspaceID: model.PersonalWorkspaceID(u), CanvasID: "canvas-" + u, ExecutionKey: "k-" + u, ExpectedRevision: &rev, BudgetMicro: 1000}
		run, err := svc.Admit(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		ins[u], runs[u] = in, run
	}
	task := startedRun(t, svc, ins["u_alice"], runs["u_alice"], false)
	clock.add(time.Hour)
	var seen int64
	if err := TenantTx(db, model.PersonalTenantID("u_alice"), func(tx *gorm.DB) error {
		return tx.Model(&model.CanvasWorkflowExecution{}).Where("id = ?", runs["u_bob"].ID).Count(&seen).Error
	}); err != nil || seen != 0 {
		t.Fatalf("alice-scoped transaction saw bob's run: %d %v", seen, err)
	}
	rep, err := (CanvasWorkflowRecovery{Executions: svc}).Sweep(ctx)
	if err != nil || len(rep.Failed) != 2 {
		t.Fatalf("sweep: %+v %v", rep, err)
	}
	for u, run := range runs {
		got, cp := recoveryRun(t, svc, run.ID)
		if got.Status != "failed" || cp.ErrorCode != CanvasRecoveryWorkerLost || got.TenantID != model.PersonalTenantID(u) {
			t.Fatalf("%s: %s %+v", u, got.Status, cp)
		}
	}
	var storedTask model.FlowModelTask
	if err := db.First(&storedTask, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	var child model.FlowBudgetReservation
	if err := db.First(&child, "id = ?", task.ReservationID).Error; err != nil {
		t.Fatal(err)
	}
	if storedTask.Status != "failed" || child.Status != "released" {
		t.Fatalf("alice's queued work not closed: %s %s", storedTask.Status, child.Status)
	}
}
