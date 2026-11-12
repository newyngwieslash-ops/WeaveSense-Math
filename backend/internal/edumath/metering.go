package edumath

import (
	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// recordStorageUsage posts a stand-in storage_bytes line for one ingested
// artifact (DEV-065, E96). Charge is false: this is cost accounting, not a
// second debit of the job reservation. Idempotent per (job, service artifact).
func (s *Service) recordStorageUsage(tx *gorm.DB, job model.EduMathJob, art model.EduArtifact) error {
	if art.Size <= 0 {
		return nil
	}
	return s.RecordVendorUsage(tx, job.TenantID, job.WorkspaceID, flow.UsageStorageBytes,
		"edu_artifact:"+art.ServiceArtifactID, "edu:storage:"+job.ID+":"+art.ServiceArtifactID, art.Size)
}

// RecordVendorUsage writes one education usage line (storage, TTS, or egress)
// priced from the current book (stand-in rates until U11 confirms production
// prices). CostMicro -1 means "use the book". The line is not charged against
// the math job hold.
func (s *Service) RecordVendorUsage(tx *gorm.DB, tenantID, workspaceID, usageType, subject, idem string, quantity int64) error {
	if quantity < 0 {
		return nil
	}
	pb := flow.CurrentPriceBook()
	_, err := flow.RecordUsage(tx, flow.UsageInput{
		TenantID: tenantID, WorkspaceID: workspaceID, UsageType: usageType,
		SubjectRef: subject, IdemKey: idem, Quantity: quantity, CostMicro: -1, Frozen: &pb,
	}, s.now())
	return err
}

func usageQuantity(tx *gorm.DB, tenantID, usageType string, from, to interface{}) int64 {
	var n int64
	tx.Model(&model.FlowUsageRecord{}).
		Where("tenant_id = ? AND usage_type = ? AND created_at >= ? AND created_at < ?", tenantID, usageType, from, to).
		Select("COALESCE(SUM(quantity), 0)").Scan(&n)
	return n
}
