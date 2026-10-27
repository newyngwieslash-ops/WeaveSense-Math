package edumath

import (
	"encoding/json"
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

// TestReviewChecklist (DEV-076, E45): the review checklist must cover every item when sent; an
// approval cannot carry an open issue or findings; a rejection must give a reason; findings must
// point at a beat or hint that exists in this exact revision and are returned to the author with
// the lesson. Runs on the real math service.
func TestReviewChecklist(t *testing.T) { runReviewChecklist(t, newHarness(t, startMathService(t))) }

func TestPGReviewChecklist(t *testing.T) {
	runReviewChecklist(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func runReviewChecklist(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	h.db.Save(&model.EduOrgPolicy{TenantID: tenant, ReviewMode: "self_allowed"})
	revID := h.confirmedRevision("2", "1")
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/problem-revisions/"+revID+"/analyze", map[string]any{"idempotencyKey": "rc-a"})
	job := h.waitJob("u_alice", data(out)["id"].(string))
	if job["status"] != "succeeded" {
		b, _ := json.Marshal(job)
		t.Fatalf("analyze did not succeed: %s", b)
	}
	lesson := func(key string) model.EduLessonRevision {
		_, out, _, _ := h.call("u_alice", "POST", "/api/edu/solution-revisions/"+job["solutionRevisionId"].(string)+"/lesson", map[string]any{"idempotencyKey": key})
		lj := h.waitJob("u_alice", data(out)["id"].(string))
		var l model.EduLessonRevision
		h.db.First(&l, "id = ?", lj["lessonRevisionId"])
		if l.State != "ready_for_review" {
			b, _ := json.Marshal(lj)
			t.Fatalf("lesson %s state %s: %s", l.ID, l.State, b)
		}
		return l
	}
	l := lesson("rc-l1")
	lr := "/api/edu/lesson-revisions/" + l.ID
	_, out, _, _ = h.call("u_alice", "GET", lr+"/editable", nil)
	b0 := data(out)["beats"].([]any)[0].(map[string]any)["id"].(string)

	all := func(v string) map[string]string {
		m := map[string]string{}
		for _, k := range reviewItems {
			m[k] = v
		}
		return m
	}
	rev := func(body map[string]any) (int, map[string]any, []byte) {
		body["lessonHash"] = l.LessonHash
		c, out, raw, _ := h.call("u_alice", "POST", lr+"/review", body)
		return c, out, raw
	}
	partial := all("pass")
	delete(partial, "hints")
	bad := all("pass")
	bad["method"] = "ok"
	extra := all("pass")
	extra["vibes"] = "pass"
	for name, body := range map[string]map[string]any{
		"partial checklist":   {"decision": "approved", "checklist": partial},
		"bad value":           {"decision": "approved", "checklist": bad},
		"unknown item":        {"decision": "approved", "checklist": extra},
		"reject with nothing": {"decision": "rejected"},
		"unknown target":      {"decision": "rejected", "findings": []map[string]any{{"target": "slide:1", "item": "method", "note": "x"}}},
		"empty note":          {"decision": "rejected", "findings": []map[string]any{{"target": "problem", "item": "problem", "note": "  "}}},
		"missing beat":        {"decision": "rejected", "findings": []map[string]any{{"target": "beat:no_such_beat", "item": "method", "note": "这一步不存在"}}},
		"missing hint":        {"decision": "rejected", "findings": []map[string]any{{"target": "hint:9", "item": "hints", "note": "提示不存在"}}},
	} {
		if c, _, raw := rev(body); c != 400 {
			t.Errorf("%s: want 400, got %d %s", name, c, raw)
		}
	}
	open := all("pass")
	open["visuals"] = "issue"
	if c, out, raw := rev(map[string]any{"decision": "approved", "checklist": open}); c != 409 || errCode(out) != "CHECKLIST_OPEN" {
		t.Fatalf("approve with open issue: %d %s", c, raw)
	}
	if c, out, raw := rev(map[string]any{"decision": "approved", "checklist": all("pass"), "findings": []map[string]any{{"target": "problem", "item": "problem", "note": "小问题"}}}); c != 409 || errCode(out) != "CHECKLIST_OPEN" {
		t.Fatalf("approve with findings: %d %s", c, raw)
	}
	var still model.EduLessonRevision
	h.db.First(&still, "id = ?", l.ID)
	if still.State != "ready_for_review" {
		t.Fatalf("refused reviews changed the state to %s", still.State)
	}
	findings := []map[string]any{
		{"target": "beat:" + b0, "item": "visuals", "note": "图中标注与板书数值不一致"},
		{"target": "hint:1", "item": "hints", "note": "提示一过于直接"},
		{"target": "answer", "item": "answer_policy", "note": "建议截止后公布"},
	}
	if c, _, raw := rev(map[string]any{"decision": "rejected", "checklist": open, "findings": findings, "comment": "请按定位修改"}); c != 200 {
		t.Fatalf("located rejection: %d %s", c, raw)
	}
	c, out, raw, _ := h.call("u_alice", "GET", lr, nil)
	if c != 200 {
		t.Fatalf("get lesson: %d %s", c, raw)
	}
	reviews := data(out)["reviews"].([]any)
	r0 := reviews[len(reviews)-1].(map[string]any)
	var got []ReviewFinding
	if json.Unmarshal([]byte(r0["findings"].(string)), &got) != nil || len(got) != 3 || got[0].Target != "beat:"+b0 || r0["decision"] != "rejected" {
		t.Fatalf("findings not returned to the author: %v", r0)
	}
	if !strings.Contains(r0["checklist"].(string), `"visuals":"issue"`) {
		t.Fatalf("checklist not stored: %v", r0["checklist"])
	}
	// A rejected revision cannot be reviewed again (the author fixes it in a new revision).
	if c, _, raw := rev(map[string]any{"decision": "approved", "checklist": all("pass")}); c != 409 {
		t.Fatalf("re-review of a rejected revision: %d %s", c, raw)
	}
	// A clean checklist approves; "na" is allowed for items that do not apply.
	l2 := lesson("rc-l2")
	ok := all("pass")
	ok["self_test"] = "na"
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+l2.ID+"/review", map[string]any{"lessonHash": l2.LessonHash, "decision": "approved", "checklist": ok}); c != 200 {
		t.Fatalf("approve with clean checklist: %d %s", c, raw)
	}
	var rec model.EduReviewRecord
	h.db.Where("lesson_revision_id = ?", l2.ID).First(&rec)
	if rec.LessonHash != l2.LessonHash || !strings.Contains(rec.Checklist, `"self_test":"na"`) || rec.Findings != "" {
		t.Fatalf("approval record: %+v", rec)
	}
	// Without a checklist the old behaviour holds (comment-only rejection and approval).
	l3 := lesson("rc-l3")
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+l3.ID+"/review", map[string]any{"lessonHash": l3.LessonHash, "decision": "rejected", "comment": "重做"}); c != 200 {
		t.Fatalf("comment-only rejection: %d %s", c, raw)
	}
}
