package edumath

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

// TestOrgAdmin (DEV-102, E52): the institution overview is aggregate only; an org_admin who
// does not teach a class reaches individual student records only with a stated purpose,
// and every such access is audited and listed; teachers of the class are unaffected.
func TestOrgAdmin(t *testing.T) { runOrgAdmin(t, newHarness(t, startMathService(t))) }

func TestPGOrgAdmin(t *testing.T) { runOrgAdmin(t, newHarnessDB(t, startMathService(t), pgTestDB(t))) }

func runOrgAdmin(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	ws := model.PersonalWorkspaceID("u_alice")
	now := time.Now().UTC()
	h.db.Create(&model.Membership{ID: "m_odave", TenantID: tenant, UserID: "u_dave", Role: "member", Status: "active", CreatedAt: now})
	h.db.Create(&model.WorkspaceMembership{ID: "wm_odave", WorkspaceID: ws, UserID: "u_dave", Role: "editor", CreatedAt: now})
	h.db.Create(&model.Membership{ID: "m_ocarol", TenantID: tenant, UserID: "u_carol", Role: "member", Status: "active", CreatedAt: now})
	h.db.Create(&model.WorkspaceMembership{ID: "wm_ocarol", WorkspaceID: ws, UserID: "u_carol", Role: "editor", CreatedAt: now})
	for u, r := range map[string]string{"u_dave": "org_admin", "u_carol": "teacher"} {
		if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/roles", map[string]any{"tenantId": tenant, "userId": u, "role": r}); c != 200 {
			t.Fatalf("bind %s: %d %s", u, c, raw)
		}
	}
	wsh := []string{"X-Workspace-Id", ws}
	pubID, _ := publishG1(t, h)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二(7)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "机构管理", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-org-1"})
	h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "bob-org-0001"}, "X-Edu-Tenant", tenant)

	// Aggregate overview: counts only, no student identity.
	code, out, raw, _ := h.call("u_dave", "GET", "/api/edu/org/overview", nil, wsh...)
	if code != 200 {
		t.Fatalf("overview: %d %s", code, raw)
	}
	d := data(out)
	st := d["staff"].(map[string]any)
	sd := d["students"].(map[string]any)
	if st["teacher"] != float64(2) || st["org_admin"] != float64(1) || sd["enrolled"] != float64(1) || sd["guardianConsent"] != float64(1) || sd["consentMissing"] != float64(0) {
		t.Fatalf("overview counts: %s", raw)
	}
	ks := d["classes"].([]any)
	k := ks[0].(map[string]any)
	if len(ks) != 1 || k["students"] != float64(1) || k["teachers"] != float64(1) || k["activeAssignments"] != float64(1) || k["youTeach"] != false {
		t.Fatalf("class aggregate: %s", raw)
	}
	if strings.Contains(string(raw), "u_bob") || strings.Contains(string(raw), "form-org-1") {
		t.Fatalf("overview must not name students or consent evidence: %s", raw)
	}
	if d["region"].(map[string]any)["state"] != "not_configured" {
		t.Fatalf("region must say not configured: %s", raw)
	}
	if c, _, _, _ := h.call("u_alice", "GET", "/api/edu/org/overview", nil); c != 200 {
		t.Fatalf("tenant owner overview: %d", c)
	}
	for _, u := range []string{"u_carol", "u_bob"} {
		if c, _, _, _ := h.call(u, "GET", "/api/edu/org/overview", nil, wsh...); c != 403 {
			t.Fatalf("%s overview must be 403, got %d", u, c)
		}
		if c, _, _, _ := h.call(u, "GET", "/api/edu/org/access-log", nil, wsh...); c != 403 {
			t.Fatalf("%s access log must be 403, got %d", u, c)
		}
	}

	audits := func() int64 {
		var n int64
		h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ?", tenant, "org.individual_access").Count(&n)
		return n
	}
	// Individual records through org_admin need a stated purpose.
	purpose := "家长投诉核查（工单 17）"
	enc := url.QueryEscape(purpose)
	paths := []string{"/api/edu/classes/" + classID + "/roster", "/api/edu/classes/" + classID + "/roster-entries", "/api/edu/assignments/" + asID + "/attempts"}
	for _, p := range paths {
		for _, hdr := range [][]string{wsh, append(append([]string{}, wsh...), accessPurposeHeader, "ab"), append(append([]string{}, wsh...), accessPurposeHeader, url.QueryEscape("<b>四个字</b>"))} {
			code, out, raw, _ := h.call("u_dave", "GET", p, nil, hdr...)
			if code != 403 || errCode(out) != "ACCESS_PURPOSE_REQUIRED" {
				t.Fatalf("%s without a valid purpose must be 403 ACCESS_PURPOSE_REQUIRED: %d %s", p, code, raw)
			}
		}
	}
	if audits() != 0 {
		t.Fatal("refused accesses must not be audited as access")
	}
	code, _, raw, _ = h.call("u_dave", "GET", paths[0], nil, append(append([]string{}, wsh...), accessPurposeHeader, enc)...)
	if code != 200 || !strings.Contains(string(raw), "u_bob") {
		t.Fatalf("roster with purpose: %d %s", code, raw)
	}
	if c, _, raw, _ := h.call("u_dave", "GET", paths[2], nil, append(append([]string{}, wsh...), accessPurposeHeader, enc)...); c != 200 {
		t.Fatalf("attempts with purpose: %d %s", c, raw)
	}
	if audits() != 2 {
		t.Fatalf("each access must be audited, got %d", audits())
	}
	// The class teacher is not asked for a purpose and is not logged as individual access.
	if c, _, _, _ := h.call("u_alice", "GET", paths[0], nil); c != 200 {
		t.Fatalf("teacher roster: %d", c)
	}
	if c, _, _, _ := h.call("u_alice", "GET", paths[2], nil); c != 200 {
		t.Fatalf("teacher attempts: %d", c)
	}
	if audits() != 2 {
		t.Fatalf("teacher access must not be logged as org access, got %d", audits())
	}
	// A teacher who does not teach the class still cannot reach it, purpose or not.
	if c, _, _, _ := h.call("u_carol", "GET", paths[0], nil, append(append([]string{}, wsh...), accessPurposeHeader, enc)...); c != 404 {
		t.Fatalf("non-teaching teacher roster must stay 404, got %d", c)
	}

	// Access log lists the accesses with their purpose; consent evidence is not shown.
	code, out, raw, _ = h.call("u_dave", "GET", "/api/edu/org/access-log", nil, wsh...)
	if code != 200 {
		t.Fatalf("access log: %d %s", code, raw)
	}
	var seen int
	for _, r := range out["data"].([]any) {
		m := r.(map[string]any)
		if m["action"] == "org.individual_access" {
			seen++
			if det, _ := m["detail"].(string); !strings.Contains(det, purpose) || m["actorId"] != "u_dave" || m["subject"] != classID {
				t.Fatalf("access row: %v", m)
			}
		}
	}
	if seen != 2 || strings.Contains(string(raw), "form-org-1") {
		t.Fatalf("access log: %s", raw)
	}
}
