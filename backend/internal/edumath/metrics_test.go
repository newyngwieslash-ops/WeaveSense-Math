package edumath

import (
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

// TestMetrics and TestPGMetrics (DEV-116, AC-58): metric events are derived on the server, counted once
// across repeated and replayed collections, split first try, after hints, independent and missing,
// carry no names or typed text, exclude demo data, and leave with a deleted student's learning data.
func TestMetrics(t *testing.T) { runMetrics(t, newHarness(t, startMathService(t))) }

func TestPGMetrics(t *testing.T) {
	runMetrics(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func runMetrics(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	mk := func(name string) (string, string) {
		_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": name})
		k := data(out)["id"].(string)
		c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/classes/"+k+"/assignments", map[string]any{"publicationId": pubID, "title": "指标", "answerPolicy": "teacher_release"})
		if c != 201 {
			t.Fatalf("assignment: %d %s", c, raw)
		}
		return k, data(out)["id"].(string)
	}
	classA, asA := mk("高一(1)班")
	_, asB := mk("高一(2)班")
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes/"+classA+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	code := data(out)["code"]
	stu := []string{"X-Edu-Tenant", tenant}
	for _, u := range []string{"u_bob", "u_dan"} {
		h.call(u, "POST", "/api/edu-join", map[string]any{"code": code})
		h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": u, "status": "granted", "evidenceRef": "知情同意书"})
	}
	const typed = "METRICMARK 我觉得是三分之一"
	p := "/api/edu/student/assignments/" + asA
	if c, _, raw, _ := h.call("u_dan", "POST", p+"/attempts", map[string]any{"kind": "answer", "response": typed, "clientKey": "metr0001"}, stu...); c != 201 {
		t.Fatalf("dan attempt: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_dan", "POST", p+"/attempts", map[string]any{"kind": "answer", "response": "1/3", "clientKey": "metr0002"}, stu...); c != 201 {
		t.Fatalf("dan retry: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_bob", "POST", p+"/hints", map[string]any{"level": 1}, stu...); c != 200 && c != 201 {
		t.Fatalf("hint: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_bob", "POST", p+"/attempts", map[string]any{"kind": "answer", "response": typed, "clientKey": "metr0003"}, stu...); c != 201 {
		t.Fatalf("bob attempt: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_bob", "POST", p+"/help", map[string]any{"kind": "ask_teacher", "message": "METRICMARK 第二步看不懂", "clientKey": "metric-help-0001"}, stu...); c != 201 {
		t.Fatalf("help: %d %s", c, raw)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/assignments/"+asA+"/help", nil)
	groups, _ := out["data"].([]any)
	if len(groups) != 1 {
		t.Fatalf("help groups: %v", out)
	}
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/assignments/"+asA+"/help/reply", map[string]any{"groupKey": groups[0].(map[string]any)["groupKey"], "body": "请先看第二步的图", "public": false}); c != 200 {
		t.Fatalf("help reply: %d %s", c, raw)
	}
	// A demo class record never becomes a metric event.
	h.db.Create(&model.EduDemoClass{ID: "edc_metricdemo", TenantID: tenant, WorkspaceID: "ws", Name: "演示班", CreatedBy: "u_alice"})

	// Students and outsiders cannot read or run the collector.
	if c, _, _, _ := h.call("u_bob", "GET", "/api/edu/metrics", nil, stu...); c == 200 {
		t.Fatal("a student must not read metrics")
	}
	if c, _, _, _ := h.call("u_bob", "POST", "/api/edu/metrics/collect", map[string]any{}, stu...); c == 200 {
		t.Fatal("a student must not run the collector")
	}
	collect := func(replay bool) map[string]any {
		c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/metrics/collect", map[string]any{"replay": replay})
		if c != 200 {
			t.Fatalf("collect: %d %s", c, raw)
		}
		return data(out)
	}
	first := collect(false)
	if first["inserted"].(float64) == 0 || first["inserted"] != first["total"] {
		t.Fatalf("first collection: %v", first)
	}
	if again := collect(false); again["inserted"].(float64) != 0 || again["total"] != first["total"] {
		t.Fatalf("a repeated collection must not duplicate: %v", again)
	}
	if replay := collect(true); replay["inserted"].(float64) != 0 || replay["total"] != first["total"] {
		t.Fatalf("a full replay must not duplicate: %v", replay)
	}
	if n, _, err := h.svc.MetricsAll(t.Context()); err != nil || n == 0 {
		t.Fatalf("worker collection: %d %v", n, err)
	}
	var total int64
	h.db.Model(&model.EduMetricEvent{}).Where("tenant_id = ?", tenant).Count(&total)
	if float64(total) != first["total"].(float64) {
		t.Fatalf("worker collection must not duplicate: %d vs %v", total, first["total"])
	}

	// Nothing typed or identifying is stored.
	var evs []model.EduMetricEvent
	h.db.Where("tenant_id = ?", tenant).Find(&evs)
	for _, e := range evs {
		blob := strings.Join([]string{e.Subject, e.ObjectID, e.Attrs, e.Source, e.ClassID}, "|")
		if strings.Contains(blob, "METRICMARK") || strings.Contains(blob, "u_bob") || strings.Contains(blob, "u_dan") || strings.Contains(blob, "u_alice") || strings.Contains(blob, "知情") {
			t.Fatalf("metric event carries identifying or typed content: %+v", e)
		}
		if strings.Contains(blob, "第二步") {
			t.Fatalf("metric event carries help text: %+v", e)
		}
		if e.ClassID == "edc_metricdemo" || e.AppVersion == "" || e.SchemaVersion != 1 {
			t.Fatalf("bad metric event: %+v", e)
		}
	}

	c, out, raw, _ := h.call("u_alice", "GET", "/api/edu/metrics", nil)
	if c != 200 {
		t.Fatalf("summary: %d %s", c, raw)
	}
	sum := data(out)
	types := sum["types"].(map[string]any)
	cnt := func(typ string) map[string]any { return types[typ].(map[string]any) }
	att := cnt("attempt_submitted")
	modes := att["modes"].(map[string]any)
	if att["events"].(float64) != 3 || att["subjects"].(float64) != 2 || att["smallSample"] != true ||
		modes["first_try"].(float64) != 1 || modes["retry"].(float64) != 1 || modes["after_hints"].(float64) != 1 || att["independent"].(float64) != 2 {
		t.Fatalf("attempt split: %v", att)
	}
	if cnt("hint_revealed")["events"].(float64) != 1 || cnt("assignment_created")["events"].(float64) != 2 || cnt("artifact_reused")["events"].(float64) != 1 || cnt("feedback_resolved")["events"].(float64) != 1 {
		t.Fatalf("hint, assignment and reuse counts: %v", types)
	}
	if a := sum["assignments"].(map[string]any); a["new"].(float64) != 1 || a["reused"].(float64) != 1 {
		t.Fatalf("new versus reused: %v", a)
	}
	for _, typ := range []string{"problem_uploaded", "problem_confirmed", "analysis_completed", "review_completed", "publication_created", "job_completed", "usage_settled"} {
		if cnt(typ)["events"].(float64) < 1 {
			t.Fatalf("%s missing: %v", typ, types)
		}
	}
	if att["missing"].(map[string]any)["terminal"].(float64) != 3 || len(cnt("review_completed")["missing"].(map[string]any)) != 1 || sum["demoExcluded"] != true {
		t.Fatalf("missing dimensions must be reported: %v", sum)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/metrics?classId="+classA, nil)
	if n := data(out)["types"].(map[string]any)["assignment_created"].(map[string]any)["events"].(float64); n != 1 {
		t.Fatalf("class filter: %v", n)
	}
	if c, _, _, _ := h.call("u_alice", "GET", "/api/edu/metrics?classId=x%27or", nil); c != 400 {
		t.Fatal("a malformed filter must be refused")
	}
	_ = asB

	// Deleting bob's learning data removes his metric events, and a replay does not bring them back.
	_, out, raw, _ = h.call("u_alice", "POST", "/api/edu/data-requests", map[string]any{"studentUserId": "u_bob", "kind": "delete", "requester": "guardian", "reason": "监护人书面申请"})
	id, _ := data(out)["id"].(string)
	if id == "" {
		t.Fatalf("delete request: %s", raw)
	}
	h.call("u_alice", "POST", "/api/edu/data-requests/"+id+"/verify", map[string]any{"verificationRef": "核验单 DR-02"})
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/data-requests/"+id+"/complete", map[string]any{"confirm": true}); c != 200 {
		t.Fatalf("complete delete: %d %s", c, raw)
	}
	students := func() int64 {
		var n int64
		h.db.Model(&model.EduMetricEvent{}).Where("tenant_id = ? AND role = ?", tenant, "student").Count(&n)
		return n
	}
	if n := students(); n != 2 {
		t.Fatalf("only dan's two attempts must remain: %d", n)
	}
	collect(true)
	if n := students(); n != 2 {
		t.Fatalf("a replay must not resurrect deleted learning data: %d", n)
	}
}
