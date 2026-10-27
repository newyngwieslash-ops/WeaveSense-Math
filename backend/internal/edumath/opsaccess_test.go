package edumath

import (
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

// TestOpsAccess (DEV-108, E56): operators have no standing access; an org admin
// grants a purpose, scope and expiry (at most 72 h); job scope shows metadata
// only, class scope shows attempt metadata and answer text only when granted;
// other classes, other routes, expiry and revocation all refuse; every grant,
// revoke and read is audited and listed for the institution.
func TestOpsAccess(t *testing.T) { runOpsAccess(t, newHarness(t, startMathService(t))) }

func TestPGOpsAccess(t *testing.T) {
	runOpsAccess(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func runOpsAccess(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	mk := func(name string) (string, string) {
		_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": name})
		k := data(out)["id"].(string)
		_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+k+"/assignments", map[string]any{"publicationId": pubID, "title": "运维", "answerPolicy": "teacher_release"})
		return k, data(out)["id"].(string)
	}
	classA, asA := mk("高一(1)班")
	classB, _ := mk("高一(2)班")
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes/"+classA+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-ops-bob"})
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu/student/assignments/"+asA+"/attempts", map[string]any{"kind": "answer", "response": "secret-answer-text", "clientKey": "ops-attempt-0001"}, "X-Edu-Tenant", tenant); c != 201 {
		t.Fatalf("attempt: %d %s", c, raw)
	}
	opsHdr := func(grant string) []string { return []string{"X-Edu-Tenant", tenant, "X-Edu-Ops-Grant", grant} }

	// No standing access.
	if c, _, _, _ := h.call("u_ops", "GET", "/api/edu/ops/jobs", nil, opsHdr("eog_none")...); c != 403 {
		t.Fatalf("operator without grant: %d", c)
	}
	// Validation.
	for _, body := range []map[string]any{
		{"operatorUserId": "u_ops", "scope": "jobs", "purpose": "排查作业失败", "hours": 73},
		{"operatorUserId": "u_ops", "scope": "jobs", "purpose": "排查作业失败", "hours": 4, "includeContent": true},
		{"operatorUserId": "u_ops", "scope": "all", "purpose": "排查作业失败", "hours": 4},
		{"operatorUserId": "u_ops", "scope": "jobs", "purpose": "短", "hours": 4},
		{"operatorUserId": "u_alice", "scope": "jobs", "purpose": "排查作业失败", "hours": 4},
	} {
		if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/ops-grants", body); c != 400 {
			t.Fatalf("invalid grant %v: %d %s", body, c, raw)
		}
	}
	if c, _, _, _ := h.call("u_alice", "POST", "/api/edu/ops-grants", map[string]any{"operatorUserId": "u_ops", "scope": "class", "classId": "ecl_other", "purpose": "排查作业失败", "hours": 4}); c != 404 {
		t.Fatalf("unknown class: %d", c)
	}
	if c, _, _, _ := h.call("u_bob", "POST", "/api/edu/ops-grants", map[string]any{"operatorUserId": "u_ops", "scope": "jobs", "purpose": "排查作业失败", "hours": 4}, "X-Edu-Tenant", tenant); c == 201 {
		t.Fatal("student must not grant access")
	}
	grant := func(body map[string]any) string {
		c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/ops-grants", body)
		if c != 201 {
			t.Fatalf("grant: %d %s", c, raw)
		}
		return data(out)["id"].(string)
	}

	// Job scope: metadata only.
	gj := grant(map[string]any{"operatorUserId": "u_ops", "scope": "jobs", "purpose": "排查作业失败 工单 OPS-7", "hours": 4})
	c, _, raw, _ := h.call("u_ops", "GET", "/api/edu/ops/jobs", nil, opsHdr(gj)...)
	if c != 200 || !strings.Contains(string(raw), `"status"`) || strings.Contains(string(raw), "inputHash") || strings.Contains(string(raw), "priceBook") {
		t.Fatalf("ops jobs: %d %s", c, raw)
	}
	if c, _, _, _ := h.call("u_ops", "GET", "/api/edu/ops/classes/"+classA+"/attempts", nil, opsHdr(gj)...); c != 403 {
		t.Fatalf("job scope reading attempts: %d", c)
	}
	// The grant opens nothing else.
	if c, _, _, _ := h.call("u_ops", "GET", "/api/edu/classes/"+classA+"/roster", nil, opsHdr(gj)...); c == 200 {
		t.Fatal("operator reached a staff route")
	}
	if c, _, _, _ := h.call("u_ops", "GET", "/api/edu/student/assignments", nil, opsHdr(gj)...); c == 200 {
		t.Fatal("operator reached a student route")
	}
	if c, _, _, _ := h.call("u_eve", "GET", "/api/edu/ops/jobs", nil, opsHdr(gj)...); c != 403 {
		t.Fatalf("another user using the grant: %d", c)
	}

	// Class scope without content: no answer text; other class refused.
	gm := grant(map[string]any{"operatorUserId": "u_ops", "scope": "class", "classId": classA, "purpose": "排查判分异常", "hours": 2})
	c, _, raw, _ = h.call("u_ops", "GET", "/api/edu/ops/classes/"+classA+"/attempts", nil, opsHdr(gm)...)
	if c != 200 || strings.Contains(string(raw), "secret-answer-text") || !strings.Contains(string(raw), `"result"`) {
		t.Fatalf("metadata attempts: %d %s", c, raw)
	}
	if c, _, _, _ := h.call("u_ops", "GET", "/api/edu/ops/classes/"+classB+"/attempts", nil, opsHdr(gm)...); c != 403 {
		t.Fatalf("other class: %d", c)
	}
	// Class scope with content.
	gc := grant(map[string]any{"operatorUserId": "u_ops", "scope": "class", "classId": classA, "includeContent": true, "purpose": "核对作答识别问题", "hours": 1})
	c, _, raw, _ = h.call("u_ops", "GET", "/api/edu/ops/classes/"+classA+"/attempts", nil, opsHdr(gc)...)
	if c != 200 || !strings.Contains(string(raw), "secret-answer-text") {
		t.Fatalf("content attempts: %d %s", c, raw)
	}

	// Revocation and expiry end access at once.
	if c, _, _, _ := h.call("u_alice", "POST", "/api/edu/ops-grants/"+gc+"/revoke", map[string]any{"reason": "x"}); c != 400 {
		t.Fatalf("revoke without reason: %d", c)
	}
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/ops-grants/"+gc+"/revoke", map[string]any{"reason": "排查已完成"}); c != 200 {
		t.Fatalf("revoke: %d %s", c, raw)
	}
	if c, _, _, _ := h.call("u_ops", "GET", "/api/edu/ops/classes/"+classA+"/attempts", nil, opsHdr(gc)...); c != 403 {
		t.Fatalf("revoked grant: %d", c)
	}
	h.now.Store(time.Now().Add(3 * time.Hour).Unix())
	if c, _, _, _ := h.call("u_ops", "GET", "/api/edu/ops/classes/"+classA+"/attempts", nil, opsHdr(gm)...); c != 403 {
		t.Fatalf("expired grant: %d", c)
	}
	_, out, _, _ = h.call("u_alice", "GET", "/api/edu/ops-grants", nil)
	states := map[string]string{}
	for _, r := range out["data"].([]any) {
		m := r.(map[string]any)
		states[m["grant"].(map[string]any)["id"].(string)] = m["effective"].(string)
	}
	if states[gj] != "active" || states[gm] != "expired" || states[gc] != "revoked" {
		t.Fatalf("grant states: %v", states)
	}
	h.now.Store(0)

	// Audit: grants, revoke and each read with the purpose; listed for the institution.
	for action, want := range map[string]int64{"ops.grant": 3, "ops.revoke": 1, "ops.read": 3} {
		var n int64
		h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ?", tenant, action).Count(&n)
		if n != want {
			t.Fatalf("audit %s: %d want %d", action, n, want)
		}
	}
	var leak int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND detail LIKE ?", tenant, "%secret-answer-text%").Count(&leak)
	if leak != 0 {
		t.Fatal("audit must not carry answer text")
	}
	_, _, raw, _ = h.call("u_alice", "GET", "/api/edu/org/access-log", nil)
	if !strings.Contains(string(raw), "ops.read") || !strings.Contains(string(raw), "工单 OPS-7") {
		t.Fatalf("access log: %s", raw)
	}
}
