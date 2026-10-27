package edumath

import (
	"testing"
	"time"

	"yingce/backend/internal/model"
)

// TestTeacherTime and TestPGTeacherTime (DEV-117): active time counts only real gaps within the idle
// threshold, parallel phases never exceed wall clock time, the time diary corrects a day and phase and is
// shown next to the measured value, the org view is an aggregate for org admins only, and teaching metrics
// keep independent, assisted, missing and demo results apart with denominator and version.
func TestTeacherTime(t *testing.T) { runTeacherTime(t, newHarness(t, startMathService(t))) }

func TestPGTeacherTime(t *testing.T) {
	runTeacherTime(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func runTeacherTime(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	ws := model.PersonalWorkspaceID("u_alice")
	now := time.Now().UTC()
	h.db.Create(&model.Membership{ID: "m_tcarol", TenantID: tenant, UserID: "u_carol", Role: "member", Status: "active", CreatedAt: now})
	h.db.Create(&model.WorkspaceMembership{ID: "wm_tcarol", WorkspaceID: ws, UserID: "u_carol", Role: "editor", CreatedAt: now})
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/roles", map[string]any{"tenantId": tenant, "userId": "u_carol", "role": "teacher"}); c != 200 {
		t.Fatalf("bind carol: %d %s", c, raw)
	}
	wsh := []string{"X-Workspace-Id", ws}

	// Yesterday 10:00 in UTC+8, so every beat and the diary day fall on one past day.
	y := time.Now().In(reportZone).AddDate(0, 0, -1)
	base := time.Date(y.Year(), y.Month(), y.Day(), 10, 0, 0, 0, reportZone)
	day := base.Format("2006-01-02")
	beat := func(user string, offset time.Duration, phase, object string, hdr ...string) (int, map[string]any) {
		h.now.Store(base.Add(offset).Unix())
		c, out, _, _ := h.call(user, "POST", "/api/edu/teacher-time/beat", map[string]any{"phase": phase, "objectId": object}, hdr...)
		return c, data(out)
	}
	steps := []struct {
		at    time.Duration
		phase string
		added float64
	}{
		{0, "prep", 0},                  // first beat opens a segment
		{time.Minute, "prep", 60},       // a real minute of work
		{2 * time.Minute, "review", 60}, // switching phase keeps counting the gap once
		{2 * time.Minute, "prep", 0},    // a parallel tab at the same instant adds nothing
		{9 * time.Minute, "prep", 0},    // seven idle minutes are not counted
		{10 * time.Minute, "prep", 60},  // work resumes
		{10*time.Minute + 30*time.Second, "edit", 30},
	}
	for i, s := range steps {
		c, d := beat("u_alice", s.at, s.phase, "")
		if c != 200 || d["addedSeconds"].(float64) != s.added || d["day"] != day {
			t.Fatalf("beat %d: %d %v", i, c, d)
		}
	}
	if c, _ := beat("u_carol", 3*time.Minute, "after_class", "", wsh...); c != 200 {
		t.Fatalf("carol beat: %d", c)
	}
	if c, _ := beat("u_carol", 4*time.Minute, "after_class", "", wsh...); c != 200 {
		t.Fatalf("carol beat: %d", c)
	}
	for _, bad := range []map[string]any{{"phase": "sleep"}, {"phase": "prep", "objectId": "x' or 1=1"}, {}} {
		if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/teacher-time/beat", bad); c != 400 {
			t.Fatalf("bad beat %v: %d %s", bad, c, raw)
		}
	}
	if c, _, _, _ := h.call("u_bob", "POST", "/api/edu/teacher-time/beat", map[string]any{"phase": "prep"}, "X-Edu-Tenant", tenant); c == 200 {
		t.Fatal("a student must not record teacher time")
	}
	h.now.Store(0)

	// Diary corrects review for that day.
	if c, _, raw, _ := h.call("u_alice", "PUT", "/api/edu/teacher-time/diary", map[string]any{"day": day, "phase": "review", "minutes": 5}); c != 200 {
		t.Fatalf("diary: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_alice", "PUT", "/api/edu/teacher-time/diary", map[string]any{"day": day, "phase": "review", "minutes": 6}); c != 200 {
		t.Fatalf("diary update: %d %s", c, raw)
	}
	future := time.Now().In(reportZone).AddDate(0, 0, 2).Format("2006-01-02")
	for _, bad := range []map[string]any{{"day": future, "phase": "review", "minutes": 5}, {"day": day, "phase": "review", "minutes": 2000}, {"day": day, "phase": "review"}, {"day": "2026-02-30", "phase": "review", "minutes": 5}} {
		if c, _, raw, _ := h.call("u_alice", "PUT", "/api/edu/teacher-time/diary", bad); c != 400 {
			t.Fatalf("bad diary %v: %d %s", bad, c, raw)
		}
	}
	c, out, raw, _ := h.call("u_alice", "GET", "/api/edu/teacher-time", nil)
	if c != 200 {
		t.Fatalf("report: %d %s", c, raw)
	}
	r := data(out)
	ph := r["phases"].(map[string]any)
	get := func(p, f string) float64 { return ph[p].(map[string]any)[f].(float64) }
	if get("prep", "measuredSeconds") != 120 || get("review", "measuredSeconds") != 60 || get("review", "diaryMinutes") != 6 || get("review", "correctedSeconds") != 360 ||
		get("edit", "measuredSeconds") != 30 || get("after_class", "measuredSeconds") != 0 {
		t.Fatalf("own report: %v", ph)
	}
	if tot := r["total"].(map[string]any); tot["measuredSeconds"].(float64) != 210 || tot["correctedSeconds"].(float64) != 510 || r["definition"] != teacherTimeDefinition || r["idleThresholdSeconds"].(float64) != 300 {
		t.Fatalf("own totals: %v", r)
	}
	_, out, _, _ = h.call("u_carol", "GET", "/api/edu/teacher-time", nil, wsh...)
	if get := data(out)["phases"].(map[string]any)["after_class"].(map[string]any)["measuredSeconds"].(float64); get != 60 {
		t.Fatalf("carol sees only her own time: %v", out)
	}
	if c, _, _, _ := h.call("u_carol", "GET", "/api/edu/teacher-time?scope=org", nil, wsh...); c != 403 {
		t.Fatalf("a teacher must not read the org aggregate: %d", c)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/teacher-time?scope=org", nil)
	if o := data(out); o["teachers"].(float64) != 2 || o["smallSample"] != true || o["total"].(map[string]any)["measuredSeconds"].(float64) != 270 {
		t.Fatalf("org aggregate: %v", o)
	}
	for _, q := range []string{"?from=2026-13-01", "?from=2026-10-10&to=2026-10-01", "?from=2025-01-01&to=2026-01-01"} {
		if c, _, _, _ := h.call("u_alice", "GET", "/api/edu/teacher-time"+q, nil); c != 400 {
			t.Fatalf("bad range %s: %d", q, c)
		}
	}

	// Teaching metrics.
	pubID, _ := publishG1(t, h)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高一(3)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "教学指标", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	stu := []string{"X-Edu-Tenant", tenant}
	for _, u := range []string{"u_bob", "u_dan", "u_eve"} {
		h.call(u, "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
		h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": u, "status": "granted", "evidenceRef": "form-tt-1"})
	}
	p := "/api/edu/student/assignments/" + asID
	h.call("u_bob", "POST", p+"/hints", map[string]any{"level": 1}, stu...)
	if c, _, raw, _ := h.call("u_bob", "POST", p+"/attempts", map[string]any{"kind": "answer", "response": "4/√22", "clientKey": "ttime0001"}, stu...); c != 201 {
		t.Fatalf("bob: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_dan", "POST", p+"/attempts", map[string]any{"kind": "answer", "response": "4/√22", "clientKey": "ttime0002"}, stu...); c != 201 {
		t.Fatalf("dan: %d %s", c, raw)
	}
	h.db.Create(&model.EduDemoClass{ID: "edc_ttdemo", TenantID: tenant, WorkspaceID: ws, Name: "演示班", CreatedBy: "u_alice", CreatedAt: now})
	c, out, raw, _ = h.call("u_alice", "GET", "/api/edu/assignments/"+asID+"/teaching-metrics", nil)
	if c != 200 {
		t.Fatalf("teaching metrics: %d %s", c, raw)
	}
	m := data(out)
	o := m["outcomes"].(map[string]any)
	if m["denominator"].(float64) != 3 || m["attempted"].(float64) != 2 || o["independent"].(float64) != 1 || o["assisted"].(float64) != 1 || o["missing"].(float64) != 1 ||
		m["smallSample"] != true || m["firstTry"].(map[string]any)["correct"].(float64) != 1 || m["firstTry"].(map[string]any)["of"].(float64) != 2 {
		t.Fatalf("teaching metrics: %v", m)
	}
	if d := m["demo"].(map[string]any); d["included"] != false || d["classes"].(float64) != 1 {
		t.Fatalf("demo must be counted apart: %v", d)
	}
	if v := m["version"].(map[string]any); v["definition"] != teachingMetricsVersion || v["publicationId"] != pubID || v["lessonRevisionId"] == "" {
		t.Fatalf("version: %v", v)
	}
	if c, _, _, _ := h.call("u_carol", "GET", "/api/edu/assignments/"+asID+"/teaching-metrics", nil, wsh...); c == 200 {
		t.Fatal("a teacher who does not teach the class must not read its metrics")
	}
	if c, _, _, _ := h.call("u_bob", "GET", "/api/edu/assignments/"+asID+"/teaching-metrics", nil, stu...); c == 200 {
		t.Fatal("a student must not read teaching metrics")
	}
}
