package edumath

import (
	"html"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

// TestExport (DEV-103, E53): content export is bound to the published revision and its
// hashes, rechecks source rights at export time, defaults to the student version built
// from the guarded student projection, and the learning report is aggregate only with
// small counts suppressed; neither file names a student and every export is audited.
func TestExport(t *testing.T) { runExport(t, newHarness(t, startMathService(t))) }

func TestPGExport(t *testing.T) { runExport(t, newHarnessDB(t, startMathService(t), pgTestDB(t))) }

func runExport(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	ws := model.PersonalWorkspaceID("u_alice")
	now := time.Now().UTC()
	h.db.Create(&model.Membership{ID: "m_xdave", TenantID: tenant, UserID: "u_dave", Role: "member", Status: "active", CreatedAt: now})
	h.db.Create(&model.WorkspaceMembership{ID: "wm_xdave", WorkspaceID: ws, UserID: "u_dave", Role: "editor", CreatedAt: now})
	h.db.Create(&model.Membership{ID: "m_xcarol", TenantID: tenant, UserID: "u_carol", Role: "member", Status: "active", CreatedAt: now})
	h.db.Create(&model.WorkspaceMembership{ID: "wm_xcarol", WorkspaceID: ws, UserID: "u_carol", Role: "editor", CreatedAt: now})
	for u, r := range map[string]string{"u_dave": "org_admin", "u_carol": "teacher"} {
		if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/roles", map[string]any{"tenantId": tenant, "userId": u, "role": r}); c != 200 {
			t.Fatalf("bind %s: %d %s", u, c, raw)
		}
	}
	wsh := []string{"X-Workspace-Id", ws}
	pubID, tl := publishG1(t, h)
	var pub model.EduPublication
	h.db.First(&pub, "id = ?", pubID)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二(8)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "导出检查", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-exp-1"})
	h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "bob-exp-0001"}, "X-Edu-Tenant", tenant)

	markers := []string{}
	for _, m := range []string{tl.Answer.Plain, tl.Answer.Exact, tl.CheckAnswerKey.Plain, tl.CheckAnswerKey.Exact} {
		if len(m) >= 2 {
			markers = append(markers, m, html.EscapeString(m))
		}
	}

	// Student version by default: guarded projection, manifest bound to the publication.
	code, out, raw, _ := h.call("u_alice", "GET", "/api/edu/publications/"+pubID+"/export", nil)
	if code != 200 {
		t.Fatalf("student export: %d %s", code, raw)
	}
	d := data(out)
	doc := d["html"].(string)
	man := d["manifest"].(map[string]any)
	if man["audience"] != "student" || man["lessonHash"] != pub.LessonHash || man["reviewRecordId"] != pub.ReviewRecordID || man["solutionHash"] == "" || man["problemInputHash"] == "" {
		t.Fatalf("manifest: %v", man)
	}
	if !strings.Contains(doc, pub.LessonHash) || !strings.Contains(doc, "学生版") || !strings.Contains(doc, "未包含的内容") || !strings.Contains(doc, "教师原创") || !strings.Contains(doc, "Apache-2.0") {
		t.Fatalf("student html sections missing")
	}
	for _, bad := range []string{"<script", "http://", "https://", "src=", "u_bob", "bob-exp", "答案</h2>", "讲述："} {
		if strings.Contains(doc, bad) {
			t.Fatalf("student html carries %q", bad)
		}
	}
	// The problem statement and self-test prompt are public givens; check everything else.
	rest := doc
	for _, sec := range []string{"<h2>题目</h2><p>", "<h2>自测题</h2><p>"} {
		if i := strings.Index(rest, sec); i >= 0 {
			j := strings.Index(rest[i+len(sec):], "</p>")
			rest = rest[:i] + rest[i+len(sec)+j:]
		}
	}
	for _, m := range markers {
		if strings.Contains(rest, m) {
			t.Fatalf("student export leaks answer marker %q", m)
		}
	}
	if len(d["omitted"].([]any)) != 5 {
		t.Fatalf("omitted fields: %v", d["omitted"])
	}

	// Teacher version only on request; it carries the answer and the full explanation.
	code, out, raw, _ = h.call("u_alice", "GET", "/api/edu/publications/"+pubID+"/export?audience=teacher", nil)
	if code != 200 {
		t.Fatalf("teacher export: %d %s", code, raw)
	}
	tdoc := data(out)["html"].(string)
	if !strings.Contains(tdoc, "教师版") || !strings.Contains(tdoc, html.EscapeString(tl.Answer.Plain)) || !strings.Contains(tdoc, "讲述：") || strings.Contains(tdoc, "<script") {
		t.Fatalf("teacher html incomplete")
	}
	if c, _, _, _ := h.call("u_alice", "GET", "/api/edu/publications/"+pubID+"/export?audience=all", nil); c != 400 {
		t.Fatalf("unknown audience: %d", c)
	}
	// Students and other tenants cannot export.
	if c, _, _, _ := h.call("u_bob", "GET", "/api/edu/publications/"+pubID+"/export", nil, "X-Edu-Tenant", tenant); c == 200 {
		t.Fatal("student must not export content")
	}
	if c, _, _, _ := h.call("u_eve", "GET", "/api/edu/publications/"+pubID+"/export", nil); c == 200 {
		t.Fatal("outsider must not export content")
	}

	// Rights are rechecked at export time.
	var lr model.EduLessonRevision
	h.db.First(&lr, "id = ?", pub.LessonRevisionID)
	var pr model.EduProblemRevision
	h.db.First(&pr, "id = ?", lr.ProblemRevisionID)
	_, out, raw, _ = h.call("u_alice", "POST", "/api/edu/source-assets", map[string]any{"title": "教材例题", "kind": "textbook", "attribution": "人教A版 选择性必修一", "licenseRef": "校内教学使用", "allowStudent": true, "allowDerivative": true})
	srcID, _ := data(out)["id"].(string)
	if c, _, raw, _ := h.call("u_alice", "PUT", "/api/edu/problems/"+pr.ProblemID+"/source", map[string]any{"sourceAssetId": srcID}); c != 200 {
		t.Fatalf("link source: %d %s", c, raw)
	}
	code, out, raw, _ = h.call("u_alice", "GET", "/api/edu/publications/"+pubID+"/export", nil)
	if code != 409 || errCode(out) != "RIGHTS_RESTRICTED" {
		t.Fatalf("export without export right: %d %s", code, raw)
	}
	if c, _, raw, _ := h.call("u_alice", "PUT", "/api/edu/source-assets/"+srcID+"/rights", map[string]any{"allowStudent": false, "allowDerivative": true, "allowExport": true, "attribution": "人教A版 选择性必修一", "licenseRef": "校内教学使用", "reason": "出版社授权导出，未授权学生使用"}); c != 200 {
		t.Fatalf("update rights: %d %s", c, raw)
	}
	if c, out, raw, _ := h.call("u_alice", "GET", "/api/edu/publications/"+pubID+"/export", nil); c != 409 || errCode(out) != "RIGHTS_RESTRICTED" {
		t.Fatalf("student export needs student use: %d %s", c, raw)
	}
	code, out, raw, _ = h.call("u_alice", "GET", "/api/edu/publications/"+pubID+"/export?audience=teacher", nil)
	if code != 200 || !strings.Contains(data(out)["html"].(string), "人教A版 选择性必修一") || !strings.Contains(data(out)["html"].(string), "校内教学使用") {
		t.Fatalf("teacher export with attribution: %d %s", code, raw)
	}

	// Learning report: aggregate only, small counts suppressed; org admin needs no purpose.
	code, out, raw, _ = h.call("u_alice", "GET", "/api/edu/assignments/"+asID+"/report-export", nil)
	if code != 200 {
		t.Fatalf("report export: %d %s", code, raw)
	}
	rep := data(out)["report"].(map[string]any)
	rdoc := data(out)["html"].(string)
	if rep["students"] != float64(1) || rep["distribution"].(map[string]any)["completed_independently"] != "少于3" && rep["distribution"].(map[string]any)["attempted"] != "少于3" {
		t.Fatalf("report counts: %s", raw)
	}
	if strings.Contains(string(raw), "u_bob") || strings.Contains(string(raw), "bob-exp") || strings.Contains(string(raw), "form-exp") || !strings.Contains(rdoc, "高二(8)班") || !strings.Contains(rdoc, "少于3") {
		t.Fatalf("report must be aggregate: %s", raw)
	}
	if c, _, raw, _ := h.call("u_dave", "GET", "/api/edu/assignments/"+asID+"/report-export", nil, wsh...); c != 200 {
		t.Fatalf("org admin aggregate report: %d %s", c, raw)
	}
	if c, _, _, _ := h.call("u_carol", "GET", "/api/edu/assignments/"+asID+"/report-export", nil, wsh...); c != 404 {
		t.Fatalf("teacher of another class: %d", c)
	}
	if c, _, _, _ := h.call("u_bob", "GET", "/api/edu/assignments/"+asID+"/report-export", nil, "X-Edu-Tenant", tenant); c == 200 {
		t.Fatal("student must not export the report")
	}

	// A withdrawn publication is not exported.
	h.db.Model(&model.EduPublication{}).Where("id = ?", pubID).Update("state", "withdrawn")
	if c, out, raw, _ := h.call("u_alice", "GET", "/api/edu/publications/"+pubID+"/export?audience=teacher", nil); c != 409 || errCode(out) != "REVISION_CONFLICT" {
		t.Fatalf("withdrawn export: %d %s", c, raw)
	}

	var n int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ?", tenant, "export.content").Count(&n)
	var r int64
	h.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ?", tenant, "export.report").Count(&r)
	if n != 3 || r != 2 {
		t.Fatalf("export audit: content %d report %d", n, r)
	}
}
