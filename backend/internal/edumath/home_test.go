package edumath

import (
	"net/url"
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

// TestTeacherHome (DEV-067, E46): first run shows only the flagged original sample; after work it
// lists my lessons, lessons I may review under the institution policy, my classes with students,
// active assignments and open (urgent) questions, and open incidents; another tenant's teacher and
// a student see none of it. Runs on the real math service.
func TestTeacherHome(t *testing.T) { runTeacherHome(t, newHarness(t, startMathService(t))) }

func TestPGTeacherHome(t *testing.T) {
	runTeacherHome(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func runTeacherHome(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	home := func(u string, hdr ...string) (int, map[string]any, []byte) {
		c, out, raw, _ := h.call(u, "GET", "/api/edu/home", nil, hdr...)
		return c, data(out), raw
	}
	c, d, raw := home("u_alice")
	if c != 200 || d["firstRun"] != true || d["sample"].(map[string]any)["sample"] != true || len(d["myLessons"].([]any)) != 0 {
		t.Fatalf("first run: %d %s", c, raw)
	}
	pubID, tl := publishG1(t, h)
	// A second lesson waiting for review.
	var pub model.EduPublication
	h.db.First(&pub, "id = ?", pubID)
	var l0 model.EduLessonRevision
	h.db.First(&l0, "id = ?", pub.LessonRevisionID)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/solution-revisions/"+l0.SolutionRevisionID+"/lesson", map[string]any{"idempotencyKey": "home-l2"})
	lj := h.waitJob("u_alice", data(out)["id"].(string))
	pendingID := lj["lessonRevisionId"].(string)

	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二(7)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "首页", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-home"})
	stu := []string{"X-Edu-Tenant", tenant}
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/help", map[string]any{"kind": "content_error", "beatId": "s1", "message": "第二步好像错了", "clientKey": "home-help-01"}, stu...); c != 201 {
		t.Fatalf("help: %d %s", c, raw)
	}
	h.call("u_alice", "POST", "/api/edu/publications/"+pubID+"/incidents", map[string]any{"severity": "minor", "reason": "措辞"})

	c, d, raw = home("u_alice")
	if c != 200 || d["firstRun"] != false {
		t.Fatalf("home: %d %s", c, raw)
	}
	if n := len(d["myLessons"].([]any)); n != 2 {
		t.Fatalf("my lessons: %d %s", n, raw)
	}
	pend := d["pendingReview"].([]any)
	if len(pend) != 1 || pend[0].(map[string]any)["id"] != pendingID || pend[0].(map[string]any)["title"] == "" {
		t.Fatalf("pending review (self_allowed): %s", raw)
	}
	ks := d["classes"].([]any)
	k := ks[0].(map[string]any)
	if len(ks) != 1 || k["students"].(float64) != 1 || k["activeAssignments"].(float64) != 1 || k["openHelp"].(float64) != 1 || k["urgentHelp"].(float64) != 1 {
		t.Fatalf("classes: %s", raw)
	}
	if d["openIncidents"].(float64) != 1 || d["urgentHelp"].(float64) != 1 {
		t.Fatalf("incidents/help: %s", raw)
	}
	for _, a := range []string{tl.Answer.Exact, tl.Answer.Plain} {
		if a != "" && strings.Contains(string(raw), a) {
			t.Fatalf("home carries the answer")
		}
	}
	// Catalog (DEV-081): search by title text and family, published filter, no answers.
	cat := func(u, query string, hdr ...string) (int, []any, []byte) {
		c, out, raw, _ := h.call(u, "GET", "/api/edu/catalog"+query, nil, hdr...)
		d := data(out)
		if d == nil || d["items"] == nil {
			return c, nil, raw
		}
		return c, d["items"].([]any), raw
	}
	var prob model.EduProblem
	h.db.Raw("SELECT p.* FROM edu_problems p JOIN edu_problem_revisions r ON r.problem_id = p.id WHERE r.id = ?", l0.ProblemRevisionID).Scan(&prob)
	word := []rune(prob.Title)
	if c, items, raw := cat("u_alice", ""); c != 200 || len(items) != 2 {
		t.Fatalf("catalog all: %d %s", c, raw)
	}
	if c, items, raw := cat("u_alice", "?state=published"); c != 200 || len(items) != 1 || items[0].(map[string]any)["publicationId"] != pubID {
		t.Fatalf("catalog published: %d %s", c, raw)
	}
	if c, items, raw := cat("u_alice", "?q="+url.QueryEscape(string(word[:min(2, len(word))]))+"&family="+prob.FamilyID); c != 200 || len(items) != 2 {
		t.Fatalf("catalog by title and family: %d %s", c, raw)
	}
	if c, items, raw := cat("u_alice", "?q=%25%25"); c != 200 || len(items) != 0 {
		t.Fatalf("catalog wildcard must be literal: %d %s", c, raw)
	}
	if c, _, raw := cat("u_alice", "?state=bogus"); c != 400 {
		t.Fatalf("catalog bad state: %d %s", c, raw)
	}
	if _, _, raw := cat("u_alice", ""); tl.Answer.Exact != "" && strings.Contains(string(raw), tl.Answer.Exact) {
		t.Fatalf("catalog carries the answer")
	}
	// Under second_reviewer the author cannot review her own lesson, so it leaves her queue.
	h.db.Save(&model.EduOrgPolicy{TenantID: tenant, ReviewMode: "second_reviewer"})
	if _, d, raw = home("u_alice"); len(d["pendingReview"].([]any)) != 0 {
		t.Fatalf("second_reviewer queue: %s", raw)
	}
	// Another tenant's teacher sees only her own (empty) home; a student has no teacher home.
	for _, r := range []string{"teacher"} {
		h.call("u_carol", "POST", "/api/edu/roles", map[string]any{"tenantId": model.PersonalTenantID("u_carol"), "userId": "u_carol", "role": r})
	}
	c, d, raw = home("u_carol")
	if c != 200 || len(d["myLessons"].([]any)) != 0 || len(d["classes"].([]any)) != 0 || strings.Contains(string(raw), classID) || strings.Contains(string(raw), pendingID) {
		t.Fatalf("carol home: %d %s", c, raw)
	}
	if c, _, raw = home("u_carol", "X-Workspace-Id", model.PersonalWorkspaceID("u_alice")); c < 400 {
		t.Fatalf("carol naming alice's workspace: %d %s", c, raw)
	}
	if c, items, raw := cat("u_carol", ""); c != 200 || len(items) != 0 || strings.Contains(string(raw), pendingID) {
		t.Fatalf("carol catalog: %d %s", c, raw)
	}
	if c, _, raw := cat("u_carol", "", "X-Workspace-Id", model.PersonalWorkspaceID("u_alice")); c < 400 {
		t.Fatalf("carol catalog naming alice's workspace: %d %s", c, raw)
	}
	if c, _, raw := cat("u_bob", "", stu...); c < 400 {
		t.Fatalf("student catalog: %d %s", c, raw)
	}
	if c, _, raw = home("u_bob", stu...); c < 400 {
		t.Fatalf("student home: %d %s", c, raw)
	}
}
