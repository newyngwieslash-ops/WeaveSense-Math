package edumath

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm/schema"

	"yingce/backend/internal/model"
)

// TestDemoClass (DEV-067, E47): the demo class with fictional learning records is labeled demo
// everywhere, lives only in its own tables, and leaves every real table, metric, dashboard, report
// and student view of the tenant byte-for-byte unchanged; it is per workspace, resettable and
// removable by the teacher, invisible to other tenants and to students. Real math service.
func TestDemoClass(t *testing.T) { runDemoClass(t, newHarness(t, startMathService(t))) }

func TestPGDemoClass(t *testing.T) {
	runDemoClass(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func runDemoClass(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二(9)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "演示隔离", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-demo"})
	stu := []string{"X-Edu-Tenant", tenant}
	h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "bob-demo-001"}, stu...)
	h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/help", map[string]any{"kind": "question", "beatId": "s1", "message": "这一步不懂", "clientKey": "bob-demo-help1"}, stu...)

	// Every real table of the tenant (all edu tables except the demo ones and the audit trail) plus platform usage.
	digest := func() map[string]string {
		res := map[string]string{}
		for _, m := range model.EduModels() {
			s, err := schema.Parse(m, &sync.Map{}, h.db.NamingStrategy)
			if err != nil {
				t.Fatal(err)
			}
			if s.LookUpField("TenantID") == nil || strings.HasPrefix(s.Table, "edu_demo_") || s.Table == "edu_audit_events" {
				continue
			}
			var rows []map[string]any
			h.db.Table(s.Table).Where("tenant_id = ?", tenant).Order("id").Find(&rows)
			b, _ := json.Marshal(rows)
			sum := sha256.Sum256(b)
			res[s.Table] = hex.EncodeToString(sum[:])
		}
		for _, tbl := range []string{"flow_usage_records", "flow_budget_reservations"} {
			var n int64
			if err := h.db.Table(tbl).Where("tenant_id = ?", tenant).Count(&n).Error; err != nil {
				t.Fatalf("count %s: %v", tbl, err)
			}
			res[tbl] = fmt.Sprint(n)
		}
		return res
	}
	// Every real view: staff lists, dashboards, reports, and the student's own views.
	views := []struct{ user, path string }{
		{"u_alice", "/api/edu/classes"}, {"u_alice", "/api/edu/classes/" + classID + "/roster"},
		{"u_alice", "/api/edu/assignments/" + asID + "/dashboard"}, {"u_alice", "/api/edu/assignments/" + asID + "/help"},
		{"u_alice", "/api/edu/assignments/" + asID + "/attempts"}, {"u_alice", "/api/edu/cost-report"},
		{"u_alice", "/api/edu/ledger/report"}, {"u_alice", "/api/edu/budget"}, {"u_alice", "/api/edu/support-costs?period=" + time.Now().UTC().Format("2006-01")},
		{"u_bob", "/api/edu/student/assignments"}, {"u_bob", "/api/edu/student/progress"}, {"u_bob", "/api/edu/student/help"},
		{"u_bob", "/api/edu/student/assignments/" + asID + "/learning"},
	}
	snap := func() map[string]string {
		res := map[string]string{}
		for _, v := range views {
			var hdr []string
			if v.user == "u_bob" {
				hdr = stu
			}
			c, out, raw, _ := h.call(v.user, "GET", v.path, nil, hdr...)
			if c != 200 {
				t.Fatalf("view %s: %d %s", v.path, c, raw)
			}
			d := data(out)
			delete(d, "from") // report windows default to "now"; the content is what must not move
			delete(d, "to")
			b, _ := json.Marshal(d)
			res[v.path] = string(b)
		}
		_, out, _, _ := h.call("u_alice", "GET", "/api/edu/home", nil)
		d := data(out)
		delete(d, "demoClass")
		b, _ := json.Marshal(d)
		res["home"] = string(b)
		return res
	}
	beforeDB, beforeViews := digest(), snap()

	// Create the demo class: labeled, fictional, per workspace.
	c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/demo-class/reset", nil)
	d := data(out)
	if c != 200 || d["demo"] != true || !strings.Contains(d["label"].(string), "演示数据") || d["class"].(map[string]any)["demo"] != true {
		t.Fatalf("reset: %d %s", c, raw)
	}
	recs := d["records"].([]any)
	dash := d["dashboard"].(map[string]any)
	if len(recs) != len(demoSeeds) || dash["students"].(float64) != 6 || dash["attempted"].(float64) != 5 || dash["correct"].(float64) != 3 || dash["openHelp"].(float64) != 2 {
		t.Fatalf("demo dashboard: %s", raw)
	}
	for _, r := range recs {
		if n := r.(map[string]any)["studentName"].(string); !strings.HasPrefix(n, "示例学生") {
			t.Fatalf("non fictional name %q", n)
		}
	}
	demoID := d["class"].(map[string]any)["id"].(string)
	// Reset is idempotent: same number of records, one demo class per workspace.
	h.call("u_alice", "POST", "/api/edu/demo-class/reset", nil)
	var nk, nr int64
	h.db.Model(&model.EduDemoClass{}).Where("tenant_id = ?", tenant).Count(&nk)
	h.db.Model(&model.EduDemoRecord{}).Where("tenant_id = ?", tenant).Count(&nr)
	if nk != 1 || nr != int64(len(demoSeeds)) {
		t.Fatalf("after second reset: classes %d records %d", nk, nr)
	}
	// Home shows it apart from real classes, labeled demo.
	_, out, raw, _ = h.call("u_alice", "GET", "/api/edu/home", nil)
	hd := data(out)
	dc, _ := hd["demoClass"].(map[string]any)
	if dc == nil || dc["demo"] != true || len(hd["classes"].([]any)) != 1 {
		t.Fatalf("home demo: %s", raw)
	}

	// Nothing real moved: tables, staff views, metrics, ledger, reports, student views.
	afterDB, afterViews := digest(), snap()
	for k, v := range beforeDB {
		if afterDB[k] != v {
			t.Fatalf("demo class changed real table %s", k)
		}
	}
	for k, v := range beforeViews {
		if afterViews[k] != v {
			t.Fatalf("demo class changed view %s:\nbefore %s\nafter  %s", k, v, afterViews[k])
		}
		if strings.Contains(afterViews[k], "示例学生") || strings.Contains(afterViews[k], demoID) {
			t.Fatalf("demo data leaked into %s", k)
		}
	}
	// Students have no route to it; other tenants see only their own (none); naming alice's workspace is refused.
	if c, _, raw, _ := h.call("u_bob", "GET", "/api/edu/demo-class", nil, stu...); c < 400 {
		t.Fatalf("student demo read: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu/demo-class/reset", nil, stu...); c < 400 {
		t.Fatalf("student demo reset: %d %s", c, raw)
	}
	h.call("u_carol", "POST", "/api/edu/roles", map[string]any{"tenantId": model.PersonalTenantID("u_carol"), "userId": "u_carol", "role": "teacher"})
	if c, out, raw, _ := h.call("u_carol", "GET", "/api/edu/demo-class", nil); c != 200 || data(out)["class"] != nil || strings.Contains(string(raw), demoID) {
		t.Fatalf("carol demo: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_carol", "POST", "/api/edu/demo-class/remove", nil, "X-Workspace-Id", model.PersonalWorkspaceID("u_alice")); c < 400 {
		t.Fatalf("carol removing alice's demo: %d %s", c, raw)
	}
	h.db.Model(&model.EduDemoRecord{}).Where("tenant_id = ?", tenant).Count(&nr)
	if nr != int64(len(demoSeeds)) {
		t.Fatalf("carol touched alice's demo")
	}
	// The teacher removes it; real data still untouched.
	if c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/demo-class/remove", nil); c != 200 || data(out)["class"] != nil {
		t.Fatalf("remove: %d %s", c, raw)
	}
	h.db.Model(&model.EduDemoClass{}).Where("tenant_id = ?", tenant).Count(&nk)
	h.db.Model(&model.EduDemoRecord{}).Where("tenant_id = ?", tenant).Count(&nr)
	if nk != 0 || nr != 0 {
		t.Fatalf("after remove: %d %d", nk, nr)
	}
	if _, out, _, _ := h.call("u_alice", "GET", "/api/edu/home", nil); data(out)["demoClass"] != nil {
		t.Fatalf("home still shows a removed demo class")
	}
	for k, v := range digest() {
		if beforeDB[k] != v {
			t.Fatalf("remove changed real table %s", k)
		}
	}
}
