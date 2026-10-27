package edumath

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEduResponsesNeverCached (DEV-097): every education response, success or
// error, for anonymous callers, students and teachers, says no-store, so no
// browser, proxy or CDN can keep a lesson, an answer released and withdrawn
// again, or another user's data.
func TestEduResponsesNeverCached(t *testing.T) {
	h := newHarness(t, startMathService(t))
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二(9)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "缓存", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	code := data(out)["code"]
	check := func(who, method, path string, c int, hdr interface{ Get(string) string }) {
		t.Helper()
		if cc := hdr.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("%s %s %s: %d Cache-Control %q", who, method, path, c, cc)
		}
	}
	_, _, _, hdr := h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": code})
	check("student", "POST", "/api/edu-join", 0, hdr)
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-cache"})
	stu := []string{"X-Edu-Tenant", tenant}
	for _, p := range []string{"/api/edu/student/assignments", "/api/edu/student/assignments/" + asID, "/api/edu/student/assignments/" + asID + "/learning"} {
		c, _, raw, hdr := h.call("u_bob", "GET", p, nil, stu...)
		if c != 200 {
			t.Fatalf("student %s: %d %s", p, c, raw)
		}
		check("student", "GET", p, c, hdr)
	}
	c, _, _, hdr := h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/hints", map[string]any{"level": 9}, stu...)
	check("student error", "POST", "hints", c, hdr)
	n := 0
	for _, rt := range h.r.Routes() {
		if !strings.HasPrefix(rt.Path, "/api/edu") || rt.Path == "/api/edu-math-events" {
			continue
		}
		p := strings.NewReplacer(":id", "x_unknown", ":studentId", "u_bob", ":userId", "u_bob", ":inviteId", "x", ":eid", "x").Replace(rt.Path)
		for _, who := range []string{"", "u_bob", "u_alice"} {
			req := httptest.NewRequest(rt.Method, p, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			if who != "" {
				req.Header.Set("X-Test-User", who)
			}
			if who == "u_bob" {
				req.Header.Set("X-Edu-Tenant", tenant)
			}
			rec := httptest.NewRecorder()
			h.r.ServeHTTP(rec, req)
			check(who, rt.Method, rt.Path, rec.Code, rec.Header())
		}
		n++
	}
	if n < 80 {
		t.Fatalf("route sweep looks incomplete: %d", n)
	}
}
