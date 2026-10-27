package edumath

import (
	"errors"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

func TestPauseFailClosed(t *testing.T) {
	runPauseFailClosed(t, newHarness(t, startMathService(t)))
}

func TestPGPauseFailClosed(t *testing.T) {
	runPauseFailClosed(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

// runPauseFailClosed (DEV-125, E66): when the pause state cannot be read, every
// gated write (new job, publish, new assignment, migration) and every student
// read and submission is refused with 503 PAUSE_CHECK_UNAVAILABLE instead of
// being allowed; nothing is written by the refused calls, and everything works
// again once the lookup recovers.
func runPauseFailClosed(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	p1, _ := publishG1(t, h)
	// A second build, published, as a migration target.
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/problem-revisions/"+firstRevision(h, p1)+"/analyze", map[string]any{"idempotencyKey": "learn-a"})
	sol := h.waitJob("u_alice", data(out)["id"].(string))["solutionRevisionId"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/solution-revisions/"+sol+"/lesson", map[string]any{"idempotencyKey": "fc-l2"})
	l2ID := h.waitJob("u_alice", data(out)["id"].(string))["lessonRevisionId"].(string)
	var l2 model.EduLessonRevision
	h.db.First(&l2, "id = ?", l2ID)
	h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+l2ID+"/review", map[string]any{"lessonHash": l2.LessonHash, "decision": "approved", "comment": "自审"})
	c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+l2ID+"/publish", map[string]any{"lessonHash": l2.LessonHash})
	if c != 200 {
		t.Fatalf("publish v2: %d %s", c, raw)
	}
	p2 := data(out)["id"].(string)
	// A third build, reviewed and left unpublished, to exercise publish.
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/solution-revisions/"+sol+"/lesson", map[string]any{"idempotencyKey": "fc-l3"})
	l3ID := h.waitJob("u_alice", data(out)["id"].(string))["lessonRevisionId"].(string)
	var l3 model.EduLessonRevision
	h.db.First(&l3, "id = ?", l3ID)
	h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+l3ID+"/review", map[string]any{"lessonHash": l3.LessonHash, "decision": "approved", "comment": "自审"})

	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高一(6)班"})
	classID := data(out)["id"].(string)
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": p1, "title": "失败即拒绝", "answerPolicy": "teacher_release"})
	if c != 201 {
		t.Fatalf("assignment: %d %s", c, raw)
	}
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-fc-bob"})
	stu := []string{"X-Edu-Tenant", tenant}
	sa := "/api/edu/student/assignments/"
	if c, _, raw, _ := h.call("u_bob", "GET", sa+asID, nil, stu...); c != 200 {
		t.Fatalf("student read before the fault: %d %s", c, raw)
	}
	rev := h.confirmedRevision("3", "2")

	// Inject a failure into every read of edu_pauses.
	var failing atomic.Bool
	const name = "test:edu_pauses_fault"
	if err := h.db.Callback().Query().Before("gorm:query").Register(name, func(db *gorm.DB) {
		if failing.Load() && db.Statement.Table == "edu_pauses" {
			_ = db.AddError(errors.New("injected pause lookup failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.db.Callback().Query().Remove(name) })
	count := func() [3]int64 {
		var r [3]int64
		h.db.Model(&model.EduMathJob{}).Where("tenant_id = ?", tenant).Count(&r[0])
		h.db.Model(&model.EduAttempt{}).Where("tenant_id = ?", tenant).Count(&r[1])
		h.db.Model(&model.EduPublication{}).Where("tenant_id = ?", tenant).Count(&r[2])
		return r
	}
	before := count()
	failing.Store(true)
	refused := func(what string, c int, out map[string]any) {
		t.Helper()
		if c != 503 || errCode(out) != "PAUSE_CHECK_UNAVAILABLE" {
			t.Fatalf("%s must fail closed, got %d %v", what, c, out)
		}
	}
	c, out, _, _ = h.call("u_alice", "POST", "/api/edu/problem-revisions/"+rev+"/analyze", map[string]any{"idempotencyKey": "fc-new"})
	refused("new job", c, out)
	c, out, _, _ = h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+l3ID+"/publish", map[string]any{"lessonHash": l3.LessonHash})
	refused("publish", c, out)
	c, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": p1, "title": "x", "answerPolicy": "teacher_release"})
	refused("new assignment", c, out)
	c, out, _, _ = h.call("u_alice", "POST", "/api/edu/assignments/"+asID+"/migrate", map[string]any{"publicationId": p2, "reason": "x", "confirm": true})
	refused("migration", c, out)
	c, out, _, _ = h.call("u_bob", "GET", sa+asID, nil, stu...)
	refused("student read", c, out)
	c, out, _, _ = h.call("u_bob", "POST", sa+asID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "fc-attempt-01"}, stu...)
	refused("student attempt", c, out)
	if _, out, _, _ := h.call("u_bob", "GET", "/api/edu/student/assignments", nil, stu...); len(out["data"].([]any)) != 0 {
		t.Fatal("the student list must hide what cannot be checked")
	}
	if after := count(); after != before {
		t.Fatalf("refused calls must write nothing: %v -> %v", before, after)
	}

	// The lookup recovers: the same calls go through.
	failing.Store(false)
	if c, _, raw, _ := h.call("u_bob", "GET", sa+asID, nil, stu...); c != 200 {
		t.Fatalf("student read after recovery: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_bob", "POST", sa+asID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "fc-attempt-01"}, stu...); c != 201 {
		t.Fatalf("attempt after recovery: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+l3ID+"/publish", map[string]any{"lessonHash": l3.LessonHash}); c != 200 {
		t.Fatalf("publish after recovery: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/problem-revisions/"+rev+"/analyze", map[string]any{"idempotencyKey": "fc-new"}); c >= 300 {
		t.Fatalf("new job after recovery: %d %s", c, raw)
	}
}
