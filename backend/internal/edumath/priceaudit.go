package edumath

import (
	"encoding/json"
	"sort"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
	"gorm.io/gorm"
)

// PriceChangeActor is the actor id on price_book.changed audit rows. Prices are
// set by the platform deployment (FLOW_PRICE_BOOK), not by a tenant user, so the
// teacher whose job first ran under the new book is never recorded as the one
// who changed it (DEV-108).
const PriceChangeActor = "system:price_book"

// priceChange compares the price book a new job is about to freeze with the
// one frozen on this tenant's previous job. Nil means no change (or no earlier
// job). Units lists every usage type whose unit price differs, as [old, new]
// (-1 when absent); versionReused flags a changed book under the same version.
func priceChange(prev, cur flow.PriceBook) map[string]any {
	if prev.Version == "" {
		return nil
	}
	units := map[string][2]int64{}
	for k, v := range cur.UnitMicro {
		if o, ok := prev.UnitMicro[k]; !ok || o != v {
			if !ok {
				o = -1
			}
			units[k] = [2]int64{o, v}
		}
	}
	for k, o := range prev.UnitMicro {
		if _, ok := cur.UnitMicro[k]; !ok {
			units[k] = [2]int64{o, -1}
		}
	}
	if len(units) == 0 && prev.Version == cur.Version {
		return nil
	}
	keys := make([]string, 0, len(units))
	for k := range units {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, map[string]any{"usageType": k, "fromMicro": units[k][0], "toMicro": units[k][1]})
	}
	return map[string]any{"fromVersion": prev.Version, "toVersion": cur.Version, "units": rows,
		"versionReused": prev.Version == cur.Version, "observedAt": "job_accept"}
}

// auditPriceChange writes price_book.changed into the tenant's audit and access
// log the first time a job is accepted under a different price book than the
// tenant's previous job. Jobs freeze their book, so existing jobs keep their
// prices; the row says when the tenant first saw the change and what changed.
func (s *Service) auditPriceChange(tx *gorm.DB, a Actor, jobID string, cur flow.PriceBook) {
	var last model.EduMathJob
	if err := tx.Select("id", "price_book").Where("tenant_id = ? AND price_book <> ''", a.TenantID).
		Order("created_at desc, id desc").Limit(1).Find(&last).Error; err != nil || last.ID == "" {
		return
	}
	var prev flow.PriceBook
	if json.Unmarshal([]byte(last.PriceBook), &prev) != nil {
		return
	}
	ch := priceChange(prev, cur)
	if ch == nil {
		return
	}
	ch["firstJobId"], ch["previousJobId"] = jobID, last.ID
	raw, _ := json.Marshal(ch)
	s.audit(tx, Actor{TenantID: a.TenantID, UserID: PriceChangeActor}, "price_book.changed", cur.Version, string(raw))
}
