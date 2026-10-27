package edumath

import (
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

// TestDataRequests (DEV-107, E55): data subject requests go through identity
// verification, impact preview and completion; access returns the student's own
// records once without teacher notes; withdraw stops new processing and keeps
// records; delete also removes learning records in every class while classmates,
// ledger and audit stay; completed deletions are replayed after a restore; only
// org admins handle requests and every step is audited.
func TestDataRequests(t *testing.T) { runDataRequests(t, newHarness(t, startMathService(t))) }

func TestPGDataRequests(t *testing.T) {
	runDataRequests(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func runDataRequests(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	ws := model.PersonalWorkspaceID("u_alice")
	now := time.Now().UTC()
	h.db.Create(&model.Membership{ID: "m_dcarol", TenantID: tenant, UserID: "u_carol", Role: "member", Status: "active", CreatedAt: now})
	h.db.Create(&model.WorkspaceMembership{ID: "wm_dcarol", WorkspaceID: ws, UserID: "u_carol", Role: "editor", CreatedAt: now})
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/roles", map[string]any{"tenantId": tenant, "userId": "u_carol", "role": "teacher"}); c != 200 {
		t.Fatalf("bind carol: %d %s", c, raw)
	}
	pubID, _ := publishG1(t, h)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二(8)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "数据请求", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	code := data(out)["code"]
	stu := []string{"X-Edu-Tenant", tenant}
	for i, u := range []string{"u_bob", "u_eve"} {
		h.call(u, "POST", "/api/edu-join", map[string]any{"code": code})
		h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": u, "status": "granted", "evidenceRef": "form-dr-" + u})
		if c, _, raw, _ := h.call(u, "POST", "/api/edu/student/assignments/"+asID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "dr-attempt-000" + string(rune('1'+i))}, stu...); c != 201 {
			t.Fatalf("attempt %s: %d %s", u, c, raw)
		}
	}
	h.db.Model(&model.EduAttempt{}).Where("tenant_id = ? AND student_user_id = ?", tenant, "u_bob").Update("teacher_note", "teacher-private-note")
	attempts := func(u string) int64 {
		var n int64
		h.db.Model(&model.EduAttempt{}).Where("tenant_id = ? AND student_user_id = ?", tenant, u).Count(&n)
		return n
	}
	roleStatus := func(u string) string {
		var rb model.EduRoleBinding
		h.db.First(&rb, "tenant_id = ? AND user_id = ? AND role = ?", tenant, u, "student")
		return rb.Status
	}
	create := func(u, kind string) string {
		c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/data-requests", map[string]any{"studentUserId": u, "kind": kind, "requester": "guardian", "reason": "监护人书面申请"})
		if c != 201 {
			t.Fatalf("create %s %s: %d %s", kind, u, c, raw)
		}
		return data(out)["id"].(string)
	}
	verify := func(id string) {
		if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/data-requests/"+id+"/verify", map[string]any{"verificationRef": "核验单 DR-01"}); c != 200 {
			t.Fatalf("verify: %d %s", c, raw)
		}
	}

	// Only org admins; validation; unknown students.
	if c, _, _, _ := h.call("u_carol", "GET", "/api/edu/data-requests", nil, "X-Workspace-Id", ws); c != 403 {
		t.Fatalf("teacher list: %d", c)
	}
	if c, _, _, _ := h.call("u_bob", "GET", "/api/edu/data-requests", nil, stu...); c == 200 {
		t.Fatal("student must not list data requests")
	}
	for _, body := range []map[string]any{
		{"studentUserId": "u_bob", "kind": "erase", "requester": "guardian", "reason": "监护人书面申请"},
		{"studentUserId": "u_bob", "kind": "delete", "requester": "parent", "reason": "监护人书面申请"},
		{"studentUserId": "u_bob", "kind": "delete", "requester": "guardian", "reason": "短"},
	} {
		if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/data-requests", body); c != 400 {
			t.Fatalf("invalid %v: %d %s", body, c, raw)
		}
	}
	if c, _, _, _ := h.call("u_alice", "POST", "/api/edu/data-requests", map[string]any{"studentUserId": "u_zed", "kind": "access", "requester": "student", "reason": "本人申请查阅"}); c != 404 {
		t.Fatalf("unknown student: %d", c)
	}

	// Access: verification first, preview, then the package once.
	acc := create("u_bob", "access")
	if c, out, _, _ := h.call("u_alice", "POST", "/api/edu/data-requests", map[string]any{"studentUserId": "u_bob", "kind": "access", "requester": "student", "reason": "本人申请查阅"}); c != 409 || errCode(out) != "DATA_REQUEST_OPEN" {
		t.Fatalf("duplicate open request: %d", c)
	}
	if c, out, _, _ := h.call("u_alice", "POST", "/api/edu/data-requests/"+acc+"/complete", map[string]any{"confirm": true}); c != 409 || errCode(out) != "DATA_REQUEST_NOT_VERIFIED" {
		t.Fatalf("complete before verify: %d", c)
	}
	if c, _, _, _ := h.call("u_alice", "POST", "/api/edu/data-requests/"+acc+"/verify", map[string]any{"verificationRef": "ab"}); c != 400 {
		t.Fatalf("short verification ref: %d", c)
	}
	verify(acc)
	c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/data-requests/"+acc+"/complete", map[string]any{})
	if c != 200 || data(out)["dryRun"] != true {
		t.Fatalf("access preview: %d %s", c, raw)
	}
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/data-requests/"+acc+"/complete", map[string]any{"confirm": true})
	if c != 200 || data(out)["status"] != "completed" {
		t.Fatalf("access: %d %s", c, raw)
	}
	pkg := data(out)["package"].(map[string]any)
	if ats := pkg["attempts"].([]any); len(ats) != 1 || ats[0].(map[string]any)["response"] != "1" {
		t.Fatalf("package attempts: %s", raw)
	}
	if strings.Contains(string(raw), "teacher-private-note") || strings.Contains(string(raw), "u_eve") {
		t.Fatalf("package leaks teacher notes or classmates: %s", raw)
	}
	var stored model.EduDataRequest
	h.db.First(&stored, "id = ?", acc)
	if strings.Contains(stored.Outcome, "response") || stored.ResolvedBy != "u_alice" {
		t.Fatalf("stored outcome must hold counts only: %s", stored.Outcome)
	}
	if c, out, _, _ := h.call("u_alice", "POST", "/api/edu/data-requests/"+acc+"/complete", map[string]any{"confirm": true}); c != 409 || errCode(out) != "DATA_REQUEST_CLOSED" {
		t.Fatalf("complete twice: %d", c)
	}

	// Delete: preview changes nothing; completion stops processing and deletes.
	del := create("u_bob", "delete")
	verify(del)
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/data-requests/"+del+"/complete", map[string]any{})
	imp := data(out)["impact"].(map[string]any)
	if c != 200 || imp["onlineDeleted"].(map[string]any)["learningRows"].(map[string]any)["edu_attempts"] != float64(1) || imp["stopNewProcessing"].(map[string]any)["studentRoleRevoked"] != float64(1) {
		t.Fatalf("delete preview: %d %s", c, raw)
	}
	if attempts("u_bob") != 1 || roleStatus("u_bob") != "active" {
		t.Fatal("preview must not change anything")
	}
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/data-requests/"+del+"/complete", map[string]any{"confirm": true})
	if c != 200 {
		t.Fatalf("delete: %d %s", c, raw)
	}
	o := data(out)["outcome"].(map[string]any)
	if o["backupExpiry"] == nil || o["legalRetention"] == nil || o["cacheCleared"] == nil {
		t.Fatalf("delete outcome categories: %s", raw)
	}
	if attempts("u_bob") != 0 || attempts("u_eve") != 1 || roleStatus("u_bob") != "revoked" {
		t.Fatalf("after delete: bob %d eve %d role %s", attempts("u_bob"), attempts("u_eve"), roleStatus("u_bob"))
	}
	if c, _, _, _ := h.call("u_bob", "GET", "/api/edu/student/progress", nil, stu...); c == 200 {
		t.Fatal("deleted student keeps access")
	}
	if c, _, _, _ := h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": code}); c == 200 {
		t.Fatal("a join code must not undo the deletion")
	}

	// Withdraw: stops processing, records stay.
	wd := create("u_eve", "withdraw")
	verify(wd)
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/data-requests/"+wd+"/complete", map[string]any{"confirm": true}); c != 200 {
		t.Fatalf("withdraw: %d %s", c, raw)
	}
	var cr model.EduConsentRecord
	h.db.First(&cr, "tenant_id = ? AND student_user_id = ?", tenant, "u_eve")
	if attempts("u_eve") != 1 || roleStatus("u_eve") != "revoked" || cr.Status != "withdrawn" {
		t.Fatalf("after withdraw: attempts %d role %s consent %s", attempts("u_eve"), roleStatus("u_eve"), cr.Status)
	}

	// Reject with a reason.
	rj := create("u_eve", "access")
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/data-requests/"+rj+"/reject", map[string]any{"reason": "无法核验申请人身份"}); c != 200 {
		t.Fatalf("reject: %d %s", c, raw)
	}

	// A restore brings bob's rows back; replay applies the deletion again.
	h.db.Create(&model.EduAttempt{ID: "eat_restored", TenantID: tenant, AssignmentID: asID, StudentUserID: "u_bob", ClientKey: "restored-0001", Item: "main", Kind: "answer", Response: "2", CreatedAt: now})
	h.db.Model(&model.EduRoleBinding{}).Where("tenant_id = ? AND user_id = ? AND role = ?", tenant, "u_bob", "student").Update("status", "active")
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/data-requests/replay", map[string]any{})
	if c != 200 || data(out)["markers"] != float64(1) || data(out)["statesReverted"] != float64(1) || attempts("u_bob") != 1 {
		t.Fatalf("replay preview: %d %s", c, raw)
	}
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/data-requests/replay", map[string]any{"confirm": true})
	if c != 200 || attempts("u_bob") != 0 || roleStatus("u_bob") != "revoked" {
		t.Fatalf("replay: %d %s", c, raw)
	}

	// Audit: every step, counts only; listed in the institution log.
	for action, want := range map[string]int64{"datareq.create": 4, "datareq.verify": 3, "datareq.complete": 3, "datareq.reject": 1, "datareq.replay": 1} {
		var n int64
		h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ?", tenant, action).Count(&n)
		if n != want {
			t.Fatalf("audit %s: %d want %d", action, n, want)
		}
	}
	var leak int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action LIKE ? AND detail LIKE ?", tenant, "datareq.%", "%teacher-private-note%").Count(&leak)
	if leak != 0 {
		t.Fatal("audit must not carry record content")
	}
	_, out, raw, _ = h.call("u_alice", "GET", "/api/edu/org/access-log", nil)
	if !strings.Contains(string(raw), "datareq.complete") || !strings.Contains(string(raw), "kind=delete") {
		t.Fatalf("access log: %s", raw)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/data-requests", nil)
	if rs, _ := out["data"].([]any); len(rs) != 4 {
		t.Fatalf("list: %v", out["data"])
	}
}
