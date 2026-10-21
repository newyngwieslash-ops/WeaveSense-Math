package flow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"yingce/backend/internal/model"
)

func TestTopicAnalystPermissions(t *testing.T) {
	db := testDB(t)
	s := memberControlOrg(t, db)
	if err := SetMemberRole(db, s, "u_bob", "analyst"); err != nil {
		t.Fatal(err)
	}
	a, err := ResolveScope(db, "u_bob", s.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	title := "analyst topic"
	v, err := CreateTopic(db, a, "u_bob", TopicInput{Title: &title}, time.Now())
	if err != nil {
		t.Fatalf("analyst create: %v", err)
	}
	if _, err := UpdateTopic(db, a, v.ID, 1, TopicInput{Title: &title}, time.Now()); err != nil {
		t.Fatalf("analyst update: %v", err)
	}
	spec, _ := FindTool("intel_save_topic")
	if _, err := spec.local(Tools{DB: db}, context.Background(), model.FlowAgentConversation{}, SubjectScope{Scope: a}, "u_bob", map[string]any{"title": "tool topic"}); err != nil {
		t.Fatalf("analyst tool: %v", err)
	}
	for _, role := range []string{"viewer", "unknown"} {
		denied := a
		denied.Role = role
		if _, err := CreateTopic(db, denied, "u_bob", TopicInput{Title: &title}, time.Now()); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s create: %v", role, err)
		}
		if _, err := UpdateTopic(db, denied, v.ID, 0, TopicInput{Title: &title}, time.Now()); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s update: %v", role, err)
		}
	}
	if err := SetMemberRole(db, a, "u_alice", "viewer"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("analyst member management: %v", err)
	}
}

func TestTopicAnalystHTTP(t *testing.T) {
	db := testDB(t)
	s := memberControlOrg(t, db)
	if err := SetMemberRole(db, s, "u_bob", "analyst"); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterTopicRoutes(r.Group("/api"), db, func(c *gin.Context) (*model.User, error) { return &model.User{ID: "u_bob"}, nil })
	call := func(method, path, ws, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/intelligence/topics"+path, strings.NewReader(body))
		req.Header.Set("X-Workspace-Id", ws)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	w := call("POST", "", s.WorkspaceID, `{"title":"HTTP analyst"}`)
	if w.Code != 201 {
		t.Fatalf("analyst POST HTTP=%d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		Data TopicView `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{200, 409} {
		w = call("PATCH", "/"+out.Data.ID, s.WorkspaceID, `{"title":"updated","ifVersion":1}`)
		if w.Code != want {
			t.Fatalf("PATCH %d: HTTP=%d want=%d", i, w.Code, want)
		}
	}
	other := model.Workspace{ID: NewID("ws_"), TenantID: s.TenantID, Slug: "second", Name: "second"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.WorkspaceMembership{ID: NewID("wm_"), WorkspaceID: other.ID, UserID: "u_bob", Role: "analyst"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "PATCH"} {
		if w := call(method, "/"+out.Data.ID, other.ID, `{"title":"cross"}`); w.Code != 404 {
			t.Fatalf("cross workspace %s=%d", method, w.Code)
		}
	}
	if err := SetMemberRole(db, s, "u_bob", "viewer"); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"POST", "PATCH"} {
		path := ""
		if method == "PATCH" {
			path = "/" + out.Data.ID
		}
		if w := call(method, path, s.WorkspaceID, `{"title":"denied"}`); w.Code != 403 {
			t.Fatalf("viewer %s=%d", method, w.Code)
		}
	}
}
