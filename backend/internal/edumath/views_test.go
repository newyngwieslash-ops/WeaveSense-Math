package edumath

import (
	"regexp"
	"testing"

	"yingce/backend/internal/model"
)

func TestReadsAndRequestTrace(t *testing.T) {
	runReadsAndRequestTrace(t, newHarness(t, startMathService(t)))
}

func TestPGReadsAndRequestTrace(t *testing.T) {
	runReadsAndRequestTrace(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

// runReadsAndRequestTrace (DEV-121, DEV-118): a successful student read is
// counted per terminal (refused reads are not); observability reports readers
// and terminals; jobs keep the request id of the HTTP request that accepted
// them and an org admin finds them by it; lesson revisions record generator,
// composer version and model from the lesson's own meta; the cost breakdown
// by method, template, model and terminal adds up to the period total; first
// reads become assignment_viewed metric events with the terminal; read rows are
// learning data, so retention and data deletion cover them.
func runReadsAndRequestTrace(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	var pub model.EduPublication
	h.db.First(&pub, "id = ?", pubID)
	var lr model.EduLessonRevision
	h.db.First(&lr, "id = ?", pub.LessonRevisionID)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高一(5)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "阅读", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	stu := []string{"X-Edu-Tenant", tenant}
	read := func(user, terminal string) int {
		t.Helper()
		hdr := append([]string{}, stu...)
		if terminal != "" {
			hdr = append(hdr, terminalHeader, terminal)
		}
		c, _, _, _ := h.call(user, "GET", "/api/edu/student/assignments/"+asID, nil, hdr...)
		return c
	}
	if c := read("u_bob", "phone"); c == 200 {
		t.Fatal("read before consent must be refused")
	}
	if c := read("u_carol", "phone"); c == 200 {
		t.Fatal("a non member read the assignment")
	}
	var n int64
	h.db.Model(&model.EduAssignmentView{}).Count(&n)
	if n != 0 {
		t.Fatalf("refused reads were counted: %d", n)
	}
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-read-bob"})
	for _, term := range []string{"phone", "phone", "desktop", "fridge"} {
		if c := read("u_bob", term); c != 200 {
			t.Fatalf("read %s: %d", term, c)
		}
	}
	var vs []model.EduAssignmentView
	h.db.Order("terminal").Find(&vs)
	got := map[string]int{}
	for _, v := range vs {
		got[v.Terminal] = v.Views
		if v.TenantID != tenant || v.AssignmentID != asID || v.StudentUserID != "u_bob" || v.FirstViewedAt.After(v.LastViewedAt) {
			t.Fatalf("view row: %+v", v)
		}
	}
	if len(vs) != 3 || got["phone"] != 2 || got["desktop"] != 1 || got["unknown"] != 1 {
		t.Fatalf("views per terminal: %v", got)
	}

	// Observability reading.
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/observability", nil)
	rd := data(out)["reading"].(map[string]any)
	byT := rd["readersByTerminal"].(map[string]any)
	if rd["collected"] != true || rd["students"].(float64) != 1 || rd["assignments"].(float64) != 1 || rd["firstReads"].(float64) != 3 ||
		byT["phone"].(float64) != 1 || byT["desktop"].(float64) != 1 || byT["unknown"].(float64) != 1 {
		t.Fatalf("reading: %v", rd)
	}

	// Request ids: echoed when well formed, generated otherwise; jobs keep theirs.
	_, _, _, hd := h.call("u_alice", "GET", "/api/edu/me", nil, "X-Request-Id", "support-case-0001")
	if hd.Get("X-Request-Id") != "support-case-0001" {
		t.Fatalf("request id not echoed: %q", hd.Get("X-Request-Id"))
	}
	_, _, _, hd = h.call("u_alice", "GET", "/api/edu/me", nil, "X-Request-Id", "bad id!")
	if rid := hd.Get("X-Request-Id"); rid == "bad id!" || !regexp.MustCompile(`^req_`).MatchString(rid) {
		t.Fatalf("malformed request id must be replaced: %q", rid)
	}
	var job model.EduMathJob
	h.db.First(&job, "id = ?", lr.JobID)
	if !requestIDRe.MatchString(job.RequestID) {
		t.Fatalf("job request id: %q", job.RequestID)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/jobs/"+job.ID+"/trace", nil)
	if data(out)["requestId"] != job.RequestID {
		t.Fatalf("trace request id: %v", data(out)["requestId"])
	}
	c, out, raw, _ := h.call("u_alice", "GET", "/api/edu/observability/requests/"+job.RequestID, nil)
	js, _ := data(out)["jobs"].([]any)
	if c != 200 || len(js) != 1 || js[0].(map[string]any)["id"] != job.ID {
		t.Fatalf("request lookup: %d %s", c, raw)
	}
	if c, _, _, _ := h.call("u_alice", "GET", "/api/edu/observability/requests/bad!", nil); c != 400 {
		t.Fatalf("malformed lookup: %d", c)
	}
	if c, _, _, _ := h.call("u_bob", "GET", "/api/edu/observability/requests/"+job.RequestID, nil, stu...); c == 200 {
		t.Fatal("a student looked up a request")
	}
	if c, out, _, _ := h.call("u_carol", "GET", "/api/edu/observability/requests/"+job.RequestID, nil); c == 200 && len(data(out)["jobs"].([]any)) != 0 {
		t.Fatal("another tenant found the job")
	}

	// Lesson origin from the lesson's own meta.
	if lr.Generator != "deterministic-template" || lr.ComposerVersion == "" || lr.ModelRef != "none" {
		t.Fatalf("lesson origin: %q %q %q", lr.Generator, lr.ComposerVersion, lr.ModelRef)
	}

	// Cost strata reconcile with the period total.
	period, _ := budgetPeriod(h.svc.now())
	_, rep, _, _ := h.call("u_alice", "GET", "/api/edu/cost-report?period="+period, nil)
	want := data(rep)["totalMicro"].(float64)
	keys := map[string]map[string]float64{}
	for _, by := range []string{"method", "template", "model", "terminal"} {
		c, out, raw, _ := h.call("u_alice", "GET", "/api/edu/cost-report/breakdown?period="+period+"&by="+by, nil)
		if c != 200 {
			t.Fatalf("breakdown %s: %d %s", by, c, raw)
		}
		keys[by] = map[string]float64{}
		var sum float64
		for _, x := range data(out)["rows"].([]any) {
			m := x.(map[string]any)
			keys[by][m["key"].(string)] = m["costMicro"].(float64)
			sum += m["costMicro"].(float64)
		}
		if sum != want || data(out)["totalMicro"].(float64) != want {
			t.Fatalf("%s: rows %v, period %v", by, sum, want)
		}
	}
	if _, ok := keys["template"]["deterministic-template@"+lr.ComposerVersion]; !ok {
		t.Fatalf("template stratum: %v", keys["template"])
	}
	if _, ok := keys["model"]["none"]; !ok {
		t.Fatalf("model stratum: %v", keys["model"])
	}
	tm := keys["terminal"]
	if tm["terminal:phone"] <= 0 || tm["terminal:desktop"] <= 0 || tm["terminal:unknown"] <= 0 {
		t.Fatalf("terminal stratum: %v", tm)
	}
	if want > 0 && len(keys["method"]) == 0 {
		t.Fatalf("method stratum empty: %v", keys["method"])
	}

	// Metrics: one assignment_viewed event per first read, with its terminal.
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/metrics/collect", nil); c != 200 {
		t.Fatalf("collect: %d %s", c, raw)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/metrics", nil)
	mv, _ := data(out)["types"].(map[string]any)["assignment_viewed"].(map[string]any)
	if mv == nil {
		t.Fatalf("metrics summary has no assignment_viewed: %v", data(out))
	}
	if tt := mv["terminals"].(map[string]any); mv["events"].(float64) != 3 || tt["phone"].(float64) != 1 || tt["unknown"].(float64) != 1 {
		t.Fatalf("assignment_viewed: %v", mv)
	}
	found := false
	for _, m := range learningTables {
		_, ok := m.(*model.EduAssignmentView)
		found = found || ok
	}
	if !found {
		t.Fatal("read rows must be covered by retention and data deletion")
	}
}
