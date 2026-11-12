package edumath

import (
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

func TestGuardianOnlineConsent(t *testing.T) {
	runGuardianOnlineConsent(t, newHarness(t, "http://127.0.0.1:1"))
}

func TestPGGuardianOnlineConsent(t *testing.T) {
	runGuardianOnlineConsent(t, newHarnessDB(t, "http://127.0.0.1:1", pgTestDB(t)))
}

func runGuardianOnlineConsent(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	ws := model.PersonalWorkspaceID("u_alice")
	now := time.Now().UTC()
	h.db.Create(&model.Membership{ID: "m_carol_ol", TenantID: tenant, UserID: "u_carol", Role: "member", Status: "active", CreatedAt: now})
	h.db.Create(&model.WorkspaceMembership{ID: "wm_carol_ol", WorkspaceID: ws, UserID: "u_carol", Role: "editor", CreatedAt: now})
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/roles", map[string]any{"tenantId": tenant, "userId": "u_carol", "role": "teacher"}); c != 200 {
		t.Fatalf("bind carol: %d %s", c, raw)
	}
	carol := []string{"X-Workspace-Id", ws}

	c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二线上同意班"})
	if c != 201 {
		t.Fatalf("class: %d %s", c, raw)
	}
	classID := data(out)["id"].(string)
	_, out, raw, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 1, "maxUses": 2})
	code := data(out)["code"].(string)
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": code}); c != 200 {
		t.Fatalf("join: %d %s", c, raw)
	}

	// Teachers cannot issue an online token (AC-04).
	if c, _, _, _ := h.call("u_carol", "POST", "/api/edu/consents/online-invites", map[string]any{"studentUserId": "u_bob", "purpose": "grant"}, carol...); c != 403 {
		t.Fatalf("teacher issuing token must be 403, got %d", c)
	}
	if c, _, _, _ := h.call("", "POST", "/api/edu/consents/online-invites", map[string]any{"studentUserId": "u_bob", "purpose": "grant"}); c != 401 {
		t.Fatalf("anonymous issue must be 401, got %d", c)
	}
	// Withdraw before a grant is refused.
	if c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/consents/online-invites", map[string]any{"studentUserId": "u_bob", "purpose": "withdraw"}); c != 409 || errCode(out) != "CONSENT_NOT_GRANTED" {
		t.Fatalf("withdraw without grant: %d %s", c, raw)
	}

	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/consents/online-invites", map[string]any{"studentUserId": "u_bob", "purpose": "grant"})
	if c != 201 {
		t.Fatalf("issue: %d %s", c, raw)
	}
	token, _ := data(out)["token"].(string)
	if token == "" || !strings.HasPrefix(token, "ect_") || strings.Contains(string(raw), "tokenHash") || strings.Contains(string(raw), `"token_hash"`) {
		t.Fatalf("token must be returned once and the hash must not: %s", raw)
	}
	if data(out)["noticeVersion"] != guardianNoticeVersion || data(out)["notice"] == "" {
		t.Fatalf("notice: %s", raw)
	}

	// No ack, wrong purpose, and a garbage token are refused without naming the student.
	if c, _, raw, _ := h.call("", "POST", "/api/edu-guardian-consent", map[string]any{"token": token, "status": "granted", "ack": false}); c != 400 || strings.Contains(string(raw), "u_bob") {
		t.Fatalf("ack required: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("", "POST", "/api/edu-guardian-consent", map[string]any{"token": token, "status": "withdrawn", "ack": true}); c != 404 || strings.Contains(string(raw), "u_bob") {
		t.Fatalf("purpose mismatch: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("", "POST", "/api/edu-guardian-consent", map[string]any{"token": "ect_deadbeef." + strings.Repeat("ab", 24), "status": "granted", "ack": true}); c != 404 || strings.Contains(string(raw), "u_bob") {
		t.Fatalf("unknown token: %d %s", c, raw)
	}

	c, out, raw, _ = h.call("", "POST", "/api/edu-guardian-consent", map[string]any{"token": token, "status": "granted", "ack": true})
	if c != 200 || data(out)["status"] != "granted" || data(out)["channel"] != "online" || strings.Contains(string(raw), "u_bob") {
		t.Fatalf("confirm: %d %s", c, raw)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/classes/"+classID+"/roster", nil)
	ro, _ := out["data"].([]any)
	if len(ro) != 1 || ro[0].(map[string]any)["consentOk"] != true {
		t.Fatalf("roster after online grant: %v", out["data"])
	}
	var rec model.EduConsentRecord
	if err := h.db.First(&rec, "tenant_id = ? AND student_user_id = ?", tenant, "u_bob").Error; err != nil || rec.Status != "granted" || !strings.HasPrefix(rec.EvidenceRef, "online:ect_") || rec.RecordedBy != "guardian:online" {
		t.Fatalf("record: %+v %v", rec, err)
	}
	// Reusing the token fails; the grant stays.
	if c, _, _, _ := h.call("", "POST", "/api/edu-guardian-consent", map[string]any{"token": token, "status": "granted", "ack": true}); c != 404 {
		t.Fatalf("reuse: %d", c)
	}

	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/consents/online-invites", map[string]any{"studentUserId": "u_bob", "purpose": "withdraw"})
	if c != 201 {
		t.Fatalf("issue withdraw: %d %s", c, raw)
	}
	wtoken, _ := data(out)["token"].(string)
	invID := strings.Split(wtoken, ".")[0]
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/consents/online-invites/"+invID+"/revoke", nil); c != 200 {
		t.Fatalf("revoke: %d %s", c, raw)
	}
	if c, _, _, _ := h.call("", "POST", "/api/edu-guardian-consent", map[string]any{"token": wtoken, "status": "withdrawn", "ack": true}); c != 404 {
		t.Fatalf("revoked token must be 404, got %d", c)
	}

	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/consents/online-invites", map[string]any{"studentUserId": "u_bob", "purpose": "withdraw"})
	if c != 201 {
		t.Fatalf("issue withdraw 2: %d %s", c, raw)
	}
	c, out, raw, _ = h.call("", "POST", "/api/edu-guardian-consent", map[string]any{"token": data(out)["token"], "status": "withdrawn", "ack": true})
	if c != 200 || data(out)["status"] != "withdrawn" {
		t.Fatalf("withdraw confirm: %d %s", c, raw)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/classes/"+classID+"/roster", nil)
	if out["data"].([]any)[0].(map[string]any)["consentOk"] != false {
		t.Fatal("withdraw must close the gate")
	}

	// Offline evidenceRef still works afterwards.
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-offline-1"}); c != 200 {
		t.Fatalf("offline grant: %d %s", c, raw)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/classes/"+classID+"/roster", nil)
	if out["data"].([]any)[0].(map[string]any)["consentOk"] != true {
		t.Fatal("offline grant must reopen the gate")
	}

	// An expired token is refused.
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/consents/online-invites", map[string]any{"studentUserId": "u_bob", "purpose": "withdraw"})
	if c != 201 {
		t.Fatalf("issue expired: %d %s", c, raw)
	}
	etoken, _ := data(out)["token"].(string)
	eid := strings.Split(etoken, ".")[0]
	if err := h.db.Model(&model.EduConsentInvite{}).Where("id = ?", eid).Update("expires_at", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if c, _, _, _ := h.call("", "POST", "/api/edu-guardian-consent", map[string]any{"token": etoken, "status": "withdrawn", "ack": true}); c != 404 {
		t.Fatalf("expired: %d", c)
	}
}
