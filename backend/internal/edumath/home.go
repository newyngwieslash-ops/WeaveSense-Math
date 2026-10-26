package edumath

import (
	"time"

	"github.com/gin-gonic/gin"

	"yingce/backend/internal/model"
)

// Teacher home (DEV-067, FR-T01): my lessons, my classes, lessons waiting for a review I may give,
// open student questions in my classes, my running jobs and open content incidents, all scoped to
// the current workspace and to the classes the caller teaches (org_admin: every class). The
// built-in example is the original SOLID-ANGLE-02 problem, flagged sample:true; it is only a
// template the author page fills in. The home never counts anything as activation or revenue.

type homeLesson struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	FamilyID  string    `json:"familyId"`
	State     string    `json:"state"`
	Mine      bool      `json:"mine"`
	CanReview bool      `json:"canReview"`
	CreatedAt time.Time `json:"createdAt"`
}

type homeClass struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Students          int64  `json:"students"`
	ActiveAssignments int64  `json:"activeAssignments"`
	OpenHelp          int64  `json:"openHelp"`
	UrgentHelp        int64  `json:"urgentHelp"`
}

func (h *API) registerHome(g *gin.RouterGroup) { g.GET("/home", h.home) }

func (h *API) home(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	// Lessons of this workspace (via their job), newest first.
	var rows []struct {
		model.EduLessonRevision
		Title    string
		FamilyID string
	}
	tx.Table("edu_lesson_revisions AS l").
		Select("l.*, p.title AS title, p.family_id AS family_id").
		Joins("JOIN edu_math_jobs j ON j.id = l.job_id AND j.tenant_id = l.tenant_id").
		Joins("JOIN edu_problem_revisions r ON r.id = l.problem_revision_id AND r.tenant_id = l.tenant_id").
		Joins("JOIN edu_problems p ON p.id = r.problem_id AND p.tenant_id = l.tenant_id").
		Where("l.tenant_id = ? AND j.workspace_id = ? AND l.state IN ?", a.TenantID, a.WorkspaceID, []string{"ready_for_review", "approved", "rejected"}).
		Order("l.created_at DESC").Limit(200).Scan(&rows)
	mine, pending := []homeLesson{}, []homeLesson{}
	for _, r := range rows {
		_, _, can := h.Svc.reviewAllowed(tx, a, r.CreatedBy)
		hl := homeLesson{ID: r.ID, Title: r.Title, FamilyID: r.FamilyID, State: r.State, Mine: r.CreatedBy == a.UserID, CanReview: can && r.State == "ready_for_review", CreatedAt: r.CreatedAt}
		if hl.Mine && len(mine) < 20 {
			mine = append(mine, hl)
		}
		if hl.CanReview && len(pending) < 20 {
			pending = append(pending, hl)
		}
	}
	// Classes the caller teaches (org_admin: all in the workspace).
	var ks []model.EduClass
	q := tx.Where("tenant_id = ? AND workspace_id = ? AND status = ?", a.TenantID, a.WorkspaceID, "active")
	if !a.Has("org_admin") {
		q = q.Where("id IN (?)", tx.Model(&model.EduTeachingAssignment{}).Select("class_id").Where("tenant_id = ? AND user_id = ? AND status = ?", a.TenantID, a.UserID, "active"))
	}
	q.Order("created_at").Find(&ks)
	classes := []homeClass{}
	var openHelp, urgent int64
	for _, k := range ks {
		hc := homeClass{ID: k.ID, Name: k.Name}
		tx.Model(&model.EduEnrollment{}).Where("tenant_id = ? AND class_id = ? AND status = ?", a.TenantID, k.ID, "active").Count(&hc.Students)
		tx.Model(&model.EduAssignment{}).Where("tenant_id = ? AND class_id = ? AND state = ?", a.TenantID, k.ID, "active").Count(&hc.ActiveAssignments)
		tx.Model(&model.EduHelpRequest{}).Where("tenant_id = ? AND class_id = ? AND state = ?", a.TenantID, k.ID, "open").Count(&hc.OpenHelp)
		tx.Model(&model.EduHelpRequest{}).Where("tenant_id = ? AND class_id = ? AND state = ? AND priority = 1", a.TenantID, k.ID, "open").Count(&hc.UrgentHelp)
		openHelp += hc.OpenHelp
		urgent += hc.UrgentHelp
		classes = append(classes, hc)
	}
	var jobs []model.EduMathJob
	tx.Where("tenant_id = ? AND workspace_id = ? AND created_by = ? AND status IN ?", a.TenantID, a.WorkspaceID, a.UserID,
		[]string{"queued", "running", "reconciliation_required"}).Order("created_at DESC").Limit(20).Find(&jobs)
	runJobs := []gin.H{}
	for _, j := range jobs {
		runJobs = append(runJobs, gin.H{"id": j.ID, "kind": j.Kind, "status": j.Status, "createdAt": j.CreatedAt})
	}
	var incidents int64
	tx.Model(&model.EduContentIncident{}).Where("tenant_id = ? AND workspace_id = ? AND state = ?", a.TenantID, a.WorkspaceID, "open").Count(&incidents)
	reply(c, 200, gin.H{
		"myLessons":     mine,
		"pendingReview": pending,
		"classes":       classes,
		"openHelp":      openHelp,
		"urgentHelp":    urgent,
		"runningJobs":   runJobs,
		"openIncidents": incidents,
		"reviewMode":    h.Svc.policy(tx, a.TenantID),
		"sample":        gin.H{"sample": true, "familyId": "SOLID-ANGLE-02", "title": "示例：正四棱锥侧棱与底面所成角", "note": "原创示例题，仅用于熟悉流程，不计入使用与经营指标"},
		"firstRun":      len(rows) == 0 && len(ks) == 0,
	}, "ok")
}
