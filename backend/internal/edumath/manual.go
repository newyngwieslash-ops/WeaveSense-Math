package edumath

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// Teacher written lessons (DEV-039, FR-M05). When a problem is outside the
// implemented families (UNSUPPORTED_FAMILY, next action manual_authoring) the
// teacher writes the statement, steps and answer. The math service only checks
// the shape and builds the same student projection and previews; it does no
// math. The lesson is manual_unverified everywhere (constraint 4): its solution
// record says so, it has no exact answer and no self-test, attempts on it are
// judged by the teacher, and no variants or wording edits are derived from it.

const (
	ManualUnverified = "manual_unverified"
	manualStatus     = "manual"
)

type manualBeat struct {
	Title         string   `json:"title"`
	Lines         []string `json:"lines"`
	RevealsAnswer bool     `json:"reveals_answer"`
}

type manualLesson struct {
	Title       string       `json:"title"`
	ProblemText string       `json:"problem_text"`
	Beats       []manualBeat `json:"beats"`
	Answer      struct {
		Plain string `json:"plain"`
	} `json:"answer"`
	Hints []string `json:"hints"`
}

func manualText(v string, limit int, what string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return appErr(400, "INPUT_AMBIGUOUS", what+" is required", "teacher_fix")
	}
	if utf8.RuneCountInString(v) > limit {
		return appErr(400, "PARAMETER_LIMIT", what+" is too long", "teacher_fix")
	}
	if strings.ContainsAny(v, "<>") || strings.IndexFunc(v, func(r rune) bool { return r < 0x20 && r != '\t' && r != '\n' || r == 0x7f }) >= 0 {
		return appErr(400, "INPUT_AMBIGUOUS", what+" contains markup or control characters", "teacher_fix")
	}
	return nil
}

// parseManual validates the content with the same limits as the service and
// returns its canonical JSON (sorted keys) for hashing and resubmission.
func parseManual(raw json.RawMessage) (manualLesson, string, error) {
	var m manualLesson
	if len(raw) == 0 || len(raw) > 32<<10 {
		return m, "", appErr(400, "PARAMETER_LIMIT", "manual lesson content is missing or larger than 32 KiB", "teacher_fix")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if dec.Decode(&m) != nil {
		return m, "", appErr(400, "INPUT_AMBIGUOUS", "manual lesson must be {title, problem_text, beats[{title, lines, reveals_answer}], answer{plain}, hints[]}", "teacher_fix")
	}
	checks := []error{manualText(m.Title, 60, "title"), manualText(m.ProblemText, 600, "problem_text"), manualText(m.Answer.Plain, 80, "answer")}
	if len(m.Beats) == 0 || len(m.Beats) > 12 {
		checks = append(checks, appErr(400, "PARAMETER_LIMIT", "1..12 beats are required", "teacher_fix"))
	}
	if len(m.Hints) > 5 {
		checks = append(checks, appErr(400, "PARAMETER_LIMIT", "at most 5 hints", "teacher_fix"))
	}
	reveals := false
	for _, b := range m.Beats {
		reveals = reveals || b.RevealsAnswer
		checks = append(checks, manualText(b.Title, 40, "beat title"))
		if len(b.Lines) == 0 || len(b.Lines) > 8 {
			checks = append(checks, appErr(400, "PARAMETER_LIMIT", "each beat needs 1..8 lines", "teacher_fix"))
		}
		for _, l := range b.Lines {
			checks = append(checks, manualText(l, 200, "beat line"))
		}
	}
	for _, hnt := range m.Hints {
		checks = append(checks, manualText(hnt, 200, "hint"))
	}
	if !reveals && len(m.Beats) > 0 {
		checks = append(checks, appErr(400, "INPUT_AMBIGUOUS", "mark the beat that reveals the answer", "teacher_fix"))
	}
	for _, e := range checks {
		if e != nil {
			return m, "", e
		}
	}
	var canon map[string]any
	_ = json.Unmarshal(raw, &canon)
	b, _ := json.Marshal(canon)
	return m, string(b), nil
}

// StartManualLesson dispatches a manual_lesson job for a confirmed problem revision.
func (s *Service) StartManualLesson(ctx context.Context, tx *gorm.DB, a Actor, revisionID string, raw json.RawMessage, idemKey string) (model.EduMathJob, error) {
	if !a.Has("teacher") {
		return model.EduMathJob{}, ErrNoRole
	}
	r, _, err := s.revision(tx, a, revisionID)
	if err != nil {
		return model.EduMathJob{}, err
	}
	if r.State != "confirmed" {
		return model.EduMathJob{}, appErr(409, "INPUT_AMBIGUOUS", "problem revision is not confirmed", "teacher_confirm_input")
	}
	_, canon, err := parseManual(raw)
	if err != nil {
		return model.EduMathJob{}, err
	}
	var problem map[string]any
	_ = json.Unmarshal([]byte(r.Structured), &problem)
	var content map[string]any
	_ = json.Unmarshal([]byte(canon), &content)
	contentHash := hashOf("manual", canon)
	payload := map[string]any{"problem": problem, "problem_hash": r.InputHash, "manual": content}
	job, err := s.startJob(ctx, tx, a, "manual_lesson", idemKey, hashOf("manual_lesson", r.ID, r.InputHash, contentHash), r.ID, "", payload)
	if err != nil || job.ID == "" {
		return job, err
	}
	in := model.EduManualInput{ID: flow.NewID("emi_"), TenantID: a.TenantID, JobID: job.ID, Content: canon, ContentHash: contentHash, CreatedBy: a.UserID, CreatedAt: s.now()}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&in).Error; err != nil {
		return job, err
	}
	return job, nil
}

// manualPayload restores the content of a manual_lesson job for a proven safe resubmission.
func manualPayload(tx *gorm.DB, job model.EduMathJob, payload map[string]any) error {
	var in model.EduManualInput
	if err := tx.First(&in, "tenant_id = ? AND job_id = ?", job.TenantID, job.ID).Error; err != nil {
		return ErrNotFound
	}
	var content map[string]any
	if json.Unmarshal([]byte(in.Content), &content) != nil {
		return ErrContract
	}
	payload["manual"] = content
	return nil
}

// materializeManual records the solution as manual_unverified (no math evidence)
// and the lesson revision bound to it, ready for human review unless the
// presentation check blocks it.
func (s *Service) materializeManual(ctx context.Context, tx *gorm.DB, job model.EduMathJob, arts map[string]model.EduArtifact) error {
	var n int64
	tx.Model(&model.EduLessonRevision{}).Where("job_id = ?", job.ID).Count(&n)
	if n > 0 {
		return nil
	}
	la := arts["lesson"]
	if la.ID == "" || arts["lesson_student"].ID == "" || arts["preview_html"].ID == "" || arts["preview_html_student"].ID == "" {
		return ErrContract
	}
	raw, _, err := s.Client.Artifact(ctx, job.TenantID, la)
	if err != nil {
		return err
	}
	var doc struct {
		Meta struct {
			Verification string `json:"verification"`
		} `json:"meta"`
		Answer struct {
			Exact string `json:"exact"`
		} `json:"answer"`
	}
	if json.Unmarshal(raw, &doc) != nil || doc.Meta.Verification != ManualUnverified || doc.Answer.Exact != "" {
		return ErrContract // a manual job must never come back looking verified
	}
	rc, rcState, rcBlocking, err := s.renderCheckOf(ctx, job, arts)
	if err != nil {
		return err
	}
	sol := model.EduSolutionRevision{ID: flow.NewID("esv_"), TenantID: job.TenantID, ProblemRevisionID: job.ProblemRevisionID, JobID: job.ID,
		SolutionArtifact: la.ID, SolutionHash: la.SHA256, SolutionStatus: manualStatus, MathState: ManualUnverified, EvidenceLevels: "[]", CreatedAt: s.now()}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&sol).Error; err != nil {
		return err
	}
	if err := tx.First(&sol, "job_id = ?", job.ID).Error; err != nil {
		return err
	}
	state, blocking := "ready_for_review", ""
	if len(rcBlocking) > 0 {
		state, blocking = "rejected", mergeBlocking("", rcBlocking)
	}
	gen, comp, mref := s.lessonOrigin(ctx, job.TenantID, la)
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.EduLessonRevision{ID: flow.NewID("elr_"), TenantID: job.TenantID,
		ProblemRevisionID: job.ProblemRevisionID, SolutionRevisionID: sol.ID, JobID: job.ID, LessonArtifact: la.ID, LessonHash: la.SHA256,
		StudentArtifact: arts["lesson_student"].ID, PreviewArtifact: arts["preview_html"].ID, StudentPreview: arts["preview_html_student"].ID,
		State: state, BlockingErrors: blocking, CreatedBy: job.CreatedBy, CreatedAt: s.now(),
		RenderCheckArtifact: arts["render_check"].ID, RenderCheckState: rcState, RenderCheck: rc,
		Generator: gen, ComposerVersion: comp, ModelRef: mref}).Error
}

// isManualSolution reports whether a lesson rests on teacher written content only.
func isManualSolution(tx *gorm.DB, l model.EduLessonRevision) bool {
	var sol model.EduSolutionRevision
	return tx.First(&sol, "id = ? AND tenant_id = ?", l.SolutionRevisionID, l.TenantID).Error == nil && sol.MathState == ManualUnverified
}

func errManual(what string) error {
	return appErr(409, "MANUAL_UNVERIFIED", what+" is not available for a teacher written lesson that was not verified automatically", "teacher_judge")
}

func (h *API) manualLesson(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	var in struct {
		Lesson         json.RawMessage `json:"lesson"`
		IdempotencyKey string          `json:"idempotencyKey"`
	}
	if c.ShouldBindJSON(&in) != nil {
		h.fail(c, appErr(400, "INPUT_AMBIGUOUS", "body must be {lesson: {...}}", "teacher_fix"))
		return
	}
	ctx, cancel := ctx15(c)
	defer cancel()
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		key = in.IdempotencyKey
	}
	j, err := h.Svc.StartManualLesson(ctx, tx, a, c.Param("id"), in.Lesson, key)
	if err != nil {
		h.fail(c, err)
		return
	}
	reply(c, 202, jobOut(j), "accepted")
}
