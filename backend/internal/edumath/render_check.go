package edumath

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"yingce/backend/internal/model"
)

// Presentation check (DEV-082). The math service rebuilds both previews in a
// separate clean, empty-environment stage (check_render) and checks them for
// external resources, host paths, glyph coverage, figure overflow, label
// occlusion and figure/text consistency. The platform validates the report
// strictly, stores a compact summary on the lesson revision, and turns every
// failed check into a blocking error, so a known presentation fault cannot be
// approved or published (constraint 3). A passed check says nothing about the
// math (constraint 4): the report must declare math_verification=false.

const renderCheckSchema = "edu.render_check/1"

var (
	renderCheckIDRe   = regexp.MustCompile(`^[a-z][a-z_]{1,39}$`)
	renderCheckCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,47}$`)
	renderCheckStatus = map[string]bool{"pass": true, "warn": true, "fail": true, "not_applicable": true}
	renderCheckStates = map[string]bool{"passed": true, "warnings": true, "failed": true}
)

type renderCheckItem struct {
	ID     string   `json:"id"`
	Status string   `json:"status"`
	Count  int      `json:"count,omitempty"`
	Codes  []string `json:"codes,omitempty"`
}

type renderCheckReport struct {
	SchemaVersion    string            `json:"schema_version"`
	Checker          string            `json:"checker"`
	Renderer         string            `json:"renderer"`
	Scope            string            `json:"scope"`
	MathVerification *bool             `json:"math_verification"`
	Environment      string            `json:"environment"`
	State            string            `json:"state"`
	Blocking         []string          `json:"blocking"`
	Warnings         []string          `json:"warnings"`
	Checks           []renderCheckItem `json:"checks"`
	TeacherHTMLSHA   string            `json:"teacher_html_sha256"`
	StudentHTMLSHA   string            `json:"student_html_sha256"`
}

// RenderCheckSummary is what the teacher API returns; codes and counts only, no lesson text.
type RenderCheckSummary struct {
	State            string            `json:"state"`
	Scope            string            `json:"scope"`
	MathVerification bool              `json:"mathVerification"`
	Checker          string            `json:"checker"`
	Renderer         string            `json:"renderer"`
	Environment      string            `json:"environment"`
	Blocking         []string          `json:"blocking"`
	Warnings         []string          `json:"warnings"`
	Checks           []renderCheckItem `json:"checks"`
}

func (r renderCheckReport) validate(teacherSHA, studentSHA string) error {
	bad := func(what string) error { return fmt.Errorf("%w: render check %s", ErrContract, what) }
	if r.SchemaVersion != renderCheckSchema || r.Scope != "presentation" || r.MathVerification == nil || *r.MathVerification {
		return bad("schema/scope")
	}
	if !renderCheckStates[r.State] || r.Checker == "" || len(r.Checker) > 40 || len(r.Renderer) > 40 || len(r.Environment) > 60 {
		return bad("state")
	}
	if r.TeacherHTMLSHA != teacherSHA || r.StudentHTMLSHA != studentSHA {
		return bad("does not cover the stored previews")
	}
	if len(r.Checks) == 0 || len(r.Checks) > 32 {
		return bad("checks")
	}
	fails, warns := map[string]bool{}, map[string]bool{}
	seen := map[string]bool{}
	for _, c := range r.Checks {
		if !renderCheckIDRe.MatchString(c.ID) || !renderCheckStatus[c.Status] || seen[c.ID] || c.Count < 0 || len(c.Codes) > 8 {
			return bad("check " + c.ID)
		}
		for _, code := range c.Codes {
			if !renderCheckCodeRe.MatchString(code) {
				return bad("code")
			}
		}
		seen[c.ID] = true
		fails[c.ID] = c.Status == "fail"
		warns[c.ID] = c.Status == "warn"
	}
	match := func(list []string, want map[string]bool) bool {
		n := 0
		for _, id := range list {
			if !want[id] {
				return false
			}
			n++
		}
		for _, v := range want {
			if v {
				n--
			}
		}
		return n == 0
	}
	if !match(r.Blocking, fails) || !match(r.Warnings, warns) {
		return bad("blocking/warnings disagree with checks")
	}
	want := "passed"
	if len(r.Blocking) > 0 {
		want = "failed"
	} else if len(r.Warnings) > 0 {
		want = "warnings"
	}
	if r.State != want {
		return bad("state disagrees with checks")
	}
	return nil
}

// renderCheckOf fetches and validates the job's render check report. It returns the
// stored summary JSON, the state, and blocking codes "render_check:<id>".
func (s *Service) renderCheckOf(ctx context.Context, job model.EduMathJob, arts map[string]model.EduArtifact) (string, string, []string, error) {
	ra := arts["render_check"]
	if ra.ID == "" {
		return "", "", nil, fmt.Errorf("%w: missing render check artifact", ErrContract)
	}
	raw, _, err := s.Client.Artifact(ctx, job.TenantID, ra)
	if err != nil {
		return "", "", nil, err
	}
	var rep renderCheckReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		return "", "", nil, fmt.Errorf("%w: render check is not JSON", ErrContract)
	}
	if err := rep.validate(arts["preview_html"].SHA256, arts["preview_html_student"].SHA256); err != nil {
		return "", "", nil, err
	}
	sum := RenderCheckSummary{State: rep.State, Scope: rep.Scope, MathVerification: false, Checker: rep.Checker, Renderer: rep.Renderer,
		Environment: rep.Environment, Blocking: nonNil(rep.Blocking), Warnings: nonNil(rep.Warnings), Checks: rep.Checks}
	b, _ := json.Marshal(sum)
	var blocking []string
	for _, id := range rep.Blocking {
		blocking = append(blocking, "render_check:"+id)
	}
	return string(b), rep.State, blocking, nil
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

// renderCheckView parses the stored summary for the teacher API (nil before v86).
func renderCheckView(l model.EduLessonRevision) *RenderCheckSummary {
	if l.RenderCheck == "" {
		return nil
	}
	var v RenderCheckSummary
	if json.Unmarshal([]byte(l.RenderCheck), &v) != nil {
		return nil
	}
	return &v
}

// mergeBlocking appends codes to a JSON list of blocking errors.
func mergeBlocking(existing string, codes []string) string {
	var list []string
	if existing != "" {
		_ = json.Unmarshal([]byte(existing), &list)
	}
	list = append(list, codes...)
	if len(list) == 0 {
		return ""
	}
	b, _ := json.Marshal(list)
	return string(b)
}
