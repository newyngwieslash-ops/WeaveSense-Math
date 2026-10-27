package edumath

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// Data subject requests (DEV-107). An org admin records a request about one
// student, records how the requester's identity was verified (a reference to an
// offline check, never the document itself), previews the impact and then
// completes it. Kinds:
//   - access: returns, once, a package of the student's own records in this
//     tenant (enrollments, consent state, claimed roster entries, attempts
//     without teacher notes, help requests, counts for the rest). Nothing of the
//     package is stored; the request keeps counts only.
//   - withdraw: stops new processing: the student role is revoked (a join code
//     cannot undo it), guardian consent is set to withdrawn and active
//     enrollments are withdrawn. Records stay until retention or a delete request.
//   - delete: withdraw, then deletes the student's learning records and claimed
//     roster entries in every class of the tenant (online deletion). There are
//     no per student caches; media links are signed for at most 5 minutes and
//     re-check access, so they stop working at once. Backups expire after the
//     confirmed backup period. Ledger, usage and audit rows are kept on their own
//     basis and hold ids and counts, not learning content.
// Completed delete requests are deletion markers: after a backup restore,
// POST /data-requests/replay applies them again.

var (
	ErrDataRequestOpen        = appErr(http.StatusConflict, "DATA_REQUEST_OPEN", "an open request of this kind exists for this student", "finish_open_request")
	ErrDataRequestUnverified  = appErr(http.StatusConflict, "DATA_REQUEST_NOT_VERIFIED", "verify the requester's identity first", "verify_identity")
	ErrDataRequestClosed      = appErr(http.StatusConflict, "DATA_REQUEST_CLOSED", "the request is already completed or rejected", "reload")
	dataRequestKinds          = map[string]bool{"access": true, "withdraw": true, "delete": true}
	dataRequestRequesters     = map[string]bool{"student": true, "guardian": true, "institution": true}
	dataRequestLegalRetention = []gin.H{
		{"category": "ledger_usage", "basis": "accounting and billing records (Go platform ledger)"},
		{"category": "audit", "basis": "accountability for access and deletion; ids and counts only"},
	}
)

func (h *API) registerDataRequests(g *gin.RouterGroup) {
	g.GET("/data-requests", h.listDataRequests)
	g.POST("/data-requests", h.createDataRequest)
	g.POST("/data-requests/replay", h.replayDataRequests)
	g.POST("/data-requests/:id/verify", h.verifyDataRequest)
	g.POST("/data-requests/:id/reject", h.rejectDataRequest)
	g.POST("/data-requests/:id/complete", h.completeDataRequest)
}

// plainText: trimmed, rune length within bounds, no angle brackets or line breaks.
func plainText(s string, lo, hi int) (string, bool) {
	s = strings.TrimSpace(s)
	n := len([]rune(s))
	return s, n >= lo && n <= hi && !strings.ContainsAny(s, "<>\r\n")
}

func (h *API) listDataRequests(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var rs []model.EduDataRequest
	tx.Where("tenant_id = ?", a.TenantID).Order("created_at desc").Limit(200).Find(&rs)
	reply(c, 200, rs, "ok")
}

func (h *API) createDataRequest(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var in struct {
		StudentUserID string `json:"studentUserId"`
		Kind          string `json:"kind"`
		Requester     string `json:"requester"`
		Reason        string `json:"reason"`
	}
	if c.ShouldBindJSON(&in) != nil || !dataRequestKinds[in.Kind] || !dataRequestRequesters[in.Requester] || strings.TrimSpace(in.StudentUserID) == "" {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "studentUserId, kind (access|withdraw|delete) and requester (student|guardian|institution) required", "fix_input"))
		return
	}
	reason, good := plainText(in.Reason, 4, 200)
	if !good {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "reason must be 4 to 200 characters without angle brackets or line breaks", "fix_input"))
		return
	}
	var n int64
	tx.Model(&model.EduRoleBinding{}).Where("tenant_id = ? AND user_id = ? AND role = ?", a.TenantID, in.StudentUserID, "student").Count(&n)
	if n == 0 {
		h.fail(c, ErrNotFound)
		return
	}
	tx.Model(&model.EduDataRequest{}).Where("tenant_id = ? AND student_user_id = ? AND kind = ? AND status IN ?", a.TenantID, in.StudentUserID, in.Kind, []string{"received", "verified"}).Count(&n)
	if n > 0 {
		h.fail(c, ErrDataRequestOpen)
		return
	}
	now := h.Svc.now()
	r := model.EduDataRequest{ID: flow.NewID("edr_"), TenantID: a.TenantID, StudentUserID: in.StudentUserID, Kind: in.Kind, Requester: in.Requester, Reason: reason, Status: "received", CreatedBy: a.UserID, CreatedAt: now, UpdatedAt: now}
	if err := tx.Create(&r).Error; err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "datareq.create", r.ID, "kind="+r.Kind+" requester="+r.Requester)
	reply(c, 201, r, "ok")
}

func (h *API) dataRequest(c *gin.Context) (Actor, *gorm.DB, model.EduDataRequest, bool) {
	a, tx, ok := h.orgAdmin(c)
	var r model.EduDataRequest
	if !ok {
		return a, tx, r, false
	}
	if tx.First(&r, "id = ? AND tenant_id = ?", c.Param("id"), a.TenantID).Error != nil {
		h.fail(c, ErrNotFound)
		return a, tx, r, false
	}
	if r.Status == "completed" || r.Status == "rejected" {
		h.fail(c, ErrDataRequestClosed)
		return a, tx, r, false
	}
	return a, tx, r, true
}

func (h *API) verifyDataRequest(c *gin.Context) {
	a, tx, r, ok := h.dataRequest(c)
	if !ok {
		return
	}
	var in struct {
		VerificationRef string `json:"verificationRef"`
	}
	_ = c.ShouldBindJSON(&in)
	ref, good := plainText(in.VerificationRef, 4, 120)
	if !good {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "verificationRef (4 to 120 characters: the reference of the offline identity check, not the document) required", "fix_input"))
		return
	}
	now := h.Svc.now()
	if err := tx.Model(&model.EduDataRequest{}).Where("id = ?", r.ID).Updates(map[string]any{"status": "verified", "verification_ref": ref, "verified_by": a.UserID, "verified_at": now, "updated_at": now}).Error; err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "datareq.verify", r.ID, ref)
	reply(c, 200, gin.H{"id": r.ID, "status": "verified"}, "ok")
}

func (h *API) rejectDataRequest(c *gin.Context) {
	a, tx, r, ok := h.dataRequest(c)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&in)
	reason, good := plainText(in.Reason, 4, 200)
	if !good {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "reason must be 4 to 200 characters without angle brackets or line breaks", "fix_input"))
		return
	}
	now := h.Svc.now()
	out, _ := json.Marshal(gin.H{"rejected": reason})
	if err := tx.Model(&model.EduDataRequest{}).Where("id = ?", r.ID).Updates(map[string]any{"status": "rejected", "outcome": string(out), "resolved_by": a.UserID, "resolved_at": now, "updated_at": now}).Error; err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "datareq.reject", r.ID, reason)
	reply(c, 200, gin.H{"id": r.ID, "status": "rejected"}, "ok")
}

// stopProcessing revokes the student role, withdraws guardian consent and active
// enrollments in the tenant. Counts what changed (or would change when dry).
func stopProcessing(tx *gorm.DB, a Actor, student, why string, now time.Time, dry bool) (gin.H, error) {
	var roles, consents, enrolls int64
	tx.Model(&model.EduRoleBinding{}).Where("tenant_id = ? AND user_id = ? AND role = ? AND status = ?", a.TenantID, student, "student", "active").Count(&roles)
	tx.Model(&model.EduConsentRecord{}).Where("tenant_id = ? AND student_user_id = ? AND status = ?", a.TenantID, student, "granted").Count(&consents)
	tx.Model(&model.EduEnrollment{}).Where("tenant_id = ? AND student_user_id = ? AND status = ?", a.TenantID, student, "active").Count(&enrolls)
	if !dry {
		if err := tx.Model(&model.EduRoleBinding{}).Where("tenant_id = ? AND user_id = ? AND role = ? AND status = ?", a.TenantID, student, "student", "active").
			Updates(map[string]any{"status": "revoked", "revoked_by": a.UserID, "revoked_at": now, "reason": why, "updated_at": now}).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&model.EduConsentRecord{}).Where("tenant_id = ? AND student_user_id = ? AND status = ?", a.TenantID, student, "granted").
			Updates(map[string]any{"status": "withdrawn", "recorded_by": a.UserID, "updated_at": now}).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&model.EduEnrollment{}).Where("tenant_id = ? AND student_user_id = ? AND status = ?", a.TenantID, student, "active").
			Updates(map[string]any{"status": "withdrawn", "updated_at": now}).Error; err != nil {
			return nil, err
		}
	}
	return gin.H{"studentRoleRevoked": roles, "consentWithdrawn": consents, "enrollmentsWithdrawn": enrolls}, nil
}

// accessPackage collects the student's own records in this tenant. Teacher notes
// and anything not written by or about the student's own work are left out.
func accessPackage(tx *gorm.DB, tenant, student string) gin.H {
	var es []model.EduEnrollment
	tx.Where("tenant_id = ? AND student_user_id = ?", tenant, student).Order("created_at").Find(&es)
	enr := []gin.H{}
	for _, e := range es {
		var k model.EduClass
		tx.Select("name").First(&k, "id = ? AND tenant_id = ?", e.ClassID, tenant)
		enr = append(enr, gin.H{"classId": e.ClassID, "className": k.Name, "status": e.Status, "joinedAt": e.CreatedAt})
	}
	var cs []model.EduConsentRecord
	tx.Where("tenant_id = ? AND student_user_id = ?", tenant, student).Find(&cs)
	consent := []gin.H{}
	for _, r := range cs {
		consent = append(consent, gin.H{"kind": r.Kind, "status": r.Status, "updatedAt": r.UpdatedAt})
	}
	var ros []model.EduRosterEntry
	tx.Where("tenant_id = ? AND claimed_by = ?", tenant, student).Find(&ros)
	roster := []gin.H{}
	for _, r := range ros {
		roster = append(roster, gin.H{"classId": r.ClassID, "externalRef": r.ExternalRef, "displayName": r.DisplayName})
	}
	var ats []model.EduAttempt
	tx.Where("tenant_id = ? AND student_user_id = ?", tenant, student).Order("created_at").Find(&ats)
	attempts := []gin.H{}
	for _, t := range ats {
		attempts = append(attempts, gin.H{"assignmentId": t.AssignmentID, "item": t.Item, "kind": t.Kind, "response": t.Response, "result": t.Result, "late": t.Late, "createdAt": t.CreatedAt})
	}
	var hs []model.EduHelpRequest
	tx.Where("tenant_id = ? AND student_user_id = ?", tenant, student).Order("created_at").Find(&hs)
	help := []gin.H{}
	for _, r := range hs {
		help = append(help, gin.H{"assignmentId": r.AssignmentID, "kind": r.Kind, "message": r.Message, "state": r.State, "createdAt": r.CreatedAt})
	}
	counts := map[string]int64{}
	for _, m := range learningTables {
		var n int64
		tx.Model(m).Where("tenant_id = ? AND student_user_id = ?", tenant, student).Count(&n)
		counts[m.(interface{ TableName() string }).TableName()] = n
	}
	return gin.H{
		"studentUserId": student, "enrollments": enr, "consent": consent, "roster": roster,
		"attempts": attempts, "helpRequests": help, "counts": counts,
		"omitted": []gin.H{
			{"field": "teacherNote", "reason": "teacher working notes on an attempt"},
			{"field": "answers", "reason": "answer keys and unreleased solutions are content, not the student's data"},
		},
	}
}

func (h *API) completeDataRequest(c *gin.Context) {
	a, tx, r, ok := h.dataRequest(c)
	if !ok {
		return
	}
	if r.Status != "verified" {
		h.fail(c, ErrDataRequestUnverified)
		return
	}
	var in struct {
		Confirm bool `json:"confirm"`
	}
	_ = c.ShouldBindJSON(&in)
	now := h.Svc.now()
	dry := !in.Confirm
	outcome := gin.H{"kind": r.Kind}
	var pkg gin.H
	switch r.Kind {
	case "access":
		pkg = accessPackage(tx, a.TenantID, r.StudentUserID)
		outcome["records"] = pkg["counts"]
		if dry {
			reply(c, 200, gin.H{"dryRun": true, "impact": outcome}, "ok")
			return
		}
	case "withdraw", "delete":
		stop, err := stopProcessing(tx, a, r.StudentUserID, "data request "+r.ID, now, dry)
		if err != nil {
			h.fail(c, err)
			return
		}
		outcome["stopNewProcessing"] = stop
		if r.Kind == "delete" {
			sum := RetentionSummary{LearningRows: map[string]int{}}
			if _, err := deleteStudentLearning(tx, a.TenantID, r.StudentUserID, "", dry, &sum); err != nil {
				h.fail(c, err)
				return
			}
			p, _ := retentionPolicy(tx, a.TenantID)
			outcome["onlineDeleted"] = sum
			outcome["cacheCleared"] = "no per student caches; signed media links last at most 5 minutes and re-check access"
			outcome["backupExpiry"] = now.AddDate(0, 0, p.BackupDays).Format("2006-01-02")
			outcome["legalRetention"] = dataRequestLegalRetention
		} else {
			outcome["recordsKept"] = "until the retention period or a delete request"
		}
		if dry {
			reply(c, 200, gin.H{"dryRun": true, "impact": outcome}, "ok")
			return
		}
	}
	out, _ := json.Marshal(outcome)
	if err := tx.Model(&model.EduDataRequest{}).Where("id = ?", r.ID).Updates(map[string]any{"status": "completed", "outcome": string(out), "resolved_by": a.UserID, "resolved_at": now, "updated_at": now}).Error; err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "datareq.complete", r.ID, dataRequestLine(r.Kind, outcome))
	res := gin.H{"dryRun": false, "id": r.ID, "status": "completed", "outcome": outcome}
	if pkg != nil {
		res["package"] = pkg
	}
	reply(c, 200, res, "ok")
}

func dataRequestLine(kind string, o gin.H) string {
	line := "kind=" + kind
	if s, ok := o["onlineDeleted"].(RetentionSummary); ok {
		line += " " + s.String()
	}
	if s, ok := o["stopNewProcessing"].(gin.H); ok {
		line += fmt.Sprintf(" role_revoked=%v consent_withdrawn=%v enrollments_withdrawn=%v", s["studentRoleRevoked"], s["consentWithdrawn"], s["enrollmentsWithdrawn"])
	}
	return line
}

// replayDataRequests re-applies every completed delete request (after a backup
// restore brought deleted rows back). Without confirm it only counts.
func (h *API) replayDataRequests(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var in struct {
		Confirm bool `json:"confirm"`
	}
	_ = c.ShouldBindJSON(&in)
	dry := !in.Confirm
	now := h.Svc.now()
	var rs []model.EduDataRequest
	tx.Where("tenant_id = ? AND kind = ? AND status = ?", a.TenantID, "delete", "completed").Order("created_at").Find(&rs)
	sum := RetentionSummary{LearningRows: map[string]int{}}
	var restored int64
	for _, r := range rs {
		stop, err := stopProcessing(tx, a, r.StudentUserID, "replay of data request "+r.ID, now, dry)
		if err != nil {
			h.fail(c, err)
			return
		}
		restored += stop["studentRoleRevoked"].(int64) + stop["consentWithdrawn"].(int64) + stop["enrollmentsWithdrawn"].(int64)
		if _, err := deleteStudentLearning(tx, a.TenantID, r.StudentUserID, "", dry, &sum); err != nil {
			h.fail(c, err)
			return
		}
	}
	if !dry {
		h.Svc.audit(tx, a, "datareq.replay", a.TenantID, fmt.Sprintf("markers=%d states_reverted=%d %s", len(rs), restored, sum.String()))
	}
	reply(c, 200, gin.H{"dryRun": dry, "markers": len(rs), "statesReverted": restored, "deleted": sum}, "ok")
}
