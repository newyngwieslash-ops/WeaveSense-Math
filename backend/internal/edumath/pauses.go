package edumath

// DEV-125: reversible pauses at four granularities, on top of the global switch
// (unset FLOW_MATH_SERVICE_URL) and the platform wide EDU_PAUSE_GENERATION.
//
//   generation   no new math job in the tenant (running jobs finish and settle)
//   family       no new job, publish, assignment or student access for one family
//   publication  no new assignment or student access for one published version
//   renderer     the same for every lesson whose render check names that renderer
//
// A pause only blocks; it never deletes or rewrites ledger rows, attempts,
// deletion markers or audit. Lifting a pause restores exactly what it blocked:
// a publication withdrawn meanwhile stays withdrawn (student access requires
// state published). Rolling a class back to an earlier valid version is the
// assignment migration (POST /edu/assignments/:id/migrate) to an older
// publication that is still published and not paused.

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

var pauseScopes = map[string]bool{"generation": true, "family": true, "publication": true, "renderer": true}

var (
	ErrGenerationPaused = appErr(423, "GENERATION_PAUSED", "new generation is paused", "wait_or_contact_admin")
	ErrContentPaused    = appErr(423, "CONTENT_PAUSED", "this content is paused by the institution", "wait_or_contact_admin")
)

func platformGenerationPaused() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("EDU_PAUSE_GENERATION")))
	return v == "1" || v == "true" || v == "on"
}

func activePause(tx *gorm.DB, tenant, scope, target string) bool {
	if target == "" && scope != "generation" {
		return false
	}
	var n int64
	tx.Model(&model.EduPause{}).Where("tenant_id = ? AND scope = ? AND target_id = ? AND state = ?", tenant, scope, target, "active").Count(&n)
	return n > 0
}

func familyOfRevision(tx *gorm.DB, tenant, problemRevisionID string) string {
	var r model.EduProblemRevision
	if tx.Select("id", "problem_id").First(&r, "id = ? AND tenant_id = ?", problemRevisionID, tenant).Error != nil {
		return ""
	}
	var p model.EduProblem
	if tx.Select("id", "family_id").First(&p, "id = ? AND tenant_id = ?", r.ProblemID, tenant).Error != nil {
		return ""
	}
	return p.FamilyID
}

func rendererOf(l model.EduLessonRevision) string {
	if v := renderCheckView(l); v != nil {
		return v.Renderer
	}
	return ""
}

// generationAllowed is checked before a new math job is created.
func generationAllowed(tx *gorm.DB, tenant, problemRevisionID string) error {
	if platformGenerationPaused() || activePause(tx, tenant, "generation", "") {
		return ErrGenerationPaused
	}
	if activePause(tx, tenant, "family", familyOfRevision(tx, tenant, problemRevisionID)) {
		return ErrGenerationPaused
	}
	return nil
}

// lessonAllowed is checked before publishing a lesson revision.
func lessonAllowed(tx *gorm.DB, tenant string, l model.EduLessonRevision) error {
	if activePause(tx, tenant, "family", familyOfRevision(tx, tenant, l.ProblemRevisionID)) || activePause(tx, tenant, "renderer", rendererOf(l)) {
		return ErrContentPaused
	}
	return nil
}

// publicationAllowed is checked before a new assignment, a migration target and
// every student read.
func publicationAllowed(tx *gorm.DB, tenant string, pub model.EduPublication, l model.EduLessonRevision) error {
	if activePause(tx, tenant, "publication", pub.ID) {
		return ErrContentPaused
	}
	return lessonAllowed(tx, tenant, l)
}

func (h *API) registerPauses(g *gin.RouterGroup) {
	g.GET("/pauses", h.listPauses)
	g.POST("/pauses", h.createPause)
	g.POST("/pauses/:id/lift", h.liftPause)
}

func (h *API) listPauses(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	q := tx.Where("tenant_id = ?", a.TenantID)
	if st := c.Query("state"); st == "active" || st == "lifted" {
		q = q.Where("state = ?", st)
	}
	var out []model.EduPause
	q.Order("created_at desc").Limit(200).Find(&out)
	reply(c, 200, gin.H{"pauses": out, "platformGenerationPaused": platformGenerationPaused()}, "ok")
}

type pauseImpact struct {
	Assignments  int64 `json:"assignments"`
	Publications int64 `json:"publications"`
	Students     int64 `json:"students"`
	OpenJobs     int64 `json:"openJobs"`
}

// pauseTarget validates the target and returns the publications it covers.
func pauseTarget(tx *gorm.DB, tenant, scope, target string) ([]string, error) {
	bad := appErr(400, "VALIDATION_FAILED", "unknown pause target", "fix_input")
	var pubs []model.EduPublication
	tx.Where("tenant_id = ? AND state = ?", tenant, "published").Find(&pubs)
	var ids []string
	switch scope {
	case "generation":
		if target != "" {
			return nil, bad
		}
		return nil, nil
	case "publication":
		var p model.EduPublication
		if tx.First(&p, "id = ? AND tenant_id = ?", target, tenant).Error != nil {
			return nil, ErrNotFound
		}
		if p.State != "published" {
			return nil, appErr(409, "REVISION_CONFLICT", "only a published version can be paused; a withdrawn one is already stopped", "reload")
		}
		return []string{p.ID}, nil
	case "family":
		if !objectIDish(target) {
			return nil, bad
		}
		var n int64
		tx.Model(&model.EduProblem{}).Where("tenant_id = ? AND family_id = ?", tenant, target).Count(&n)
		if n == 0 {
			return nil, ErrNotFound
		}
		for _, p := range pubs {
			var l model.EduLessonRevision
			if tx.Select("id", "problem_revision_id").First(&l, "id = ?", p.LessonRevisionID).Error == nil && familyOfRevision(tx, tenant, l.ProblemRevisionID) == target {
				ids = append(ids, p.ID)
			}
		}
		return ids, nil
	case "renderer":
		if target == "" || len(target) > 40 || strings.ContainsAny(target, " \t\n") {
			return nil, bad
		}
		found := false
		var ls []model.EduLessonRevision
		tx.Select("id", "render_check").Where("tenant_id = ? AND render_check <> ''", tenant).Find(&ls)
		byLesson := map[string]bool{}
		for _, l := range ls {
			if rendererOf(l) == target {
				found = true
				byLesson[l.ID] = true
			}
		}
		if !found {
			return nil, ErrNotFound
		}
		for _, p := range pubs {
			if byLesson[p.LessonRevisionID] {
				ids = append(ids, p.ID)
			}
		}
		return ids, nil
	}
	return nil, bad
}

func objectIDish(s string) bool {
	if s == "" || len(s) > 60 {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r == '-' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

func impactOf(tx *gorm.DB, tenant, scope string, pubs []string) pauseImpact {
	var imp pauseImpact
	imp.Publications = int64(len(pubs))
	if scope == "generation" {
		tx.Model(&model.EduMathJob{}).Where("tenant_id = ? AND status IN ?", tenant, []string{"queued", "running", "reconciliation_required"}).Count(&imp.OpenJobs)
		return imp
	}
	if len(pubs) == 0 {
		return imp
	}
	var classIDs []string
	tx.Model(&model.EduAssignment{}).Where("tenant_id = ? AND publication_id IN ? AND state = 'active'", tenant, pubs).Count(&imp.Assignments)
	tx.Model(&model.EduAssignment{}).Where("tenant_id = ? AND publication_id IN ? AND state = 'active'", tenant, pubs).Distinct("class_id").Pluck("class_id", &classIDs)
	if len(classIDs) > 0 {
		tx.Model(&model.EduEnrollment{}).Where("tenant_id = ? AND class_id IN ? AND status = 'active'", tenant, classIDs).Distinct("student_user_id").Count(&imp.Students)
	}
	return imp
}

// createPause previews the impact without confirm, and with confirm records an
// active pause (idempotent per scope and target).
func (h *API) createPause(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var in struct {
		Scope    string `json:"scope"`
		TargetID string `json:"targetId"`
		Reason   string `json:"reason"`
		Confirm  bool   `json:"confirm"`
	}
	if c.ShouldBindJSON(&in) != nil || !pauseScopes[in.Scope] {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "scope must be generation, family, publication or renderer", "fix_input"))
		return
	}
	in.TargetID, in.Reason = strings.TrimSpace(in.TargetID), strings.TrimSpace(in.Reason)
	if in.Reason == "" || len([]rune(in.Reason)) > 400 {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "reason 1..400 chars is required", "fix_input"))
		return
	}
	pubs, err := pauseTarget(tx, a.TenantID, in.Scope, in.TargetID)
	if err != nil {
		h.fail(c, err)
		return
	}
	imp := impactOf(tx, a.TenantID, in.Scope, pubs)
	if !in.Confirm {
		reply(c, 200, gin.H{"dryRun": true, "scope": in.Scope, "targetId": in.TargetID, "impact": imp}, "ok")
		return
	}
	if tx.Dialector.Name() == "postgres" {
		tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "edu-pause:"+a.TenantID+":"+in.Scope+":"+in.TargetID)
	}
	var existing model.EduPause
	if tx.Where("tenant_id = ? AND scope = ? AND target_id = ? AND state = ?", a.TenantID, in.Scope, in.TargetID, "active").First(&existing).Error == nil {
		reply(c, 200, gin.H{"pause": existing, "impact": imp, "created": false}, "ok")
		return
	}
	p := model.EduPause{ID: flow.NewID("eps_"), TenantID: a.TenantID, Scope: in.Scope, TargetID: in.TargetID, Reason: in.Reason, State: "active", CreatedBy: a.UserID, CreatedAt: h.Svc.now()}
	if err := tx.Create(&p).Error; err != nil {
		h.fail(c, err)
		return
	}
	raw, _ := json.Marshal(imp)
	h.Svc.audit(tx, a, "pause."+in.Scope, p.ID, in.TargetID+" "+string(raw))
	reply(c, 201, gin.H{"pause": p, "impact": imp, "created": true}, "ok")
}

func (h *API) liftPause(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var in struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&in)
	in.Note = strings.TrimSpace(in.Note)
	if in.Note == "" || len([]rune(in.Note)) > 400 {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "note 1..400 chars is required", "fix_input"))
		return
	}
	var p model.EduPause
	if tx.First(&p, "id = ? AND tenant_id = ?", c.Param("id"), a.TenantID).Error != nil {
		h.fail(c, ErrNotFound)
		return
	}
	if p.State != "active" {
		reply(c, 200, gin.H{"pause": p, "lifted": false}, "ok")
		return
	}
	now := h.Svc.now()
	if err := tx.Model(&model.EduPause{}).Where("id = ? AND state = ?", p.ID, "active").
		Updates(map[string]any{"state": "lifted", "lifted_by": a.UserID, "lifted_at": now, "lift_note": in.Note}).Error; err != nil {
		h.fail(c, err)
		return
	}
	p.State, p.LiftedBy, p.LiftedAt, p.LiftNote = "lifted", a.UserID, &now, in.Note
	h.Svc.audit(tx, a, "pause.lift", p.ID, p.Scope+" "+p.TargetID)
	reply(c, 200, gin.H{"pause": p, "lifted": true}, "ok")
}
