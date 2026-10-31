package edumath

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

func TestObservability(t *testing.T) { runObservability(t, newHarness(t, startMathService(t))) }

func TestPGObservability(t *testing.T) {
	runObservability(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

// runObservability (DEV-121, E64): after a real analyze, lesson build and student
// submission, the tenant view counts generation by kind and submissions
// separately, all alerts are quiet; stuck, queued, unknown cost, reconciliation,
// unsettled reservation, ledger anomaly and failure rate each fire on their own
// seeded fact; a job trace chains platform events, stage settlements and the
// revisions it produced with metadata only; availability is listed as not
// collected and reads are counted (views_test.go); only an org admin or an operator with a jobs grant (audited)
// may look.
func runObservability(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高一(3)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "观测", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-obs-bob"})
	stu := []string{"X-Edu-Tenant", tenant}
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/attempts", map[string]any{"kind": "answer", "response": "obs-secret-answer", "clientKey": "obs-attempt-0001"}, stu...); c != 201 {
		t.Fatalf("attempt: %d %s", c, raw)
	}

	get := func(q string) map[string]any {
		t.Helper()
		c, out, raw, hd := h.call("u_alice", "GET", "/api/edu/observability"+q, nil)
		if c != 200 || hd.Get("Cache-Control") != "private, no-store" {
			t.Fatalf("observability: %d %s", c, raw)
		}
		for _, bad := range []string{"obs-secret-answer", "inputHash", "priceBook", "errorJson", "idemKey"} {
			if strings.Contains(string(raw), bad) {
				t.Fatalf("observability leaks %s: %s", bad, raw)
			}
		}
		return data(out)
	}
	alerts := func(v map[string]any) map[string]string {
		out := map[string]string{}
		for _, x := range v["alerts"].([]any) {
			m := x.(map[string]any)
			out[m["id"].(string)] = m["state"].(string)
		}
		return out
	}
	v := get("")
	if v["definition"] != "observability/1" || v["windowHours"].(float64) != 24 || len(v["notCollected"].([]any)) != 5 {
		t.Fatalf("observability shape: %v", v)
	}
	if av := v["availability"].(map[string]any); av["target"].(float64) != 0.995 || av["measured"] != nil {
		t.Fatalf("availability must be a target without a measured value: %v", av)
	}
	if v["reading"].(map[string]any)["collected"] != true || v["reading"].(map[string]any)["students"].(float64) != 0 || v["submission"].(map[string]any)["attempts"].(float64) != 1 {
		t.Fatalf("reading is collected (no reads here); submissions are counted: %v %v", v["reading"], v["submission"])
	}
	kinds := map[string]map[string]any{}
	for _, x := range v["generation"].(map[string]any)["byKind"].([]any) {
		m := x.(map[string]any)
		kinds[m["kind"].(string)] = m
	}
	if kinds["analyze"]["succeeded"].(float64) != 1 || kinds["lesson_build"]["succeeded"].(float64) != 1 || kinds["analyze"]["failureRate"].(float64) != 0 || kinds["analyze"]["p95Seconds"] == nil {
		t.Fatalf("generation by kind: %v", kinds)
	}
	for id, s := range alerts(v) {
		if s == "firing" {
			t.Fatalf("a healthy tenant has no firing alert: %s", id)
		}
	}
	if alerts(v)["failure_rate"] != "insufficient_data" {
		t.Fatalf("two finished jobs are not enough for a failure rate: %v", alerts(v))
	}

	// Seed one fact per alert.
	now := h.svc.now()
	ws := model.PersonalWorkspaceID("u_alice")
	var revID string
	if err := flow.TenantTx(h.db, tenant, func(tx *gorm.DB) error {
		var pr model.EduProblemRevision
		if err := tx.Where("tenant_id = ?", tenant).First(&pr).Error; err != nil {
			return err
		}
		revID = pr.ID
		job := func(id, status, settlement, code string, created, updated time.Time) *model.EduMathJob {
			return &model.EduMathJob{ID: id, TenantID: tenant, WorkspaceID: ws, Kind: "analyze", IdemKey: id, InputHash: "h", ProblemRevisionID: revID,
				Status: status, Settlement: settlement, ErrorCode: code, CreatedAt: created, UpdatedAt: updated}
		}
		rows := []any{
			job("ejb_obs_stuck", "running", "open", "", now.Add(-50*time.Minute), now.Add(-40*time.Minute)),
			job("ejb_obs_queued", "queued", "open", "", now.Add(-20*time.Minute), now.Add(-20*time.Minute)),
			job("ejb_obs_recon", "reconciliation_required", "held_unknown", "", now.Add(-time.Hour), now.Add(-time.Hour)),
			job("ejb_obs_leak", "failed", "open", "SERVICE_FAILED", now.Add(-45*time.Minute), now.Add(-20*time.Minute)),
			&model.EduLedgerAnomaly{ID: "ela_obs", TenantID: tenant, JobID: "ejb_obs_recon", Kind: "unknown_charge", Status: "open", DetectedAt: now, LastSeenAt: now},
			&model.EduJobEvent{ID: "eje_obs_1", TenantID: tenant, JobID: "ejb_obs_leak", Seq: 1, Type: "job.accepted", Status: "queued", Settlement: "open", CreatedAt: now.Add(-45 * time.Minute)},
			&model.EduJobEvent{ID: "eje_obs_2", TenantID: tenant, JobID: "ejb_obs_leak", Seq: 2, Type: "job.failed", Status: "failed", Settlement: "open", CreatedAt: now.Add(-20 * time.Minute)},
			&model.EduStageRun{ID: "esr_obs_leak", TenantID: tenant, JobID: "ejb_obs_leak", Stage: "analyze", Status: "failed", ComputeMS: 900, CostMicro: 77, CreatedAt: now.Add(-30 * time.Minute)},
		}
		for i := 0; i < 3; i++ {
			rows = append(rows, job("ejb_obs_fail"+string(rune('a'+i)), "failed", "settled", "TIMEOUT", now.Add(-2*time.Hour), now.Add(-time.Hour)))
		}
		for _, x := range rows {
			if err := tx.Create(x).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	v = get("")
	al := alerts(v)
	for _, id := range []string{"stuck_jobs", "queue_age", "reconciliation_required", "unknown_cost", "unsettled_reservations", "ledger_anomalies", "failure_rate"} {
		if al[id] != "firing" {
			t.Fatalf("%s must fire: %v", id, v["alerts"])
		}
	}
	q := v["queue"].(map[string]any)
	if q["queued"].(float64) != 1 || q["running"].(float64) != 1 || q["stuckTotal"].(float64) != 1 || q["oldestQueuedSeconds"].(float64) < 1200 {
		t.Fatalf("queue: %v", q)
	}
	codes := v["generation"].(map[string]any)["errorCodes"].([]any)
	if top := codes[0].(map[string]any); top["errorCode"] != "TIMEOUT" || top["count"].(float64) != 3 {
		t.Fatalf("error codes: %v", codes)
	}
	// A one hour window drops the jobs created two hours ago.
	if fin := get("?hours=1")["generation"].(map[string]any)["failed"].(float64); fin != 1 {
		t.Fatalf("window: failed %v", fin)
	}
	for _, bad := range []string{"0", "169", "x"} {
		if c, _, _, _ := h.call("u_alice", "GET", "/api/edu/observability?hours="+bad, nil); c != 400 {
			t.Fatalf("hours=%s: %d", bad, c)
		}
	}

	// Trace of the real lesson job and of the seeded failure.
	var lesson model.EduLessonRevision
	h.db.Where("tenant_id = ?", tenant).First(&lesson)
	trace := func(user, id string, hdr ...string) (int, map[string]any, string) {
		c, out, raw, _ := h.call(user, "GET", "/api/edu/jobs/"+id+"/trace", nil, hdr...)
		return c, data(out), string(raw)
	}
	c, tr, raw := trace("u_alice", lesson.JobID)
	if c != 200 || tr["traceId"] != lesson.JobID || tr["revisions"].(map[string]any)["lessonRevisionId"] != lesson.ID || len(tr["steps"].([]any)) == 0 {
		t.Fatalf("lesson trace: %d %s", c, raw)
	}
	for _, bad := range []string{"inputHash", "idemKey", "priceBook", "errorJson", "options"} {
		if strings.Contains(raw, bad) {
			t.Fatalf("trace leaks %s: %s", bad, raw)
		}
	}
	c, tr, raw = trace("u_alice", "ejb_obs_leak")
	steps := tr["steps"].([]any)
	if c != 200 || len(steps) != 3 || steps[0].(map[string]any)["type"] != "job.accepted" || steps[1].(map[string]any)["source"] != "stage" ||
		tr["usage"].(map[string]any)["costMicro"].(float64) != 77 || tr["job"].(map[string]any)["errorCode"] != "SERVICE_FAILED" {
		t.Fatalf("seeded trace must be ordered by time: %d %s", c, raw)
	}
	if c, _, _ := trace("u_alice", "ejb_missing"); c != 404 {
		t.Fatalf("unknown job: %d", c)
	}

	// Who may look.
	if c, _, _ := trace("u_bob", lesson.JobID, stu...); c == 200 {
		t.Fatal("student read a trace")
	}
	if c, _, _, _ := h.call("u_bob", "GET", "/api/edu/observability", nil, stu...); c == 200 {
		t.Fatal("student read observability")
	}
	if c, _, _ := trace("u_carol", lesson.JobID); c == 200 {
		t.Fatal("another tenant read a trace")
	}
	ops := func(grant string) []string { return []string{"X-Edu-Tenant", tenant, "X-Edu-Ops-Grant", grant} }
	if c, _, _, _ := h.call("u_ops", "GET", "/api/edu/ops/observability", nil, ops("eog_none")...); c != 403 {
		t.Fatalf("operator without grant: %d", c)
	}
	grant := func(body map[string]any) string {
		c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/ops-grants", body)
		if c != 201 {
			t.Fatalf("grant: %d %s", c, raw)
		}
		return data(out)["id"].(string)
	}
	gc := grant(map[string]any{"operatorUserId": "u_ops", "scope": "class", "classId": classID, "purpose": "排查班级作答", "hours": 2})
	if c, _, _, _ := h.call("u_ops", "GET", "/api/edu/ops/jobs/"+lesson.JobID+"/trace", nil, ops(gc)...); c != 403 {
		t.Fatalf("class grant reading a trace: %d", c)
	}
	gj := grant(map[string]any{"operatorUserId": "u_ops", "scope": "jobs", "purpose": "排查卡住的作业 OPS-9", "hours": 2})
	if c, _, raw, _ := h.call("u_ops", "GET", "/api/edu/ops/observability", nil, ops(gj)...); c != 200 || !strings.Contains(string(raw), "stuck_jobs") {
		t.Fatalf("ops observability: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_ops", "GET", "/api/edu/ops/jobs/ejb_obs_stuck/trace", nil, ops(gj)...); c != 200 || !strings.Contains(string(raw), "ejb_obs_stuck") {
		t.Fatalf("ops trace: %d %s", c, raw)
	}
	if c, _, _, _ := h.call("u_ops", "GET", "/api/edu/jobs/ejb_obs_stuck/trace", nil, ops(gj)...); c == 200 {
		t.Fatal("an operator must use the audited ops route")
	}
	var reads int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ? AND subject = ?", tenant, "ops.read", gj).Count(&reads)
	if reads != 2 {
		t.Fatalf("every operator read is audited: %d", reads)
	}
}
