package edumath

import (
	"encoding/json"
	"strings"
	"testing"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// TestPriceChangeAudit (DEV-108): the first job accepted under a different
// price book than the tenant's previous job writes price_book.changed, with the
// system as actor (never the teacher), the old and new version and every
// changed unit price; an unchanged book writes nothing; a changed book under a
// reused version is flagged; the row shows in the institution access log.
func TestPriceChangeAudit(t *testing.T) {
	_, base := newScripted(t)
	h := newHarness(t, base)
	h.setupTeacher()
	rev := h.confirmedRevision("2", "1")
	start := func(key, book string) string {
		t.Helper()
		t.Setenv("FLOW_PRICE_BOOK", book)
		code, out, raw, _ := h.call("u_alice", "POST", "/api/edu/problem-revisions/"+rev+"/analyze", map[string]any{"idempotencyKey": key})
		if code >= 300 {
			t.Fatalf("start %s: %d %s", key, code, raw)
		}
		return data(out)["id"].(string)
	}
	rows := func() []model.EduAuditEvent {
		var es []model.EduAuditEvent
		h.db.Where("action = ?", "price_book.changed").Order("created_at, id").Find(&es)
		return es
	}
	start("p1", `{"version":"pv_a","unit_micro":{"math_compute_ms":1}}`)
	start("p2", `{"version":"pv_a","unit_micro":{"math_compute_ms":1}}`)
	if n := len(rows()); n != 0 {
		t.Fatalf("unchanged book must not audit, got %d", n)
	}
	j3 := start("p3", `{"version":"pv_b","unit_micro":{"math_compute_ms":7,"ocr_pages":2000}}`)
	start("p4", `{"version":"pv_b","unit_micro":{"math_compute_ms":7,"ocr_pages":2000}}`)
	j5 := start("p5", `{"version":"pv_b","unit_micro":{"math_compute_ms":9,"ocr_pages":2000}}`)
	es := rows()
	if len(es) != 2 {
		t.Fatalf("want 2 price_book.changed rows, got %d", len(es))
	}
	var d1, d2 map[string]any
	json.Unmarshal([]byte(es[0].Detail), &d1)
	json.Unmarshal([]byte(es[1].Detail), &d2)
	if es[0].ActorID != PriceChangeActor || es[0].Subject != "pv_b" || d1["fromVersion"] != "pv_a" || d1["toVersion"] != "pv_b" ||
		d1["firstJobId"] != j3 || d1["versionReused"] != false || d1["observedAt"] != "job_accept" {
		t.Fatalf("first change row: %+v %v", es[0], d1)
	}
	u1, _ := json.Marshal(d1["units"])
	if string(u1) != `[{"fromMicro":1,"toMicro":7,"usageType":"math_compute_ms"},{"fromMicro":-1,"toMicro":2000,"usageType":"ocr_pages"}]` {
		t.Fatalf("units: %s", u1)
	}
	if d2["versionReused"] != true || d2["firstJobId"] != j5 {
		t.Fatalf("reused version must be flagged: %v", d2)
	}
	_, _, rb, _ := h.call("u_alice", "GET", "/api/edu/org/access-log", nil)
	raw := string(rb)
	if !strings.Contains(raw, `"price_book.changed"`) || !strings.Contains(raw, PriceChangeActor) || !strings.Contains(raw, `versionReused`) {
		t.Fatalf("access log must list the price change: %s", raw)
	}
}

func TestPriceChangeDiff(t *testing.T) {
	a := flow.PriceBook{Version: "v1", UnitMicro: map[string]int64{"x": 1, "y": 2}}
	if priceChange(flow.PriceBook{}, a) != nil || priceChange(a, a) != nil {
		t.Fatal("no previous book or same book must be nil")
	}
	b := flow.PriceBook{Version: "v2", UnitMicro: map[string]int64{"x": 1, "y": 2}}
	if c := priceChange(a, b); c == nil || len(c["units"].([]map[string]any)) != 0 {
		t.Fatalf("version only change: %v", c)
	}
	c := priceChange(a, flow.PriceBook{Version: "v1", UnitMicro: map[string]int64{"x": 1}})
	if c == nil || c["versionReused"] != true || c["units"].([]map[string]any)[0]["toMicro"] != int64(-1) {
		t.Fatalf("removed unit under same version: %v", c)
	}
}
