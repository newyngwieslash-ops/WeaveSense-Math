package edumath

import (
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

// TestWithdrawImpact (DEV-112, E59): withdrawing an assignment first shows who
// and what it touches; only a teacher of the class can confirm, with a reason;
// the confirmed withdrawal hides it from students, keeps their records and is
// audited with the counts. Ordinary reads need no confirmation.
func TestWithdrawImpact(t *testing.T) { runWithdrawImpact(t, newHarness(t, startMathService(t))) }

func TestPGWithdrawImpact(t *testing.T) {
	runWithdrawImpact(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func runWithdrawImpact(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高一(1)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "撤回影响", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	code := data(out)["code"]
	for _, u := range []string{"u_bob", "u_dan"} {
		h.call(u, "POST", "/api/edu-join", map[string]any{"code": code})
		h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": u, "status": "granted", "evidenceRef": "form-impact-" + u})
	}
	stu := []string{"X-Edu-Tenant", tenant}
	pA := "/api/edu/student/assignments/" + asID
	h.call("u_bob", "POST", pA+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "impact-attempt-01"}, stu...)
	h.call("u_bob", "POST", pA+"/attempts", map[string]any{"kind": "answer", "response": "2", "clientKey": "impact-attempt-02"}, stu...)
	h.call("u_bob", "POST", pA+"/help", map[string]any{"kind": "ask_teacher", "message": "第二步", "clientKey": "impact-help-0001"}, stu...)
	w := "/api/edu/assignments/" + asID + "/withdraw"

	// Students and outsiders cannot preview or withdraw.
	if c, _, _, _ := h.call("u_bob", "POST", w, map[string]any{}, stu...); c == 200 {
		t.Fatal("student previewed a withdrawal")
	}
	if c, _, _, _ := h.call("u_carol", "POST", w, map[string]any{"confirm": true, "reason": "不再使用"}); c == 200 {
		t.Fatal("another workspace withdrew the assignment")
	}
	// Preview: concrete counts, nothing changes.
	c, out, raw, _ := h.call("u_alice", "POST", w, map[string]any{})
	if c != 200 || data(out)["dryRun"] != true {
		t.Fatalf("preview: %d %s", c, raw)
	}
	imp := data(out)["impact"].(map[string]any)
	if imp["students"].(float64) != 2 || imp["studentsWithAttempts"].(float64) != 1 || imp["attempts"].(float64) != 2 || imp["openHelp"].(float64) != 1 {
		t.Fatalf("impact: %v", imp)
	}
	if c, _, _, _ := h.call("u_bob", "GET", pA, nil, stu...); c != 200 {
		t.Fatalf("preview must not hide the assignment: %d", c)
	}
	// Confirm needs a reason.
	if c, _, _, _ := h.call("u_alice", "POST", w, map[string]any{"confirm": true}); c != 400 {
		t.Fatalf("confirm without reason: %d", c)
	}
	if c, _, _, _ := h.call("u_alice", "POST", w, map[string]any{"confirm": true, "reason": "<b>换题</b>"}); c != 400 {
		t.Fatalf("confirm with markup: %d", c)
	}
	c, out, raw, _ = h.call("u_alice", "POST", w, map[string]any{"confirm": true, "reason": "题目换成新版本"})
	if c != 200 || data(out)["state"] != "withdrawn" {
		t.Fatalf("withdraw: %d %s", c, raw)
	}
	if c, _, _, _ := h.call("u_bob", "GET", pA, nil, stu...); c == 200 {
		t.Fatal("withdrawn assignment still served to students")
	}
	if c, _, _, _ := h.call("u_alice", "POST", w, map[string]any{"confirm": true, "reason": "题目换成新版本"}); c != 409 {
		t.Fatalf("second withdrawal: %d", c)
	}
	var attempts int64
	h.db.Model(&model.EduAttempt{}).Where("assignment_id = ?", asID).Count(&attempts)
	if attempts != 2 {
		t.Fatalf("records must be kept: %d", attempts)
	}
	var ev model.EduAuditEvent
	if h.db.Where("tenant_id = ? AND action = ?", tenant, "assignment.withdraw").First(&ev).Error != nil ||
		!strings.Contains(ev.Detail, "students=2 attempts=2 help=1") || !strings.Contains(ev.Detail, "题目换成新版本") {
		t.Fatalf("audit: %+v", ev)
	}
}
