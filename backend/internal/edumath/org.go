package edumath

import (
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/model"
)

// DEV-102 institution administration. The overview is aggregate only: counts of staff,
// courses, classes, enrolled students and consent coverage, never a student identity.
// An org_admin who does not teach a class reaches its individual student records
// (roster, roster entries, attempts, help, figure use, extensions) only by stating a
// business purpose (X-Edu-Access-Purpose, percent-encoded UTF-8, 4 to 200 characters);
// each such access is audited as org.individual_access and listed in the access log.

const accessPurposeHeader = "X-Edu-Access-Purpose"

// ErrPurposeRequired: individual records need a stated purpose when reached through org_admin.
var ErrPurposeRequired = &AppError{Status: http.StatusForbidden, Code: "ACCESS_PURPOSE_REQUIRED",
	Message: "individual student records need a stated business purpose", NextAction: "state_access_purpose"}

// governance actions shown in the access log; detail is shown only where it holds no student data.
var orgLogActions = []string{"org.individual_access", "policy.set", "budget.set", "class.teacher.add", "class.create",
	"class.roster.import", "class.roster.reissue", "class.roster.revoke", "class.enrollment.withdraw", "ledger.export",
	"role.bind", "role.renew", "role.revoke", "export.content", "export.report", "retention.set", "retention.run"}
var orgLogDetail = map[string]bool{"org.individual_access": true, "policy.set": true, "retention.set": true, "retention.run": true}

func (h *API) registerOrg(g *gin.RouterGroup) {
	g.GET("/org/overview", h.orgOverview)
	g.GET("/org/access-log", h.orgAccessLog)
}

func teachesClass(tx *gorm.DB, a Actor, classID string) bool {
	if !a.Has("teacher") {
		return false
	}
	var n int64
	tx.Model(&model.EduTeachingAssignment{}).Where("tenant_id = ? AND class_id = ? AND user_id = ? AND status = ?", a.TenantID, classID, a.UserID, "active").Count(&n)
	return n > 0
}

// individualAccess is called after classForStaff on routes that return or change one
// student's records. Teachers of the class pass; an org_admin must state a purpose,
// which is audited with the route.
func (h *API) individualAccess(c *gin.Context, tx *gorm.DB, a Actor, classID string) error {
	if teachesClass(tx, a, classID) || !a.Has("org_admin") {
		return nil
	}
	raw := c.GetHeader(accessPurposeHeader)
	p, err := url.QueryUnescape(raw)
	p = strings.TrimSpace(p)
	if err != nil || !utf8.ValidString(p) || utf8.RuneCountInString(p) < 4 || utf8.RuneCountInString(p) > 200 || strings.ContainsAny(p, "<>\r\n") {
		return ErrPurposeRequired
	}
	h.Svc.audit(tx, a, "org.individual_access", classID, c.Request.Method+" "+c.FullPath()+" | "+p)
	return nil
}

func (h *API) orgAdmin(c *gin.Context) (Actor, *gorm.DB, bool) {
	a, tx, ok := h.staff(c)
	if !ok {
		return a, tx, false
	}
	if !a.Has("org_admin") && !canManageRoles(tx, a.TenantID, a.UserID) {
		h.fail(c, ErrForbidden)
		return a, tx, false
	}
	return a, tx, true
}

func (h *API) orgOverview(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	now := h.Svc.now()
	staff := gin.H{}
	for _, r := range []string{"teacher", "reviewer", "org_admin"} {
		var n int64
		tx.Model(&model.EduRoleBinding{}).Where("tenant_id = ? AND role = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", a.TenantID, r, "active", now).
			Distinct("user_id").Count(&n)
		staff[r] = n
	}
	var courses int64
	tx.Model(&model.EduCourse{}).Where("tenant_id = ?", a.TenantID).Count(&courses)

	var ks []model.EduClass
	tx.Where("tenant_id = ? AND status = ?", a.TenantID, "active").Order("created_at, id").Limit(500).Find(&ks)
	classes := make([]gin.H, 0, len(ks))
	for _, k := range ks {
		var teachers, students, assignments int64
		tx.Model(&model.EduTeachingAssignment{}).Where("tenant_id = ? AND class_id = ? AND status = ?", a.TenantID, k.ID, "active").Count(&teachers)
		tx.Model(&model.EduEnrollment{}).Where("tenant_id = ? AND class_id = ? AND status = ?", a.TenantID, k.ID, "active").Count(&students)
		tx.Model(&model.EduAssignment{}).Where("tenant_id = ? AND class_id = ? AND state = ?", a.TenantID, k.ID, "active").Count(&assignments)
		classes = append(classes, gin.H{"id": k.ID, "name": k.Name, "workspaceId": k.WorkspaceID, "courseId": k.CourseID,
			"teachers": teachers, "students": students, "activeAssignments": assignments, "youTeach": teachesClass(tx, a, k.ID)})
	}
	var enrolled, consented int64
	sub := tx.Model(&model.EduEnrollment{}).Select("student_user_id").Where("tenant_id = ? AND status = ?", a.TenantID, "active")
	tx.Model(&model.EduEnrollment{}).Where("tenant_id = ? AND status = ?", a.TenantID, "active").Distinct("student_user_id").Count(&enrolled)
	tx.Model(&model.EduConsentRecord{}).Where("tenant_id = ? AND kind = ? AND status = ? AND student_user_id IN (?)", a.TenantID, "guardian", "granted", sub).
		Distinct("student_user_id").Count(&consented)

	var p model.EduOrgPolicy
	if tx.First(&p, "tenant_id = ?", a.TenantID).Error != nil {
		p = model.EduOrgPolicy{TenantID: a.TenantID, ReviewMode: "self_allowed", ConsentMode: "guardian_required", EditReviewInherit: inheritNone}
	}
	reply(c, 200, gin.H{
		"tenantId": a.TenantID, "staff": staff, "courses": courses, "classes": classes,
		"students":   gin.H{"enrolled": enrolled, "guardianConsent": consented, "consentMissing": enrolled - consented},
		"policy":     gin.H{"reviewMode": p.ReviewMode, "consentMode": p.ConsentMode, "editReviewInherit": p.EditReviewInherit, "updatedAt": p.UpdatedAt},
		"region":     gin.H{"state": "not_configured", "blockedBy": "DEV-105"},
		"retention":  retentionOverview(tx, a.TenantID),
		"individual": gin.H{"requiresPurpose": true, "header": accessPurposeHeader},
	}, "ok")
}

func (h *API) orgAccessLog(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var es []model.EduAuditEvent
	tx.Where("tenant_id = ? AND action IN ?", a.TenantID, orgLogActions).Order("created_at desc, id desc").Limit(200).Find(&es)
	out := make([]gin.H, 0, len(es))
	for _, e := range es {
		row := gin.H{"id": e.ID, "action": e.Action, "actorId": e.ActorID, "subject": e.Subject, "createdAt": e.CreatedAt}
		if orgLogDetail[e.Action] {
			row["detail"] = e.Detail
		}
		out = append(out, row)
	}
	reply(c, 200, out, "ok")
}

// retentionOverview reports whether the retention periods are confirmed and applied (DEV-106).
func retentionOverview(tx *gorm.DB, tenant string) gin.H {
	p, ok := retentionPolicy(tx, tenant)
	if !ok {
		return gin.H{"state": "suggested_not_enforced", "reference": "09 3.3"}
	}
	return gin.H{"state": "confirmed", "reference": "09 3.3", "learningDays": p.LearningDays, "tempDays": p.TempDays, "lastRunAt": p.LastRunAt}
}
