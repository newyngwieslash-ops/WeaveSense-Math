package edumath

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// Reads (DEV-121) and client terminal (DEV-118). The student web client sends
// X-Edu-Terminal (phone, tablet or desktop, classified from the viewport width);
// anything else is recorded as unknown. Only successful reads of an assignment
// the student may open are counted; the row holds counts and times, no content.

const terminalHeader = "X-Edu-Terminal"

var terminals = map[string]bool{"phone": true, "tablet": true, "desktop": true}

func terminalOf(c *gin.Context) string {
	if t := c.GetHeader(terminalHeader); terminals[t] {
		return t
	}
	return "unknown"
}

// recordView counts one successful read. Failures never fail the read itself.
func (s *Service) recordView(tx *gorm.DB, tenant, assignment, student, terminal string) {
	now := s.now()
	where := "tenant_id = ? AND assignment_id = ? AND student_user_id = ? AND terminal = ?"
	upd := func(t *gorm.DB) *gorm.DB {
		return t.Model(&model.EduAssignmentView{}).Where(where, tenant, assignment, student, terminal).
			Updates(map[string]any{"views": gorm.Expr("views + 1"), "last_viewed_at": now})
	}
	if res := upd(tx); res.Error != nil || res.RowsAffected > 0 {
		return
	}
	row := model.EduAssignmentView{ID: flow.NewID("eav_"), TenantID: tenant, AssignmentID: assignment, StudentUserID: student, Terminal: terminal,
		Views: 1, FirstViewedAt: now, LastViewedAt: now}
	if tx.Transaction(func(t *gorm.DB) error { return t.Create(&row).Error }) != nil {
		upd(tx)
	}
}

// lessonOrigin reads generator, composer version and model from the lesson
// document's own meta (DEV-118). ModelRef is "none" only when the lesson
// explicitly records ai_generated=false with no AI parts; a recorded model
// name is kept; otherwise it stays empty (reported as not recorded).
func (s *Service) lessonOrigin(ctx context.Context, tenant string, art model.EduArtifact) (generator, composer, modelRef string) {
	raw, _, err := s.Client.Artifact(ctx, tenant, art)
	if err != nil {
		return "", "", ""
	}
	return lessonOriginOf(raw)
}

func lessonOriginOf(raw []byte) (generator, composer, modelRef string) {
	var doc struct {
		ComposerVersion string `json:"composer_version"`
		Meta            struct {
			Generator   string   `json:"generator"`
			AIGenerated *bool    `json:"ai_generated"`
			AIParts     []string `json:"ai_parts"`
			Model       string   `json:"model"`
		} `json:"meta"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return "", "", ""
	}
	clip := func(v string, n int) string {
		if len(v) > n {
			return v[:n]
		}
		return v
	}
	switch {
	case doc.Meta.Model != "":
		modelRef = doc.Meta.Model
	case doc.Meta.AIGenerated != nil && !*doc.Meta.AIGenerated && len(doc.Meta.AIParts) == 0:
		modelRef = "none"
	}
	return clip(doc.Meta.Generator, 40), clip(doc.ComposerVersion, 40), clip(modelRef, 60)
}

// readingStats summarizes reads for the observability view: students and
// assignments read in the window (by last read), first reads in the window,
// and distinct readers per terminal. Read counts are cumulative per row.
func readingStats(tx *gorm.DB, tenant string, from time.Time) (gin.H, error) {
	var rows []model.EduAssignmentView
	if err := tx.Select("assignment_id", "student_user_id", "terminal", "views", "first_viewed_at", "last_viewed_at").
		Where("tenant_id = ? AND last_viewed_at >= ?", tenant, from).Find(&rows).Error; err != nil {
		return nil, err
	}
	students, assignments := map[string]bool{}, map[string]bool{}
	byTerminal := map[string]map[string]bool{}
	var first int64
	for _, r := range rows {
		students[r.StudentUserID], assignments[r.AssignmentID] = true, true
		if byTerminal[r.Terminal] == nil {
			byTerminal[r.Terminal] = map[string]bool{}
		}
		byTerminal[r.Terminal][r.StudentUserID] = true
		if !r.FirstViewedAt.Before(from) {
			first++
		}
	}
	term := gin.H{}
	for t, s := range byTerminal {
		term[t] = len(s)
	}
	return gin.H{"collected": true, "students": len(students), "assignments": len(assignments), "firstReads": first, "readersByTerminal": term}, nil
}
