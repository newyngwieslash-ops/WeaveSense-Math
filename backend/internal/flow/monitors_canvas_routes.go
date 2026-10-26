package flow

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

const MonitorCanvasRecurringPurpose = "monitor_canvas_recurring"

type MonitorCanvasApprovedPlan struct {
	Purpose              string   `json:"purpose"`
	TenantID             string   `json:"tenant_id"`
	WorkspaceID          string   `json:"workspace_id"`
	MonitorID            string   `json:"monitor_id"`
	CanvasID             string   `json:"canvas_id"`
	Actor                string   `json:"actor"`
	ConfigHash           string   `json:"config_hash"`
	ConfigVersion        int64    `json:"config_version"`
	PolicyVersion        string   `json:"policy_version"`
	SourceIDs            []string `json:"source_ids"`
	ToolIDs              []string `json:"tool_ids"`
	IntervalSeconds      int64    `json:"interval_seconds"`
	Timezone             string   `json:"timezone"`
	BudgetMicroPerWindow int64    `json:"budget_micro_per_window"`
	TotalBudgetMicro     int64    `json:"total_budget_micro"`
	MaxRuns              int64    `json:"max_runs"`
	ExpiresAt            string   `json:"expires_at"`
	RevokedAt            string   `json:"revoked_at,omitempty"`
	ExecutionID          string   `json:"execution_id,omitempty"`
	ApprovalID           string   `json:"approval_id,omitempty"`
}

type MonitorCanvasApprovalVerifier func(context.Context, *gorm.DB, string, string, string, string) (json.RawMessage, error)

var monitorCanvasVerifier struct {
	sync.RWMutex
	verify MonitorCanvasApprovalVerifier
}

// Only the app composition root registers the verifier of persisted CloudAgent decisions.
func RegisterMonitorCanvasApprovalVerifier(fn MonitorCanvasApprovalVerifier) {
	monitorCanvasVerifier.Lock()
	defer monitorCanvasVerifier.Unlock()
	monitorCanvasVerifier.verify = fn
}

// MonitorCanvasRecurringTool reports the single real agent tool a recurring
// canvas plan dispatches through. Only the app composition root registers it:
// flow does not own the agent tool registry, and a plan must never freeze a
// tool name that no runtime would actually execute.
type MonitorCanvasRecurringTool func() (string, error)

var monitorCanvasRecurringTool struct {
	sync.RWMutex
	provider MonitorCanvasRecurringTool
}

func RegisterMonitorCanvasRecurringTool(fn MonitorCanvasRecurringTool) {
	monitorCanvasRecurringTool.Lock()
	defer monitorCanvasRecurringTool.Unlock()
	monitorCanvasRecurringTool.provider = fn
}

func monitorPlanError(reason string) error { return fmt.Errorf("%w: %s", ErrForbidden, reason) }

func monitorEngineData(ctx context.Context, engine EngineCaller, scope Scope, user, path string, out any) error {
	if engine == nil {
		return monitorPlanError("canvas_engine_unavailable")
	}
	status, raw, err := engine.Do(ctx, scope, Actor{Type: "user", ID: user}, http.MethodGet, path, "", nil)
	if err != nil {
		return err
	}
	var env struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if status != 200 || json.Unmarshal(raw, &env) != nil || env.Code != 0 {
		return monitorPlanError("canvas_monitor_unavailable")
	}
	return json.Unmarshal(env.Data, out)
}

type canvasMonitorRecord struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	WorkspaceID     string          `json:"workspace_id"`
	SourceIDs       []string        `json:"source_ids"`
	IntervalSeconds int64           `json:"interval_seconds"`
	Timezone        string          `json:"timezone"`
	Budget          int64           `json:"budget_micro_per_window"`
	Version         int64           `json:"version"`
	Grant           json.RawMessage `json:"canvas_auto_run_grant"`
}

func loadCanvasMonitor(ctx context.Context, engine EngineCaller, scope Scope, user, id string) (canvasMonitorRecord, error) {
	var rows []canvasMonitorRecord
	if err := monitorEngineData(ctx, engine, scope, user, "/v1/monitors", &rows); err != nil {
		return canvasMonitorRecord{}, err
	}
	for _, row := range rows {
		if row.ID == id && row.TenantID == scope.TenantID && row.WorkspaceID == scope.WorkspaceID {
			return row, nil
		}
	}
	return canvasMonitorRecord{}, monitorPlanError("canvas_monitor_not_found")
}

// Matches Loom's canonical configuration hash; output-only fields are removed by node role.
func monitorCanvasConfig(raw string) (map[string]any, string, error) {
	var doc map[string]any
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return nil, "", ErrInvalid
	}
	nodes, ok := doc["nodes"].([]any)
	if !ok {
		return nil, "", ErrInvalid
	}
	conns, ok := doc["connections"].([]any)
	if !ok {
		return nil, "", ErrInvalid
	}
	incoming := map[string]bool{}
	for _, v := range conns {
		if c, ok := v.(map[string]any); ok && c["relation"] != "batch-output" && c["relation"] != "storyboard-output" {
			if id, ok := c["toNodeId"].(string); ok {
				incoming[id] = true
			}
		}
	}
	for _, key := range []string{"updatedAt", "createdAt", "revision", "viewport", "workflowRunId"} {
		delete(doc, key)
	}
	for _, v := range nodes {
		n, ok := v.(map[string]any)
		if !ok {
			return nil, "", ErrInvalid
		}
		m, _ := n["metadata"].(map[string]any)
		if m == nil {
			continue
		}
		// Run outputs (managed write-back) are snapshots, not configuration: a
		// recurring plan must survive its own runs without forced re-approval.
		for _, key := range []string{"lastRunSnapshot", "computed", "computedBranches", "skillOutput", "derived"} {
			delete(m, key)
		}
		ds, _ := m["dataSource"].(map[string]any)
		sb, _ := ds["sourceBinding"].(map[string]any)
		// Sync timestamps are run artifacts, not configuration, bound or not.
		delete(ds, "lastSyncedAt")
		bound := asString(sb["kind"]) != "" && asString(sb["refId"]) != ""
		if bound {
			delete(ds, "rawText")
			delete(sb, "lastSyncedAt")
		}
		typ := asString(n["type"])
		if typ == "logic" || typ == "skill" || incoming[asString(n["id"])] || (typ == "data-source" && (bound || strings.TrimSpace(asString(ds["rawText"])) != "")) {
			delete(m, "content")
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
	return doc, hex.EncodeToString(digest[:]), nil
}

func monitorInteger(v any) (int64, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return 0, ErrInvalid
	}
	var n int64
	if json.Unmarshal(raw, &n) != nil || n < 0 || n > 9007199254740991 {
		return 0, ErrInvalid
	}
	return n, nil
}

// Prepare is called before displaying the real CloudAgent approval. Client limits
// are requests only; identity, sources, frequency, configuration and tools are server-derived.
func PrepareCanvasMonitorApproval(ctx context.Context, db *gorm.DB, engine EngineCaller, scope Scope, userID string, args map[string]any) (json.RawMessage, error) {
	if args["purpose"] != MonitorCanvasRecurringPurpose {
		return nil, ErrInvalid
	}
	for _, key := range []string{"config_version", "config_hash", "actor", "grant_actor", "canvas_auto_run_grant", "canvas_config"} {
		if _, ok := args[key]; ok {
			return nil, ErrInvalid
		}
	}
	canvasID, monitorID := asString(args["canvas_id"]), asString(args["monitor_id"])
	if canvasID == "" || monitorID == "" || scope.TenantID != model.PersonalTenantID(userID) || scope.WorkspaceID != model.PersonalWorkspaceID(userID) {
		return nil, ErrForbidden
	}
	if err := AuthorizeGrant(db, "user:"+userID, scope.TenantID, scope.WorkspaceID, "intel.monitors.write"); err != nil {
		return nil, err
	}
	var canvas model.CanvasProject
	if err := db.WithContext(ctx).First(&canvas, "id = ? AND user_id = ?", canvasID, userID).Error; err != nil {
		return nil, ErrForbidden
	}
	_, hash, err := monitorCanvasConfig(canvas.PayloadJSON)
	if err != nil {
		return nil, err
	}
	monitor, err := loadCanvasMonitor(ctx, engine, scope, userID, monitorID)
	if err != nil {
		return nil, err
	}
	monitorCanvasRecurringTool.RLock()
	toolProvider := monitorCanvasRecurringTool.provider
	monitorCanvasRecurringTool.RUnlock()
	if toolProvider == nil {
		return nil, monitorPlanError("canvas_tool_unavailable")
	}
	toolName, err := toolProvider()
	if err != nil || toolName == "" {
		return nil, monitorPlanError("canvas_tool_unavailable")
	}
	tools := []string{toolName}
	if requested, ok := args["tool_ids"]; ok {
		raw, _ := json.Marshal(requested)
		var names []string
		if json.Unmarshal(raw, &names) != nil || len(names) != 1 || names[0] != tools[0] {
			return nil, ErrInvalid
		}
	}
	total, err := monitorInteger(args["total_budget_micro"])
	if err != nil {
		return nil, err
	}
	runs, err := monitorInteger(args["max_runs"])
	if err != nil {
		return nil, err
	}
	expires, err := time.Parse(time.RFC3339, asString(args["expires_at"]))
	if err != nil || !expires.After(time.Now()) || runs < 1 || total < monitor.Budget || monitor.IntervalSeconds < 300 {
		return nil, ErrInvalid
	}
	var revisions []struct {
		Spec struct {
			Grant json.RawMessage `json:"canvas_auto_run_grant"`
		} `json:"spec"`
	}
	if err = monitorEngineData(ctx, engine, scope, userID, "/v1/monitors/"+monitorID+"/revisions", &revisions); err != nil {
		return nil, err
	}
	var previous struct {
		Hash    string `json:"config_hash"`
		Version int64  `json:"config_version"`
	}
	for _, raw := range append([]json.RawMessage{monitor.Grant}, func() []json.RawMessage {
		out := []json.RawMessage{}
		for _, r := range revisions {
			out = append(out, r.Spec.Grant)
		}
		return out
	}()...) {
		var p struct {
			Hash    string `json:"config_hash"`
			Version int64  `json:"config_version"`
		}
		if json.Unmarshal(raw, &p) == nil && p.Version >= previous.Version {
			previous = p
		}
	}
	version := previous.Version
	if version < 1 {
		version = 1
	} else if previous.Hash != hash {
		version++
	}
	sources := append([]string(nil), monitor.SourceIDs...)
	sort.Strings(sources)
	if len(sources) == 0 {
		return nil, ErrInvalid
	}
	return json.Marshal(MonitorCanvasApprovedPlan{Purpose: MonitorCanvasRecurringPurpose, TenantID: scope.TenantID, WorkspaceID: scope.WorkspaceID, MonitorID: monitorID, CanvasID: canvasID, Actor: "user:" + userID, ConfigHash: hash, ConfigVersion: version, PolicyVersion: "policy_v1", SourceIDs: sources, ToolIDs: tools, IntervalSeconds: monitor.IntervalSeconds, Timezone: monitor.Timezone, BudgetMicroPerWindow: monitor.Budget, TotalBudgetMicro: total, MaxRuns: runs, ExpiresAt: expires.UTC().Format(time.RFC3339)})
}

// Verify re-reads real persisted approval and current scope/configuration. T22 owns
// atomic budget/run consumption; a signed transport proof is never a substitute.
func VerifyCanvasMonitorGrant(ctx context.Context, db *gorm.DB, engine EngineCaller, scope Scope, userID, monitorID, executionID, approvalID string) (MonitorCanvasApprovedPlan, error) {
	var plan MonitorCanvasApprovedPlan
	monitorCanvasVerifier.RLock()
	verify := monitorCanvasVerifier.verify
	monitorCanvasVerifier.RUnlock()
	if verify == nil {
		return plan, monitorPlanError("canvas_approval_unavailable")
	}
	raw, err := verify(ctx, db, userID, executionID, approvalID, MonitorCanvasRecurringPurpose)
	if err != nil {
		return plan, err
	}
	if json.Unmarshal(raw, &plan) != nil || plan.Purpose != MonitorCanvasRecurringPurpose || plan.MonitorID != monitorID || plan.Actor != "user:"+userID || plan.RevokedAt != "" || (plan.ExecutionID != "" && plan.ExecutionID != executionID) || (plan.ApprovalID != "" && plan.ApprovalID != approvalID) {
		return plan, ErrForbidden
	}
	currentRaw, err := PrepareCanvasMonitorApproval(ctx, db, engine, scope, userID, map[string]any{"purpose": MonitorCanvasRecurringPurpose, "monitor_id": monitorID, "canvas_id": plan.CanvasID, "total_budget_micro": plan.TotalBudgetMicro, "max_runs": plan.MaxRuns, "expires_at": plan.ExpiresAt, "tool_ids": plan.ToolIDs})
	if err != nil {
		return plan, err
	}
	var current MonitorCanvasApprovedPlan
	_ = json.Unmarshal(currentRaw, &current)
	frozen := plan
	frozen.ExecutionID = ""
	frozen.ApprovalID = ""
	a, _ := json.Marshal(frozen)
	b, _ := json.Marshal(current)
	if !bytes.Equal(a, b) {
		return plan, monitorPlanError("canvas_reapproval_required")
	}
	plan.ExecutionID = executionID
	plan.ApprovalID = approvalID
	return plan, nil
}

func applyMonitorCanvasApproval(c *gin.Context, db *gorm.DB, client EngineClient, scope Scope, user string, body map[string]any) bool {
	raw, exists := body["canvas_approval_ref"]
	if !exists {
		return false
	}
	if len(body) != 1 {
		monitorReason(c, 400, "canvas_approval_mutation_forbidden")
		return true
	}
	ref, ok := raw.(map[string]any)
	if !ok {
		monitorReason(c, 400, "canvas_approval_invalid")
		return true
	}
	executionID, approvalID := asString(ref["execution_id"]), asString(ref["approval_id"])
	if executionID == "" || approvalID == "" {
		monitorReason(c, 400, "canvas_approval_invalid")
		return true
	}
	plan, err := VerifyCanvasMonitorGrant(c.Request.Context(), db, client, scope, user, c.Param("id"), executionID, approvalID)
	if err != nil {
		status := http.StatusForbidden
		if errors.Is(err, ErrApprovalRequired) {
			status = http.StatusConflict
		}
		monitorReason(c, status, "canvas_approval_denied")
		return true
	}
	monitor, err := loadCanvasMonitor(c.Request.Context(), client, scope, user, plan.MonitorID)
	if err != nil {
		monitorReason(c, 409, "canvas_monitor_unavailable")
		return true
	}
	var canvas model.CanvasProject
	if db.First(&canvas, "id = ? AND user_id = ?", plan.CanvasID, user).Error != nil {
		monitorReason(c, 404, "canvas_not_found")
		return true
	}
	config, hash, err := monitorCanvasConfig(canvas.PayloadJSON)
	if err != nil || hash != plan.ConfigHash {
		monitorReason(c, 409, "canvas_reapproval_required")
		return true
	}
	grantRaw, _ := json.Marshal(plan)
	var grant map[string]any
	_ = json.Unmarshal(grantRaw, &grant)
	grant["scope"] = map[string]any{"tenant_id": plan.TenantID, "workspace_id": plan.WorkspaceID, "canvas_id": plan.CanvasID, "source_ids": plan.SourceIDs, "tool_ids": plan.ToolIDs}
	mutation := map[string]any{"canvas_id": plan.CanvasID, "canvas_config": config, "canvas_auto_run_grant": grant, "expected_version": monitor.Version}
	payloadRaw, _ := json.Marshal(map[string]any{"tenant_id": scope.TenantID, "workspace_id": scope.WorkspaceID, "actor": "user:" + user, "monitor_id": plan.MonitorID, "expires_at": time.Now().Unix() + 60, "mutation": mutation})
	payload := base64.RawURLEncoding.EncodeToString(payloadRaw)
	mac := hmac.New(sha256.New, client.Cfg.SigningKey)
	_, _ = mac.Write([]byte("monitor-canvas-v1\n" + payload))
	sealed, _ := json.Marshal(map[string]any{"canvas_authorization": map[string]any{"payload": payload, "signature": hex.EncodeToString(mac.Sum(nil))}})
	status, out, err := client.Do(c.Request.Context(), scope, Actor{Type: "user", ID: user}, http.MethodPatch, "/v1/monitors/"+plan.MonitorID, "", sealed)
	if err != nil {
		monitorReason(c, 502, "canvas_engine_unavailable")
		return true
	}
	c.Data(status, "application/json", out)
	return true
}

func monitorReason(c *gin.Context, status int, reason string) {
	c.JSON(status, gin.H{"code": status, "data": nil, "msg": reason, "reason": reason})
}

// This route boundary cannot turn an arbitrary approval ID into a durable grant.
// Pi's existing tool approvals do not attest a specific recurring canvas plan.
func guardMonitorCanvas(c *gin.Context, body map[string]any) bool {
	if body == nil {
		monitorReason(c, http.StatusBadRequest, "monitor_body_invalid")
		return false
	}
	if body["canvas_auto_run_grant"] != nil {
		monitorReason(c, http.StatusConflict, "canvas_approval_unavailable")
		return false
	}
	for _, key := range []string{"canvas_config", "canvas_snapshot", "grant_actor", "capability_snapshot_id", "canvas_authorization"} {
		if _, ok := body[key]; ok {
			monitorReason(c, http.StatusBadRequest, "canvas_client_snapshot_forbidden")
			return false
		}
	}
	return true
}

func validateMonitorCanvas(c *gin.Context, db *gorm.DB, scope Scope, userID string, body map[string]any) bool {
	if !guardMonitorCanvas(c, body) {
		return false
	}
	raw, exists := body["canvas_id"]
	if !exists || raw == nil {
		return true
	}
	id, ok := raw.(string)
	if !ok || strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
		monitorReason(c, http.StatusBadRequest, "canvas_id_invalid")
		return false
	}
	if err := AuthorizeGrant(db, "user:"+userID, scope.TenantID, scope.WorkspaceID, "intel.monitors.write"); err != nil {
		monitorReason(c, http.StatusForbidden, "canvas_permission_denied")
		return false
	}
	// Require the actual persisted canvas, not a sidebar conversation bearing its ID.
	var canvas model.CanvasProject
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&canvas).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			monitorReason(c, http.StatusNotFound, "canvas_not_found")
		} else {
			monitorReason(c, http.StatusInternalServerError, "canvas_lookup_failed")
		}
		return false
	}
	// Current Canvas ownership maps to the personal workspace (ResolveSubject contract).
	personal, err := ResolveScope(db, userID, "")
	if err != nil || personal.TenantID != scope.TenantID || personal.WorkspaceID != scope.WorkspaceID {
		monitorReason(c, http.StatusForbidden, "canvas_workspace_mismatch")
		return false
	}
	return true
}

func monitorCreateBoundary(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		var body map[string]any
		if err := c.ShouldBindBodyWithJSON(&body); err != nil || body == nil {
			monitorReason(c, http.StatusBadRequest, "monitor_body_invalid")
			return
		}
		if !guardMonitorCanvas(c, body) {
			return
		}
		// Binding uses PATCH on an existing monitor, after ownership checks, not an opaque preview.
		if _, ok := body["canvas_id"]; ok {
			monitorReason(c, http.StatusBadRequest, "canvas_binding_requires_monitor")
			return
		}
		if cached, ok := c.Get(gin.BodyBytesKey); ok {
			c.Request.Body = io.NopCloser(bytes.NewReader(cached.([]byte)))
		}
		next(c)
	}
}
