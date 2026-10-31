package edumath

import (
	"context"
	"encoding/json"
	"testing"

	"yingce/backend/internal/model"
)

func TestLessonPlan(t *testing.T) { runLessonPlan(t, newHarness(t, startMathService(t))) }

func TestPGLessonPlan(t *testing.T) {
	runLessonPlan(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

// runLessonPlan (DEV-070, E71): before a lesson build the teacher sees whether
// it would be admitted (blockers with a next action), the goal, style and 3 to
// 8 minute choices, per stage upper bounds and the queue, read only; a build
// with teaching params records them in the lesson meta (not in the student
// projection) while one without them stays byte identical to earlier builds;
// bad params are refused before anything is dispatched; reusing a key with
// other params is a conflict; the editable view shows a labelled length
// estimate; a job that did not succeed carries recovery actions.
func runLessonPlan(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	revID := h.confirmedRevision("2", "1")
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/problem-revisions/"+revID+"/analyze", map[string]any{"idempotencyKey": "plan-a"})
	sol := h.waitJob("u_alice", data(out)["id"].(string))["solutionRevisionId"].(string)
	planURL := "/api/edu/solution-revisions/" + sol + "/lesson-plan"
	buildURL := "/api/edu/solution-revisions/" + sol + "/lesson"

	var jobsBefore, auditsBefore int64
	h.db.Model(&model.EduMathJob{}).Count(&jobsBefore)
	h.db.Model(&model.EduAuditEvent{}).Count(&auditsBefore)
	c, out, raw, _ := h.call("u_alice", "GET", planURL, nil)
	if c != 200 || data(out)["admitted"] != true || len(data(out)["blockers"].([]any)) != 0 {
		t.Fatalf("plan: %d %s", c, raw)
	}
	est := data(out)["estimate"].(map[string]any)
	if len(est["stages"].([]any)) != 4 || est["maxComputeMs"].(float64) != 75000 {
		t.Fatalf("plan estimate: %s", raw)
	}
	tc := data(out)["teaching"].(map[string]any)
	if tc["minMinutes"].(float64) != 3 || tc["maxMinutes"].(float64) != 8 || len(tc["goals"].([]any)) != 3 || len(tc["styles"].([]any)) != 3 {
		t.Fatalf("plan teaching: %s", raw)
	}
	if _, ok := data(out)["queue"].(map[string]any)["queued"]; !ok {
		t.Fatalf("plan queue: %s", raw)
	}
	var jobsAfter, auditsAfter int64
	h.db.Model(&model.EduMathJob{}).Count(&jobsAfter)
	h.db.Model(&model.EduAuditEvent{}).Count(&auditsAfter)
	if jobsAfter != jobsBefore || auditsAfter != auditsBefore {
		t.Fatal("the plan must be read only")
	}

	// A generation pause shows up as a blocker with its next action; lifting clears it.
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/pauses", map[string]any{"scope": "generation", "reason": "检查", "confirm": true})
	if c != 201 && c != 200 {
		t.Fatalf("pause: %d %s", c, raw)
	}
	pauseID := data(out)["pause"].(map[string]any)["id"].(string)
	_, out, raw, _ = h.call("u_alice", "GET", planURL, nil)
	bl := data(out)["blockers"].([]any)
	if data(out)["admitted"] != false || len(bl) != 1 || bl[0].(map[string]any)["code"] != "GENERATION_PAUSED" || bl[0].(map[string]any)["action"] == "" {
		t.Fatalf("paused plan: %s", raw)
	}
	h.call("u_alice", "POST", "/api/edu/pauses/"+pauseID+"/lift", map[string]any{"note": "好了"})
	if _, out, raw, _ = h.call("u_alice", "GET", planURL, nil); data(out)["admitted"] != true {
		t.Fatalf("plan after lift: %s", raw)
	}

	// Bad params: refused, nothing dispatched.
	for _, bad := range []map[string]any{{"goal": "x"}, {"style": "funny"}, {"targetMinutes": 2}, {"targetMinutes": 9}, {"targetMinutes": "5"}} {
		bad["idempotencyKey"] = "plan-bad"
		if c, _, raw, _ := h.call("u_alice", "POST", buildURL, bad); c != 400 {
			t.Fatalf("bad teaching %v: %d %s", bad, c, raw)
		}
	}
	var n int64
	h.db.Model(&model.EduMathJob{}).Where("idem_key = ?", "plan-bad").Count(&n)
	if n != 0 {
		t.Fatal("refused params must not create a job")
	}

	lessonOf := func(key string, body map[string]any) (map[string]any, string) {
		t.Helper()
		body["idempotencyKey"] = key
		c, out, raw, _ := h.call("u_alice", "POST", buildURL, body)
		if c != 202 {
			t.Fatalf("build %s: %d %s", key, c, raw)
		}
		lid := h.waitJob("u_alice", data(out)["id"].(string))["lessonRevisionId"].(string)
		var l model.EduLessonRevision
		h.db.First(&l, "id = ?", lid)
		var ta model.EduArtifact
		h.db.First(&ta, "id = ?", l.LessonArtifact)
		b, _, err := h.svc.Client.Artifact(context.Background(), tenant, ta)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		_ = json.Unmarshal(b, &doc)
		return doc, lid
	}
	plain, _ := lessonOf("plan-l0", map[string]any{})
	if m := plain["meta"].(map[string]any); m["style"] != "standard" || m["goal"] != nil || m["target_minutes"] != nil {
		t.Fatalf("default build must keep the earlier meta: %v", m)
	}
	doc, lid := lessonOf("plan-l1", map[string]any{"goal": "fix_mistake", "style": "detailed", "targetMinutes": 6})
	m := doc["meta"].(map[string]any)
	if m["goal"] != "fix_mistake" || m["style"] != "detailed" || m["target_minutes"].(float64) != 6 {
		t.Fatalf("teaching meta: %v", m)
	}
	// DEV-070: style and goal change only the spoken wording; the steps, board
	// lines and what each step reveals stay the same.
	pbs, dbs := plain["beats"].([]any), doc["beats"].([]any)
	if len(pbs) != len(dbs) {
		t.Fatalf("teaching params must not change the step count: %d vs %d", len(pbs), len(dbs))
	}
	reworded := false
	for i := range pbs {
		pbm, dbm := pbs[i].(map[string]any), dbs[i].(map[string]any)
		for _, k := range []string{"id", "title", "board", "board_latex", "figure_focus", "reveals_answer", "solution_step_ref"} {
			x, _ := json.Marshal(pbm[k])
			y, _ := json.Marshal(dbm[k])
			if string(x) != string(y) {
				t.Fatalf("teaching params must not change beat %d %s: %s vs %s", i, k, x, y)
			}
		}
		reworded = reworded || pbm["say"] != dbm["say"]
	}
	if !reworded {
		t.Fatal("detailed style with fix_mistake goal must change the wording")
	}
	if c, _, raw, _ := h.call("u_alice", "POST", buildURL, map[string]any{"idempotencyKey": "plan-l1", "goal": "practice", "style": "detailed", "targetMinutes": 6}); c != 409 {
		t.Fatalf("key reused with other params: %d %s", c, raw)
	}
	if c, out, raw, _ := h.call("u_alice", "POST", buildURL, map[string]any{"idempotencyKey": "plan-l1", "goal": "fix_mistake", "style": "detailed", "targetMinutes": 6}); c != 202 || data(out)["status"] != "succeeded" {
		t.Fatalf("same key and params must answer the same job: %d %s", c, raw)
	}
	var l model.EduLessonRevision
	h.db.First(&l, "id = ?", lid)
	var sp model.EduArtifact
	h.db.First(&sp, "id = ?", l.StudentArtifact)
	sb, _, _ := h.svc.Client.Artifact(context.Background(), tenant, sp)
	var stu map[string]any
	_ = json.Unmarshal(sb, &stu)
	if sm := stu["meta"].(map[string]any); sm["goal"] != nil || sm["target_minutes"] != nil {
		t.Fatalf("student projection must not carry teaching params: %v", sm)
	}
	c, out, raw, _ = h.call("u_alice", "GET", "/api/edu/lesson-revisions/"+lid+"/editable", nil)
	d, _ := data(out)["duration"].(map[string]any)
	tv, _ := data(out)["teaching"].(map[string]any)
	if c != 200 || d == nil || d["basis"] != "narration_text_estimate" || d["estimatedSeconds"].(float64) <= 0 || d["targetMinutes"].(float64) != 6 || d["targetFit"] == nil || tv["goal"] != "fix_mistake" {
		t.Fatalf("editable duration: %d %s", c, raw)
	}

	// Recovery actions on jobs that did not succeed.
	for _, cse := range []struct {
		status, code, want string
	}{{"reconciliation_required", "", "reconcile"}, {"failed", "TIMEOUT_UNDETERMINED", "restart_new_key"}, {"failed", "BUDGET_EXCEEDED", "raise_budget"},
		{"failed", "VALIDATION_FAILED", "edit_problem"}, {"failed", "RECEIPT_INVALID", "contact_admin"}, {"cancelled", "", "restart_new_key"}} {
		got := recoveryActions(model.EduMathJob{Status: cse.status, ErrorCode: cse.code})
		if len(got) == 0 || got[0] != cse.want {
			t.Fatalf("recovery %s/%s: %v", cse.status, cse.code, got)
		}
	}
	if len(recoveryActions(model.EduMathJob{Status: "succeeded"})) != 0 {
		t.Fatal("a succeeded job has no recovery action")
	}

	// Students cannot see the plan.
	if c, _, _, _ := h.call("u_bob", "GET", planURL, nil, "X-Edu-Tenant", tenant); c != 403 && c != 404 {
		t.Fatalf("student plan: %d", c)
	}
}

func TestLessonDurationEstimate(t *testing.T) {
	long := ""
	for i := 0; i < 400; i++ {
		long += "讲"
	}
	d := lessonDuration([]string{long, long}, 4, "standard", 4)
	if d["fit"] != "within_guidance" || d["targetFit"] != "close" {
		t.Fatalf("standard: %v", d)
	}
	if lessonDuration([]string{"短"}, 0, "concise", 0)["fit"] != "shorter_than_guidance" {
		t.Fatal("short lesson")
	}
	if _, ok := lessonDuration([]string{"短"}, 0, "", 0)["targetMinutes"]; ok {
		t.Fatal("no target, no targetMinutes")
	}
	if lessonDuration([]string{long, long, long, long, long}, 0, "detailed", 0)["fit"] != "longer_than_guidance" {
		t.Fatal("long lesson")
	}
}
