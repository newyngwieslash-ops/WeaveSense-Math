package flow

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"yingce/backend/internal/model"
)

func TestT21PrepareRealCanvasPlan(t *testing.T) {
	db := testDB(t)
	if err := db.AutoMigrate(&model.CanvasProject{}); err != nil {
		t.Fatal(err)
	}
	canvas := model.CanvasProject{ID: "canvas_plan", UserID: "u_alice", PayloadJSON: `{"nodes":[{"id":"logic","type":"logic","metadata":{"logic":{"operator":"compute","expression":"n*2"}}}],"connections":[]}`}
	if err := db.Create(&canvas).Error; err != nil {
		t.Fatal(err)
	}
	scope, err := ResolveScope(db, "u_alice", "")
	if err != nil {
		t.Fatal(err)
	}
	var oldHash string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		if strings.HasSuffix(r.URL.Path, "/revisions") {
			data = []any{map[string]any{"version": 2, "spec": map[string]any{"canvas_auto_run_grant": map[string]any{"config_version": int64(4), "config_hash": oldHash}}}}
		} else {
			data = []any{map[string]any{"id": "mon_plan", "tenant_id": scope.TenantID, "workspace_id": scope.WorkspaceID, "source_ids": []string{"src_1"}, "interval_seconds": 3600, "timezone": "UTC", "budget_micro_per_window": 100, "version": 3}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}))
	defer srv.Close()
	engine := EngineClient{Cfg: Config{EngineURL: srv.URL, SigningKey: key, Audience: "loom-engine", TTL: MaxTokenTTL}}
	args := map[string]any{"purpose": MonitorCanvasRecurringPurpose, "monitor_id": "mon_plan", "canvas_id": canvas.ID, "total_budget_micro": 1000, "max_runs": 10, "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
	// Without the app-registered tool provider the plan fails closed.
	RegisterMonitorCanvasRecurringTool(nil)
	t.Cleanup(func() { RegisterMonitorCanvasRecurringTool(nil) })
	if _, err := PrepareCanvasMonitorApproval(context.Background(), db, engine, scope, "u_alice", args); err == nil || !strings.Contains(err.Error(), "canvas_tool_unavailable") {
		t.Fatalf("unregistered tool provider accepted: %v", err)
	}
	RegisterMonitorCanvasRecurringTool(func() (string, error) { return "canvas_run_workflow", nil })
	raw, err := PrepareCanvasMonitorApproval(context.Background(), db, engine, scope, "u_alice", args)
	if err != nil {
		t.Fatal(err)
	}
	var plan MonitorCanvasApprovedPlan
	if err = json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Actor != "user:u_alice" || plan.ConfigVersion != 5 || plan.IntervalSeconds != 3600 || plan.BudgetMicroPerWindow != 100 || plan.ConfigHash == "" {
		t.Fatalf("not server frozen: %+v", plan)
	}
	oldHash = plan.ConfigHash
	canvas.PayloadJSON = `{"nodes":[{"id":"logic","type":"logic","metadata":{"logic":{"operator":"compute","expression":"n*2"},"lastRunSnapshot":{"status":"success"},"content":"derived"}}],"connections":[]}`
	if err = db.Model(&canvas).Updates(map[string]any{"payload_json": canvas.PayloadJSON, "revision": 999}).Error; err != nil {
		t.Fatal(err)
	}
	raw, err = PrepareCanvasMonitorApproval(context.Background(), db, engine, scope, "u_alice", args)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &plan)
	if plan.ConfigVersion != 4 || plan.ConfigHash != oldHash {
		t.Fatal("runtime snapshot changed semantic config")
	}
	for _, change := range []map[string]any{{"canvas_id": "foreign"}, {"tool_ids": []string{"unregistered_tool"}}, {"max_runs": 0}, {"total_budget_micro": 99}, {"expires_at": "2000-01-01T00:00:00Z"}, {"config_version": 999}} {
		in := map[string]any{}
		for k, v := range args {
			in[k] = v
		}
		for k, v := range change {
			in[k] = v
		}
		if _, err := PrepareCanvasMonitorApproval(context.Background(), db, engine, scope, "u_alice", in); err == nil {
			t.Fatalf("accepted %+v", change)
		}
	}
}

func TestT21MonitorCanvasBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testDB(t)
	if err := db.AutoMigrate(&model.CanvasProject{}); err != nil {
		t.Fatal(err)
	}
	for _, canvas := range []model.CanvasProject{{ID: "canvas_owned", UserID: "u_alice", PayloadJSON: `{"nodes":[],"connections":[]}`}, {ID: "canvas_foreign", UserID: "u_bob", PayloadJSON: `{}`}} {
		if err := db.Create(&canvas).Error; err != nil {
			t.Fatal(err)
		}
	}
	scope, err := ResolveScope(db, "u_alice", "")
	if err != nil {
		t.Fatal(err)
	}
	var forwarded []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := Verify(key, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "loom-engine", timeNow())
		if err != nil || ctx.Actor.ID != "u_alice" || ctx.WorkspaceID != scope.WorkspaceID {
			t.Error("invalid signed context")
		}
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(b, &body); err != nil {
			t.Fatal(err)
		}
		forwarded = append(forwarded, body)
		_, _ = w.Write([]byte(`{"code":0,"data":{},"msg":"ok"}`))
	}))
	defer srv.Close()
	r := gin.New()
	RegisterRoutes(r.Group("/api"), db, Config{EngineURL: srv.URL, SigningKey: key, Issuer: "platform", Audience: "loom-engine", TTL: MaxTokenTTL}, func(*gin.Context) (*model.User, error) { return &model.User{ID: "u_alice"}, nil })
	for _, tc := range []struct {
		name, method, path, body, workspace, reason string
		status                                      int
		forward                                     bool
	}{
		{name: "bind owned canvas", body: `{"canvas_id":"canvas_owned"}`, status: 200, forward: true},
		{name: "foreign owner", body: `{"canvas_id":"canvas_foreign"}`, status: 404, reason: "canvas_not_found"},
		{name: "missing actual canvas", body: `{"canvas_id":"sidebar_only"}`, status: 404, reason: "canvas_not_found"},
		{name: "foreign workspace", body: `{"canvas_id":"canvas_owned"}`, workspace: "foreign_workspace", status: 404},
		{name: "forged grant", body: `{"canvas_id":"canvas_owned","canvas_auto_run_grant":{"approval_id":"fake","actor":"user:mallory"}}`, status: 409, reason: "canvas_approval_unavailable"},
		{name: "confirm is not approval", body: `{"canvas_id":"canvas_owned","canvas_auto_run_grant":{"approval_id":"fake"},"confirmExpansion":true}`, status: 409, reason: "canvas_approval_unavailable"},
		{name: "injected snapshot", body: `{"canvas_id":"canvas_owned","canvas_config":{"nodes":[]}}`, status: 400, reason: "canvas_client_snapshot_forbidden"},
		{name: "injected actor", body: `{"canvas_id":"canvas_owned","grant_actor":"user:mallory"}`, status: 400, reason: "canvas_client_snapshot_forbidden"},
		{name: "null body", body: `null`, status: 400, reason: "monitor_body_invalid"},
		{name: "revoke grant", body: `{"canvas_auto_run_grant":null}`, status: 200, forward: true},
		{name: "unlink", body: `{"canvas_id":null}`, status: 200, forward: true},
		{name: "pause", body: `{"enabled":false}`, status: 200, forward: true},
		{name: "resume", body: `{"enabled":true}`, status: 200, forward: true},
		{name: "create cannot smuggle grant", method: "POST", path: "/api/intelligence/monitors", body: `{"preview_id":"pv","canvas_auto_run_grant":{"approval_id":"fake"}}`, status: 409, reason: "canvas_approval_unavailable"},
		{name: "preview cannot smuggle grant", method: "POST", path: "/api/intelligence/monitors/preview", body: `{"canvas_auto_run_grant":{"approval_id":"fake"}}`, status: 409, reason: "canvas_approval_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method, path := tc.method, tc.path
			if method == "" {
				method = "PATCH"
			}
			if path == "" {
				path = "/api/intelligence/monitors/mon_1"
			}
			req := httptest.NewRequest(method, path, strings.NewReader(tc.body))
			req.Header.Set("X-Idempotency-Key", "t21")
			req.Header.Set("X-Workspace-Id", tc.workspace)
			n := len(forwarded)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.reason != "" {
				var body map[string]any
				_ = json.Unmarshal(w.Body.Bytes(), &body)
				if body["reason"] != tc.reason {
					t.Fatalf("reason: %s", w.Body.String())
				}
			}
			if (len(forwarded) > n) != tc.forward {
				t.Fatalf("unexpected forwarding: %d -> %d", n, len(forwarded))
			}
		})
	}
	// Current permission is checked again, not inherited from an earlier successful binding.
	if err := db.Model(&model.WorkspaceMembership{}).Where("workspace_id = ? AND user_id = ?", scope.WorkspaceID, "u_alice").Update("role", "viewer").Error; err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("PATCH", "/api/intelligence/monitors/mon_1", strings.NewReader(`{"canvas_id":"canvas_owned"}`))
	req.Header.Set("X-Workspace-Id", scope.WorkspaceID)
	w := httptest.NewRecorder()
	n := len(forwarded)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || len(forwarded) != n {
		t.Fatalf("revoked permission: %d %s", w.Code, w.Body.String())
	}
}

func TestT21MonitorCanvasRejectsAccessibleWrongWorkspace(t *testing.T) {
	db := testDB(t)
	if err := db.AutoMigrate(&model.CanvasProject{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CanvasProject{ID: "owned", UserID: "u_alice", PayloadJSON: `{}`}).Error; err != nil {
		t.Fatal(err)
	}
	s, err := ResolveScope(db, "u_alice", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Workspace{ID: "other", TenantID: s.TenantID, Name: "other"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.WorkspaceMembership{WorkspaceID: "other", UserID: "u_alice", Role: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	s.WorkspaceID = "other"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	if validateMonitorCanvas(c, db, s, "u_alice", map[string]any{"canvas_id": "owned"}) {
		t.Fatal("cross-workspace binding accepted")
	}
	if w.Code != 403 || !strings.Contains(w.Body.String(), `"reason":"canvas_workspace_mismatch"`) {
		t.Fatal(w.Body.String())
	}
}
