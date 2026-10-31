package edumath

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

// TestServeBrowserE2E serves a seeded edumath API for the real Chrome E2E
// (web/scripts/edu-chrome-e2e.mjs). It is skipped unless EDU_E2E_ADDR is set.
// The real isolated math service builds and verifies the lesson; identities are
// test-only (X-Test-User injected for the seeded student on /api/edu/student
// paths). Control endpoints exist only in this test file.
func TestServeBrowserE2E(t *testing.T) {
	addr := os.Getenv("EDU_E2E_ADDR")
	if addr == "" {
		t.Skip("EDU_E2E_ADDR not set")
	}
	h := newHarness(t, startMathService(t))
	tenant := h.setupTeacher()
	pubID, lesson := publishG1(t, h)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "浏览器验收班"})
	classID := data(out)["id"].(string)
	_, out, raw, _ := h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "浏览器验收", "answerPolicy": "teacher_release"})
	asID, _ := data(out)["id"].(string)
	if asID == "" {
		t.Fatalf("assignment: %s", raw)
	}
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]}); c != 200 {
		t.Fatalf("join: %d %s", c, raw)
	}
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-e2e-1"})

	// A second assignment whose self-test is an approved variant set (AC-22, FR-S08).
	var pub model.EduPublication
	h.db.First(&pub, "id = ?", pubID)
	_, out, raw, _ = h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+pub.LessonRevisionID+"/variant-sets", map[string]any{"count": 3, "idempotencyKey": "e2e-vs"})
	job := h.waitJob("u_alice", data(out)["id"].(string))
	setID, _ := job["variantSetId"].(string)
	if setID == "" {
		t.Fatalf("variant set: %s %v", raw, job)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/variant-sets/"+setID, nil)
	vs := data(out)
	setMarkers := []string{}
	for _, k := range vs["answerKeys"].(map[string]any) {
		for _, f := range []string{"exact", "plain", "latex"} {
			if m, _ := k.(map[string]any)[f].(string); len(m) >= 3 {
				setMarkers = append(setMarkers, m)
			}
		}
	}
	h.db.Save(&model.EduOrgPolicy{TenantID: tenant, ReviewMode: "self_allowed"})
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/variant-sets/"+setID+"/review", map[string]any{"setHash": vs["setHash"], "decision": "approved"}); c != 200 {
		t.Fatalf("approve set: %d %s", c, raw)
	}
	_, out, raw, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "题组自测", "answerPolicy": "teacher_release", "variantSetId": setID})
	setAsID, _ := data(out)["id"].(string)
	if setAsID == "" {
		t.Fatalf("set assignment: %s", raw)
	}

	// DEV-088 and DEV-096: a third assignment for the weak network, figure failure and 320px phase,
	// so its attempts and hints do not change what the other phases count.
	_, out, raw, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "弱网窄屏验收", "answerPolicy": "teacher_release"})
	weakAsID, _ := data(out)["id"].(string)
	if weakAsID == "" {
		t.Fatalf("weak assignment: %s", raw)
	}

	// DEV-112 teacher flow: its own class and assignment on the same publication, a second published
	// revision to migrate to, and one stale attempt result for the batch regrade to correct.
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "影响确认班"})
	tClassID := data(out)["id"].(string)
	_, out, raw, _ = h.call("u_alice", "POST", "/api/edu/classes/"+tClassID+"/assignments", map[string]any{"publicationId": pubID, "title": "影响确认任务", "answerPolicy": "teacher_release"})
	tAsID, _ := data(out)["id"].(string)
	if tAsID == "" {
		t.Fatalf("teacher assignment: %s", raw)
	}
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+tClassID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]}); c != 200 {
		t.Fatalf("join teacher class: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu/student/assignments/"+tAsID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "e2e-teacher-attempt-1"}, "X-Edu-Tenant", tenant); c != 201 {
		t.Fatalf("teacher-flow attempt: %d %s", c, raw)
	}
	h.db.Model(&model.EduAttempt{}).Where("assignment_id = ? AND client_key = ?", tAsID, "e2e-teacher-attempt-1").Updates(map[string]any{"result": "correct", "grader": "old_key"})
	var lr model.EduLessonRevision
	h.db.First(&lr, "id = ?", pub.LessonRevisionID)
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/lesson-revisions/"+lr.ID+"/editable", nil)
	beat := data(out)["beats"].([]any)[0].(map[string]any)["id"].(string)
	c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+lr.ID+"/edits", map[string]any{"baseLessonHash": lr.LessonHash, "idempotencyKey": "e2e-fix", "saveAs": "branch",
		"edits": []map[string]any{{"target": "beat", "id": beat, "field": "say", "value": "重新整理这一步的说明。"}}})
	if c != 202 {
		t.Fatalf("edit: %d %s", c, raw)
	}
	ej := h.waitJob("u_alice", data(out)["job"].(map[string]any)["id"].(string))
	var fix model.EduLessonRevision
	h.db.First(&fix, "id = ?", ej["lessonRevisionId"])
	if fix.ID == "" {
		t.Fatalf("edit produced no revision: %v", ej)
	}
	if fix.State != "approved" {
		if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+fix.ID+"/review", map[string]any{"lessonHash": fix.LessonHash, "decision": "approved", "comment": "重审"}); c != 200 {
			t.Fatalf("review fix: %d %s", c, raw)
		}
	}
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+fix.ID+"/publish", map[string]any{"lessonHash": fix.LessonHash})
	if c != 200 {
		t.Fatalf("publish fix: %d %s", c, raw)
	}
	fixPubID := data(out)["id"].(string)

	done := make(chan struct{})
	mux := http.NewServeMux()
	// Server state the teacher flow must have produced; read from the database, not from the page.
	mux.HandleFunc("/__e2e/teacher-state", func(w http.ResponseWriter, r *http.Request) {
		var pubs []model.EduPublication
		h.db.Where("id IN ?", []string{pubID, fixPubID}).Find(&pubs)
		ps := map[string]string{}
		for _, p := range pubs {
			ps[p.ID] = p.State
		}
		var as []model.EduAssignment
		h.db.Where("class_id = ?", tClassID).Order("created_at").Find(&as)
		rows := []map[string]string{}
		for _, a := range as {
			rows = append(rows, map[string]string{"id": a.ID, "publicationId": a.PublicationID, "state": a.State})
		}
		var att model.EduAttempt
		h.db.First(&att, "assignment_id = ? AND client_key = ?", tAsID, "e2e-teacher-attempt-1")
		var audits []model.EduAuditEvent
		h.db.Where("tenant_id = ?", tenant).Order("created_at").Find(&audits)
		acts := []string{}
		for _, a := range audits {
			acts = append(acts, a.Action)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"publications": ps, "assignments": rows, "attemptResult": att.Result, "audit": acts})
	})
	// The teacher page lists the caller's workspaces from the platform; the E2E backend has one.
	mux.HandleFunc("/api/intelligence/workspaces", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "ok", "data": []map[string]any{{"id": model.PersonalWorkspaceID("u_alice"), "name": "E2E 教师工作区"}}})
	})
	mux.HandleFunc("/__e2e/release", func(w http.ResponseWriter, r *http.Request) {
		c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/assignments/"+asID+"/release-answer", map[string]any{})
		w.WriteHeader(c)
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("/__e2e/summary", func(w http.ResponseWriter, r *http.Request) {
		c, _, raw, _ := h.call("u_alice", "GET", "/api/edu/assignments/"+asID+"/figure-summary", nil)
		w.WriteHeader(c)
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("/__e2e/weak-release", func(w http.ResponseWriter, r *http.Request) {
		c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/assignments/"+weakAsID+"/release-answer", map[string]any{})
		w.WriteHeader(c)
		_, _ = w.Write(raw)
	})
	// What the server received from the weak network phase; read from the database and the teacher summary.
	mux.HandleFunc("/__e2e/weak-state", func(w http.ResponseWriter, r *http.Request) {
		var attempts int64
		h.db.Model(&model.EduAttempt{}).Where("assignment_id = ?", weakAsID).Count(&attempts)
		_, out, _, _ := h.call("u_alice", "GET", "/api/edu/assignments/"+weakAsID+"/figure-summary", nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"attempts": attempts, "figure": data(out)["figure"]})
	})
	mux.HandleFunc("/__e2e/done", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
		select {
		case <-done:
		default:
			close(done)
		}
	})
	// The web shell confirms the login session before rendering any route and blocks the page when
	// /api/auth/session fails. The E2E backend has no platform auth, so answer as the platform does for
	// a browser without a session cookie (user null); edu identity still comes from X-Test-User below.
	mux.HandleFunc("/api/auth/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"user":null},"msg":"ok"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/edu/student/") || strings.HasPrefix(r.URL.Path, "/api/edu/files/") {
			r.Header.Set("X-Test-User", "u_bob")
		} else if strings.HasPrefix(r.URL.Path, "/api/edu/") && strings.Contains(r.Referer(), "/dev/edu-teach") {
			r.Header.Set("X-Test-User", "u_alice") // the teacher page acts as the seeded teacher
		} else if strings.HasPrefix(r.URL.Path, "/api/edu/") {
			r.Header.Del("X-Test-User") // the student page acts only as the seeded student
		}
		h.r.ServeHTTP(w, r)
	})
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(l) }()
	defer srv.Close()

	markers := []string{}
	for _, m := range []string{lesson.Answer.Exact, lesson.Answer.Latex, lesson.Answer.Plain} {
		if len(m) >= 3 {
			markers = append(markers, m)
		}
	}
	ready, _ := json.Marshal(map[string]any{"ready": true, "assignmentId": asID, "tenant": tenant, "answerMarkers": markers,
		"setAssignmentId": setAsID, "weakAssignmentId": weakAsID, "setItems": len(vs["items"].([]any)), "setMarkers": setMarkers,
		"teacher": map[string]any{"classId": tClassID, "assignmentId": tAsID, "publicationId": pubID, "fixPublicationId": fixPubID}})
	fmt.Println("EDU_E2E_READY " + string(ready))
	select {
	case <-done:
	case <-time.After(10 * time.Minute):
		t.Fatal("browser E2E did not finish within 10 minutes")
	}
}
