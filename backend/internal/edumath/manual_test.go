package edumath

import (
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

// TestManualLesson covers DEV-039 (FR-M05): a problem outside the implemented
// families gets UNSUPPORTED_FAMILY with next action manual_authoring; the
// teacher writes the lesson; it is reviewed and published as a human product
// that is marked manual_unverified everywhere, never graded automatically, has
// no self-test and yields no variants or wording edit jobs.
func TestManualLesson(t *testing.T) { runManualLesson(t, newHarness(t, startMathService(t))) }

func TestPGManualLesson(t *testing.T) {
	runManualLesson(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func manualBody(key string) map[string]any {
	return map[string]any{"idempotencyKey": key, "lesson": map[string]any{
		"title": "数列求和（人工）", "problem_text": "已知数列 a(n) = 2n − 1，求前 10 项和。",
		"beats": []any{
			map[string]any{"title": "观察", "lines": []any{"这是首项为 1、公差为 2 的等差数列。"}},
			map[string]any{"title": "求和", "lines": []any{"用首末项求和公式。", "所以前 10 项和为 一百整。"}, "reveals_answer": true},
		},
		"answer": map[string]any{"plain": "一百整"}, "hints": []any{"先判断数列类型。"}}}
}

func runManualLesson(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	stu := []string{"X-Edu-Tenant", tenant}
	code, out, raw, _ := h.call("u_alice", "POST", "/api/edu/problems", map[string]any{"familyId": "SEQ-SUM-99", "title": "数列求和",
		"structured": map[string]any{"family_id": "SEQ-SUM-99", "input_state": "sufficient", "unit": "", "params": map[string]any{"n": "10"}}})
	if code != 201 {
		t.Fatalf("create problem: %d %s", code, raw)
	}
	rev := data(out)["revision"].(map[string]any)
	revID := rev["id"].(string)
	base := "/api/edu/problem-revisions/" + revID
	if code, out, _, _ := h.call("u_alice", "POST", base+"/manual-lesson", manualBody("m-early")); code != 409 {
		t.Fatalf("manual lesson before confirmation must be 409: %d %v", code, out)
	}
	h.call("u_alice", "POST", base+"/confirm", map[string]any{"inputHash": rev["inputHash"]})

	// The automatic path refuses the family and points at manual authoring.
	_, out, _, _ = h.call("u_alice", "POST", base+"/analyze", map[string]any{"idempotencyKey": "m-an"})
	aj := h.waitJob("u_alice", data(out)["id"].(string))
	ae, _ := aj["error"].(map[string]any)
	if aj["status"] != "failed" || ae["code"] != "UNSUPPORTED_FAMILY" || ae["next_action"] != "manual_authoring" {
		t.Fatalf("analysis of an unknown family must fail UNSUPPORTED_FAMILY → manual_authoring: %v", aj)
	}

	// Shape is checked synchronously; markup and missing answer beat are refused.
	bad := manualBody("m-bad")
	bad["lesson"].(map[string]any)["problem_text"] = "<b>x</b>"
	if code, out, _, _ := h.call("u_alice", "POST", base+"/manual-lesson", bad); code != 400 || errCode(out) != "INPUT_AMBIGUOUS" {
		t.Fatalf("markup must be 400 INPUT_AMBIGUOUS: %d %v", code, out)
	}
	bad = manualBody("m-bad2")
	bad["lesson"].(map[string]any)["extra"] = "x"
	if code, _, _, _ := h.call("u_alice", "POST", base+"/manual-lesson", bad); code != 400 {
		t.Fatalf("unknown fields must be 400, got %d", code)
	}
	if code, _, _, _ := h.call("u_bob", "POST", base+"/manual-lesson", manualBody("m-bob"), stu...); code != 403 && code != 404 {
		t.Fatalf("a non-teacher must not start a manual lesson, got %d", code)
	}
	if code, _, _, _ := h.call("u_carol", "POST", base+"/manual-lesson", manualBody("m-carol")); code != 403 && code != 404 {
		t.Fatalf("another tenant must not reach the revision, got %d", code)
	}

	code, out, raw, _ = h.call("u_alice", "POST", base+"/manual-lesson", manualBody("m-1"))
	if code != 202 || data(out)["kind"] != "manual_lesson" {
		t.Fatalf("manual lesson: %d %s", code, raw)
	}
	jobID := data(out)["id"].(string)
	// Same key, same content: same job; same key, other content: conflict.
	if _, again, _, _ := h.call("u_alice", "POST", base+"/manual-lesson", manualBody("m-1")); data(again)["id"] != jobID {
		t.Fatalf("idempotent replay must return the same job: %v", again)
	}
	other := manualBody("m-1")
	other["lesson"].(map[string]any)["title"] = "另一个标题"
	if code, out, _, _ := h.call("u_alice", "POST", base+"/manual-lesson", other); code != 409 || errCode(out) != "REVISION_CONFLICT" {
		t.Fatalf("key reuse with other content must be 409: %d %v", code, out)
	}
	lj := h.waitJob("u_alice", jobID)
	if lj["status"] != "succeeded" {
		t.Fatalf("manual job: %v", lj)
	}
	lessonID, _ := lj["lessonRevisionId"].(string)
	var l model.EduLessonRevision
	if h.db.First(&l, "id = ?", lessonID).Error != nil || l.State != "ready_for_review" || l.BlockingErrors != "" {
		t.Fatalf("manual lesson revision: %+v", l)
	}
	var sol model.EduSolutionRevision
	h.db.First(&sol, "id = ?", l.SolutionRevisionID)
	if sol.MathState != ManualUnverified || sol.SolutionStatus != "manual" || sol.EvidenceLevels != "[]" {
		t.Fatalf("the solution record must say manual_unverified with no evidence: %+v", sol)
	}
	var in model.EduManualInput
	if h.db.First(&in, "job_id = ?", jobID).Error != nil || !strings.Contains(in.Content, "一百整") {
		t.Fatalf("manual input must be kept for safe resubmission: %+v", in)
	}

	_, out, raw, _ = h.call("u_alice", "GET", "/api/edu/lesson-revisions/"+lessonID, nil)
	q, _ := data(out)["quality"].(map[string]any)
	qm, _ := q["math"].(map[string]any)
	if qm["state"] != ManualUnverified || qm["source"] != "teacher_manual" || len(qm["evidenceLevels"].([]any)) != 0 || qm["domainProof"] != false {
		t.Fatalf("quality panel must show manual_unverified from the teacher: %s", raw)
	}

	// Variants and wording edit jobs are refused before anything is dispatched.
	if code, out, _, _ := h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+lessonID+"/variant-sets", map[string]any{"count": 2, "idempotencyKey": "m-v"}); code != 409 || errCode(out) != "MANUAL_UNVERIFIED" {
		t.Fatalf("variants from a manual lesson must be 409 MANUAL_UNVERIFIED: %d %v", code, out)
	}
	if code, out, _, _ := h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+lessonID+"/edits", map[string]any{"baseLessonHash": l.LessonHash, "idempotencyKey": "m-e",
		"edits": []any{map[string]any{"target": "beat", "id": "m1", "field": "say", "value": "x"}}}); code != 409 || errCode(out) != "MANUAL_UNVERIFIED" {
		t.Fatalf("edit jobs on a manual lesson must be 409 MANUAL_UNVERIFIED: %d %v", code, out)
	}

	// Human review and publication work as for any lesson; the student view says it is not verified.
	h.db.Save(&model.EduOrgPolicy{TenantID: tenant, ReviewMode: "self_allowed"})
	if code, _, raw, _ := h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+lessonID+"/review", map[string]any{"lessonHash": l.LessonHash, "decision": "approved", "comment": "人工审校"}); code != 200 {
		t.Fatalf("review: %d %s", code, raw)
	}
	code, out, raw, _ = h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+lessonID+"/publish", map[string]any{"lessonHash": l.LessonHash})
	if code != 200 {
		t.Fatalf("publish: %d %s", code, raw)
	}
	pubID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "人工课件班"})
	classID := data(out)["id"].(string)
	code, out, raw, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "数列求和", "answerPolicy": "teacher_release"})
	if code != 201 {
		t.Fatalf("assignment: %d %s", code, raw)
	}
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-manual-1"})

	p := "/api/edu/student/assignments/" + asID
	code, _, raw, _ = h.call("u_bob", "GET", p, nil, stu...)
	if code != 200 || strings.Contains(string(raw), "一百整") || !strings.Contains(string(raw), ManualUnverified) {
		t.Fatalf("student assignment must carry the manual marker and no answer: %d %s", code, raw)
	}
	code, out, raw, _ = h.call("u_bob", "POST", p+"/attempts", map[string]any{"kind": "answer", "response": "100", "clientKey": "manual-0001"}, stu...)
	at, _ := data(out)["attempt"].(map[string]any)
	if code != 201 || at["result"] != "undetermined" || at["reason"] != "manual_lesson_teacher_judges" || at["grader"] != nil {
		t.Fatalf("a manual lesson attempt must wait for the teacher, never be auto graded: %d %s", code, raw)
	}
	if strings.Contains(string(raw), "一百整") {
		t.Fatalf("attempt reply leaked the answer: %s", raw)
	}
	if code, out, _, _ := h.call("u_bob", "POST", p+"/selftest/start", map[string]any{}, stu...); code != 409 || errCode(out) != "MANUAL_UNVERIFIED" {
		t.Fatalf("no self-test on a manual lesson: %d %v", code, out)
	}
	if code, out, raw, _ := h.call("u_alice", "POST", "/api/edu/attempts/"+at["id"].(string)+"/judge", map[string]any{"result": "correct", "note": "人工判定"}); code != 200 {
		t.Fatalf("teacher judge: %d %v %s", code, out, raw)
	}
}
