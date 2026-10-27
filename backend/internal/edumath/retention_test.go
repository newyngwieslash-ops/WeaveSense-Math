package edumath

import (
	"context"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

// TestRetention (DEV-106, E54): retention periods are suggested defaults and nothing is
// cleaned until an org admin confirms them with a reason; a run first shows its impact,
// then deletes only the learning records of students who left the class longer ago than
// the period (classmates untouched), revokes old artifacts of failed jobs, keeps ledger
// and audit, is idempotent, is audited with counts only, and also runs on schedule.
func TestRetention(t *testing.T) { runRetention(t, newHarness(t, startMathService(t))) }

func TestPGRetention(t *testing.T) {
	runRetention(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func runRetention(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	ws := model.PersonalWorkspaceID("u_alice")
	now := time.Now().UTC()
	h.db.Create(&model.Membership{ID: "m_rcarol", TenantID: tenant, UserID: "u_carol", Role: "member", Status: "active", CreatedAt: now})
	h.db.Create(&model.WorkspaceMembership{ID: "wm_rcarol", WorkspaceID: ws, UserID: "u_carol", Role: "editor", CreatedAt: now})
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/roles", map[string]any{"tenantId": tenant, "userId": "u_carol", "role": "teacher"}); c != 200 {
		t.Fatalf("bind carol: %d %s", c, raw)
	}
	pubID, _ := publishG1(t, h)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二(9)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "保留检查", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	code := data(out)["code"]
	stu := []string{"X-Edu-Tenant", tenant}
	for i, u := range []string{"u_bob", "u_eve"} {
		h.call(u, "POST", "/api/edu-join", map[string]any{"code": code})
		h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": u, "status": "granted", "evidenceRef": "form-ret-" + u})
		if c, _, raw, _ := h.call(u, "POST", "/api/edu/student/assignments/"+asID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "ret-attempt-000" + string(rune('1'+i))}, stu...); c != 201 {
			t.Fatalf("attempt %s: %d %s", u, c, raw)
		}
	}
	countAttempts := func(u string) int64 {
		var n int64
		h.db.Model(&model.EduAttempt{}).Where("tenant_id = ? AND student_user_id = ?", tenant, u).Count(&n)
		return n
	}
	var enr model.EduEnrollment
	h.db.First(&enr, "tenant_id = ? AND class_id = ? AND student_user_id = ?", tenant, classID, "u_bob")
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/enrollments/"+enr.ID+"/withdraw", nil); c != 200 {
		t.Fatalf("withdraw: %d %s", c, raw)
	}

	// Suggested defaults, nothing applied.
	c, out, raw, _ := h.call("u_alice", "GET", "/api/edu/retention", nil)
	if c != 200 {
		t.Fatalf("get retention: %d %s", c, raw)
	}
	d := data(out)
	p := d["policy"].(map[string]any)
	if d["confirmed"] != false || p["state"] != "suggested" || p["tempDays"] != float64(7) || p["imageDays"] != float64(90) || p["learningDays"] != float64(180) || p["backupDays"] != float64(30) {
		t.Fatalf("defaults: %s", raw)
	}
	if c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/retention/run", map[string]any{"confirm": true}); c != 409 || errCode(out) != "RETENTION_NOT_CONFIRMED" {
		t.Fatalf("run before confirmation: %d %s", c, raw)
	}
	// The scheduled run skips unconfirmed tenants.
	h.db.Model(&model.EduEnrollment{}).Where("id = ?", enr.ID).Update("updated_at", now.AddDate(0, 0, -400))
	if n, err := h.svc.RetentionAll(context.Background()); err != nil || n != 0 || countAttempts("u_bob") != 1 {
		t.Fatalf("scheduled run on suggested defaults: %d %v %d", n, err, countAttempts("u_bob"))
	}

	// Only org admins (or tenant owners) manage retention.
	for _, u := range []string{"u_carol"} {
		if c, _, _, _ := h.call(u, "GET", "/api/edu/retention", nil, "X-Workspace-Id", ws); c != 403 {
			t.Fatalf("%s get retention: %d", u, c)
		}
	}
	if c, _, _, _ := h.call("u_bob", "GET", "/api/edu/retention", nil, stu...); c == 200 {
		t.Fatal("student must not read retention")
	}
	// Validation: bounds and reason.
	for _, body := range []map[string]any{
		{"tempDays": 7, "imageDays": 90, "learningDays": 10, "backupDays": 30, "reason": "校务会决定"},
		{"tempDays": 7, "imageDays": 90, "learningDays": 180, "backupDays": 60, "reason": "校务会决定"},
		{"tempDays": 7, "imageDays": 90, "learningDays": 180, "backupDays": 30, "reason": "短"},
		{"tempDays": 7, "imageDays": 90, "learningDays": 180, "backupDays": 30, "reason": "<b>校务会</b>"},
	} {
		if c, _, raw, _ := h.call("u_alice", "PUT", "/api/edu/retention", body); c != 400 {
			t.Fatalf("invalid retention %v: %d %s", body, c, raw)
		}
	}

	// Old failed job artifact: temporary file.
	old := now.AddDate(0, 0, -30)
	h.db.Create(&model.EduMathJob{ID: "emj_rfail", TenantID: tenant, WorkspaceID: ws, Kind: "analyze", IdemKey: "ret-fail", InputHash: "x", ProblemRevisionID: "epr_none", Status: "failed", Settlement: "released", CreatedAt: old, UpdatedAt: old})
	h.db.Create(&model.EduArtifact{ID: "ema_rfail", TenantID: tenant, JobID: "emj_rfail", ServiceArtifactID: "ma_retention_fail_0001", Kind: "report", Audience: "teacher", CreatedAt: old})

	var auditBefore int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ?", tenant).Count(&auditBefore)
	c, out, raw, _ = h.call("u_alice", "PUT", "/api/edu/retention", map[string]any{"tempDays": 7, "imageDays": 90, "learningDays": 180, "backupDays": 30, "reason": "校务会确认采用建议值"})
	if c != 200 || data(out)["confirmed"] != true {
		t.Fatalf("confirm: %d %s", c, raw)
	}
	pv := data(out)["preview"].(map[string]any)
	if pv["enrollments"] != float64(1) || pv["artifacts"] != float64(1) || pv["learningRows"].(map[string]any)["edu_attempts"] != float64(1) {
		t.Fatalf("preview: %s", raw)
	}
	// Impact first, nothing deleted without confirm.
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/retention/run", map[string]any{})
	if c != 200 || data(out)["dryRun"] != true || countAttempts("u_bob") != 1 {
		t.Fatalf("dry run: %d %s", c, raw)
	}
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/retention/run", map[string]any{"confirm": true})
	if c != 200 || data(out)["dryRun"] != false {
		t.Fatalf("run: %d %s", c, raw)
	}
	if countAttempts("u_bob") != 0 || countAttempts("u_eve") != 1 {
		t.Fatalf("after run: bob %d eve %d", countAttempts("u_bob"), countAttempts("u_eve"))
	}
	var art model.EduArtifact
	h.db.First(&art, "id = ?", "ema_rfail")
	if art.RevokedAt == nil {
		t.Fatal("old failed job artifact must be revoked")
	}
	var e2 model.EduEnrollment
	h.db.First(&e2, "id = ?", enr.ID)
	if e2.Status != "withdrawn" {
		t.Fatal("the withdrawn enrollment row stays for accounting")
	}
	// Audit kept; the run is recorded with counts only.
	var auditAfter, runs int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ?", tenant).Count(&auditAfter)
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ? AND detail LIKE ?", tenant, "retention.run", "enrollments=1 learning_rows=% artifacts=1").Count(&runs)
	if auditAfter != auditBefore+2 || runs != 1 {
		t.Fatalf("audit: before %d after %d runs %d", auditBefore, auditAfter, runs)
	}
	// Idempotent, and the overview shows the confirmed state.
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/retention/run", map[string]any{"confirm": true})
	if c != 200 || data(out)["summary"].(map[string]any)["enrollments"] != float64(0) {
		t.Fatalf("second run: %d %s", c, raw)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/org/overview", nil)
	if data(out)["retention"].(map[string]any)["state"] != "confirmed" {
		t.Fatalf("overview retention: %v", data(out)["retention"])
	}
	// Scheduled run applies confirmed policies as the system actor.
	if n, err := h.svc.RetentionAll(context.Background()); err != nil || n != 1 {
		t.Fatalf("scheduled run: %d %v", n, err)
	}
	var sys int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ? AND actor_id = ?", tenant, "retention.run", "system:retention").Count(&sys)
	if sys != 1 {
		t.Fatalf("system retention audit: %d", sys)
	}
}
