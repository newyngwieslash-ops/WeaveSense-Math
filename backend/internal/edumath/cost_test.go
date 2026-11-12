package edumath

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

func TestDeliveryCostReport(t *testing.T) { runCostReport(t, newHarness(t, startMathService(t))) }

func TestPGDeliveryCostReport(t *testing.T) {
	runCostReport(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

// runCostReport (DEV-065): after the real G1 chain the period report splits
// delivered compute, rendering, failed or cancelled work and human support;
// its metered machine cost equals the ledger's stage cost to the micro unit;
// unconnected providers are flagged not metered rather than zero cost; support
// time is attributed and audited; denominators are accepted lessons and active
// classes; only admins see it.
func runCostReport(t *testing.T, h *harness) {
	runG1(t, h)
	tenant := model.PersonalTenantID("u_alice")
	period, _ := budgetPeriod(h.svc.now())
	report := func() map[string]any {
		t.Helper()
		code, out, raw, hd := h.call("u_alice", "GET", "/api/edu/cost-report?period="+period, nil)
		if code != 200 || hd.Get("Cache-Control") != "private, no-store" {
			t.Fatalf("cost report: %d %s", code, raw)
		}
		return data(out)
	}
	r := report()
	lines := map[string]map[string]any{}
	var sum float64
	for _, x := range r["lines"].([]any) {
		m := x.(map[string]any)
		lines[m["component"].(string)] = m
		sum += m["costMicro"].(float64)
	}
	if sum != r["totalMicro"].(float64) || lines["compute_succeeded"]["costMicro"].(float64) <= 0 {
		t.Fatalf("report lines: %v", r)
	}
	pb := flow.CurrentPriceBook()
	storageQty := int64(lines["storage"]["quantity"].(float64))
	if storageQty <= 0 {
		t.Fatalf("stored artifact bytes must be counted: %v", lines["storage"])
	}
	if pb.UnitMicro[flow.UsageStorageBytes] > 0 {
		want := float64(storageQty * pb.UnitMicro[flow.UsageStorageBytes])
		if lines["storage"]["metered"] != true || lines["storage"]["costMicro"].(float64) != want {
			t.Fatalf("storage must be metered at the stand-in unit price: %v want %v", lines["storage"], want)
		}
		if src, _ := lines["storage"]["source"].(string); !containsStandIn(src) {
			t.Fatalf("storage source must name the stand-in price: %v", lines["storage"])
		}
	}
	for _, k := range []string{"tts", "egress"} {
		if pb.UnitMicro[map[string]string{"tts": flow.UsageTTSCharacters, "egress": flow.UsageEgressBytes}[k]] > 0 {
			if lines[k]["metered"] != true {
				t.Fatalf("%s must be metered at the stand-in unit price: %v", k, lines[k])
			}
		}
	}
	// Machine cost equals the ledger's stage cost.
	var ledger int64
	if err := flow.TenantTx(h.db, tenant, func(tx *gorm.DB) error {
		ls, err := h.svc.LedgerReport(tx, tenant, time.Now().AddDate(0, -2, 0), time.Now().Add(time.Hour))
		for _, l := range ls {
			ledger += l.StageCost
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	machine := lines["compute_succeeded"]["costMicro"].(float64) + lines["render"]["costMicro"].(float64) + lines["failed_and_cancelled"]["costMicro"].(float64)
	if int64(machine) != ledger {
		t.Fatalf("machine cost %v != ledger stage cost %d", machine, ledger)
	}
	if r["acceptedLessons"].(float64) < 1 || r["perAcceptedLessonMicro"] == nil {
		t.Fatalf("denominators: %v", r)
	}

	// Human support with attribution.
	for _, bad := range []map[string]any{{"period": "2026-13", "category": "other", "minutes": 5}, {"period": period, "category": "lunch", "minutes": 5},
		{"period": period, "category": "other", "minutes": 0}, {"period": period, "category": "other", "minutes": 5, "costMicro": -1}} {
		if code, _, _, _ := h.call("u_alice", "POST", "/api/edu/support-costs", bad); code != 400 {
			t.Fatalf("bad support cost %v: %d", bad, code)
		}
	}
	code, _, raw, _ := h.call("u_alice", "POST", "/api/edu/support-costs", map[string]any{"period": period, "category": "review_support", "minutes": 30,
		"costMicro": 50000, "subjectRef": "class:c1", "note": "审核辅导"})
	if code != 201 {
		t.Fatalf("support cost: %d %s", code, raw)
	}
	h.call("u_alice", "POST", "/api/edu/support-costs", map[string]any{"period": period, "category": "incident", "minutes": 10, "costMicro": 10000})
	r2 := report()
	if r2["totalMicro"].(float64) != r["totalMicro"].(float64)+60000 || r2["supportUnattributed"].(float64) != 1 ||
		r2["supportByCategoryMicro"].(map[string]any)["review_support"].(float64) != 50000 {
		t.Fatalf("support in report: %v", r2)
	}
	code, out, _, _ := h.call("u_alice", "GET", "/api/edu/support-costs?period="+period, nil)
	if rows, _ := out["data"].([]any); code != 200 || len(rows) != 2 || rows[0].(map[string]any)["recordedBy"] != "u_alice" {
		t.Fatalf("support list: %d %v", code, out)
	}
	if code, _, _, _ := h.call("u_alice", "GET", "/api/edu/cost-report?period=26-1", nil); code != 400 {
		t.Fatalf("bad period must be 400, got %d", code)
	}
	if code, _, _, _ := h.call("u_dave", "GET", "/api/edu/cost-report", nil); code == 200 {
		t.Fatal("outsider read cost report")
	}

	// Stand-in TTS / egress lines post to the ledger and show up in the report (E96).
	if err := flow.TenantTx(h.db, tenant, func(tx *gorm.DB) error {
		if err := h.svc.RecordVendorUsage(tx, tenant, "", flow.UsageTTSCharacters, "edu_tts:test", "edu:tts:test-1", 100); err != nil {
			return err
		}
		return h.svc.RecordVendorUsage(tx, tenant, "", flow.UsageEgressBytes, "edu_egress:test", "edu:egress:test-1", 50)
	}); err != nil {
		t.Fatal(err)
	}
	r3 := report()
	lines3 := map[string]map[string]any{}
	for _, x := range r3["lines"].([]any) {
		m := x.(map[string]any)
		lines3[m["component"].(string)] = m
	}
	if lines3["tts"]["quantity"].(float64) != 100 || lines3["tts"]["costMicro"].(float64) != float64(100*pb.UnitMicro[flow.UsageTTSCharacters]) {
		t.Fatalf("tts stand-in line: %v", lines3["tts"])
	}
	if lines3["egress"]["quantity"].(float64) != 50 || lines3["egress"]["costMicro"].(float64) != float64(50*pb.UnitMicro[flow.UsageEgressBytes]) {
		t.Fatalf("egress stand-in line: %v", lines3["egress"])
	}
	var stored, tts, egress int64
	if err := flow.TenantTx(h.db, tenant, func(tx *gorm.DB) error {
		tx.Model(&model.FlowUsageRecord{}).Where("tenant_id = ? AND usage_type = ?", tenant, flow.UsageStorageBytes).Select("COALESCE(SUM(cost_micro),0)").Scan(&stored)
		tx.Model(&model.FlowUsageRecord{}).Where("tenant_id = ? AND usage_type = ?", tenant, flow.UsageTTSCharacters).Select("COALESCE(SUM(cost_micro),0)").Scan(&tts)
		tx.Model(&model.FlowUsageRecord{}).Where("tenant_id = ? AND usage_type = ?", tenant, flow.UsageEgressBytes).Select("COALESCE(SUM(cost_micro),0)").Scan(&egress)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if stored != int64(lines3["storage"]["costMicro"].(float64)) || tts != int64(lines3["tts"]["costMicro"].(float64)) || egress != int64(lines3["egress"]["costMicro"].(float64)) {
		t.Fatalf("ledger usage must match report: storage %d vs %v, tts %d vs %v, egress %d vs %v", stored, lines3["storage"], tts, lines3["tts"], egress, lines3["egress"])
	}
}

func containsStandIn(s string) bool { return strings.Contains(s, "stand-in") }
