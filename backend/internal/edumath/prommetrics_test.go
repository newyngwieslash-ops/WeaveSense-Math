package edumath

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

func TestEduPrometheusMetrics(t *testing.T) {
	h := newHarness(t, "http://127.0.0.1:1")
	h.setupTeacher()
	revID := h.confirmedRevision("2", "1")
	// Disabled when no token.
	if c, _, _, _ := h.call("", "GET", "/api/edu-metrics", nil); c != 404 {
		t.Fatalf("disabled metrics served: %d", c)
	}
	t.Setenv("EDU_METRICS_TOKEN", "edu-tok")
	if c, _, _, _ := h.call("", "GET", "/api/edu-metrics", nil); c != 401 {
		t.Fatalf("missing token: %d", c)
	}
	if c, _, _, _ := h.call("", "GET", "/api/edu-metrics", nil, "Authorization", "Bearer nope"); c != 401 {
		t.Fatalf("bad token: %d", c)
	}

	now := h.svc.now()
	tenant := "tn_u_alice"
	ws := model.PersonalWorkspaceID("u_alice")
	if err := flow.TenantTx(h.db, tenant, func(tx *gorm.DB) error {
		job := func(id, status, settlement string, created, updated time.Time) *model.EduMathJob {
			return &model.EduMathJob{ID: id, TenantID: tenant, WorkspaceID: ws, Kind: "analyze", IdemKey: id, InputHash: "h",
				ProblemRevisionID: revID, Status: status, Settlement: settlement, CreatedAt: created, UpdatedAt: updated}
		}
		rows := []any{
			job("ejb_prom_stuck", "running", "open", now.Add(-50*time.Minute), now.Add(-40*time.Minute)),
			job("ejb_prom_queued", "queued", "open", now.Add(-20*time.Minute), now.Add(-20*time.Minute)),
			job("ejb_prom_recon", "reconciliation_required", "held_unknown", now.Add(-time.Hour), now.Add(-time.Hour)),
			job("ejb_prom_leak", "failed", "open", now.Add(-45*time.Minute), now.Add(-20*time.Minute)),
			&model.EduLedgerAnomaly{ID: "ela_prom", TenantID: tenant, JobID: "ejb_prom_recon", Kind: "unknown_charge", Status: "open", DetectedAt: now, LastSeenAt: now},
		}
		for i := 0; i < 4; i++ {
			id := "ejb_prom_fail" + string(rune('a'+i))
			rows = append(rows, job(id, "failed", "settled", now.Add(-2*time.Hour), now.Add(-time.Hour)))
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

	c, _, raw, _ := h.call("", "GET", "/api/edu-metrics", nil, "Authorization", "Bearer edu-tok")
	body := string(raw)
	if c != 200 {
		t.Fatalf("status %d %s", c, body)
	}
	for _, want := range []string{
		"edu_up 1",
		`edu_math_jobs{status="queued"} 1`,
		`edu_math_jobs{status="running"} 1`,
		`edu_math_jobs{status="reconciliation_required"} 1`,
		"edu_math_jobs_stuck 1",
		"edu_math_jobs_reconciliation_required 1",
		"edu_math_jobs_unknown_cost 1",
		"edu_math_jobs_unsettled_reservations 1",
		"edu_ledger_anomalies_open 1",
		"edu_math_jobs_oldest_queued_seconds 1", // at least 1200, checked below
		"edu_math_jobs_failure_ratio",
	} {
		if !strings.Contains(body, want) && want != "edu_math_jobs_oldest_queued_seconds 1" {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "edu_math_jobs_oldest_queued_seconds 12") { // 20 minutes → 1200s
		t.Fatalf("oldest queued not around 20m:\n%s", body)
	}
	if strings.Contains(body, tenant) || strings.Contains(body, "u_alice") || strings.Contains(body, "ejb_prom") {
		t.Fatalf("metrics leaked identifiers:\n%s", body)
	}
	// 5 failed, 0 succeeded in the window → ratio 1.0 (min jobs is 5)
	if !strings.Contains(body, "edu_math_jobs_failure_ratio 1.0000") && !strings.Contains(body, "edu_math_jobs_failure_ratio 1") {
		t.Fatalf("expected failure ratio 1:\n%s", body)
	}
}

func TestEduPrometheusTokenFallsBackToFlow(t *testing.T) {
	h := newHarness(t, "http://127.0.0.1:1")
	t.Setenv("FLOW_METRICS_TOKEN", "flow-tok")
	c, _, raw, _ := h.call("", "GET", "/api/edu-metrics", nil, "Authorization", "Bearer flow-tok")
	if c != 200 || !strings.Contains(string(raw), "edu_up 1") {
		t.Fatalf("%d %s", c, raw)
	}
}
