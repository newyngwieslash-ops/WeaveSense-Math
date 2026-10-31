package edumath

import (
	"testing"

	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

func TestCostBreakdown(t *testing.T) { runCostBreakdown(t, newHarness(t, startMathService(t))) }

func TestPGCostBreakdown(t *testing.T) {
	runCostBreakdown(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

// runCostBreakdown (DEV-118): after the real G1 chain, failed and retried jobs and
// attributed support stay with their batch and are carried by its accepted lessons;
// every view (tenant, family, class, reuse, method, template, model, terminal) adds up to the same total, which equals
// the period cost report; zero denominators are not computable; strata with partial
// sources are listed; only admins read it.
func runCostBreakdown(t *testing.T, h *harness) {
	runG1(t, h)
	tenant := model.PersonalTenantID("u_alice")
	period, _ := budgetPeriod(h.svc.now())
	get := func(by string) map[string]any {
		t.Helper()
		code, out, raw, hd := h.call("u_alice", "GET", "/api/edu/cost-report/breakdown?period="+period+"&by="+by, nil)
		if code != 200 || hd.Get("Cache-Control") != "private, no-store" {
			t.Fatalf("breakdown %s: %d %s", by, code, raw)
		}
		return data(out)
	}
	rows := func(r map[string]any) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, x := range r["rows"].([]any) {
			m := x.(map[string]any)
			out[m["key"].(string)] = m
		}
		return out
	}
	reconcile := func() float64 {
		t.Helper()
		_, rep, _, _ := h.call("u_alice", "GET", "/api/edu/cost-report?period="+period, nil)
		want := data(rep)["totalMicro"].(float64)
		for _, by := range []string{"tenant", "family", "class", "reuse", "method", "template", "model", "terminal"} {
			r := get(by)
			var sum float64
			for _, x := range rows(r) {
				sum += x["costMicro"].(float64)
				if x["computable"] == false && x["perAcceptedMicro"] != nil {
					t.Fatalf("%s: zero denominator must not have a ratio: %v", by, x)
				}
			}
			if r["totalMicro"].(float64) != want || sum != want || r["definition"] != "cost_breakdown/1" {
				t.Fatalf("%s: rows %v total %v, cost report %v", by, sum, r["totalMicro"], want)
			}
			if len(r["notCollected"].([]any)) != 3 {
				t.Fatalf("template, model and terminal must list their partial sources: %v", r["notCollected"])
			}
		}
		return want
	}
	base := reconcile()

	var pubID, revID, family, classID string
	if err := flow.TenantTx(h.db, tenant, func(tx *gorm.DB) error {
		var pub model.EduPublication
		if err := tx.Where("tenant_id = ?", tenant).First(&pub).Error; err != nil {
			return err
		}
		var lr model.EduLessonRevision
		if err := tx.First(&lr, "id = ?", pub.LessonRevisionID).Error; err != nil {
			return err
		}
		var pr model.EduProblemRevision
		if err := tx.First(&pr, "id = ?", lr.ProblemRevisionID).Error; err != nil {
			return err
		}
		var p model.EduProblem
		if err := tx.First(&p, "id = ?", pr.ProblemID).Error; err != nil {
			return err
		}
		var as model.EduAssignment
		if err := tx.First(&as, "publication_id = ?", pub.ID).Error; err != nil {
			return err
		}
		pubID, revID, family, classID = pub.ID, pr.ID, p.FamilyID, as.ClassID
		// A failed retry of the same batch, and a failed job of a batch that never produced a lesson.
		now := h.svc.now()
		other := model.EduProblem{ID: "epb_costx", TenantID: tenant, WorkspaceID: model.PersonalWorkspaceID("u_alice"), FamilyID: "COST-ORPHAN", LatestRevision: 1, CreatedAt: now, UpdatedAt: now}
		otherRev := model.EduProblemRevision{ID: "epr_costx", TenantID: tenant, ProblemID: other.ID, Revision: 1, Structured: "{}", InputHash: "sha256:x", State: "confirmed", CreatedAt: now}
		for _, x := range []any{&other, &otherRev,
			&model.EduMathJob{ID: "ejb_cost_retry", TenantID: tenant, WorkspaceID: model.PersonalWorkspaceID("u_alice"), Kind: "analyze", IdemKey: "cost-retry", InputHash: "h", ProblemRevisionID: revID, Status: "failed", Settlement: "settled", CreatedAt: now, UpdatedAt: now},
			&model.EduStageRun{ID: "esr_cost_retry", TenantID: tenant, JobID: "ejb_cost_retry", Stage: "analyze", Status: "failed", CostMicro: 1234, CreatedAt: now},
			&model.EduMathJob{ID: "ejb_cost_orphan", TenantID: tenant, WorkspaceID: model.PersonalWorkspaceID("u_alice"), Kind: "analyze", IdemKey: "cost-orphan", InputHash: "h", ProblemRevisionID: otherRev.ID, Status: "cancelled", Settlement: "released", CreatedAt: now, UpdatedAt: now},
			&model.EduStageRun{ID: "esr_cost_orphan", TenantID: tenant, JobID: "ejb_cost_orphan", Stage: "analyze", Status: "cancelled", CostMicro: 500, CreatedAt: now},
			&model.EduMathJob{ID: "ejb_cost_open", TenantID: tenant, WorkspaceID: model.PersonalWorkspaceID("u_alice"), Kind: "lesson_build", IdemKey: "cost-open", InputHash: "h", ProblemRevisionID: revID, Status: "running", Settlement: "open", CreatedAt: now, UpdatedAt: now},
		} {
			if err := tx.Create(x).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fam := rows(get("family"))[family]
	if fam["failedMicro"].(float64) < 1234 || fam["retriedJobs"].(float64) < 1 || fam["accepted"].(float64) != 1 || fam["computable"] != true ||
		fam["perAcceptedMicro"].(float64) != fam["costMicro"].(float64) {
		t.Fatalf("failed retry must stay with its accepted batch: %v", fam)
	}
	orphan := rows(get("family"))["COST-ORPHAN"]
	if orphan["costMicro"].(float64) != 500 || orphan["computable"] != false || orphan["perAcceptedMicro"] != nil {
		t.Fatalf("a family with no accepted lesson is not computable: %v", orphan)
	}
	if r := get("tenant"); r["excludedOpenJobs"].(float64) != 1 {
		t.Fatalf("running jobs are excluded until settled: %v", r)
	}
	if after := reconcile(); after != base+1234+500 {
		t.Fatalf("total %v, want %v", after, base+1734)
	}

	// Support attributed to the lesson, the family, a class, and unattributed.
	for _, s := range []map[string]any{
		{"period": period, "category": "review_support", "minutes": 20, "costMicro": 7000, "subjectRef": "publication:" + pubID},
		{"period": period, "category": "content", "minutes": 10, "costMicro": 3000, "subjectRef": "class:" + classID},
		{"period": period, "category": "content", "minutes": 10, "costMicro": 2000, "subjectRef": "family:" + family},
		{"period": period, "category": "other", "minutes": 5, "costMicro": 1000},
	} {
		if code, _, raw, _ := h.call("u_alice", "POST", "/api/edu/support-costs", s); code != 201 {
			t.Fatalf("support: %d %s", code, raw)
		}
	}
	fam2 := rows(get("family"))[family]
	if fam2["supportMicro"].(float64) != 9000 || fam2["costMicro"].(float64) != fam["costMicro"].(float64)+9000 {
		t.Fatalf("lesson and family support join the family: %v", fam2)
	}
	cls := rows(get("class"))
	if cls["class:"+classID]["supportMicro"].(float64) != 7000+3000 || cls["support_family_not_split"]["supportMicro"].(float64) != 2000 ||
		cls["support_unattributed"]["supportMicro"].(float64) != 1000 || cls["no_accepted_lesson"]["costMicro"].(float64) != 500 {
		t.Fatalf("class view: %v", cls)
	}
	reconcile()

	// Reuse: the same lesson assigned to a second class halves its per class delivery cost.
	single := rows(get("reuse"))["single_class"]
	code, out, raw, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二(4)班"})
	if code != 201 {
		t.Fatalf("class: %d %s", code, raw)
	}
	class2 := data(out)["id"].(string)
	if code, _, raw, _ := h.call("u_alice", "POST", "/api/edu/classes/"+class2+"/assignments", map[string]any{"publicationId": pubID, "title": "复用"}); code != 201 {
		t.Fatalf("reuse assignment: %d %s", code, raw)
	}
	reuse := rows(get("reuse"))
	if reuse["single_class"] != nil || reuse["reused"]["classDeliveries"].(float64) != 2 || reuse["reused"]["costMicro"].(float64) != single["costMicro"].(float64) ||
		reuse["reused"]["perClassDeliveryMicro"].(float64) != float64(int64(single["costMicro"].(float64))/2) {
		t.Fatalf("reuse view: %v (before %v)", reuse, single)
	}
	cls = rows(get("class"))
	if cls["class:"+class2] == nil || cls["class:"+class2]["accepted"].(float64) != 1 {
		t.Fatalf("second class carries half the lesson: %v", cls)
	}
	reconcile()

	for _, q := range []string{"by=method", "by=terminal", "by=x", "period=2026-13"} {
		code, out, _, _ := h.call("u_alice", "GET", "/api/edu/cost-report/breakdown?"+q, nil)
		if code != 400 {
			t.Fatalf("%s must be 400, got %d", q, code)
		}
		if q == "by=method" && len(data(out)["notCollected"].([]any)) != 4 {
			t.Fatalf("400 lists what is not collected: %v", out)
		}
	}
	for _, u := range []string{"u_bob", "u_dave"} {
		if code, _, _, _ := h.call(u, "GET", "/api/edu/cost-report/breakdown?by=family", nil); code == 200 {
			t.Fatalf("%s read the cost breakdown", u)
		}
	}
}
