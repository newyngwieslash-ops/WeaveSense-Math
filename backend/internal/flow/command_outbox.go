package flow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrCommandConflict indicates an idempotency key was reused with different command parameters (PRD §11.1 / §11.3 → 409).
var ErrCommandConflict = errors.New("flow: command idempotency key reused with different parameters")

// CommandInput describes a manual platform → Engine command to persist in the outbox and dispatch.
type CommandInput struct {
	Scope              Scope
	Actor              Actor
	Command            string // collection.request | research.request
	Method             string
	Path               string
	IdempotencyKey     string
	Payload            []byte
	ReserveMicro       int64
	CapabilitySnapshot string
	MaxAttempts        int
}

// CommandDispatcher manages the platform-side command outbox (PRD §11.3):
// manual commands are committed in the same platform DB transaction as the initial
// operation projection and optional budget reservation, then dispatched idempotently
// to Loom Engine with bounded retries and reconciliation protection.
type CommandDispatcher struct {
	DB      *gorm.DB
	Engine  EngineCaller
	Now     func() time.Time
	Lease   time.Duration
	Backoff time.Duration
	// Local executes platform-owned commands in-process instead of calling the
	// engine (canvas.run runs the managed canvas workflow locally). The handler
	// returns (httpStatus, body, err) with engine-call semantics: 2xx marks the
	// command dispatched, 4xx fails it terminally, 5xx/err retries with backoff.
	Local map[string]func(ctx context.Context, row model.FlowCommandOutbox) (int, []byte, error)
}

func (d CommandDispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now().UTC()
}

func (d CommandDispatcher) lease() time.Duration {
	if d.Lease > 0 {
		return d.Lease
	}
	return 30 * time.Second
}

func (d CommandDispatcher) backoff() time.Duration {
	if d.Backoff > 0 {
		return d.Backoff
	}
	return 2 * time.Second
}

func commandRequestHash(command, workspaceID, method, path string, payload []byte) string {
	var canonical []byte
	var obj any
	if len(payload) > 0 && json.Unmarshal(payload, &obj) == nil {
		canonical, _ = json.Marshal(obj)
	} else {
		canonical = payload
	}
	h := sha256.Sum256([]byte(command + "\x00" + workspaceID + "\x00" + method + "\x00" + path + "\x00" + string(canonical)))
	return hex.EncodeToString(h[:])
}

// Enqueue persists a command in flow_command_outbox in the same transaction as its
// initial FlowOperationProjection (and optional FlowBudgetReservation).
// If (tenant_id, idempotency_key) already exists with the same request hash, it returns the
// existing row and replayed=true; if the hash differs, it returns ErrCommandConflict.
func (d CommandDispatcher) Enqueue(ctx context.Context, in CommandInput) (model.FlowCommandOutbox, bool, error) {
	var out model.FlowCommandOutbox
	if d.DB == nil {
		return out, false, errors.New("flow: command outbox db not configured")
	}
	now := d.now()
	hash := commandRequestHash(in.Command, in.Scope.WorkspaceID, in.Method, in.Path, in.Payload)
	rawHeader := strings.TrimSpace(in.IdempotencyKey)
	cmdID := NewID("cmd_")
	opID := NewID("op_")
	storeKey := rawHeader
	if storeKey == "" {
		storeKey = "auto:" + cmdID
	}
	maxAttempts := in.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	capSnap := in.CapabilitySnapshot
	if capSnap == "" {
		capSnap = capabilityFromCtx(ctx)
	}
	replayed := false
	err := d.DB.Transaction(func(tx *gorm.DB) error {
		if rawHeader != "" {
			var prior model.FlowCommandOutbox
			if err := tx.First(&prior, "tenant_id = ? AND idempotency_key = ?", in.Scope.TenantID, rawHeader).Error; err == nil {
				if prior.RequestHash != hash {
					return ErrCommandConflict
				}
				out = prior
				replayed = true
				return nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		var resID string
		if in.ReserveMicro > 0 {
			rsv, err := Billing{DB: tx, Now: d.Now}.Reserve(in.Scope.TenantID, "cmd:"+storeKey, in.Command, opID, in.ReserveMicro)
			if err != nil {
				return err
			}
			resID = rsv.ID
		}
		proj := model.FlowOperationProjection{
			OperationID:   opID,
			TenantID:      in.Scope.TenantID,
			WorkspaceID:   in.Scope.WorkspaceID,
			Status:        "queued",
			Sequence:      0,
			AsOf:          now,
			SourceVersion: 0,
			SyncStatus:    "synced",
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&proj).Error; err != nil {
			return err
		}
		actorRole := in.Scope.Role
		if actorRole == "" {
			actorRole = "editor"
		}
		row := model.FlowCommandOutbox{
			ID:                 cmdID,
			OperationID:        opID,
			TenantID:           in.Scope.TenantID,
			WorkspaceID:        in.Scope.WorkspaceID,
			ActorType:          in.Actor.Type,
			ActorID:            in.Actor.ID,
			ActorRole:          actorRole,
			Command:            in.Command,
			Method:             in.Method,
			Path:               in.Path,
			IdempotencyKey:     storeKey,
			ForwardIdemHeader:  rawHeader,
			RequestHash:        hash,
			Payload:            string(in.Payload),
			ReservationID:      resID,
			CapabilitySnapshot: capSnap,
			Status:             "pending",
			Attempts:           0,
			MaxAttempts:        maxAttempts,
			NextAttemptAt:      now,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		out = row
		return nil
	})
	return out, replayed, err
}

// DispatchOne claims and dispatches one outbox row by ID. If the row is already
// dispatched or permanently failed, it returns the stored state without re-calling the Engine.
func (d CommandDispatcher) DispatchOne(ctx context.Context, id string) (model.FlowCommandOutbox, error) {
	var row model.FlowCommandOutbox
	if d.DB == nil {
		return row, errors.New("flow: command outbox db not configured")
	}
	if err := d.DB.First(&row, "id = ?", id).Error; err != nil {
		return row, err
	}
	if row.Status == "dispatched" || row.Status == "failed" || row.Status == StatusReconciliation {
		return row, nil
	}
	// A nested GORM transaction is only a savepoint. The outbox must be durable
	// before any external effect; the background dispatcher sends after commit.
	if _, inTransaction := d.DB.Statement.ConnPool.(gorm.TxCommitter); inTransaction {
		return row, nil
	}
	now := d.now()
	expired := row.Status == "dispatching" && (row.LeaseUntil == nil || !row.LeaseUntil.After(now))
	if row.Status == "dispatching" && !expired {
		return row, nil
	}
	var status int
	var body []byte
	var callErr error
	if !expired {
		local, hasLocal := d.Local[row.Command]
		if !hasLocal && d.Engine == nil {
			return row, errors.New("flow: engine caller not configured")
		}
		until := now.Add(d.lease())
		claim := d.DB.Model(&model.FlowCommandOutbox{}).
			Where("id = ? AND attempts = ? AND status IN ? AND next_attempt_at <= ?", id, row.Attempts, []string{"pending", StatusRetryWait}, now).
			Updates(map[string]any{"status": "dispatching", "lease_until": until, "attempts": gorm.Expr("attempts + 1"), "updated_at": now})
		if claim.Error != nil {
			return row, claim.Error
		}
		if claim.RowsAffected == 0 {
			err := d.DB.First(&row, "id = ?", id).Error
			return row, err
		}
		// Retain our claimed version; re-reading here could adopt another owner's lease.
		row.Status, row.Attempts, row.LeaseUntil = "dispatching", row.Attempts+1, &until
		callCtx := ctx
		if row.CapabilitySnapshot != "" {
			callCtx = WithCapabilitySnapshot(callCtx, row.CapabilitySnapshot)
		}
		if hasLocal {
			status, body, callErr = local(callCtx, row)
		} else {
			scope := Scope{TenantID: row.TenantID, WorkspaceID: row.WorkspaceID, Role: row.ActorRole}
			if scope.Role == "" {
				scope.Role = "editor"
			}
			actor := Actor{Type: row.ActorType, ID: row.ActorID}
			idem := row.ForwardIdemHeader
			if idem == "" {
				idem = row.IdempotencyKey
			}
			status, body, callErr = d.Engine.Do(callCtx, scope, actor, row.Method, row.Path, idem, []byte(row.Payload))
		}
	}
	finNow := d.now()
	expired = expired || row.LeaseUntil == nil || !row.LeaseUntil.After(finNow)
	nextStatus, errMsg := "dispatched", ""
	engineOpID, jobID, projStatus := parseEngineCommandReceipt(row.OperationID, body)
	updates := map[string]any{"response_status": status, "response_body": string(body), "lease_until": nil, "updated_at": finNow}
	switch {
	case expired:
		nextStatus, errMsg = StatusReconciliation, "dispatch lease expired; outcome unknown"
	case callErr == nil && status > 0 && status < 400:
		updates["engine_operation_id"] = engineOpID
	case callErr == nil && status >= 400 && status < 500:
		nextStatus, errMsg = "failed", extractEngineErrorMsg(body, status)
	default:
		// Preserve the existing bounded retry policy for network/5xx failures.
		nextStatus, errMsg = StatusRetryWait, "engine unavailable"
		if callErr != nil {
			errMsg = callErr.Error()
		} else if status >= 500 {
			errMsg = extractEngineErrorMsg(body, status)
		}
		shift := row.Attempts - 1
		if shift < 0 {
			shift = 0
		}
		if shift > 6 {
			shift = 6
		}
		updates["next_attempt_at"] = finNow.Add(d.backoff() * time.Duration(1<<shift))
		if row.Attempts >= row.MaxAttempts {
			nextStatus = StatusReconciliation
		}
		if callErr == nil {
			callErr = errors.New(errMsg)
		}
	}
	updates["status"], updates["last_error"] = nextStatus, truncRunes(errMsg, 480)
	err := d.DB.Transaction(func(tx *gorm.DB) error {
		// Fence every side effect before touching projections or money.
		result := tx.Model(&model.FlowCommandOutbox{}).
			Where("id = ? AND status = 'dispatching' AND attempts = ?", row.ID, row.Attempts).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrLeaseHeld
		}
		if row.ReservationID != "" {
			switch nextStatus {
			case "failed":
				if _, err := (Billing{DB: tx, Now: d.Now}).Release(row.ReservationID); err != nil {
					return err
				}
			case StatusReconciliation:
				if err := (Billing{DB: tx, Now: d.Now}).MarkUnknown(row.ReservationID, row.OperationID, errMsg); err != nil {
					return err
				}
			}
		}
		if nextStatus == "dispatched" && engineOpID != row.OperationID {
			if err := tx.Where("operation_id = ? AND sequence = 0", row.OperationID).Delete(&model.FlowOperationProjection{}).Error; err != nil {
				return err
			}
			p := model.FlowOperationProjection{OperationID: engineOpID, TenantID: row.TenantID, WorkspaceID: row.WorkspaceID,
				JobID: jobID, Status: projStatus, AsOf: finNow, SyncStatus: "synced"}
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&p).Error
		}
		if nextStatus == "dispatched" || nextStatus == "failed" || nextStatus == StatusReconciliation {
			if nextStatus != "dispatched" {
				projStatus = nextStatus
			}
			upd := map[string]any{"status": projStatus, "as_of": finNow, "sync_status": "synced"}
			if jobID != "" && nextStatus == "dispatched" {
				upd["job_id"] = jobID
			}
			return tx.Model(&model.FlowOperationProjection{}).Where("operation_id = ? AND sequence = 0", row.OperationID).Updates(upd).Error
		}
		return nil
	})
	readErr := d.DB.First(&row, "id = ?", row.ID).Error
	if err != nil || readErr != nil {
		return row, errors.Join(err, readErr, callErr)
	}
	return row, callErr
}

// EnqueueAndDispatch atomically enqueues a command in the platform outbox and
// immediately attempts the first dispatch step. Replayed commands that were already
// dispatched return their stored response without calling the Engine again.
func (d CommandDispatcher) EnqueueAndDispatch(ctx context.Context, in CommandInput) (model.FlowCommandOutbox, bool, error) {
	row, replayed, err := d.Enqueue(ctx, in)
	if err != nil {
		return row, false, err
	}
	if replayed && (row.Status == "dispatched" || row.Status == "failed") {
		return row, true, nil
	}
	dispatched, derr := d.DispatchOne(ctx, row.ID)
	return dispatched, replayed, derr
}

// DispatchDue claims and dispatches due pending / retry_wait outbox rows.
// Expired dispatching leases are moved to reconciliation_required (PRD §11.3 no blind resend after lost ACK).
func (d CommandDispatcher) DispatchDue(ctx context.Context, limit int) (int, error) {
	if d.DB == nil || (d.Engine == nil && len(d.Local) == 0) {
		return 0, nil
	}
	if limit <= 0 {
		limit = 25
	}
	now := d.now()
	var due []model.FlowCommandOutbox
	if err := d.DB.Where("(status IN ? AND next_attempt_at <= ?) OR (status = 'dispatching' AND (lease_until IS NULL OR lease_until <= ?))", []string{"pending", StatusRetryWait}, now, now).
		Order("next_attempt_at ASC, created_at ASC").Limit(limit).Find(&due).Error; err != nil {
		return 0, err
	}
	n := 0
	var dispatchErr error
	for _, r := range due {
		if ctx.Err() != nil {
			return n, errors.Join(dispatchErr, ctx.Err())
		}
		updated, err := d.DispatchOne(ctx, r.ID)
		dispatchErr = errors.Join(dispatchErr, err)
		if err == nil && updated.Status == "dispatched" {
			n++
		}
	}
	return n, dispatchErr
}

// RunCommandDispatcher periodically dispatches due platform command outbox rows.
func RunCommandDispatcher(ctx context.Context, d CommandDispatcher, every time.Duration) {
	if every <= 0 {
		every = 2 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := d.DispatchDue(ctx, 25); err != nil {
				// Upstream errors can include sensitive URLs; keep them out of logs.
				slog.Warn("flow: command dispatch failed; retry or reconciliation pending")
			}
		}
	}
}

func parseEngineCommandReceipt(defaultOpID string, body []byte) (engineOpID, jobID, projStatus string) {
	projStatus = "running"
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	raw := body
	if json.Unmarshal(body, &env) == nil && len(env.Data) > 0 {
		raw = env.Data
	}
	var d struct {
		OperationID string   `json:"operation_id"`
		ID          string   `json:"id"`
		JobID       string   `json:"job_id"`
		JobIDs      []string `json:"job_ids"`
		Status      string   `json:"status"`
	}
	if json.Unmarshal(raw, &d) == nil {
		if d.OperationID != "" {
			engineOpID = d.OperationID
		} else if d.ID != "" {
			engineOpID = d.ID
		}
		if d.JobID != "" {
			jobID = d.JobID
		} else if len(d.JobIDs) > 0 {
			jobID = d.JobIDs[0]
		}
		if d.Status == "succeeded" || d.Status == "failed" || d.Status == "cancelled" {
			projStatus = d.Status
		}
	}
	if engineOpID == "" {
		engineOpID = defaultOpID
	}
	return engineOpID, jobID, projStatus
}

func extractEngineErrorMsg(body []byte, status int) string {
	var env struct {
		Msg string `json:"msg"`
	}
	if json.Unmarshal(body, &env) == nil && env.Msg != "" {
		return env.Msg
	}
	return http.StatusText(status)
}
