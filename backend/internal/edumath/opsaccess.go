package edumath

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// Time-limited support access (DEV-108). A platform operator has no standing
// access to any institution's education data. An org admin grants one operator
// access for a stated purpose, a scope and at most 72 hours:
//   - scope "jobs": job metadata of the tenant (kind, status, stage, error code,
//     settlement, timestamps) for diagnosing failures; never inputs or outputs.
//   - scope "class": one class's attempt metadata (assignment, item, result,
//     time); student answer text only when includeContent was granted.
// Every operator read is audited (ops.read, with the purpose) and listed in the
// institution's access log; the grant can be revoked at any time. Operator
// requests carry X-Edu-Tenant and X-Edu-Ops-Grant and work on /edu/ops/* only.
//
// Not covered here (see 04 DEV-108): price changes are made in the Go platform's
// price book, outside the education module; direct database access by operators
// is controlled by the deployment (07), not by the application.

const maxOpsHours = 72

var ErrOpsGrant = appErr(http.StatusForbidden, "OPS_GRANT_INVALID", "no active support access grant for this request", "ask_institution")

func (h *API) registerOps(g *gin.RouterGroup) {
	g.GET("/ops-grants", h.listOpsGrants)
	g.POST("/ops-grants", h.createOpsGrant)
	g.POST("/ops-grants/:id/revoke", h.revokeOpsGrant)
	g.GET("/ops/jobs", h.opsJobs)
	g.GET("/ops/classes/:id/attempts", h.opsAttempts)
}

func opsState(gr model.EduOpsGrant, now time.Time) string {
	switch {
	case gr.Status == "revoked":
		return "revoked"
	case !gr.ExpiresAt.After(now):
		return "expired"
	}
	return "active"
}

func (h *API) listOpsGrants(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var gs []model.EduOpsGrant
	tx.Where("tenant_id = ?", a.TenantID).Order("created_at desc").Limit(100).Find(&gs)
	now := h.Svc.now()
	out := []gin.H{}
	for _, g := range gs {
		out = append(out, gin.H{"grant": g, "effective": opsState(g, now)})
	}
	reply(c, 200, out, "ok")
}

func (h *API) createOpsGrant(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var in struct {
		OperatorUserID string `json:"operatorUserId"`
		Scope          string `json:"scope"`
		ClassID        string `json:"classId"`
		IncludeContent bool   `json:"includeContent"`
		Purpose        string `json:"purpose"`
		Hours          int    `json:"hours"`
	}
	if c.ShouldBindJSON(&in) != nil || in.OperatorUserID == "" || len(in.OperatorUserID) > 40 || (in.Scope != "jobs" && in.Scope != "class") || in.Hours < 1 || in.Hours > maxOpsHours {
		h.fail(c, appErr(400, "VALIDATION_FAILED", fmt.Sprintf("operatorUserId, scope (jobs|class) and hours (1 to %d) required", maxOpsHours), "fix_input"))
		return
	}
	purpose, good := plainText(in.Purpose, 4, 200)
	if !good {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "purpose must be 4 to 200 characters without angle brackets or line breaks", "fix_input"))
		return
	}
	if in.Scope == "jobs" {
		if in.IncludeContent || in.ClassID != "" {
			h.fail(c, appErr(400, "VALIDATION_FAILED", "job scope carries metadata only; content needs a class scope", "fix_input"))
			return
		}
	} else {
		var n int64
		tx.Model(&model.EduClass{}).Where("id = ? AND tenant_id = ?", in.ClassID, a.TenantID).Count(&n)
		if n == 0 {
			h.fail(c, ErrNotFound)
			return
		}
	}
	if in.OperatorUserID == a.UserID {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "an admin cannot grant support access to themselves", "fix_input"))
		return
	}
	now := h.Svc.now()
	gr := model.EduOpsGrant{ID: flow.NewID("eog_"), TenantID: a.TenantID, OperatorUserID: in.OperatorUserID, Scope: in.Scope, ClassID: in.ClassID, IncludeContent: in.IncludeContent,
		Purpose: purpose, ExpiresAt: now.Add(time.Duration(in.Hours) * time.Hour), Status: "active", GrantedBy: a.UserID, CreatedAt: now, UpdatedAt: now}
	if err := tx.Create(&gr).Error; err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "ops.grant", gr.ID, fmt.Sprintf("operator=%s scope=%s class=%s content=%t hours=%d | %s", gr.OperatorUserID, gr.Scope, gr.ClassID, gr.IncludeContent, in.Hours, purpose))
	reply(c, 201, gr, "ok")
}

func (h *API) revokeOpsGrant(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var gr model.EduOpsGrant
	if tx.First(&gr, "id = ? AND tenant_id = ?", c.Param("id"), a.TenantID).Error != nil {
		h.fail(c, ErrNotFound)
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
	if err := tx.Model(&model.EduOpsGrant{}).Where("id = ?", gr.ID).Updates(map[string]any{"status": "revoked", "revoked_by": a.UserID, "revoked_at": now, "revoke_reason": reason, "updated_at": now}).Error; err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "ops.revoke", gr.ID, reason)
	reply(c, 200, gin.H{"id": gr.ID, "status": "revoked"}, "ok")
}

// ops resolves the operator's grant for this request: same user, same tenant,
// active and not expired. It never yields a staff or student actor.
func (h *API) ops(c *gin.Context) (Actor, *gorm.DB, model.EduOpsGrant, bool) {
	u, _ := c.Get("edumath.user")
	user, _ := u.(*model.User)
	tx := reqDB(c, h.DB)
	var gr model.EduOpsGrant
	if user == nil {
		reply(c, http.StatusUnauthorized, nil, "login required")
		return Actor{}, tx, gr, false
	}
	tenant, id := c.GetHeader("X-Edu-Tenant"), c.GetHeader("X-Edu-Ops-Grant")
	if tenant == "" || id == "" || tx.First(&gr, "id = ? AND tenant_id = ? AND operator_user_id = ?", id, tenant, user.ID).Error != nil || opsState(gr, h.Svc.now()) != "active" {
		h.fail(c, ErrOpsGrant)
		return Actor{}, tx, gr, false
	}
	return Actor{UserID: user.ID, TenantID: tenant}, tx, gr, true
}

func (h *API) opsJobs(c *gin.Context) {
	a, tx, gr, ok := h.ops(c)
	if !ok {
		return
	}
	if gr.Scope != "jobs" {
		h.fail(c, ErrOpsGrant)
		return
	}
	var js []model.EduMathJob
	tx.Select("id", "kind", "status", "stage", "error_code", "settlement", "created_at", "updated_at").Where("tenant_id = ?", a.TenantID).Order("created_at desc").Limit(100).Find(&js)
	out := []gin.H{}
	for _, j := range js {
		out = append(out, gin.H{"id": j.ID, "kind": j.Kind, "status": j.Status, "stage": j.Stage, "errorCode": j.ErrorCode, "settlement": j.Settlement, "createdAt": j.CreatedAt, "updatedAt": j.UpdatedAt})
	}
	h.Svc.audit(tx, a, "ops.read", gr.ID, "GET /api/edu/ops/jobs | "+gr.Purpose)
	reply(c, 200, out, "ok")
}

func (h *API) opsAttempts(c *gin.Context) {
	a, tx, gr, ok := h.ops(c)
	if !ok {
		return
	}
	if gr.Scope != "class" || gr.ClassID != c.Param("id") {
		h.fail(c, ErrOpsGrant)
		return
	}
	var ats []model.EduAttempt
	tx.Where("tenant_id = ? AND assignment_id IN (?)", a.TenantID, tx.Model(&model.EduAssignment{}).Select("id").Where("tenant_id = ? AND class_id = ?", a.TenantID, gr.ClassID)).
		Order("created_at desc").Limit(500).Find(&ats)
	out := []gin.H{}
	for _, t := range ats {
		row := gin.H{"assignmentId": t.AssignmentID, "studentUserId": t.StudentUserID, "item": t.Item, "kind": t.Kind, "result": t.Result, "valid": t.Valid, "createdAt": t.CreatedAt}
		if gr.IncludeContent {
			row["response"] = t.Response
		}
		out = append(out, row)
	}
	h.Svc.audit(tx, a, "ops.read", gr.ID, fmt.Sprintf("GET /api/edu/ops/classes/:id/attempts content=%t | %s", gr.IncludeContent, gr.Purpose))
	reply(c, 200, gin.H{"includeContent": gr.IncludeContent, "attempts": out}, "ok")
}
