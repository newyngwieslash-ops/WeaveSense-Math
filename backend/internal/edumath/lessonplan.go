package edumath

import (
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// Teaching goal, style and target length for a lesson build (DEV-070). The
// math service records them in the lesson meta; they set the pacing used for
// the length estimate and never change the verified mathematics. Absent means
// the defaults and a byte-identical lesson to builds before DEV-070.
var (
	teachingGoals  = []string{"understand_method", "practice", "fix_mistake"}
	teachingStyles = []string{"concise", "standard", "detailed"}
)

const (
	minLessonMinutes     = 3
	maxLessonMinutes     = 8
	defaultLessonMinutes = 5
)

// Upper bounds of the lesson_build stages: the math service's per stage CPU
// budgets (jobs.py TIMEOUTS). Settlement is on measured compute only.
var lessonStageMaxMS = []struct {
	Name  string
	MaxMS int64
}{{"compose_lesson", 10000}, {"project_student", 5000}, {"render_preview", 30000}, {"check_render", 30000}}

type teachingInput struct {
	Goal          string `json:"goal"`
	Style         string `json:"style"`
	TargetMinutes int    `json:"targetMinutes"`
}

// teachingParams validates the optional body; nil means none was given.
func teachingParams(in teachingInput) (map[string]any, error) {
	if in.Goal == "" && in.Style == "" && in.TargetMinutes == 0 {
		return nil, nil
	}
	if in.Goal != "" && !containsKind(teachingGoals, in.Goal) {
		return nil, appErr(400, "INPUT_AMBIGUOUS", "unsupported teaching goal", "")
	}
	if in.Style == "" {
		in.Style = "standard"
	}
	if !containsKind(teachingStyles, in.Style) {
		return nil, appErr(400, "INPUT_AMBIGUOUS", "unsupported teaching style", "")
	}
	if in.TargetMinutes == 0 {
		in.TargetMinutes = defaultLessonMinutes
	}
	if in.TargetMinutes < minLessonMinutes || in.TargetMinutes > maxLessonMinutes {
		return nil, appErr(400, "INPUT_AMBIGUOUS", "targetMinutes must be 3 to 8", "")
	}
	return map[string]any{"goal": in.Goal, "style": in.Style, "target_minutes": in.TargetMinutes}, nil
}

func teachingHash(t map[string]any) string {
	if t == nil {
		return ""
	}
	raw, _ := json.Marshal(t)
	return string(raw)
}

func (h *API) buildLesson(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	var in struct {
		IdempotencyKey string `json:"idempotencyKey"`
		teachingInput
	}
	raw, _ := io.ReadAll(io.LimitReader(c.Request.Body, 1<<16))
	if len(raw) > 0 && json.Unmarshal(raw, &in) != nil {
		reply(c, 400, nil, "invalid lesson body")
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		key = in.IdempotencyKey
	}
	teaching, err := teachingParams(in.teachingInput)
	if err != nil {
		h.fail(c, err)
		return
	}
	ctx, cancel := ctx15(c)
	defer cancel()
	j, err := h.Svc.StartLessonBuild(ctx, tx, a, c.Param("id"), key, teaching)
	if err != nil {
		h.fail(c, err)
		return
	}
	reply(c, 202, jobOut(j), "accepted")
}

type planBlocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Action  string `json:"action"`
}

// lessonPlan (GET /solution-revisions/:id/lesson-plan) is what the teacher
// sees before starting a lesson build: whether it would be admitted now and,
// if not, why and what to do; the goal, style and length choices; the per
// stage upper bound and cost; and the organization's queue. Read only: it
// reserves nothing and raises no budget alert.
func (h *API) lessonPlan(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	var sol model.EduSolutionRevision
	if err := tx.First(&sol, "id = ? AND tenant_id = ?", c.Param("id"), a.TenantID).Error; err != nil {
		h.fail(c, ErrNotFound)
		return
	}
	r, _, err := h.Svc.revision(tx, a, sol.ProblemRevisionID)
	if err != nil {
		h.fail(c, err)
		return
	}
	blockers := []planBlocker{}
	add := func(err error) {
		if err == nil {
			return
		}
		var ae *AppError
		if errors.As(err, &ae) {
			blockers = append(blockers, planBlocker{ae.Code, ae.Message, ae.NextAction})
			return
		}
		if errors.Is(err, ErrNoRole) {
			blockers = append(blockers, planBlocker{"FORBIDDEN", "teacher role required", "contact_admin"})
			return
		}
		blockers = append(blockers, planBlocker{"INTERNAL", "admission check failed", "retry_later"})
	}
	if !a.Has("teacher") {
		add(ErrNoRole)
	}
	if sol.MathState != "verified" {
		add(appErr(409, "VALIDATION_FAILED", "solution is not verified; lesson generation is blocked", "teacher_review"))
	}
	add(generationAllowed(tx, a.TenantID, r.ID))
	if !h.Svc.Client.Configured() {
		add(appErr(503, "PROVIDER_UNAVAILABLE", "math service is not configured", "contact_admin"))
	}
	pb := flow.CurrentPriceBook()
	add(h.Svc.admission(tx, a, "lesson_build", JobBudgetMicro(), unitPrice(pb) > 0, true))
	unit := unitPrice(pb)
	stages, total := []gin.H{}, int64(0)
	for _, s := range lessonStageMaxMS {
		stages = append(stages, gin.H{"name": s.Name, "maxComputeMs": s.MaxMS, "maxCostMicro": s.MaxMS * unit})
		total += s.MaxMS
	}
	maxCost := total * unit
	if b := JobBudgetMicro(); maxCost > b {
		maxCost = b
	}
	var queued, running int64
	tx.Model(&model.EduMathJob{}).Where("tenant_id = ? AND status = ?", a.TenantID, "queued").Count(&queued)
	tx.Model(&model.EduMathJob{}).Where("tenant_id = ? AND status IN ?", a.TenantID, []string{"running", "reconciliation_required"}).Count(&running)
	reply(c, 200, gin.H{
		"admitted": len(blockers) == 0, "blockers": blockers,
		"teaching": gin.H{"goals": teachingGoals, "styles": teachingStyles, "minMinutes": minLessonMinutes, "maxMinutes": maxLessonMinutes, "defaultMinutes": defaultLessonMinutes},
		"estimate": gin.H{"usageType": flow.UsageMathComputeMS, "priceVersion": pb.Version, "unitMicro": unit, "stages": stages, "maxComputeMs": total,
			"maxCostMicro": maxCost, "reservationMicro": JobBudgetMicro(), "note": "upper bound from stage timeouts; settled on measured compute only"},
		"queue": gin.H{"queued": queued, "running": running},
	}, "ok")
}

// recoveryActions are the next steps a teacher can take for a job that did
// not succeed (DEV-070). An unknown provider outcome is reconciled, never
// retried blindly (hard constraint 5); a timeout is "undetermined", not "no
// solution".
func recoveryActions(j model.EduMathJob) []string {
	switch j.Status {
	case "reconciliation_required":
		return []string{"reconcile"}
	case "cancelled":
		return []string{"restart_new_key"}
	case "failed":
		switch j.ErrorCode {
		case "TIMEOUT_UNDETERMINED", "PROVIDER_UNAVAILABLE", "INTERNAL", "RATE_LIMITED":
			return []string{"restart_new_key"}
		case "BUDGET_EXCEEDED":
			return []string{"raise_budget", "restart_new_key"}
		case "VALIDATION_FAILED", "INPUT_AMBIGUOUS", "UNSUPPORTED_FAMILY", "PARAMETER_LIMIT", "CONDITION_CONFLICT", "METHOD_MISMATCH", "REVISION_CONFLICT":
			return []string{"edit_problem"}
		case "GENERATION_PAUSED":
			return []string{"wait_for_resume"}
		}
		return []string{"contact_admin"}
	}
	return []string{}
}

// lessonDuration estimates a lesson's spoken length from its narration text
// at the style's pace (about 4, 3.3 or 2.8 Chinese characters a second) plus a
// pause per board line. There is no audio yet (DEV-022 blocked), so this is an
// estimate, labelled as such, and is compared with the 3 to 8 minute guidance
// and the teacher's target.
func lessonDuration(says []string, boardLines int, style string, target int) gin.H {
	perSec := map[string]float64{"concise": 4.0, "standard": 3.3, "detailed": 2.8}[style]
	if perSec == 0 {
		perSec = 3.3
	}
	chars := 0
	for _, s := range says {
		chars += utf8.RuneCountInString(s)
	}
	secs := int(float64(chars)/perSec) + 2*boardLines + 3*len(says)
	fit := "within_guidance"
	switch {
	case secs < minLessonMinutes*60:
		fit = "shorter_than_guidance"
	case secs > maxLessonMinutes*60:
		fit = "longer_than_guidance"
	}
	out := gin.H{"estimatedSeconds": secs, "basis": "narration_text_estimate", "guidance": gin.H{"minMinutes": minLessonMinutes, "maxMinutes": maxLessonMinutes}, "fit": fit}
	if target > 0 {
		out["targetMinutes"] = target
		d := secs - target*60
		switch {
		case d < -60:
			out["targetFit"] = "shorter"
		case d > 60:
			out["targetFit"] = "longer"
		default:
			out["targetFit"] = "close"
		}
	}
	return out
}
