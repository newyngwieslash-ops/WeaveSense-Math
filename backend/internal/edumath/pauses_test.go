package edumath

import (
	"testing"

	"yingce/backend/internal/model"
)

func TestPauses(t *testing.T) { runPauses(t, newHarness(t, startMathService(t))) }

func TestPGPauses(t *testing.T) {
	runPauses(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

// runPauses (DEV-125, E66): an org admin pauses new generation, one family, one
// published version or one renderer version, each with an impact preview and an
// audited, idempotent confirm; a pause stops new jobs (existing idempotency keys
// still answer), publishing, new assignments, migration targets and every
// student read at once, without deleting attempts, jobs, ledger or audit; lifting
// restores exactly that access, never a publication withdrawn meanwhile; the
// platform wide EDU_PAUSE_GENERATION stops generation in every tenant; a class
// rolls back to an earlier valid version by migration while the newer one is
// paused; students and teachers who are not admins cannot pause.
func runPauses(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	p1, _ := publishG1(t, h)
	// A second, newer version of the same lesson (second build, reviewed and published).
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/problem-revisions/"+firstRevision(h, p1)+"/analyze", map[string]any{"idempotencyKey": "learn-a"})
	sol := h.waitJob("u_alice", data(out)["id"].(string))["solutionRevisionId"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/solution-revisions/"+sol+"/lesson", map[string]any{"idempotencyKey": "pause-l2"})
	l2ID := h.waitJob("u_alice", data(out)["id"].(string))["lessonRevisionId"].(string)
	var l2 model.EduLessonRevision
	h.db.First(&l2, "id = ?", l2ID)
	h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+l2ID+"/review", map[string]any{"lessonHash": l2.LessonHash, "decision": "approved", "comment": "自审"})
	c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+l2ID+"/publish", map[string]any{"lessonHash": l2.LessonHash})
	if c != 200 {
		t.Fatalf("publish v2: %d %s", c, raw)
	}
	p2 := data(out)["id"].(string)
	renderer := rendererOf(l2)
	if renderer == "" {
		t.Fatal("lesson must carry its renderer version")
	}

	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高一(5)班"})
	classID := data(out)["id"].(string)
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": p2, "title": "停用", "answerPolicy": "teacher_release"})
	if c != 201 {
		t.Fatalf("assignment: %d %s", c, raw)
	}
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-pause-bob"})
	stu := []string{"X-Edu-Tenant", tenant}
	sa := "/api/edu/student/assignments/"
	if c, _, raw, _ := h.call("u_bob", "POST", sa+asID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "pause-attempt-01"}, stu...); c != 201 {
		t.Fatalf("attempt: %d %s", c, raw)
	}
	counts := func() [3]int64 {
		var r [3]int64
		h.db.Model(&model.EduAttempt{}).Where("tenant_id = ?", tenant).Count(&r[0])
		h.db.Model(&model.EduMathJob{}).Where("tenant_id = ?", tenant).Count(&r[1])
		h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action NOT LIKE ?", tenant, "pause.%").Count(&r[2])
		return r
	}
	studentSees := func(as string) int {
		c, out, _, _ := h.call("u_bob", "GET", sa+as, nil, stu...)
		if c == 423 && errCode(out) != "CONTENT_PAUSED" {
			t.Fatalf("paused content must say so, got %v", out)
		}
		return c
	}
	listed := func() int {
		_, out, _, _ := h.call("u_bob", "GET", "/api/edu/student/assignments", nil, stu...)
		return len(out["data"].([]any))
	}
	pause := func(scope, target string, confirm bool) (int, map[string]any) {
		t.Helper()
		c, out, _, _ := h.call("u_alice", "POST", "/api/edu/pauses", map[string]any{"scope": scope, "targetId": target, "reason": "渲染版本问题排查", "confirm": confirm})
		return c, out
	}
	lift := func(id string) {
		t.Helper()
		if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/pauses/"+id+"/lift", map[string]any{"note": "已修复"}); c != 200 {
			t.Fatalf("lift: %d %s", c, raw)
		}
	}
	if studentSees(asID) != 200 || listed() != 1 {
		t.Fatal("student must see the assignment before any pause")
	}

	// Who and what.
	for _, u := range []string{"u_bob", "u_carol"} {
		if c, _, _, _ := h.call(u, "POST", "/api/edu/pauses", map[string]any{"scope": "generation", "reason": "x", "confirm": true}, stu...); c != 403 && c != 404 {
			t.Fatalf("%s must not pause, got %d", u, c)
		}
	}
	for _, bad := range []map[string]any{{"scope": "tenant", "reason": "x"}, {"scope": "generation", "reason": " "}, {"scope": "generation", "targetId": "x", "reason": "x"}, {"scope": "family", "targetId": "a b", "reason": "x"}} {
		if c, _, _, _ := h.call("u_alice", "POST", "/api/edu/pauses", bad); c != 400 {
			t.Fatalf("bad pause %v accepted: %d", bad, c)
		}
	}
	for scope, target := range map[string]string{"publication": "epu_none", "family": "NO-SUCH-FAMILY", "renderer": "nope/9"} {
		if c, _ := pause(scope, target, false); c != 404 {
			t.Fatalf("unknown %s target must be 404, got %d", scope, c)
		}
	}

	// Publication pause: preview, confirm, idempotent, blocks, keeps data, lifts.
	before := counts()
	c, out = pause("publication", p2, false)
	imp := data(out)["impact"].(map[string]any)
	if c != 200 || data(out)["dryRun"] != true || imp["assignments"].(float64) != 1 || imp["students"].(float64) != 1 {
		t.Fatalf("preview: %d %v", c, out)
	}
	if studentSees(asID) != 200 {
		t.Fatal("a preview must not pause")
	}
	c, out = pause("publication", p2, true)
	if c != 201 {
		t.Fatalf("pause: %d %v", c, out)
	}
	pubPause := data(out)["pause"].(map[string]any)["id"].(string)
	if c, out = pause("publication", p2, true); c != 200 || data(out)["created"] != false {
		t.Fatalf("pause must be idempotent: %d %v", c, out)
	}
	if studentSees(asID) != 423 || listed() != 0 {
		t.Fatal("a paused publication must stop student access at once")
	}
	if c, _, _, _ := h.call("u_bob", "POST", sa+asID+"/attempts", map[string]any{"kind": "answer", "response": "2", "clientKey": "pause-attempt-02"}, stu...); c != 423 {
		t.Fatalf("attempt on paused content: %d", c)
	}
	if c, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": p2, "title": "x", "answerPolicy": "teacher_release"}); c != 423 || errCode(out) != "CONTENT_PAUSED" {
		t.Fatalf("new assignment of a paused version: %d", c)
	}
	// Roll the class back to the earlier valid version while v2 is paused.
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/assignments/"+asID+"/migrate", map[string]any{"publicationId": p1, "reason": "回退到上一有效版本", "confirm": true})
	if c != 201 {
		t.Fatalf("rollback migration: %d %s", c, raw)
	}
	asOld := data(out)["toAssignmentId"]
	if asOld == nil {
		var m model.EduAssignmentMigration
		h.db.First(&m, "from_assignment_id = ?", asID)
		asOld = m.ToAssignmentID
	}
	if studentSees(asOld.(string)) != 200 {
		t.Fatal("the rolled back version must be available")
	}
	if c, _, _, _ := h.call("u_alice", "POST", "/api/edu/assignments/"+asOld.(string)+"/migrate", map[string]any{"publicationId": p2, "reason": "x", "confirm": true}); c != 423 {
		t.Fatalf("a paused version must not be a migration target, got %d", c)
	}
	if after := counts(); after[0] != before[0] || after[1] != before[1] || after[2] < before[2] {
		t.Fatalf("a pause must not delete attempts, jobs or audit: %v -> %v", before, after)
	}
	lift(pubPause)
	if c, out, _, _ := h.call("u_alice", "POST", "/api/edu/pauses/"+pubPause+"/lift", map[string]any{"note": "again"}); c != 200 || data(out)["lifted"] != false {
		t.Fatal("lifting twice is a no-op")
	}
	if c, _, _, _ := h.call("u_alice", "POST", "/api/edu/pauses/"+pubPause+"/lift", map[string]any{}); c != 400 {
		t.Fatal("a lift needs a note")
	}

	// Family pause: student access, new generation and publishing stop.
	c, out = pause("family", "SOLID-ANGLE-02", true)
	if c != 201 {
		t.Fatalf("family pause: %d %v", c, out)
	}
	famPause := data(out)["pause"].(map[string]any)["id"].(string)
	if studentSees(asOld.(string)) != 423 {
		t.Fatal("family pause must stop student access")
	}
	rev := h.confirmedRevision("3", "2")
	if c, out, _, _ := h.call("u_alice", "POST", "/api/edu/problem-revisions/"+rev+"/analyze", map[string]any{"idempotencyKey": "pause-new"}); c != 423 || errCode(out) != "GENERATION_PAUSED" {
		t.Fatalf("new job in a paused family: %d %v", c, out)
	}
	if c, _, _, _ := h.call("u_alice", "POST", "/api/edu/problem-revisions/"+firstRevision(h, p1)+"/analyze", map[string]any{"idempotencyKey": "learn-a"}); c >= 300 {
		t.Fatalf("an existing idempotency key must still answer, got %d", c)
	}
	lift(famPause)
	if studentSees(asOld.(string)) != 200 {
		t.Fatal("lifting the family pause restores access")
	}

	// Renderer pause covers every lesson rendered by that version.
	c, out = pause("renderer", renderer, true)
	if c != 201 || data(out)["impact"].(map[string]any)["publications"].(float64) != 2 {
		t.Fatalf("renderer pause: %d %v", c, out)
	}
	if studentSees(asOld.(string)) != 423 {
		t.Fatal("renderer pause must stop student access")
	}
	lift(data(out)["pause"].(map[string]any)["id"].(string))

	// Generation pause, tenant and platform wide.
	c, out = pause("generation", "", true)
	if c != 201 {
		t.Fatalf("generation pause: %d %v", c, out)
	}
	if c, _, _, _ := h.call("u_alice", "POST", "/api/edu/problem-revisions/"+rev+"/analyze", map[string]any{"idempotencyKey": "pause-new"}); c != 423 {
		t.Fatalf("generation pause must stop new jobs, got %d", c)
	}
	if studentSees(asOld.(string)) != 200 {
		t.Fatal("a generation pause leaves student access alone")
	}
	lift(data(out)["pause"].(map[string]any)["id"].(string))
	t.Setenv("EDU_PAUSE_GENERATION", "1")
	if c, _, _, _ := h.call("u_alice", "POST", "/api/edu/problem-revisions/"+rev+"/analyze", map[string]any{"idempotencyKey": "pause-new"}); c != 423 {
		t.Fatalf("platform pause must stop new jobs, got %d", c)
	}
	if _, out, _, _ := h.call("u_alice", "GET", "/api/edu/pauses?state=lifted", nil); data(out)["platformGenerationPaused"] != true || len(data(out)["pauses"].([]any)) != 4 {
		t.Fatalf("list: %v", out)
	}
	t.Setenv("EDU_PAUSE_GENERATION", "")
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/problem-revisions/"+rev+"/analyze", map[string]any{"idempotencyKey": "pause-new"}); c >= 300 {
		t.Fatalf("after lifting, generation works: %d %s", c, raw)
	}

	// Lifting never restores a version withdrawn while paused.
	c, out = pause("publication", p1, true)
	p1Pause := data(out)["pause"].(map[string]any)["id"].(string)
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/publications/"+p1+"/incidents", map[string]any{"severity": "blocking", "reason": "答案错误", "confirm": true}); c != 201 {
		t.Fatalf("withdraw: %d %s", c, raw)
	}
	lift(p1Pause)
	if c := studentSees(asOld.(string)); c == 200 {
		t.Fatal("a withdrawn version must stay unavailable after a pause is lifted")
	}
	if c, _ := pause("publication", p1, false); c != 409 {
		t.Fatalf("a withdrawn version cannot be paused, got %d", c)
	}
	var audits int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action LIKE ?", tenant, "pause.%").Count(&audits)
	if audits != 10 {
		t.Fatalf("pause and lift are audited, got %d", audits)
	}
}

func firstRevision(h *harness, pubID string) string {
	var p model.EduPublication
	h.db.First(&p, "id = ?", pubID)
	var l model.EduLessonRevision
	h.db.First(&l, "id = ?", p.LessonRevisionID)
	return l.ProblemRevisionID
}
