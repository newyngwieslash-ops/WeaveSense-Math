package flow

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/http"
	"strings"
	"time"
	"yingce/backend/internal/model"
)

var ErrCanvasWorkflowConflict = errors.New("flow: canvas workflow request or revision conflict")

// Authority must resolve server-side approval, policy and model access; a caller
// supplied approval id or boolean is not authorization. Frozen requests recheck
// current revocations without replacing the admitted capability snapshot.
type CanvasWorkflowAuthority interface {
	Authorize(context.Context, *gorm.DB, CanvasWorkflowAccess) (CapabilityInput, error)
}
type CanvasWorkflowAccess struct {
	UserID, TenantID, WorkspaceID, CanvasID, RunID, RequestHash string
	CanvasRevision, BudgetMicro                                 int64
	Frozen                                                      *model.FlowCapabilitySnapshot
	AuthorityBinding                                            json.RawMessage
	RequestJSON                                                 string
}
type CanvasWorkflowExecutions struct {
	DB        *gorm.DB
	Authority CanvasWorkflowAuthority
	Now       func() time.Time
}
type CanvasWorkflowAdmission struct {
	UserID, TenantID, WorkspaceID, CanvasID, ExecutionKey string
	ExpectedRevision                                      *int64
	BudgetMicro                                           int64
	RefreshBindings, SaveReport                           bool
	ReportTitle, TargetNodeID                             string
	// AuthorityBinding is populated by trusted adapters only, never HTTP JSON decoding.
	AuthorityBinding json.RawMessage
}
type CanvasWorkflowCheckpoint struct {
	Stage    string                                  `json:"stage"`
	Nodes    map[string]CanvasWorkflowNodeCheckpoint `json:"nodes"`
	Result   json.RawMessage                         `json:"result,omitempty"`
	Error    string                                  `json:"error,omitempty"`
	ReportID string                                  `json:"report_id,omitempty"`
}
type CanvasWorkflowNodeCheckpoint struct {
	TaskID      string          `json:"task_id,omitempty"`
	TaskKey     string          `json:"task_key,omitempty"`
	RequestHash string          `json:"request_hash,omitempty"`
	PromptHash  string          `json:"prompt_hash,omitempty"`
	Status      string          `json:"status"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       string          `json:"error,omitempty"`
}

func (s CanvasWorkflowExecutions) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// CanvasWorkflowRunID is Admit's deterministic run identity; trusted adapters
// use it to detect an already-admitted cycle before consuming plan budget.
func CanvasWorkflowRunID(tenantID, workspaceID, canvasID, executionKey string) string {
	return "cw_" + hashJSON([]string{tenantID, workspaceID, canvasID, executionKey})[:48]
}

// CanvasMonitorConfigHash exposes the monitor-plan configuration hash so the
// application authority can verify a recurring plan against the live canvas.
func CanvasMonitorConfigHash(payloadJSON string) (string, error) {
	_, hash, err := monitorCanvasConfig(payloadJSON)
	return hash, err
}

// CanvasProject has no workspace foreign key today. Only its owner's existing
// personal workspace is supported; conversation documents are not ownership proof.
func (s CanvasWorkflowExecutions) scope(tx *gorm.DB, user, tenant, workspace, canvas string) (model.CanvasProject, error) {
	var p model.CanvasProject
	if user == "" || tenant != model.PersonalTenantID(user) || workspace != model.PersonalWorkspaceID(user) {
		return p, ErrForbidden
	}
	if err := AuthorizeGrant(tx, "user:"+user, tenant, workspace, "intel.collect"); err != nil {
		return p, err
	}
	if err := tx.First(&p, "id = ? AND user_id = ?", canvas, user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return p, ErrForbidden
		}
		return p, err
	}
	return p, nil
}

func (s CanvasWorkflowExecutions) authorize(ctx context.Context, tx *gorm.DB, r model.CanvasWorkflowExecution, frozen *model.FlowCapabilitySnapshot) (CapabilityInput, error) {
	if s.Authority == nil {
		return CapabilityInput{}, ErrForbidden
	}
	var admission CanvasWorkflowAdmission
	if err := json.Unmarshal([]byte(r.RequestJSON), &admission); err != nil {
		return CapabilityInput{}, err
	}
	return s.Authority.Authorize(ctx, tx, CanvasWorkflowAccess{UserID: r.UserID, TenantID: r.TenantID, WorkspaceID: r.WorkspaceID, CanvasID: r.CanvasID, RunID: r.ID, RequestHash: r.RequestHash, CanvasRevision: r.CanvasRevision, BudgetMicro: r.BudgetMicro, Frozen: frozen, RequestJSON: r.RequestJSON, AuthorityBinding: admission.AuthorityBinding})
}

// Admit must precede evaluation for REST, agent and scheduler adapters. It never
// starts a worker. The transaction couples identity, capability, budget and status.
func (s CanvasWorkflowExecutions) Admit(ctx context.Context, in CanvasWorkflowAdmission) (out model.CanvasWorkflowExecution, err error) {
	if s.DB == nil || in.ExpectedRevision == nil || *in.ExpectedRevision < 0 || strings.TrimSpace(in.ExecutionKey) == "" || len(in.ExecutionKey) > 160 || in.BudgetMicro <= 0 || in.TargetNodeID != "" {
		return out, ErrInvalid
	}
	if err := ValidateCanvasWorkflowPayload(in); err != nil {
		return out, err
	}
	raw, e := json.Marshal(in)
	if e != nil {
		return out, e
	}
	id := "cw_" + hashJSON([]string{in.TenantID, in.WorkspaceID, in.CanvasID, in.ExecutionKey})[:48]
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, e := s.scope(tx, in.UserID, in.TenantID, in.WorkspaceID, in.CanvasID)
		if e != nil {
			return e
		}
		existing := tx.First(&out, "id = ?", id).Error
		if existing == nil {
			if out.UserID != in.UserID || out.RequestHash != hashJSON(in) {
				return ErrCanvasWorkflowConflict
			}
			return s.recheck(ctx, tx, out)
		}
		if !errors.Is(existing, gorm.ErrRecordNotFound) {
			return existing
		}
		if err := ValidateCanvasWorkflowPayload(json.RawMessage(p.PayloadJSON)); err != nil {
			return err
		}
		if p.Revision != *in.ExpectedRevision {
			return ErrCanvasWorkflowConflict
		}
		// A conditional write locks the source revision through admission on both DBs.
		lock := tx.Model(&model.CanvasProject{}).Where("id = ? AND user_id = ? AND revision = ?", p.ID, in.UserID, p.Revision).UpdateColumn("revision", gorm.Expr("revision"))
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return ErrCanvasWorkflowConflict
		}
		// A concurrent PostgreSQL admission may have committed while this writer
		// waited on the canvas lock. Re-read before allocating any budget or pins.
		if e = tx.First(&out, "id = ?", id).Error; e == nil {
			if out.UserID != in.UserID || out.RequestHash != hashJSON(in) {
				return ErrCanvasWorkflowConflict
			}
			return s.recheck(ctx, tx, out)
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		out = model.CanvasWorkflowExecution{ID: id, TenantID: in.TenantID, WorkspaceID: in.WorkspaceID, CanvasID: in.CanvasID, UserID: in.UserID, ExecutionKey: in.ExecutionKey, RequestHash: hashJSON(in), RequestJSON: string(raw), CanvasRevision: p.Revision, CanvasJSON: p.PayloadJSON, BudgetMicro: in.BudgetMicro, Status: "admitted", Revision: 1, CheckpointJSON: `{"stage":"admitted","nodes":{}}`, CreatedAt: s.now(), UpdatedAt: s.now()}
		capability, e := s.authorize(ctx, tx, out, nil)
		if e != nil {
			return e
		}
		if capability.PolicyHash == "" || len(capability.ModelRoute) == 0 || len(FrozenModels(capability.ModelRoute)) == 0 {
			return ErrForbidden
		}
		if _, e = json.Marshal(capability); e != nil {
			return ErrInvalid
		}
		// Do not let Billing.Account silently provision a missing authorization/quota.
		var account model.FlowBillingAccount
		if e = tx.First(&account, "tenant_id = ?", in.TenantID).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrForbidden
			}
			return e
		}
		if account.PayerUserID == "" {
			return ErrForbidden
		}
		capability.TenantID = in.TenantID
		capability.WorkspaceID = in.WorkspaceID
		capability.RunID = id
		capability.SubjectKind = SubjectCanvas
		installed, e := ListInstalledSkills(tx, Scope{TenantID: in.TenantID, WorkspaceID: in.WorkspaceID})
		if e != nil {
			return e
		}
		for _, ref := range capability.Skills {
			found := false
			for _, i := range installed {
				if i.SkillID == ref.ID && i.Version == ref.Version && i.Hash == ref.Hash {
					found = true
				}
			}
			skill, ok := FindSkill(ref.ID, ref.Version)
			if !found || !ok || skill.Hash != ref.Hash {
				return ErrSkillNotFound
			}
			if e = tx.Create(&model.FlowSkillRunPin{RunID: id, SkillID: ref.ID, TenantID: in.TenantID, Version: ref.Version, Hash: ref.Hash, CreatedAt: s.now()}).Error; e != nil {
				return e
			}
		}
		snap, e := FreezeCapabilitySnapshot(tx, capability, s.now())
		if e != nil {
			return e
		}
		reservation, e := (Billing{DB: tx, Now: s.Now}).Reserve(in.TenantID, id, "canvas_workflow", id, in.BudgetMicro)
		if e != nil {
			return e
		}
		out.CapabilitySnapshotID = snap.ID
		out.ReservationID = reservation.ID
		if e = tx.Create(&out).Error; e != nil {
			return e
		}
		return tx.Create(&model.FlowOperationProjection{OperationID: id, TenantID: in.TenantID, WorkspaceID: in.WorkspaceID, Status: out.Status, Sequence: 1, SourceVersion: 1, SyncStatus: "synced", AsOf: s.now()}).Error
	})
	return out, err
}

func (s CanvasWorkflowExecutions) recheck(ctx context.Context, tx *gorm.DB, r model.CanvasWorkflowExecution) error {
	if _, err := s.scope(tx, r.UserID, r.TenantID, r.WorkspaceID, r.CanvasID); err != nil {
		return err
	}
	snap, err := RunCapability(tx, r.ID)
	if err != nil {
		return ErrNoCapabilitySnapshot
	}
	if snap.ID != r.CapabilitySnapshotID || snap.TenantID != r.TenantID {
		return ErrForbidden
	}
	content, err := decodeCapability(snap)
	if err != nil {
		return err
	}
	if content.PriceBook.Version == "" || len(FrozenModels(content.ModelRoute)) == 0 {
		return ErrNoCapabilitySnapshot
	}
	_, err = s.authorize(ctx, tx, r, &snap)
	return err
}

func (s CanvasWorkflowExecutions) get(ctx context.Context, tx *gorm.DB, user, id string) (r model.CanvasWorkflowExecution, err error) {
	if user == "" || id == "" {
		return r, ErrForbidden
	}
	if err = tx.First(&r, "id = ? AND user_id = ?", id, user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrNotFound
		}
		return
	}
	err = s.recheck(ctx, tx, r)
	return
}
func (s CanvasWorkflowExecutions) Get(ctx context.Context, user, id string) (out model.CanvasWorkflowExecution, err error) {
	if s.DB == nil {
		return out, ErrInvalid
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var e error; out, e = s.get(ctx, tx, user, id); return e })
	return
}
func (s CanvasWorkflowExecutions) Acquire(ctx context.Context, user, id, worker string) (token int64, err error) {
	if s.DB == nil || strings.TrimSpace(worker) == "" {
		return 0, ErrInvalid
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, e := s.get(ctx, tx, user, id)
		if e != nil {
			return e
		}
		if r.Status == "succeeded" || r.Status == "failed" || r.Status == "cancel_requested" || r.Status == "cancelled" {
			return ErrCanvasWorkflowConflict
		}
		token, e = (RunLeases{DB: tx, Now: s.Now}).Acquire(id, worker)
		return e
	})
	return
}
func (s CanvasWorkflowExecutions) fence(tx *gorm.DB, id, worker string, token int64) error {
	// A write lock, not merely a read, fences lease takeover until checkpoint commit.
	q := tx.Model(&model.FlowRunLease{}).Where("run_id = ? AND owner = ? AND fencing_token = ? AND expires_at > ?", id, worker, token, s.now()).UpdateColumn("updated_at", s.now())
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return ErrLeaseHeld
	}
	return nil
}

// PrepareModelNode persists a stable model request identity BEFORE enqueue. The
// returned input can be passed to ModelTasks.Enqueue in the caller's transaction.
// This is not permission to dispatch: the worker still needs current revocation
// and frozen-route enforcement immediately before its external model call.
func (s CanvasWorkflowExecutions) PrepareModelNode(ctx context.Context, user, id, worker string, token int64, node, modelID string, in ModelTaskInput) (out ModelTaskInput, err error) {
	if s.DB == nil || node == "" || modelID == "" || in.BudgetMicro <= 0 || in.MaxTokens <= 0 {
		return out, ErrInvalid
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, e := s.get(ctx, tx, user, id)
		if e != nil {
			return e
		}
		if e = s.fence(tx, id, worker, token); e != nil {
			return e
		}
		if r.Status == "succeeded" || r.Status == "failed" || r.Status == "cancel_requested" || r.Status == "cancelled" {
			return ErrCanvasWorkflowConflict
		}
		if e = CheckRunModel(tx, id, modelID); e != nil {
			return e
		}
		var doc struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		}
		if e = json.Unmarshal([]byte(r.CanvasJSON), &doc); e != nil {
			return e
		}
		found := false
		for _, n := range doc.Nodes {
			if n.ID == node {
				found = true
			}
		}
		if !found {
			return ErrForbidden
		}
		in.TenantID = r.TenantID
		in.ParentResID = r.ReservationID
		in.Key = "cwn_" + hashJSON([]string{id, node})[:48]
		in.Purpose = "canvas_workflow_node"
		in.RefID = id
		requestHash := hashJSON(struct {
			Model string
			Input ModelTaskInput
		}{modelID, in})
		cp, e := DecodeCanvasWorkflowCheckpoint(r)
		if e != nil {
			return e
		}
		if previous, ok := cp.Nodes[node]; ok && previous.RequestHash != "" {
			if previous.RequestHash != requestHash || previous.TaskKey != in.Key {
				return ErrCanvasWorkflowConflict
			}
		} else {
			if cp.Nodes == nil {
				cp.Nodes = map[string]CanvasWorkflowNodeCheckpoint{}
			}
			cp.Nodes[node] = CanvasWorkflowNodeCheckpoint{TaskKey: in.Key, RequestHash: requestHash, Status: "prepared"}
			if _, e = s.save(tx, r, cp); e != nil {
				return e
			}
		}
		envelope, e := json.Marshal(map[string]any{"canvas_workflow": map[string]any{"run_id": id, "node_id": node, "capability_snapshot_id": r.CapabilitySnapshotID, "request_hash": requestHash}, "payload": in.Context})
		if e != nil {
			return e
		}
		in.Context = string(envelope)
		out = in
		return nil
	})
	return
}

func (s CanvasWorkflowExecutions) Checkpoint(ctx context.Context, user, id, worker string, token, revision int64, cp CanvasWorkflowCheckpoint) (out model.CanvasWorkflowExecution, err error) {
	if s.DB == nil {
		return out, ErrInvalid
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, e := s.get(ctx, tx, user, id)
		if e != nil {
			return e
		}
		if e = s.fence(tx, id, worker, token); e != nil {
			return e
		}
		if r.Revision != revision || r.Status == "succeeded" || r.Status == "failed" || r.Status == "cancel_requested" || r.Status == "cancelled" {
			return ErrCanvasWorkflowConflict
		}
		old, e := DecodeCanvasWorkflowCheckpoint(r)
		if e != nil {
			return e
		}
		for key, n := range old.Nodes {
			next, ok := cp.Nodes[key]
			if !ok || next.TaskKey != n.TaskKey || next.RequestHash != n.RequestHash || (n.TaskID != "" && next.TaskID != n.TaskID) {
				return ErrCanvasWorkflowConflict
			}
		}
		var doc struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		}
		if e = json.Unmarshal([]byte(r.CanvasJSON), &doc); e != nil {
			return e
		}
		known := map[string]bool{}
		for _, n := range doc.Nodes {
			known[n.ID] = true
		}
		for key, n := range cp.Nodes {
			if !known[key] {
				return ErrForbidden
			}
			prior, ok := old.Nodes[key]
			if (!ok || prior.RequestHash == "") && (n.TaskID != "" || n.TaskKey != "" || n.RequestHash != "") {
				return ErrCanvasWorkflowConflict
			}
			if n.TaskID != "" {
				var task model.FlowModelTask
				if e = tx.First(&task, "id = ? AND tenant_id = ? AND idem_key = ?", n.TaskID, r.TenantID, n.TaskKey).Error; e != nil {
					return ErrForbidden
				}
				var res model.FlowBudgetReservation
				if e = tx.First(&res, "id = ? AND parent_id = ?", task.ReservationID, r.ReservationID).Error; e != nil {
					return ErrForbidden
				}
			}
		}
		out, e = s.save(tx, r, cp)
		return e
	})
	return
}
func (s CanvasWorkflowExecutions) save(tx *gorm.DB, r model.CanvasWorkflowExecution, cp CanvasWorkflowCheckpoint) (model.CanvasWorkflowExecution, error) {
	switch cp.Stage {
	case "admitted", "running", "waiting_external", "saving_result", "saving_report", "succeeded", "failed", "cancel_requested", "cancelled":
	default:
		return r, ErrInvalid
	}
	cp.Error = SanitizeCanvasWorkflowError(cp.Error)
	for id, node := range cp.Nodes {
		node.Error = SanitizeCanvasWorkflowError(node.Error)
		cp.Nodes[id] = node
	}
	if err := ValidateCanvasWorkflowPayload(cp); err != nil {
		return r, err
	}
	raw, err := json.Marshal(cp)
	if err != nil {
		return r, err
	}
	if len(raw) > 512*1024 {
		return r, ErrInvalid
	}
	if cp.Stage == "succeeded" || cp.Stage == "failed" || cp.Stage == "cancelled" {
		// ModelTasks settles children; root settlement rolls their consumed amounts
		// into the existing ledger exactly once and refuses unresolved children.
		if _, err = (Billing{DB: tx, Now: s.Now}).Settle(r.ReservationID, 0); err != nil {
			return r, err
		}
	}
	q := tx.Model(&model.CanvasWorkflowExecution{}).Where("id = ? AND revision = ?", r.ID, r.Revision).Updates(map[string]any{"checkpoint_json": string(raw), "status": cp.Stage, "revision": r.Revision + 1, "updated_at": s.now()})
	if q.Error != nil {
		return r, q.Error
	}
	if q.RowsAffected != 1 {
		return r, ErrCanvasWorkflowConflict
	}
	r.CheckpointJSON = string(raw)
	r.Status = cp.Stage
	r.Revision++
	r.UpdatedAt = s.now()
	err = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operation_id"}}, DoUpdates: clause.AssignmentColumns([]string{"status", "sequence", "source_version", "as_of"})}).Create(&model.FlowOperationProjection{OperationID: r.ID, TenantID: r.TenantID, WorkspaceID: r.WorkspaceID, Status: r.Status, Sequence: r.Revision, SourceVersion: r.Revision, SyncStatus: "synced", AsOf: s.now()}).Error
	return r, err
}

// CloseFailedRun is a fenced internal cleanup, not authorization to execute.
// Current revocation cannot prevent releasing a run that was never dispatched.
func (s CanvasWorkflowExecutions) CloseFailedRun(ctx context.Context, userID, runID, worker string, token int64) error {
	if s.DB == nil || userID == "" || worker == "" {
		return ErrInvalid
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run model.CanvasWorkflowExecution
		if err := tx.First(&run, "id = ? AND user_id = ?", runID, userID).Error; err != nil {
			return err
		}
		if run.Status == "succeeded" || run.Status == "failed" || run.Status == "cancel_requested" || run.Status == "cancelled" || run.Status == "saving_result" || run.Status == "saving_report" {
			return nil
		}
		if err := s.fence(tx, runID, worker, token); err != nil {
			return err
		}
		cp, err := DecodeCanvasWorkflowCheckpoint(run)
		if err != nil {
			return err
		}
		var reservations []model.FlowBudgetReservation
		if err := tx.Where("parent_id = ? AND tenant_id = ?", run.ReservationID, run.TenantID).Find(&reservations).Error; err != nil {
			return err
		}
		uncertain := false
		for _, reservation := range reservations {
			if reservation.Status == "settled" || reservation.Status == "released" {
				continue
			}
			var task model.FlowModelTask
			if err := tx.First(&task, "reservation_id = ? AND tenant_id = ?", reservation.ID, run.TenantID).Error; err != nil {
				uncertain = true
				continue
			}
			switch task.Status {
			case "queued", StatusRetryWait:
				q := tx.Model(&model.FlowModelTask{}).Where("id = ? AND status IN ?", task.ID, []string{"queued", StatusRetryWait}).Updates(map[string]any{"status": "failed", "last_error": "workflow stopped before dispatch", "updated_at": s.now()})
				if q.Error != nil {
					return q.Error
				}
				if q.RowsAffected != 1 {
					uncertain = true
					continue
				}
				if _, err := (Billing{DB: tx, Now: s.Now}).Release(reservation.ID); err != nil {
					return err
				}
			default:
				uncertain = true
			}
		}
		cp.Stage = "failed"
		cp.Error = "workflow stopped before completion"
		if uncertain {
			cp.Stage = "waiting_external"
			cp.Error = "workflow stopped with unresolved external work"
		}
		_, err = s.save(tx, run, cp)
		return err
	})
}

// CloseRejectedModelTask is an internal, identity-bound cleanup operation. It
// grants no execution permission and never releases an uncertain external call.
func (s CanvasWorkflowExecutions) CloseRejectedModelTask(ctx context.Context, taskID, expectedTaskKey string) error {
	if s.DB == nil || taskID == "" || expectedTaskKey == "" {
		return ErrInvalid
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task model.FlowModelTask
		if err := tx.First(&task, "id = ? AND idem_key = ?", taskID, expectedTaskKey).Error; err != nil {
			return err
		}
		var env canvasSkillTaskEnvelope
		if json.Unmarshal([]byte(task.Context), &env) != nil || env.Workflow == nil {
			return ErrForbidden
		}
		var run model.CanvasWorkflowExecution
		if err := tx.First(&run, "id = ? AND tenant_id = ?", env.Workflow.RunID, task.TenantID).Error; err != nil {
			return ErrForbidden
		}
		cp, err := DecodeCanvasWorkflowCheckpoint(run)
		if err != nil {
			return err
		}
		node, ok := cp.Nodes[env.Workflow.NodeID]
		if !ok || node.TaskKey != expectedTaskKey || node.RequestHash != env.Workflow.RequestHash || (node.TaskID != "" && node.TaskID != task.ID) {
			return ErrForbidden
		}
		var reservation model.FlowBudgetReservation
		if err := tx.First(&reservation, "id = ? AND tenant_id = ? AND parent_id = ?", task.ReservationID, run.TenantID, run.ReservationID).Error; err != nil {
			return ErrForbidden
		}
		if task.Status != "queued" && task.Status != StatusRetryWait {
			return nil
		}
		update := tx.Model(&model.FlowModelTask{}).Where("id = ? AND status IN ?", task.ID, []string{"queued", StatusRetryWait}).Updates(map[string]any{"status": "failed", "last_error": "managed workflow dispatch rejected", "updated_at": s.now()})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrCanvasWorkflowConflict
		}
		if _, err := (Billing{DB: tx, Now: s.Now}).Release(task.ReservationID); err != nil {
			return err
		}
		node.TaskID = task.ID
		node.Status = "failed"
		node.Error = "managed workflow dispatch rejected"
		cp.Nodes[env.Workflow.NodeID] = node
		cp.Stage = "failed"
		cp.Error = node.Error
		var parent model.FlowBudgetReservation
		if err := tx.First(&parent, "id = ?", run.ReservationID).Error; err != nil {
			return err
		}
		if parent.AllocatedMicro > 0 {
			cp.Stage = "waiting_external"
		}
		if _, err := s.save(tx, run, cp); err != nil {
			return err
		}
		return tx.Model(&model.FlowRunLease{}).Where("run_id = ?", run.ID).Updates(map[string]any{"fencing_token": gorm.Expr("fencing_token + 1"), "owner": "dispatch-rejected", "expires_at": s.now(), "updated_at": s.now()}).Error
	})
}

func DecodeCanvasWorkflowCheckpoint(r model.CanvasWorkflowExecution) (CanvasWorkflowCheckpoint, error) {
	var c CanvasWorkflowCheckpoint
	err := json.Unmarshal([]byte(r.CheckpointJSON), &c)
	return c, err
}
func CanvasWorkflowHTTPStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrCanvasWorkflowConflict), errors.Is(err, ErrLeaseHeld):
		return http.StatusConflict
	case errors.Is(err, ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrApprovalRequired), errors.Is(err, ErrToolPermission), errors.Is(err, ErrNoCapabilitySnapshot), errors.Is(err, ErrModelNotInSnapshot), errors.Is(err, ErrToolNotInSnapshot):
		return http.StatusForbidden
	case errors.Is(err, ErrBudgetInsufficient), errors.Is(err, ErrBudgetQuota), errors.Is(err, ErrBudgetProvider), errors.Is(err, ErrBudgetParent):
		return http.StatusPaymentRequired
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
