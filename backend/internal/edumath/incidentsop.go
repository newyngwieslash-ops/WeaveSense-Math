package edumath

// DEV-126 incident SOP, the parts that were missing on top of DEV-112 and
// DEV-125: locate (impact preview, trace), pause (pauses), revise (fix as an
// edit with a fresh human review) were already enforced. This adds
//
//   revision notices  when an incident is resolved with a fix publication, every
//                     current teacher of every class whose task still uses the
//                     incident publication gets one notice per task; it closes
//                     itself as "migrated" when the task moves to another
//                     publication, or the teacher acknowledges it.
//   post-incident     a resolved incident gets exactly one written review (root
//   review            cause, detection, corrective actions, follow ups); it is not
//                     editable, must not name students or carry contact details,
//                     and resolved blocking incidents without one are listed as
//                     review pending.

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// createRevisionNotices runs inside ResolveIncident when a fix is published.
func (s *Service) createRevisionNotices(tx *gorm.DB, inc model.EduContentIncident) (int, error) {
	if inc.FixPublicationID == "" {
		return 0, nil
	}
	var as []model.EduAssignment
	if err := tx.Where("tenant_id = ? AND publication_id = ? AND state IN ?", inc.TenantID, inc.PublicationID, []string{"active", "suspended"}).Order("created_at").Find(&as).Error; err != nil {
		return 0, err
	}
	n := 0
	now := s.now()
	for _, x := range as {
		var teachers []string
		if err := tx.Model(&model.EduTeachingAssignment{}).Where("tenant_id = ? AND class_id = ? AND status = 'active'", inc.TenantID, x.ClassID).Order("user_id").Pluck("user_id", &teachers).Error; err != nil {
			return n, err
		}
		for _, t := range teachers {
			rn := model.EduRevisionNotice{ID: flow.NewID("ern_"), TenantID: inc.TenantID, IncidentID: inc.ID, AssignmentID: x.ID, TeacherUserID: t, ClassID: x.ClassID,
				FromPublicationID: inc.PublicationID, FixPublicationID: inc.FixPublicationID, State: "unread", CreatedAt: now}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rn)
			if res.Error != nil {
				return n, res.Error
			}
			n += int(res.RowsAffected)
		}
	}
	return n, nil
}

// closeRevisionNotices marks the notices of a task as migrated once it moved.
func closeRevisionNotices(tx *gorm.DB, tenant, assignmentID string, now time.Time) error {
	return tx.Model(&model.EduRevisionNotice{}).Where("tenant_id = ? AND assignment_id = ? AND state IN ?", tenant, assignmentID, []string{"unread", "acknowledged"}).
		Updates(map[string]any{"state": "migrated", "closed_at": now}).Error
}

func (h *API) registerIncidentSOP(g *gin.RouterGroup) {
	g.GET("/revision-notices", h.listRevisionNotices)
	g.POST("/revision-notices/:id/ack", h.ackRevisionNotice)
	g.POST("/incidents/:id/review", h.reviewIncident)
}

func (h *API) listRevisionNotices(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	q := tx.Where("tenant_id = ? AND teacher_user_id = ?", a.TenantID, a.UserID)
	switch st := c.Query("state"); st {
	case "":
	case "unread", "acknowledged", "migrated":
		q = q.Where("state = ?", st)
	default:
		h.fail(c, appErr(400, "VALIDATION_FAILED", "state must be unread, acknowledged or migrated", "fix_input"))
		return
	}
	var rows []model.EduRevisionNotice
	q.Order("created_at desc").Limit(200).Find(&rows)
	out := []gin.H{}
	for _, r := range rows {
		// Only classes the teacher still teaches.
		var n int64
		tx.Model(&model.EduTeachingAssignment{}).Where("tenant_id = ? AND class_id = ? AND user_id = ? AND status = 'active'", a.TenantID, r.ClassID, a.UserID).Count(&n)
		if n == 0 {
			continue
		}
		var as model.EduAssignment
		tx.Select("id", "title", "state").First(&as, "id = ? AND tenant_id = ?", r.AssignmentID, a.TenantID)
		var inc model.EduContentIncident
		tx.Select("id", "severity", "reason").First(&inc, "id = ? AND tenant_id = ?", r.IncidentID, a.TenantID)
		out = append(out, gin.H{"notice": r, "assignmentTitle": as.Title, "assignmentState": as.State, "severity": inc.Severity, "reason": inc.Reason})
	}
	reply(c, 200, out, "ok")
}

func (h *API) ackRevisionNotice(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	var r model.EduRevisionNotice
	if tx.First(&r, "id = ? AND tenant_id = ? AND teacher_user_id = ?", c.Param("id"), a.TenantID, a.UserID).Error != nil {
		h.fail(c, ErrNotFound)
		return
	}
	if r.State != "unread" {
		reply(c, 200, gin.H{"notice": r, "changed": false}, "ok")
		return
	}
	now := h.Svc.now()
	if err := tx.Model(&model.EduRevisionNotice{}).Where("id = ? AND state = 'unread'", r.ID).Updates(map[string]any{"state": "acknowledged", "closed_at": now}).Error; err != nil {
		h.fail(c, err)
		return
	}
	r.State, r.ClosedAt = "acknowledged", &now
	h.Svc.audit(tx, a, "revision_notice.ack", r.ID, r.AssignmentID)
	reply(c, 200, gin.H{"notice": r, "changed": true}, "ok")
}

func (h *API) reviewIncident(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	var in struct {
		RootCause         string `json:"rootCause"`
		Detection         string `json:"detection"`
		CorrectiveActions string `json:"correctiveActions"`
		FollowUps         string `json:"followUps"`
	}
	if c.ShouldBindJSON(&in) != nil {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "invalid body", "fix_input"))
		return
	}
	in.RootCause, in.Detection = strings.TrimSpace(in.RootCause), strings.TrimSpace(in.Detection)
	in.CorrectiveActions, in.FollowUps = strings.TrimSpace(in.CorrectiveActions), strings.TrimSpace(in.FollowUps)
	bad := func(s string, lo, hi int) bool { n := len([]rune(s)); return n < lo || n > hi }
	if bad(in.RootCause, 4, 800) || bad(in.Detection, 2, 400) || bad(in.CorrectiveActions, 4, 800) || bad(in.FollowUps, 0, 800) {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "rootCause 4..800, detection 2..400, correctiveActions 4..800 and followUps up to 800 chars", "fix_input"))
		return
	}
	var inc model.EduContentIncident
	if tx.First(&inc, "id = ? AND tenant_id = ? AND workspace_id = ?", c.Param("id"), a.TenantID, a.WorkspaceID).Error != nil {
		h.fail(c, ErrNotFound)
		return
	}
	if _, _, err := lessonForStaff(tx, a, inc.LessonRevisionID); err != nil {
		h.fail(c, err)
		return
	}
	if inc.State != "resolved" {
		h.fail(c, appErr(409, "INCIDENT_OPEN", "resolve the incident before its review", "resolve_first"))
		return
	}
	// A review may be shared beyond the class: no student names or contact details.
	var classes []string
	tx.Model(&model.EduAssignment{}).Where("tenant_id = ? AND publication_id = ?", a.TenantID, inc.PublicationID).Distinct("class_id").Pluck("class_id", &classes)
	all := in.RootCause + "\n" + in.Detection + "\n" + in.CorrectiveActions + "\n" + in.FollowUps
	if emailRe.MatchString(all) || phoneRe.MatchString(all) {
		h.fail(c, appErr(http.StatusUnprocessableEntity, "PERSONAL_INFO", "a review must not name students or contain contact details", "remove_personal_info"))
		return
	}
	for _, k := range classes {
		if containsPersonalInfo(tx, a.TenantID, k, all) {
			h.fail(c, appErr(http.StatusUnprocessableEntity, "PERSONAL_INFO", "a review must not name students or contain contact details", "remove_personal_info"))
			return
		}
	}
	rv := model.EduIncidentReview{ID: flow.NewID("eir_"), TenantID: a.TenantID, IncidentID: inc.ID, RootCause: in.RootCause, Detection: in.Detection,
		CorrectiveActions: in.CorrectiveActions, FollowUps: in.FollowUps, CreatedBy: a.UserID, CreatedAt: h.Svc.now()}
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rv)
	if res.Error != nil {
		h.fail(c, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		h.fail(c, appErr(409, "REVIEW_EXISTS", "this incident already has a review; reviews are not edited", "reload"))
		return
	}
	h.Svc.audit(tx, a, "incident.review", inc.ID, rv.ID)
	reply(c, 201, rv, "ok")
}

// incidentSOPView adds the review and notice counts to one incident.
func incidentSOPView(tx *gorm.DB, inc model.EduContentIncident) gin.H {
	var rv model.EduIncidentReview
	var review any
	if tx.First(&rv, "incident_id = ? AND tenant_id = ?", inc.ID, inc.TenantID).Error == nil {
		review = rv
	}
	counts := gin.H{"unread": int64(0), "acknowledged": int64(0), "migrated": int64(0)}
	type row struct {
		State string
		N     int64
	}
	var rows []row
	tx.Model(&model.EduRevisionNotice{}).Select("state, COUNT(*) AS n").Where("tenant_id = ? AND incident_id = ?", inc.TenantID, inc.ID).Group("state").Scan(&rows)
	for _, r := range rows {
		counts[r.State] = r.N
	}
	return gin.H{"review": review, "reviewPending": inc.State == "resolved" && inc.Severity == "blocking" && review == nil, "notices": counts}
}
