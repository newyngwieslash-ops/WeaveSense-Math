package edumath

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// Retention (DEV-106). The periods below are suggested defaults pending the
// institution's decision (09 3.3): temporary files 7 days, answer images 90
// days after review, learning records 180 days after the student left the
// class, backups at most 30 days. Nothing is cleaned up until an org admin
// confirms the periods with a reason; from then on a scheduled run (and a
// manual run) applies them. Ledger, usage and audit records are not part of
// this policy: they are kept on their own accounting and legal basis.
//
// What a run does:
//   - learning records: for an enrollment withdrawn longer ago than
//     learningDays (and not re-joined), the student's attempts, hint unlocks,
//     self tests, review items, help requests, predictions, figure
//     interactions, regrades and extensions in that class's assignments, and
//     the roster entry they claimed, are deleted. The withdrawn enrollment row
//     (no content) stays so the deletion can be accounted for.
//   - temporary files: artifacts of failed or cancelled jobs older than
//     tempDays are revoked on the platform (never served again). Deleting the
//     bytes in the math service store is not implemented yet.
//   - answer images: the product does not accept image answers yet (OCR is
//     DEV-069), so there is nothing to clean; the period is stored.
//   - backups: enforced by the deployment (07), not by the application.
//
// Every run writes an audit event with counts only.

var retentionDefaults = model.EduRetentionPolicy{TempDays: 7, ImageDays: 90, LearningDays: 180, BackupDays: 30}

var retentionBounds = map[string][2]int{"tempDays": {1, 30}, "imageDays": {7, 365}, "learningDays": {30, 1095}, "backupDays": {1, 30}}

// ErrRetentionNotConfirmed: the suggested defaults are not applied until confirmed.
var ErrRetentionNotConfirmed = appErr(http.StatusConflict, "RETENTION_NOT_CONFIRMED", "retention periods are suggested defaults until an org admin confirms them", "confirm_retention")

// learningTables hold one student's learning data per assignment.
var learningTables = []any{&model.EduAttempt{}, &model.EduHintUnlock{}, &model.EduSelfTest{}, &model.EduReviewItem{}, &model.EduHelpRequest{},
	&model.EduPrediction{}, &model.EduFigureInteraction{}, &model.EduRegrade{}, &model.EduAssignmentExtension{}}

func (h *API) registerRetention(g *gin.RouterGroup) {
	g.GET("/retention", h.getRetention)
	g.PUT("/retention", h.setRetention)
	g.POST("/retention/run", h.runRetention)
}

func retentionPolicy(tx *gorm.DB, tenant string) (model.EduRetentionPolicy, bool) {
	var p model.EduRetentionPolicy
	if tx.First(&p, "tenant_id = ?", tenant).Error != nil || p.State != "confirmed" {
		d := retentionDefaults
		d.TenantID, d.State = tenant, "suggested"
		if p.TenantID != "" {
			d.LastRunAt, d.LastRunSummary = p.LastRunAt, p.LastRunSummary
		}
		return d, false
	}
	return p, true
}

// RetentionSummary counts what a run removes (or would remove).
type RetentionSummary struct {
	Enrollments   int            `json:"enrollments"`
	LearningRows  map[string]int `json:"learningRows"`
	RosterEntries int            `json:"rosterEntries"`
	Artifacts     int            `json:"artifacts"`
	ProblemImages int            `json:"problemImages"` // unattached uploads past the temporary period (DEV-068)
}

func (s RetentionSummary) total() int {
	n := s.RosterEntries + s.Artifacts + s.ProblemImages
	for _, v := range s.LearningRows {
		n += v
	}
	return n
}

func (s RetentionSummary) String() string {
	return fmt.Sprintf("enrollments=%d learning_rows=%d roster_entries=%d problem_images=%d artifacts=%d", s.Enrollments, s.total()-s.RosterEntries-s.Artifacts-s.ProblemImages, s.RosterEntries, s.ProblemImages, s.Artifacts)
}

// applyRetention computes (dry) or performs the cleanup for one tenant inside tx.
func applyRetention(tx *gorm.DB, tenant string, p model.EduRetentionPolicy, now time.Time, dry bool) (RetentionSummary, error) {
	sum := RetentionSummary{LearningRows: map[string]int{}}
	var es []model.EduEnrollment
	if err := tx.Where("tenant_id = ? AND status = ? AND updated_at < ?", tenant, "withdrawn", now.AddDate(0, 0, -p.LearningDays)).Find(&es).Error; err != nil {
		return sum, err
	}
	for _, e := range es {
		touched, err := deleteStudentLearning(tx, tenant, e.StudentUserID, e.ClassID, dry, &sum)
		if err != nil {
			return sum, err
		}
		if touched {
			sum.Enrollments++
		}
	}
	var jobIDs []string
	tx.Model(&model.EduMathJob{}).Where("tenant_id = ? AND status IN ? AND updated_at < ?", tenant, []string{"failed", "cancelled"}, now.AddDate(0, 0, -p.TempDays)).Pluck("id", &jobIDs)
	if len(jobIDs) > 0 {
		var n int64
		q := tx.Model(&model.EduArtifact{}).Where("tenant_id = ? AND job_id IN ? AND revoked_at IS NULL", tenant, jobIDs)
		q.Count(&n)
		sum.Artifacts = int(n)
		if !dry && n > 0 {
			if err := tx.Model(&model.EduArtifact{}).Where("tenant_id = ? AND job_id IN ? AND revoked_at IS NULL", tenant, jobIDs).Update("revoked_at", now).Error; err != nil {
				return sum, err
			}
		}
	}
	// Problem pictures never attached to a problem are temporary files: removed
	// with their bytes after the temporary period. Attached ones stay with the problem.
	imgQ := func() *gorm.DB {
		return tx.Model(&model.EduProblemImage{}).Where("tenant_id = ? AND (problem_id = '' OR problem_id IS NULL) AND created_at < ?", tenant, now.AddDate(0, 0, -p.TempDays))
	}
	var ni int64
	if err := imgQ().Count(&ni).Error; err != nil {
		return sum, err
	}
	sum.ProblemImages = int(ni)
	if !dry && ni > 0 {
		if err := imgQ().Delete(&model.EduProblemImage{}).Error; err != nil {
			return sum, err
		}
	}
	return sum, nil
}

func retentionView(p model.EduRetentionPolicy, confirmed bool, preview RetentionSummary) gin.H {
	return gin.H{
		"policy": p, "confirmed": confirmed, "defaults": retentionDefaults, "bounds": retentionBounds,
		"preview": preview,
		"categories": gin.H{
			"learning": "withdrawn_enrollments",
			"temp":     "revoked_on_platform; math service store deletion not implemented; unattached problem images deleted",
			"images":   "no_image_answers_yet",
			"backup":   "deployment (07)",
		},
		"keptSeparately": []string{"ledger", "usage", "audit"},
	}
}

func (h *API) getRetention(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	p, confirmed := retentionPolicy(tx, a.TenantID)
	preview, err := applyRetention(tx, a.TenantID, p, h.Svc.now(), true)
	if err != nil {
		h.fail(c, err)
		return
	}
	reply(c, 200, retentionView(p, confirmed, preview), "ok")
}

func (h *API) setRetention(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var in struct {
		TempDays     int    `json:"tempDays"`
		ImageDays    int    `json:"imageDays"`
		LearningDays int    `json:"learningDays"`
		BackupDays   int    `json:"backupDays"`
		Reason       string `json:"reason"`
	}
	if c.ShouldBindJSON(&in) != nil {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "retention periods required", "fix_input"))
		return
	}
	for k, v := range map[string]int{"tempDays": in.TempDays, "imageDays": in.ImageDays, "learningDays": in.LearningDays, "backupDays": in.BackupDays} {
		if b := retentionBounds[k]; v < b[0] || v > b[1] {
			h.fail(c, appErr(400, "VALIDATION_FAILED", fmt.Sprintf("%s must be %d to %d days", k, b[0], b[1]), "fix_input"))
			return
		}
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if n := len([]rune(in.Reason)); n < 4 || n > 200 || strings.ContainsAny(in.Reason, "<>\r\n") {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "reason (4 to 200 characters, the basis of the decision) required", "fix_input"))
		return
	}
	now := h.Svc.now()
	old, _ := retentionPolicy(tx, a.TenantID)
	p := model.EduRetentionPolicy{TenantID: a.TenantID, TempDays: in.TempDays, ImageDays: in.ImageDays, LearningDays: in.LearningDays, BackupDays: in.BackupDays,
		State: "confirmed", Reason: in.Reason, ConfirmedBy: a.UserID, ConfirmedAt: &now, LastRunAt: old.LastRunAt, LastRunSummary: old.LastRunSummary, UpdatedAt: now}
	if err := tx.Save(&p).Error; err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "retention.set", a.TenantID, fmt.Sprintf("temp=%d image=%d learning=%d backup=%d | %s", p.TempDays, p.ImageDays, p.LearningDays, p.BackupDays, in.Reason))
	preview, _ := applyRetention(tx, a.TenantID, p, now, true)
	reply(c, 200, retentionView(p, true, preview), "ok")
}

func (h *API) runRetention(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var in struct {
		Confirm bool `json:"confirm"`
	}
	_ = c.ShouldBindJSON(&in)
	p, confirmed := retentionPolicy(tx, a.TenantID)
	if !confirmed {
		h.fail(c, ErrRetentionNotConfirmed)
		return
	}
	now := h.Svc.now()
	if !in.Confirm {
		// Impact first: the caller sees exactly what will be removed and confirms.
		preview, err := applyRetention(tx, a.TenantID, p, now, true)
		if err != nil {
			h.fail(c, err)
			return
		}
		reply(c, 200, gin.H{"dryRun": true, "summary": preview}, "ok")
		return
	}
	sum, err := h.Svc.runRetentionTx(tx, a, p, now)
	if err != nil {
		h.fail(c, err)
		return
	}
	reply(c, 200, gin.H{"dryRun": false, "summary": sum}, "ok")
}

func (s *Service) runRetentionTx(tx *gorm.DB, a Actor, p model.EduRetentionPolicy, now time.Time) (RetentionSummary, error) {
	sum, err := applyRetention(tx, a.TenantID, p, now, false)
	if err != nil {
		return sum, err
	}
	line := sum.String()
	if err := tx.Model(&model.EduRetentionPolicy{}).Where("tenant_id = ?", a.TenantID).Updates(map[string]any{"last_run_at": now, "last_run_summary": line}).Error; err != nil {
		return sum, err
	}
	s.audit(tx, a, "retention.run", a.TenantID, line)
	return sum, nil
}

// RetentionEvery is the scheduled cleanup interval (EDU_RETENTION_EVERY, Go duration;
// default 24h; "0" or "off" disables; at least 1h).
func RetentionEvery() time.Duration {
	v := os.Getenv("EDU_RETENTION_EVERY")
	if v == "" {
		return 24 * time.Hour
	}
	if v == "off" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 24 * time.Hour
	}
	if d > 0 && d < time.Hour {
		d = time.Hour
	}
	return d
}

// RetentionAll applies every confirmed policy, one tenant transaction each, as the system actor.
func (s *Service) RetentionAll(ctx context.Context) (tenants int, err error) {
	var ids []string
	if err = s.DB.Model(&model.EduRetentionPolicy{}).Where("state = ?", "confirmed").Pluck("tenant_id", &ids).Error; err != nil {
		return 0, err
	}
	now := s.now()
	for _, tenant := range ids {
		if ctx.Err() != nil {
			return tenants, ctx.Err()
		}
		e := flow.TenantTx(s.DB, tenant, func(tx *gorm.DB) error {
			p, ok := retentionPolicy(tx, tenant)
			if !ok {
				return nil
			}
			_, err := s.runRetentionTx(tx, Actor{UserID: "system:retention", TenantID: tenant}, p, now)
			return err
		})
		if e != nil {
			slog.Warn("edumath retention run failed", "tenant", tenant, "error", e)
			continue
		}
		tenants++
	}
	return tenants, nil
}

// deleteStudentLearning counts (dry) or deletes one student's learning records and
// claimed roster entries: in one class's assignments, or in every class of the
// tenant when classID is "". Ledger, usage and audit rows are never touched.
func deleteStudentLearning(tx *gorm.DB, tenant, student, classID string, dry bool, sum *RetentionSummary) (bool, error) {
	scope := func(q *gorm.DB) *gorm.DB {
		q = q.Where("tenant_id = ? AND student_user_id = ?", tenant, student)
		if classID != "" {
			q = q.Where("assignment_id IN (?)", tx.Model(&model.EduAssignment{}).Select("id").Where("tenant_id = ? AND class_id = ?", tenant, classID))
		}
		return q
	}
	touched := false
	for _, m := range learningTables {
		var n int64
		if err := scope(tx.Model(m)).Count(&n).Error; err != nil {
			return touched, err
		}
		if n == 0 {
			continue
		}
		touched = true
		sum.LearningRows[m.(interface{ TableName() string }).TableName()] += int(n)
		if !dry {
			if err := scope(tx.Session(&gorm.Session{NewDB: true})).Delete(m).Error; err != nil {
				return touched, err
			}
		}
	}
	mn, err := deleteStudentMetrics(tx, tenant, student, classID, dry)
	if err != nil {
		return touched, err
	}
	if mn > 0 {
		touched = true
		sum.LearningRows["edu_metric_events"] += mn
	}
	rq := func(q *gorm.DB) *gorm.DB {
		q = q.Where("tenant_id = ? AND claimed_by = ?", tenant, student)
		if classID != "" {
			q = q.Where("class_id = ?", classID)
		}
		return q
	}
	var rn int64
	if err := rq(tx.Model(&model.EduRosterEntry{})).Count(&rn).Error; err != nil {
		return touched, err
	}
	if rn > 0 {
		touched = true
		sum.RosterEntries += int(rn)
		if !dry {
			if err := rq(tx.Session(&gorm.Session{NewDB: true})).Delete(&model.EduRosterEntry{}).Error; err != nil {
				return touched, err
			}
		}
	}
	return touched, nil
}
