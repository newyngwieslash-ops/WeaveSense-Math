package edumath

import (
	"testing"

	"yingce/backend/internal/model"
)

func TestIncidentSOP(t *testing.T) { runIncidentSOP(t, newHarness(t, startMathService(t))) }

func TestPGIncidentSOP(t *testing.T) {
	runIncidentSOP(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

// runIncidentSOP (DEV-126): the whole content incident procedure on one lesson,
// locate (impact preview), stop (withdraw, pause), revise (edit with a fresh
// review, publish refused while paused), notify (one revision notice per task
// and current teacher, closed by acknowledge or migration), and review (one
// written review per resolved incident, no student names or contact details).
func runIncidentSOP(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	stu := []string{"X-Edu-Tenant", tenant}
	var pub model.EduPublication
	h.db.First(&pub, "id = ?", pubID)
	l0 := pub.LessonRevisionID
	lr := "/api/edu/lesson-revisions/"

	classes, tasks := []string{}, []string{}
	for i, name := range []string{"高二(1)班", "高二(2)班"} {
		_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": name})
		k := data(out)["id"].(string)
		c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/classes/"+k+"/assignments", map[string]any{"publicationId": pubID, "title": "线面角" + name, "answerPolicy": "after_submission"})
		if c != 201 {
			t.Fatalf("assignment %d: %d %s", i, c, raw)
		}
		classes, tasks = append(classes, k), append(tasks, data(out)["id"].(string))
	}
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes/"+classes[0]+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-sop-1"})
	if c, _, raw, _ := h.call("u_bob", "GET", "/api/edu/student/assignments/"+tasks[0], nil, stu...); c != 200 {
		t.Fatalf("student view: %d %s", c, raw)
	}

	// Locate: the preview changes nothing; confirm withdraws and suspends.
	inc := "/api/edu/publications/" + pubID + "/incidents"
	c, out, raw, _ := h.call("u_alice", "POST", inc, map[string]any{"severity": "blocking", "reason": "答案错误：线面角应重新计算"})
	if c != 200 || data(out)["dryRun"] != true || len(data(out)["impact"].(map[string]any)["assignments"].([]any)) != 2 {
		t.Fatalf("impact preview: %d %s", c, raw)
	}
	c, out, raw, _ = h.call("u_alice", "POST", inc, map[string]any{"severity": "blocking", "reason": "答案错误：线面角应重新计算", "confirm": true})
	if c != 201 {
		t.Fatalf("blocking: %d %s", c, raw)
	}
	incID := data(out)["incident"].(map[string]any)["id"].(string)
	ir := "/api/edu/incidents/" + incID
	review := map[string]any{"rootCause": "求解器对线面角的投影取错了底面", "detection": "教师课堂反馈", "correctiveActions": "修正投影并补充回归题", "followUps": "同族题复查"}
	if c, o, _, _ := h.call("u_alice", "POST", ir+"/review", review); c != 409 || errCode(o) != "INCIDENT_OPEN" {
		t.Fatalf("review before resolve must be 409 INCIDENT_OPEN: %d %v", c, o)
	}

	// Revise: an edit with a fresh review; publishing waits while the family is paused.
	var l0r model.EduLessonRevision
	h.db.First(&l0r, "id = ?", l0)
	_, out, _, _ = h.call("u_alice", "GET", lr+l0+"/editable", nil)
	b0 := data(out)["beats"].([]any)[0].(map[string]any)["id"].(string)
	c, out, raw, _ = h.call("u_alice", "POST", lr+l0+"/edits", map[string]any{"baseLessonHash": l0r.LessonHash, "idempotencyKey": "sop-fix-1", "saveAs": "branch",
		"edits": []map[string]any{{"target": "beat", "id": b0, "field": "say", "value": "重新计算线面角。"}}})
	if c != 202 {
		t.Fatalf("edit: %d %s", c, raw)
	}
	done := h.waitJob("u_alice", data(out)["job"].(map[string]any)["id"].(string))
	var f1 model.EduLessonRevision
	h.db.First(&f1, "id = ?", done["lessonRevisionId"])
	if c, _, raw, _ := h.call("u_alice", "POST", lr+f1.ID+"/review", map[string]any{"lessonHash": f1.LessonHash, "decision": "approved", "comment": "重审修正"}); c != 200 {
		t.Fatalf("review fix: %d %s", c, raw)
	}
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/pauses", map[string]any{"scope": "family", "targetId": "SOLID-ANGLE-02", "reason": "事故排查", "confirm": true})
	if c != 201 {
		t.Fatalf("family pause: %d %s", c, raw)
	}
	pauseID := data(out)["pause"].(map[string]any)["id"].(string)
	if c, o, _, _ := h.call("u_alice", "POST", lr+f1.ID+"/publish", map[string]any{"lessonHash": f1.LessonHash}); c != 423 || errCode(o) != "CONTENT_PAUSED" {
		t.Fatalf("publishing a paused family must be 423: %d %v", c, o)
	}
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/pauses/"+pauseID+"/lift", map[string]any{"note": "修订已审"}); c != 200 {
		t.Fatalf("lift: %d %s", c, raw)
	}
	c, out, raw, _ = h.call("u_alice", "POST", lr+f1.ID+"/publish", map[string]any{"lessonHash": f1.LessonHash})
	if c != 200 {
		t.Fatalf("publish fix: %d %s", c, raw)
	}
	fixPub := data(out)["id"].(string)

	// Notify: resolving creates one notice per task and current teacher.
	c, out, raw, _ = h.call("u_alice", "POST", ir+"/resolve", map[string]any{"note": "已重算并重审", "fixPublicationId": fixPub})
	if c != 200 {
		t.Fatalf("resolve: %d %s", c, raw)
	}
	sop := data(out)["sop"].(map[string]any)
	if sop["notices"].(map[string]any)["unread"].(float64) != 2 || sop["reviewPending"] != true || sop["review"] != nil {
		t.Fatalf("sop after resolve: %v", sop)
	}
	notices := func(user, state string) []any {
		t.Helper()
		c, o, raw, _ := h.call(user, "GET", "/api/edu/revision-notices?state="+state, nil)
		if c != 200 {
			t.Fatalf("notices %s: %d %s", user, c, raw)
		}
		return o["data"].([]any)
	}
	unread := notices("u_alice", "unread")
	if len(unread) != 2 {
		t.Fatalf("alice must have 2 unread notices: %v", unread)
	}
	for _, u := range []string{"u_bob", "u_carol"} {
		if c, o, _, _ := h.call(u, "GET", "/api/edu/revision-notices", nil); c == 200 && len(o["data"].([]any)) != 0 {
			t.Fatalf("%s must not see alice's notices: %v", u, o)
		}
	}
	if c, _, _, _ := h.call("u_alice", "GET", "/api/edu/revision-notices?state=bogus", nil); c != 400 {
		t.Fatalf("bad state must be 400, got %d", c)
	}
	if c, o, _, _ := h.call("u_alice", "GET", "/api/edu/home", nil); c != 200 || data(o)["revisionNotices"].(float64) != 2 {
		t.Fatalf("home must count unread notices: %d %v", c, o)
	}
	noticeFor := func(task string) string {
		for _, x := range notices("u_alice", "") {
			n := x.(map[string]any)["notice"].(map[string]any)
			if n["assignmentId"] == task {
				return n["id"].(string)
			}
		}
		t.Fatalf("no notice for %s", task)
		return ""
	}
	n0, n1 := noticeFor(tasks[0]), noticeFor(tasks[1])
	for _, u := range []string{"u_bob", "u_carol"} {
		if c, _, _, _ := h.call(u, "POST", "/api/edu/revision-notices/"+n1+"/ack", nil); c != 403 && c != 404 {
			t.Fatalf("%s must not acknowledge alice's notice, got %d", u, c)
		}
	}
	if c, o, raw, _ := h.call("u_alice", "POST", "/api/edu/revision-notices/"+n1+"/ack", nil); c != 200 || data(o)["changed"] != true {
		t.Fatalf("ack: %d %s", c, raw)
	}
	if c, o, _, _ := h.call("u_alice", "POST", "/api/edu/revision-notices/"+n1+"/ack", nil); c != 200 || data(o)["changed"] != false {
		t.Fatalf("second ack must be a no op: %d %v", c, o)
	}
	// Migrating the first task closes its notice.
	mg := "/api/edu/assignments/" + tasks[0] + "/migrate"
	if c, _, raw, _ := h.call("u_alice", "POST", mg, map[string]any{"publicationId": fixPub, "reason": "内容事故修正", "confirm": true}); c != 201 {
		t.Fatalf("migrate: %d %s", c, raw)
	}
	var rn model.EduRevisionNotice
	h.db.First(&rn, "id = ?", n0)
	if rn.State != "migrated" || rn.ClosedAt == nil {
		t.Fatalf("migration must close the notice: %+v", rn)
	}
	_, out, _, _ = h.call("u_alice", "GET", ir, nil)
	cnt := data(out)["sop"].(map[string]any)["notices"].(map[string]any)
	if cnt["unread"].(float64) != 0 || cnt["acknowledged"].(float64) != 1 || cnt["migrated"].(float64) != 1 {
		t.Fatalf("notice counts: %v", cnt)
	}
	if c, o, _, _ := h.call("u_alice", "GET", "/api/edu/home", nil); c != 200 || data(o)["revisionNotices"].(float64) != 0 {
		t.Fatalf("home after ack and migrate: %d %v", c, o)
	}

	// Review: listed as pending, validated, no personal data, written once.
	pending := func() int {
		_, o, _, _ := h.call("u_alice", "GET", "/api/edu/incidents?reviewPending=1", nil)
		return len(o["data"].([]any))
	}
	if pending() != 1 {
		t.Fatal("a resolved blocking incident without review must be pending")
	}
	if c, _, _, _ := h.call("u_carol", "POST", ir+"/review", review); c != 403 && c != 404 {
		t.Fatalf("another workspace must not review, got %d", c)
	}
	if c, _, _, _ := h.call("u_bob", "POST", ir+"/review", review); c != 403 && c != 404 {
		t.Fatalf("a student must not review, got %d", c)
	}
	for _, bad := range []map[string]any{{"rootCause": "x", "detection": "教师反馈", "correctiveActions": "修正投影并补测"}, {"rootCause": "求解器取错底面", "detection": "", "correctiveActions": "修正投影并补测"}} {
		if c, _, _, _ := h.call("u_alice", "POST", ir+"/review", bad); c != 400 {
			t.Fatalf("bad review %v must be 400, got %d", bad, c)
		}
	}
	for _, leak := range []string{"由u_bob的作答发现", "联系 a.b@example.com", "电话13800138000"} {
		r := map[string]any{"rootCause": "求解器对线面角的投影取错了底面", "detection": leak, "correctiveActions": "修正投影并补充回归题"}
		if c, o, _, _ := h.call("u_alice", "POST", ir+"/review", r); c != 422 || errCode(o) != "PERSONAL_INFO" {
			t.Fatalf("review with %q must be 422 PERSONAL_INFO: %d %v", leak, c, o)
		}
	}
	if c, _, raw, _ := h.call("u_alice", "POST", ir+"/review", review); c != 201 {
		t.Fatalf("review: %d %s", c, raw)
	}
	if c, o, _, _ := h.call("u_alice", "POST", ir+"/review", review); c != 409 || errCode(o) != "REVIEW_EXISTS" {
		t.Fatalf("second review must be 409 REVIEW_EXISTS: %d %v", c, o)
	}
	if pending() != 0 {
		t.Fatal("a reviewed incident is no longer pending")
	}
	_, out, _, _ = h.call("u_alice", "GET", ir, nil)
	rv := data(out)["sop"].(map[string]any)["review"].(map[string]any)
	if rv["rootCause"] != review["rootCause"] || data(out)["sop"].(map[string]any)["reviewPending"] != false {
		t.Fatalf("incident review view: %v", data(out)["sop"])
	}
	var audits int64
	h.db.Model(&model.EduAuditEvent{}).Where("action IN ?", []string{"incident.review", "revision_notice.ack"}).Count(&audits)
	if audits != 2 {
		t.Fatalf("audit: %d", audits)
	}
}
