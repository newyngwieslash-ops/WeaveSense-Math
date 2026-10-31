package flow

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

// CanvasWorkflowErrorCode classifies why a managed canvas run stopped. It is
// stored in the checkpoint (error_code) next to the sanitized message.
func CanvasWorkflowErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrModelUnknownOutcome):
		return "unknown_outcome"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, ErrLeaseHeld):
		return "lease_lost"
	case errors.Is(err, ErrCanvasWorkflowConflict):
		return "conflict"
	case errors.Is(err, ErrBudgetInsufficient), errors.Is(err, ErrBudgetQuota), errors.Is(err, ErrBudgetProvider), errors.Is(err, ErrBudgetParent):
		return "budget"
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrApprovalRequired), errors.Is(err, ErrToolPermission), errors.Is(err, ErrNoCapabilitySnapshot), errors.Is(err, ErrModelNotInSnapshot), errors.Is(err, ErrToolNotInSnapshot):
		return "forbidden"
	case errors.Is(err, ErrNotFound):
		return "not_found"
	case errors.Is(err, ErrInvalid):
		return "invalid"
	default:
		return "internal"
	}
}

// Recovery codes written by the sweeper.
const (
	CanvasRecoveryWorkerLost = "worker_lost"
	CanvasRecoveryExhausted  = "recovery_exhausted"
)

// CanvasWorkflowRecovery finds managed canvas runs that no worker owns and
// settles them. The durable admitted row is the recovery queue (see
// wakeCanvasWorkflow); this is the sweeper that drains it after a crash, a
// restart, or a runner error that happened before the run took its lease.
//
// Policy, chosen so no paid call is ever repeated blindly:
//   - a run still "admitted" whose checkpoint has no node and no child budget
//     reservation has dispatched nothing, so it is requeued (Run is called
//     again; the lease fences duplicates) at most MaxRequeue times;
//   - every other stale run (it started nodes, or requeues are used up, or its
//     authority no longer holds) is closed under a sweeper lease with the same
//     fenced cleanup the runner uses: queued model tasks are failed and their
//     holds released, any child call whose outcome is unknown keeps its hold
//     and the run parks in waiting_external for reconciliation.
//
// Runs with a live lease, or updated within StaleAfter, are left alone.
type CanvasWorkflowRecovery struct {
	Executions CanvasWorkflowExecutions
	// Run requeues a never-started run; nil means close instead of requeue.
	Run        func(context.Context, CanvasWorkflowExecutions, model.CanvasWorkflowExecution) error
	StaleAfter time.Duration
	MaxRequeue int
	Limit      int
	Worker     string
}

type CanvasWorkflowRecoveryReport struct {
	Requeued  []string
	Failed    []string
	Reconcile []string
	Skipped   []string
}

func (r CanvasWorkflowRecovery) staleAfter() time.Duration {
	if r.StaleAfter > 0 {
		return r.StaleAfter
	}
	return 10 * time.Minute
}

// Sweep settles at most Limit stale runs. Candidate discovery runs in system
// context (no tenant scope), like the other platform workers; each run is
// then handled in a transaction scoped to its own tenant (TenantTx), so RLS
// confines every write to that tenant.
func (r CanvasWorkflowRecovery) Sweep(ctx context.Context) (rep CanvasWorkflowRecoveryReport, err error) {
	ex := r.Executions
	if ex.DB == nil {
		return rep, ErrInvalid
	}
	limit := r.Limit
	if limit <= 0 {
		limit = 20
	}
	now := ex.now()
	var stale []model.CanvasWorkflowExecution
	if err = ex.DB.WithContext(ctx).Where("status IN ? AND updated_at < ?", []string{"admitted", "running"}, now.Add(-r.staleAfter())).
		Order("updated_at, id").Limit(limit).Find(&stale).Error; err != nil {
		return rep, err
	}
	for _, run := range stale {
		outcome, e := r.settle(ctx, run, nil)
		if e != nil {
			slog.Warn("flow: canvas recovery", "run_id", run.ID, "tenant_id", run.TenantID, "error", e)
			err = errors.Join(err, e)
			continue
		}
		switch outcome {
		case "requeued":
			rep.Requeued = append(rep.Requeued, run.ID)
		case "failed":
			rep.Failed = append(rep.Failed, run.ID)
		case "waiting_external":
			rep.Reconcile = append(rep.Reconcile, run.ID)
		default:
			rep.Skipped = append(rep.Skipped, run.ID)
		}
	}
	return rep, err
}

// Settle records a runner error that left the run without an owner: it logs
// the error and, unless another worker holds the run or it already reached a
// terminal or parked state, closes it with the error's code. It never
// requeues; the sweeper decides that later.
func (r CanvasWorkflowRecovery) Settle(ctx context.Context, run model.CanvasWorkflowExecution, cause error) {
	if cause == nil {
		return
	}
	code := CanvasWorkflowErrorCode(cause)
	slog.Warn("flow: managed canvas run stopped", "run_id", run.ID, "tenant_id", run.TenantID, "code", code, "error", SanitizeCanvasWorkflowError(cause.Error()))
	if code == "lease_lost" || code == "unknown_outcome" {
		return // another worker owns it, or the run is parked for reconciliation
	}
	if _, err := r.settle(ctx, run, cause); err != nil && !errors.Is(err, ErrLeaseHeld) {
		slog.Warn("flow: canvas run settle", "run_id", run.ID, "error", err)
	}
}

func (r CanvasWorkflowRecovery) settle(ctx context.Context, stale model.CanvasWorkflowExecution, cause error) (outcome string, err error) {
	ex := r.Executions
	worker := r.Worker
	if worker == "" {
		worker = "canvas-recovery"
	}
	worker += ":" + NewID("")
	requeue := false
	var current model.CanvasWorkflowExecution
	err = TenantTx(ex.DB.WithContext(ctx), stale.TenantID, func(tx *gorm.DB) error {
		if e := tx.First(&current, "id = ? AND tenant_id = ?", stale.ID, stale.TenantID).Error; e != nil {
			return e
		}
		if current.Status != "admitted" && current.Status != "running" {
			outcome = "skipped"
			return nil
		}
		if cause == nil && current.UpdatedAt.After(ex.now().Add(-r.staleAfter())) {
			outcome = "skipped" // progressed since discovery
			return nil
		}
		token, e := (RunLeases{DB: tx, Now: ex.Now}).Acquire(current.ID, worker)
		if errors.Is(e, ErrLeaseHeld) {
			outcome = "skipped" // a live worker owns it
			return nil
		}
		if e != nil {
			return e
		}
		cp, e := DecodeCanvasWorkflowCheckpoint(current)
		if e != nil {
			return e
		}
		var children int64
		if e = tx.Model(&model.FlowBudgetReservation{}).Where("parent_id = ? AND tenant_id = ?", current.ReservationID, current.TenantID).Count(&children).Error; e != nil {
			return e
		}
		untouched := current.Status == "admitted" && len(cp.Nodes) == 0 && children == 0
		authorized := ex.recheck(ctx, tx, current) == nil
		if cause == nil && r.Run != nil && untouched && authorized && cp.Recovery < r.maxRequeue() {
			cp.Recovery++
			if current, e = ex.save(tx, current, cp); e != nil {
				return e
			}
			requeue = true
			outcome = "requeued"
			// Hand the run back free: the requeued runner takes its own lease.
			return tx.Where("run_id = ? AND owner = ? AND fencing_token = ?", current.ID, worker, token).Delete(&model.FlowRunLease{}).Error
		}
		code := CanvasRecoveryWorkerLost
		switch {
		case cause != nil:
			code = CanvasWorkflowErrorCode(cause)
		case !authorized:
			code = "forbidden"
		case untouched && r.Run != nil:
			code = CanvasRecoveryExhausted
		}
		if e = ex.closeFailedRun(tx, current, worker, token, code); e != nil {
			return e
		}
		if e = tx.First(&current, "id = ?", current.ID).Error; e != nil {
			return e
		}
		outcome = current.Status
		return tx.Where("run_id = ? AND owner = ? AND fencing_token = ?", current.ID, worker, token).Delete(&model.FlowRunLease{}).Error
	})
	if err == nil && requeue {
		run := current
		go func() {
			if e := r.Run(context.Background(), ex, run); e != nil {
				r.Settle(context.Background(), run, e)
			}
		}()
	}
	return outcome, err
}

func (r CanvasWorkflowRecovery) maxRequeue() int {
	if r.MaxRequeue > 0 {
		return r.MaxRequeue
	}
	return 3
}
