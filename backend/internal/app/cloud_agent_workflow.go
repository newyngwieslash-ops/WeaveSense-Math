package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

const canvasWorkflowPurpose = "canvas_workflow_once"

type cloudAgentWorkflowCall struct {
	ExpectedRevision *int64          `json:"expected_revision"`
	Ops              []agentCanvasOp `json:"ops"`
	Run              *struct {
		BudgetMicro     int64  `json:"budget_micro"`
		RefreshBindings bool   `json:"refresh_bindings,omitempty"`
		SaveReport      bool   `json:"save_report,omitempty"`
		ReportTitle     string `json:"report_title,omitempty"`
	} `json:"run"`
}

type cloudAgentWorkflowBinding struct {
	Admission   flow.CanvasWorkflowAdmission `json:"admission"`
	Capability  flow.CapabilityInput         `json:"capability"`
	PriceHash   string                       `json:"priceHash"`
	CanvasHash  string                       `json:"canvasHash"`
	OpsHash     string                       `json:"opsHash"`
	CallHash    string                       `json:"callHash"`
	RunID       string                       `json:"runId,omitempty"`
	RequestHash string                       `json:"requestHash,omitempty"`
}

func decodeCloudAgentWorkflowCall(raw string) (cloudAgentWorkflowCall, error) {
	var args cloudAgentWorkflowCall
	if err := decodeCloudAgentJSONObject(raw, &args); err != nil {
		return args, flow.ErrInvalid
	}
	if args.ExpectedRevision == nil || *args.ExpectedRevision < 0 || args.Ops == nil || args.Run == nil || args.Run.BudgetMicro <= 0 || len(args.Ops) > 100 || len(args.Run.ReportTitle) > 240 {
		return args, flow.ErrInvalid
	}
	return args, nil
}

func workflowGatewayRoute(g flow.ModelGateway) (map[string]any, error) {
	gateway, ok := g.(*flow.OpenAICompatGateway)
	if !ok || gateway == nil || gateway.Model == "" || gateway.URL == "" || gateway.MicroPer1KTok <= 0 {
		return nil, flow.ErrForbidden
	}
	hash := sha256.Sum256([]byte(strings.TrimRight(gateway.URL, "/")))
	statusHash := sha256.Sum256([]byte(strings.TrimRight(flow.ModelStatusFromEnv().URL, "/")))
	return map[string]any{"flow_model_gateway": map[string]any{"model": gateway.Model, "url_hash": hex.EncodeToString(hash[:]), "pricing_hash": capabilityPolicyHash(gateway.MicroPer1KTok), "micro_per_1k_tokens": gateway.MicroPer1KTok, "status_url_hash": hex.EncodeToString(statusHash[:])}}, nil
}

func (s *Service) ConfigureCanvasWorkflowRunner(run func(context.Context, flow.CanvasWorkflowExecutions, model.CanvasWorkflowExecution) error) {
	s.canvasWorkflowRunner = run
}
func (s *Service) CanvasWorkflowExecutions() flow.CanvasWorkflowExecutions {
	return flow.CanvasWorkflowExecutions{DB: s.repo.DB(), Authority: cloudAgentWorkflowAuthority{s: s}}
}

type cloudAgentWorkflowAuthority struct{ s *Service }

func (a cloudAgentWorkflowAuthority) Authorize(ctx context.Context, db *gorm.DB, access flow.CanvasWorkflowAccess) (flow.CapabilityInput, error) {
	var empty flow.CapabilityInput
	if binding := string(access.AuthorityBinding); binding != "" && binding != "null" {
		return a.authorizeRecurringCanvas(ctx, db, access)
	}
	if access.UserID == "" || access.TenantID != model.PersonalTenantID(access.UserID) || access.WorkspaceID != model.PersonalWorkspaceID(access.UserID) {
		return empty, flow.ErrForbidden
	}
	if err := flow.AuthorizeGrant(db, "user:"+access.UserID, access.TenantID, access.WorkspaceID, "intel.collect"); err != nil {
		return empty, err
	}
	var index model.FlowAgentToolCall
	if err := db.WithContext(ctx).Where("tenant_id = ? AND call_id = ? AND tool_name = ?", access.TenantID, access.RunID, "canvas_run_workflow").First(&index).Error; err != nil {
		return empty, flow.ErrApprovalRequired
	}
	run, err := repository.New(db).CloudAgent(access.UserID, index.Result)
	if err != nil {
		return empty, flow.ErrForbidden
	}
	state, err := cloudAgentDecode(run)
	if err != nil {
		return empty, flow.ErrForbidden
	}
	approval, ok := state.WorkflowApprovals[index.ApprovalID]
	if !ok || approval.Purpose != canvasWorkflowPurpose || approval.Workflow == nil || approval.Decision != "approve" || state.Decisions[index.ApprovalID] != "approve" || run.Status == "rejected" || run.Status == "cancelled" {
		return empty, flow.ErrApprovalRequired
	}
	if state.Request.PermissionMode != "auto" && state.Request.PermissionMode != "request_approval" {
		return empty, flow.ErrForbidden
	}
	b := approval.Workflow
	in := b.Admission
	args, e := decodeCloudAgentWorkflowCall(approval.Call.Function.Arguments)
	if e != nil || b.CallHash != cloudAgentApprovalCallHash(approval.Call) || b.OpsHash != capabilityPolicyHash(args.Ops) || index.RequestHash != b.RequestHash || b.RequestHash != capabilityPolicyHash(in) || b.RunID != access.RunID || b.RequestHash != access.RequestHash || in.ExpectedRevision == nil || *in.ExpectedRevision != access.CanvasRevision || in.UserID != access.UserID || in.TenantID != access.TenantID || in.WorkspaceID != access.WorkspaceID || in.CanvasID != access.CanvasID || in.BudgetMicro != access.BudgetMicro {
		return empty, flow.ErrForbidden
	}
	var canvas model.CanvasProject
	if err = db.First(&canvas, "id = ? AND user_id = ?", access.CanvasID, access.UserID).Error; err != nil {
		return empty, flow.ErrForbidden
	}
	// A managed write-back may advance the revision. Only its durable result can
	// explain that transition; arbitrary user edits never refresh an approval.
	if canvas.Revision != access.CanvasRevision {
		var execution model.CanvasWorkflowExecution
		if err = db.First(&execution, "id = ? AND user_id = ?", access.RunID, access.UserID).Error; err != nil {
			return empty, flow.ErrCanvasWorkflowConflict
		}
		cp, e := flow.DecodeCanvasWorkflowCheckpoint(execution)
		var result struct {
			Revision           int64  `json:"revision"`
			WrittenPayloadHash string `json:"written_payload_hash"`
		}
		var payload map[string]any
		if e != nil || json.Unmarshal(cp.Result, &result) != nil || json.Unmarshal([]byte(canvas.PayloadJSON), &payload) != nil || result.Revision != canvas.Revision || result.Revision != access.CanvasRevision+1 || result.WrittenPayloadHash == "" || capabilityPolicyHash(payload) != result.WrittenPayloadHash || stringValue(payload["workflowRunId"]) != access.RunID || (cp.Stage != "saving_result" && cp.Stage != "saving_report" && cp.Stage != "succeeded" && cp.Stage != "failed") {
			return empty, flow.ErrCanvasWorkflowConflict
		}
	}
	route, err := workflowGatewayRoute(a.s.flowModel)
	if err != nil || capabilityPolicyHash(route) != capabilityPolicyHash(b.Capability.ModelRoute) {
		return empty, flow.ErrForbidden
	}
	if b.PriceHash != capabilityPolicyHash(flow.CurrentPriceBook()) {
		return empty, flow.ErrForbidden
	}
	if access.Frozen != nil && (access.Frozen.ModelRouteHash != capabilityPolicyHash(b.Capability.ModelRoute) || access.Frozen.ToolSchemaHash != capabilityPolicyHash(b.Capability.Tools)) {
		return empty, flow.ErrForbidden
	}
	return b.Capability, nil
}

func (s *Service) prepareCloudAgentWorkflow(db *gorm.DB, run *model.CloudAgentExecution, state *cloudAgentRuntime, call cloudAgentCall) (*cloudAgentApproval, error) {
	if state.Request.PermissionMode != "auto" && state.Request.PermissionMode != "request_approval" {
		return nil, flow.ErrForbidden
	}
	args, err := decodeCloudAgentWorkflowCall(call.Function.Arguments)
	if err != nil {
		return nil, err
	}
	tenant, workspace := model.PersonalTenantID(run.UserID), model.PersonalWorkspaceID(run.UserID)
	if err = flow.AuthorizeGrant(db, "user:"+run.UserID, tenant, workspace, "intel.collect"); err != nil {
		return nil, err
	}
	canvas, err := repository.New(db).CanvasProjectForUser(run.UserID, state.Request.CanvasID)
	if err != nil {
		return nil, err
	}
	if canvas.Revision != *args.ExpectedRevision {
		return nil, flow.ErrCanvasWorkflowConflict
	}
	doc, err := creationDocument(canvas.PayloadJSON)
	if err != nil {
		return nil, err
	}
	beforeHash := cloudAgentCanvasHash(doc)
	items, err := applyCloudAgentCanvasPlan(doc, args.Ops)
	if err != nil {
		return nil, err
	}
	route, err := workflowGatewayRoute(s.flowModel)
	if err != nil {
		return nil, err
	}
	installed, err := flow.ListInstalledSkills(db, flow.Scope{TenantID: tenant, WorkspaceID: workspace})
	if err != nil {
		return nil, err
	}
	refs := []flow.SkillRef{}
	for _, skill := range installed {
		refs = append(refs, flow.SkillRef{ID: skill.SkillID, Version: skill.Version, Hash: skill.Hash})
	}
	capability := flow.CapabilityInput{TenantID: tenant, WorkspaceID: workspace, SubjectKind: flow.SubjectCanvas, Skills: refs, Tools: state.Canonical.Tools, ModelRoute: route, PolicyHash: capabilityPolicyHash(state.Policy)}
	approvalID := fmt.Sprintf("%s-%d-%d", run.ID, state.Step, state.CallIndex)
	binding := &cloudAgentWorkflowBinding{Admission: flow.CanvasWorkflowAdmission{UserID: run.UserID, TenantID: tenant, WorkspaceID: workspace, CanvasID: canvas.ID, ExecutionKey: "agent:" + approvalID, ExpectedRevision: args.ExpectedRevision, BudgetMicro: args.Run.BudgetMicro, RefreshBindings: args.Run.RefreshBindings, SaveReport: args.Run.SaveReport, ReportTitle: args.Run.ReportTitle}, Capability: capability, CanvasHash: beforeHash, OpsHash: capabilityPolicyHash(args.Ops), CallHash: cloudAgentApprovalCallHash(call), PriceHash: capabilityPolicyHash(flow.CurrentPriceBook())}
	items = append(items, cloudAgentApprovalPreviewItem{Operation: "run_workflow", Summary: fmt.Sprintf("运行 revision %d 上的具体计划，预算上限 %d micro；建图 %d 项；保存报告 %t", canvas.Revision, args.Run.BudgetMicro, len(args.Ops), args.Run.SaveReport)})
	return &cloudAgentApproval{ID: approvalID, Call: call, CallHash: binding.CallHash, Purpose: canvasWorkflowPurpose, Workflow: binding, Preview: cloudAgentApprovalPreview{Kind: "canvas_workflow", Title: "批准画布构建与运行", Description: "本次批准仅覆盖所列操作、执行参数、冻结能力与预算；改图后必须重新审批。", Items: items}}, nil
}

func (s *Service) advanceCloudAgentWorkflow(run *model.CloudAgentExecution, state *cloudAgentRuntime, call cloudAgentCall, allowed bool) error {
	if !allowed || (state.Request.PermissionMode != "auto" && state.Request.PermissionMode != "request_approval") {
		return s.repo.MutateCloudAgent(run.UserID, run.ID, run.Revision, func(current *model.CloudAgentExecution, _ *repository.Repository) error {
			cloudAgentRecordToolResult(current, state, call, nil, flow.ErrForbidden)
			return cloudAgentSave(current, state)
		})
	}
	if state.Approval == nil {
		return s.repo.MutateCloudAgent(run.UserID, run.ID, run.Revision, func(current *model.CloudAgentExecution, repo *repository.Repository) error {
			approval, err := s.prepareCloudAgentWorkflow(repo.DB(), run, state, call)
			if err != nil {
				cloudAgentRecordToolResult(current, state, call, nil, err)
				return cloudAgentSave(current, state)
			}
			state.Approval = approval
			current.Status = "waiting_approval"
			state.event(run.ID, "approval_requested", map[string]any{"approvalId": approval.ID, "toolName": call.Function.Name, "arguments": json.RawMessage(call.Function.Arguments), "preview": approval.Preview, "scope": approval.Workflow.Admission, "text": approval.Preview.Description})
			return cloudAgentSave(current, state)
		})
	}
	if state.Approval.Decision != "approve" {
		return flow.ErrApprovalRequired
	}
	record, err := s.consumeCloudAgentWorkflow(context.Background(), run.UserID, run.ID, state.Approval.ID)
	if err != nil {
		return err
	}
	state.WorkflowWaitID = record.ID
	return s.wakeCanvasWorkflow(record)
}

func (s *Service) consumeCloudAgentWorkflow(ctx context.Context, user, agentID, approvalID string) (out model.CanvasWorkflowExecution, err error) {
	if s.canvasWorkflowRunner == nil {
		return out, fmt.Errorf("%w: managed canvas runner unavailable", flow.ErrForbidden)
	}
	root, err := s.repo.CloudAgent(user, agentID)
	if err != nil {
		return out, err
	}
	state, err := cloudAgentDecode(root)
	if err != nil {
		return out, err
	}
	if old, ok := state.WorkflowApprovals[approvalID]; ok && old.Workflow != nil && old.Workflow.RunID != "" {
		return s.CanvasWorkflowExecutions().Get(ctx, user, old.Workflow.RunID)
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return out, err
	}
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	err = s.repo.MutateCloudAgent(user, agentID, root.Revision, func(current *model.CloudAgentExecution, repo *repository.Repository) error {
		state, e := cloudAgentDecode(current)
		if e != nil {
			return e
		}
		ap := state.Approval
		if ap == nil || ap.ID != approvalID || ap.Purpose != canvasWorkflowPurpose || ap.Decision != "approve" || state.Decisions[approvalID] != "approve" || ap.Workflow == nil {
			return flow.ErrApprovalRequired
		}
		if state.Request.PermissionMode != "auto" && state.Request.PermissionMode != "request_approval" {
			return flow.ErrForbidden
		}
		args, e := decodeCloudAgentWorkflowCall(ap.Call.Function.Arguments)
		if e != nil {
			return e
		}
		b := ap.Workflow
		if b.CallHash != cloudAgentApprovalCallHash(ap.Call) || b.OpsHash != capabilityPolicyHash(args.Ops) {
			return flow.ErrForbidden
		}
		canvas, e := repo.CanvasProjectForUser(user, b.Admission.CanvasID)
		if e != nil {
			return e
		}
		doc, e := creationDocument(canvas.PayloadJSON)
		if e != nil {
			return e
		}
		if canvas.Revision != *args.ExpectedRevision || cloudAgentCanvasHash(doc) != b.CanvasHash {
			return flow.ErrCanvasWorkflowConflict
		}
		if len(args.Ops) > 0 {
			raw, _ := json.Marshal(agentCanvasArgs{SnapshotHash: b.CanvasHash, Ops: args.Ops})
			opCall := ap.Call
			opCall.Function.Name = "canvas_apply_ops"
			opCall.Function.Arguments = string(raw)
			if _, e = applyCloudAgentCanvas(repo, user, canvas.ID, opCall, policy, cloudAgentCanvasEventRecorder(agentID, &state)); e != nil {
				return e
			}
			canvas, e = repo.CanvasProjectForUser(user, canvas.ID)
			if e != nil {
				return e
			}
		}
		revision := canvas.Revision
		b.Admission.ExpectedRevision = &revision
		b.RequestHash = capabilityPolicyHash(b.Admission)
		b.RunID = "cw_" + capabilityPolicyHash([]string{b.Admission.TenantID, b.Admission.WorkspaceID, b.Admission.CanvasID, b.Admission.ExecutionKey})[:48]
		if state.WorkflowApprovals == nil {
			state.WorkflowApprovals = map[string]cloudAgentApproval{}
		}
		state.WorkflowApprovals[approvalID] = *ap
		// Persist the authoritative decision inside this same transaction before Admit
		// calls back into Authority. A later error rolls back both ops and admission.
		if e = cloudAgentSave(current, &state); e != nil {
			return e
		}
		if e = repo.DB().Model(current).Update("state_json", current.StateJSON).Error; e != nil {
			return e
		}
		index := model.FlowAgentToolCall{ID: "cwa_" + capabilityPolicyHash(b.RunID)[:32], TenantID: b.Admission.TenantID, ConversationID: agentID, CallID: b.RunID, ToolName: "canvas_run_workflow", Class: "write", RequestHash: b.RequestHash, ApprovalID: approvalID, Arguments: ap.Call.Function.Arguments, Result: agentID, Status: "approved", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if e = repo.DB().Create(&index).Error; e != nil {
			return e
		}
		executions := s.CanvasWorkflowExecutions()
		executions.DB = repo.DB()
		out, e = executions.Admit(ctx, b.Admission)
		if e != nil {
			return e
		}
		cp, _ := json.Marshal(state.Request)
		if _, e = (flow.Waits{DB: repo.DB()}).Park(model.FlowExternalWait{TenantID: out.TenantID, WorkspaceID: out.WorkspaceID, UserID: user, RunID: agentID, ToolCallID: ap.Call.ID, OperationID: out.ID, Reservation: out.ReservationID, Checkpoint: string(cp)}); e != nil {
			return e
		}
		state.WorkflowWaitID = out.ID
		cloudAgentRecordToolResult(current, &state, ap.Call, map[string]any{"run_id": out.ID, "status": out.Status, "revision": out.CanvasRevision, "operation_id": out.ID}, nil)
		current.Status = "waiting_external"
		cloudAgentInvalidateReadCache(&state)
		return cloudAgentSave(current, &state)
	})
	return out, err
}

func (s *Service) wakeCanvasWorkflow(record model.CanvasWorkflowExecution) error {
	if s.canvasWorkflowRunner == nil {
		return flow.ErrForbidden
	}
	// The durable admitted row is the recovery queue; managed execution owns leases
	// and checkpoints. No model/engine calls happen under the approval transaction.
	go func() { _ = s.canvasWorkflowRunner(context.Background(), s.CanvasWorkflowExecutions(), record) }()
	return nil
}

// VerifyCloudAgentPlanApproval is an in-process server verifier, never a client
// supplied verified flag. Recurring plans are a different approval purpose.
func (s *Service) VerifyCloudAgentPlanApproval(ctx context.Context, db *gorm.DB, user, agentID, approvalID, purpose string) (json.RawMessage, error) {
	if purpose != "monitor_canvas_recurring" {
		return nil, flow.ErrForbidden
	}
	root, err := repository.New(db.WithContext(ctx)).CloudAgent(user, agentID)
	if err != nil {
		return nil, flow.ErrForbidden
	}
	state, err := cloudAgentDecode(root)
	if err != nil {
		return nil, err
	}
	ap, ok := state.WorkflowApprovals[approvalID]
	if !ok || ap.Purpose != purpose || ap.Call.Function.Name != "intel_apply_monitor" || ap.Decision != "approve" || state.Decisions[approvalID] != "approve" || len(ap.ServerPlan) == 0 || ap.CallHash != cloudAgentApprovalCallHash(ap.Call) || root.Status == "cancelled" || root.Status == "rejected" {
		return nil, flow.ErrApprovalRequired
	}
	return append(json.RawMessage(nil), ap.ServerPlan...), nil
}

// MonitorCanvasRecurringTool reports the real canvas workflow tool a recurring
// plan dispatches through, after checking it is actually compiled for an
// approval-gated, canvas-scoped request. flow must not freeze a tool name that
// no runtime would execute.
func (s *Service) MonitorCanvasRecurringTool() (string, error) {
	req := CloudAgentRequest{PermissionMode: "auto", ContextScope: []string{"canvas"}}
	for _, t := range compileCanvasAgentTools(req, false) {
		if fn, ok := t["function"].(map[string]any); ok && fn["name"] == "canvas_run_workflow" {
			return "canvas_run_workflow", nil
		}
	}
	return "", flow.ErrForbidden
}

var _ flow.CanvasWorkflowAuthority = cloudAgentWorkflowAuthority{}
var _ = errors.Is

// authorizeRecurringCanvas is the T21/T22 authority branch for admissions that
// carry a monitor_canvas_recurring AuthorityBinding: the persisted approval
// chain must still approve this exact plan, and the live canvas must still
// hash to the approved configuration. Budget/run caps are consumed atomically
// by the executor before admission, never re-derived here.
func (a cloudAgentWorkflowAuthority) authorizeRecurringCanvas(ctx context.Context, db *gorm.DB, access flow.CanvasWorkflowAccess) (flow.CapabilityInput, error) {
	var empty flow.CapabilityInput
	var plan flow.MonitorCanvasApprovedPlan
	if json.Unmarshal(access.AuthorityBinding, &plan) != nil || plan.Purpose != flow.MonitorCanvasRecurringPurpose || plan.Actor == "" {
		return empty, flow.ErrForbidden
	}
	userID := strings.TrimPrefix(plan.Actor, "user:")
	if userID == "" || access.UserID != userID || access.TenantID != plan.TenantID || access.WorkspaceID != plan.WorkspaceID || access.CanvasID != plan.CanvasID {
		return empty, flow.ErrForbidden
	}
	if err := flow.AuthorizeGrant(db, plan.Actor, plan.TenantID, plan.WorkspaceID, "intel.collect"); err != nil {
		return empty, err
	}
	raw, err := a.s.VerifyCloudAgentPlanApproval(ctx, db, userID, plan.ExecutionID, plan.ApprovalID, flow.MonitorCanvasRecurringPurpose)
	if err != nil {
		return empty, err
	}
	var frozen flow.MonitorCanvasApprovedPlan
	if json.Unmarshal(raw, &frozen) != nil || frozen.RevokedAt != "" || plan.RevokedAt != "" {
		return empty, flow.ErrForbidden
	}
	frozen.ExecutionID, frozen.ApprovalID = plan.ExecutionID, plan.ApprovalID
	planCopy := plan
	planCopy.ExecutionID, planCopy.ApprovalID = "", ""
	frozenCopy := frozen
	frozenCopy.ExecutionID, frozenCopy.ApprovalID = "", ""
	want, _ := json.Marshal(planCopy)
	got, _ := json.Marshal(frozenCopy)
	if !bytes.Equal(want, got) {
		return empty, flow.ErrForbidden
	}
	var canvas model.CanvasProject
	if err = db.First(&canvas, "id = ? AND user_id = ?", plan.CanvasID, userID).Error; err != nil {
		return empty, flow.ErrForbidden
	}
	hash, err := flow.CanvasMonitorConfigHash(canvas.PayloadJSON)
	if err != nil || hash != plan.ConfigHash {
		return empty, flow.ErrCanvasWorkflowConflict
	}
	if access.BudgetMicro != plan.BudgetMicroPerWindow {
		return empty, flow.ErrForbidden
	}
	// The admission must be the run the executor just paid for: its execution key
	// names the consumed cycle, so a direct/forged Admit cannot ride the plan.
	var admission struct {
		ExecutionKey string `json:"ExecutionKey"`
	}
	if json.Unmarshal([]byte(access.RequestJSON), &admission) != nil || !strings.HasPrefix(admission.ExecutionKey, "monitor:"+plan.MonitorID+":") {
		return empty, flow.ErrForbidden
	}
	var usage model.CanvasMonitorRunUsage
	if err := db.Where("tenant_id = ? AND monitor_id = ?", plan.TenantID, plan.MonitorID).First(&usage).Error; err != nil || usage.LastCycleKey != admission.ExecutionKey || usage.RunsUsed > plan.MaxRuns || usage.MicroUsed > plan.TotalBudgetMicro {
		return empty, flow.ErrForbidden
	}
	route, err := workflowGatewayRoute(a.s.flowModel)
	if err != nil {
		return empty, err
	}
	installed, err := flow.ListInstalledSkills(db, flow.Scope{TenantID: plan.TenantID, WorkspaceID: plan.WorkspaceID})
	if err != nil {
		return empty, err
	}
	refs := []flow.SkillRef{}
	for _, skill := range installed {
		refs = append(refs, flow.SkillRef{ID: skill.SkillID, Version: skill.Version, Hash: skill.Hash})
	}
	policy, err := a.s.RuntimePolicy()
	if err != nil {
		return empty, err
	}
	return flow.CapabilityInput{TenantID: plan.TenantID, WorkspaceID: plan.WorkspaceID, SubjectKind: flow.SubjectCanvas, Skills: refs, Tools: []map[string]any{}, ModelRoute: route, PolicyHash: capabilityPolicyHash(policy)}, nil
}

// ExecuteMonitorCanvasRun is the local canvas.run executor: full current grant
// verification (persisted approval + engine re-prepare) outside any
// transaction, then atomic budget/run consumption and admission in one
// transaction. Replayed cycles return the existing run without re-consuming.
func (s *Service) ExecuteMonitorCanvasRun(ctx context.Context, row model.FlowCommandOutbox) (int, []byte, error) {
	if s.canvasWorkflowRunner == nil {
		return 500, nil, fmt.Errorf("managed canvas runner unavailable")
	}
	var payload struct {
		CanvasID    string          `json:"canvas_id"`
		MonitorID   string          `json:"monitor_id"`
		Grant       json.RawMessage `json:"grant"`
		ScheduledAt string          `json:"scheduled_at"`
	}
	if json.Unmarshal([]byte(row.Payload), &payload) != nil || payload.CanvasID == "" || payload.MonitorID == "" || len(payload.Grant) == 0 || payload.ScheduledAt == "" {
		return 400, nil, fmt.Errorf("invalid canvas.run payload")
	}
	var plan flow.MonitorCanvasApprovedPlan
	if json.Unmarshal(payload.Grant, &plan) != nil || plan.Purpose != flow.MonitorCanvasRecurringPurpose || plan.Actor == "" {
		return 400, nil, fmt.Errorf("invalid canvas grant")
	}
	userID := strings.TrimPrefix(plan.Actor, "user:")
	if userID == "" || row.TenantID != plan.TenantID || row.WorkspaceID != plan.WorkspaceID {
		return 403, nil, flow.ErrForbidden
	}
	if s.flowEngine == nil {
		return 500, nil, fmt.Errorf("flow engine not configured")
	}
	if _, err := flow.VerifyCanvasMonitorGrant(ctx, s.repo.DB(), s.flowEngine, flow.Scope{TenantID: plan.TenantID, WorkspaceID: plan.WorkspaceID}, userID, plan.MonitorID, plan.ExecutionID, plan.ApprovalID); err != nil {
		if errors.Is(err, flow.ErrApprovalRequired) {
			return 409, nil, err
		}
		return 500, nil, err
	}
	var canvas model.CanvasProject
	if err := s.repo.DB().First(&canvas, "id = ? AND user_id = ?", payload.CanvasID, userID).Error; err != nil {
		return 404, nil, err
	}
	rev := canvas.Revision
	executionKey := "monitor:" + plan.MonitorID + ":" + payload.ScheduledAt
	runID := flow.CanvasWorkflowRunID(plan.TenantID, plan.WorkspaceID, payload.CanvasID, executionKey)
	exec := s.CanvasWorkflowExecutions()
	var record model.CanvasWorkflowExecution
	err := s.repo.DB().Transaction(func(tx *gorm.DB) error {
		if e := tx.First(&record, "id = ?", runID).Error; e == nil {
			return nil
		}
		if e := tx.AutoMigrate(&model.CanvasMonitorRunUsage{}); e != nil {
			return e
		}
		if e := flow.ConsumeMonitorCanvasBudget(tx, plan.TenantID, plan.WorkspaceID, plan.MonitorID, plan.CanvasID, executionKey, plan.BudgetMicroPerWindow, plan.MaxRuns, plan.TotalBudgetMicro, time.Now().UTC()); e != nil {
			return e
		}
		local := exec
		local.DB = tx
		var e2 error
		record, e2 = local.Admit(ctx, flow.CanvasWorkflowAdmission{UserID: userID, TenantID: plan.TenantID, WorkspaceID: plan.WorkspaceID, CanvasID: payload.CanvasID, ExecutionKey: executionKey, ExpectedRevision: &rev, BudgetMicro: plan.BudgetMicroPerWindow, AuthorityBinding: payload.Grant})
		return e2
	})
	if err != nil {
		if errors.Is(err, flow.ErrBudgetInsufficient) {
			body, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"skipped": "monitor_plan_exhausted"}})
			return 200, body, nil
		}
		if errors.Is(err, flow.ErrCanvasWorkflowConflict) {
			return 409, nil, err
		}
		return 500, nil, err
	}
	if err := s.wakeCanvasWorkflow(record); err != nil {
		return 500, nil, err
	}
	body, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"operation_id": record.ID, "run_id": record.ID, "status": record.Status}})
	return 200, body, nil
}
