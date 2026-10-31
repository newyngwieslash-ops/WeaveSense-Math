package edumath

import (
	"context"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

// TestArtifactPurgeAndClassArchive (DEV-106, E78): the retention run deletes the
// bytes of revoked temporary artifacts from the math service store (outside the
// transaction, idempotent, audited; a service failure leaves them pending and an
// unknown artifact is recorded as missing), and staff archive a class with
// confirmation and a reason: invites are revoked, the class takes no new invites
// or assignments (CLASS_ARCHIVED), its assignments close to students while their
// records stay, and once the
// learning period has passed since archiving the students' learning records in
// that class are removed by the scheduled run.
func TestArtifactPurgeAndClassArchive(t *testing.T) {
	runArtifactPurgeAndClassArchive(t, newHarness(t, startMathService(t)))
}

func TestPGArtifactPurgeAndClassArchive(t *testing.T) {
	runArtifactPurgeAndClassArchive(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func runArtifactPurgeAndClassArchive(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	ws := model.PersonalWorkspaceID("u_alice")
	now := time.Now().UTC()
	pubID, _ := publishG1(t, h)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高三(2)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "归档检查", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	code := data(out)["code"]
	stu := []string{"X-Edu-Tenant", tenant}
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": code})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-arch-bob"})
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "arch-attempt-0001"}, stu...); c != 201 {
		t.Fatalf("attempt: %d %s", c, raw)
	}
	countAttempts := func() int64 {
		var n int64
		h.db.Model(&model.EduAttempt{}).Where("tenant_id = ? AND student_user_id = ?", tenant, "u_bob").Count(&n)
		return n
	}

	// Archive: impact first, reason required, other tenants cannot see the class.
	c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/archive", map[string]any{"reason": "学期结束"})
	if c != 200 || data(out)["dryRun"] != true || data(out)["activeInvites"] != float64(1) || data(out)["activeEnrollments"] != float64(1) {
		t.Fatalf("archive dry run: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/archive", map[string]any{"confirm": true, "reason": "短"}); c != 400 {
		t.Fatalf("archive without reason: %d %s", c, raw)
	}
	if c, _, _, _ := h.call("u_carol", "POST", "/api/edu/classes/"+classID+"/archive", map[string]any{"confirm": true, "reason": "学期结束了"}); c == 200 {
		t.Fatal("another tenant must not archive the class")
	}
	if c, _, _, _ := h.call("u_bob", "POST", "/api/edu/classes/"+classID+"/archive", map[string]any{"confirm": true, "reason": "学期结束了"}, stu...); c == 200 {
		t.Fatal("a student must not archive the class")
	}
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/archive", map[string]any{"confirm": true, "reason": "学期结束，班级归档"})
	if c != 200 || data(out)["status"] != "archived" || data(out)["invitesRevoked"] != float64(1) {
		t.Fatalf("archive: %d %s", c, raw)
	}
	var k model.EduClass
	h.db.First(&k, "id = ?", classID)
	if k.Status != "archived" || k.ArchivedAt == nil {
		t.Fatalf("class after archive: %+v", k)
	}
	var activeInv int64
	h.db.Model(&model.EduClassInvite{}).Where("class_id = ? AND status = ?", classID, "active").Count(&activeInv)
	if activeInv != 0 {
		t.Fatal("archiving revokes active invites")
	}
	for _, p := range []string{"/invites", "/assignments", "/archive"} {
		if c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/classes/"+classID+p, map[string]any{"ttlHours": 24, "maxUses": 5, "publicationId": pubID, "title": "新任务", "answerPolicy": "teacher_release", "confirm": true, "reason": "再次归档测试"}); c != 409 || errCode(out) != "CLASS_ARCHIVED" {
			t.Fatalf("POST %s on archived class: %d %s", p, c, raw)
		}
	}
	if c, _, _, _ := h.call("u_eve", "POST", "/api/edu-join", map[string]any{"code": code}); c == 200 || c == 201 {
		t.Fatal("an archived class takes no joins")
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/classes", nil)
	for _, x := range out["data"].([]any) {
		if x.(map[string]any)["id"] == classID {
			t.Fatal("archived classes leave the active class list")
		}
	}
	// Students lose access to the class's assignments; their records stay (for
	// data requests and export) until the learning period removes them.
	if c, _, raw, _ := h.call("u_bob", "GET", "/api/edu/student/assignments/"+asID, nil, stu...); c != 404 {
		t.Fatalf("archived class assignments are closed to students: %d %s", c, raw)
	}
	if countAttempts() != 1 {
		t.Fatal("archiving keeps learning records until the retention period")
	}
	var arch int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ? AND subject = ? AND detail LIKE ?", tenant, "class.archive", classID, "invites_revoked=1 enrollments=1 | %").Count(&arch)
	if arch != 1 {
		t.Fatalf("archive audit: %d", arch)
	}

	// Temporary artifact of an old failed job, with real bytes in the service store.
	var real model.EduArtifact
	if err := h.db.Where("tenant_id = ? AND audience = ?", tenant, "teacher").Order("created_at").First(&real).Error; err != nil {
		t.Fatalf("no teacher artifact: %v", err)
	}
	if _, _, err := h.svc.Client.Artifact(context.Background(), tenant, real); err != nil {
		t.Fatalf("artifact readable before purge: %v", err)
	}
	old := now.AddDate(0, 0, -30)
	h.db.Create(&model.EduMathJob{ID: "emj_pfail", TenantID: tenant, WorkspaceID: ws, Kind: "analyze", IdemKey: "purge-fail", InputHash: "x", ProblemRevisionID: "epr_none", Status: "failed", Settlement: "released", CreatedAt: old, UpdatedAt: old})
	h.db.Model(&model.EduArtifact{}).Where("id = ?", real.ID).Update("job_id", "emj_pfail")
	h.db.Create(&model.EduArtifact{ID: "ema_pgone", TenantID: tenant, JobID: "emj_pfail", ServiceArtifactID: "ma_" + strings.Repeat("0", 32), Kind: "report", Audience: "teacher", CreatedAt: old})
	// An incident revocation keeps its bytes.
	h.db.Create(&model.EduArtifact{ID: "ema_pinc", TenantID: tenant, JobID: "emj_other", ServiceArtifactID: "ma_" + strings.Repeat("1", 32), Kind: "report", Audience: "teacher", RevokedAt: &now, CreatedAt: old})

	if c, _, raw, _ := h.call("u_alice", "PUT", "/api/edu/retention", map[string]any{"tempDays": 7, "imageDays": 90, "learningDays": 180, "backupDays": 30, "reason": "校务会确认采用建议值"}); c != 200 {
		t.Fatalf("confirm retention: %d %s", c, raw)
	}
	// Archived less than the learning period ago: learning records stay.
	if n, err := h.svc.RetentionAll(context.Background()); err != nil || n != 1 || countAttempts() != 1 {
		t.Fatalf("first scheduled run: %d %v attempts %d", n, err, countAttempts())
	}
	var got, gone, inc model.EduArtifact
	h.db.First(&got, "id = ?", real.ID)
	h.db.First(&gone, "id = ?", "ema_pgone")
	h.db.First(&inc, "id = ?", "ema_pinc")
	if got.RevokedAt == nil || got.PurgeState != "purged" || got.PurgedAt == nil {
		t.Fatalf("real artifact after run: %+v", got)
	}
	if gone.PurgeState != "missing" || gone.PurgedAt == nil {
		t.Fatalf("unknown artifact after run: %+v", gone)
	}
	if inc.PurgeState != "" {
		t.Fatalf("incident revocation must keep its bytes: %+v", inc)
	}
	if _, _, err := h.svc.Client.Artifact(context.Background(), tenant, real); err == nil {
		t.Fatal("purged bytes must be gone from the math service store")
	}
	var purgeAudit int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ? AND actor_id = ?", tenant, "artifact.purge", "system:retention").Count(&purgeAudit)
	if purgeAudit != 2 {
		t.Fatalf("purge audit: %d", purgeAudit)
	}
	if n, m, err := h.svc.PurgeArtifacts(context.Background(), tenant, 50); err != nil || n != 0 || m != 0 {
		t.Fatalf("purge is idempotent: %d %d %v", n, m, err)
	}
	// Service failure leaves the artifact pending for the next run.
	h.db.Create(&model.EduArtifact{ID: "ema_pretry", TenantID: tenant, JobID: "emj_pfail", ServiceArtifactID: "ma_" + strings.Repeat("2", 32), Kind: "report", Audience: "teacher", RevokedAt: &now, PurgeState: "pending", CreatedAt: old})
	down := *h.svc
	down.Client.BaseURL = "http://127.0.0.1:1"
	if _, _, err := down.PurgeArtifacts(context.Background(), tenant, 50); err == nil {
		t.Fatal("an unreachable service must report an error")
	}
	var retry model.EduArtifact
	h.db.First(&retry, "id = ?", "ema_pretry")
	if retry.PurgeState != "pending" || retry.PurgedAt != nil {
		t.Fatalf("failed purge must stay pending: %+v", retry)
	}
	if n, m, err := h.svc.PurgeArtifacts(context.Background(), tenant, 50); err != nil || n+m != 1 {
		t.Fatalf("retry purge: %d %d %v", n, m, err)
	}

	// Learning period passed since archiving: the scheduled run removes the class's learning records.
	h.db.Model(&model.EduClass{}).Where("id = ?", classID).Update("archived_at", now.AddDate(0, 0, -200))
	if n, err := h.svc.RetentionAll(context.Background()); err != nil || n != 1 || countAttempts() != 0 {
		t.Fatalf("archived class retention: %d %v attempts %d", n, err, countAttempts())
	}
	var runs int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ? AND detail LIKE ?", tenant, "retention.run", "enrollments=0 archived_classes=1 %").Count(&runs)
	if runs != 1 {
		t.Fatalf("retention audit with archived classes: %d", runs)
	}
}
