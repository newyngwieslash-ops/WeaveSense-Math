package edumath

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Review checklist and located rejection (DEV-076). The checklist is optional; when sent it
// must cover every item, and an approval cannot carry an "issue" verdict or findings. A
// rejection must say why: a finding, an "issue" item, or a comment. Findings name a target
// in the lesson (a beat id, a hint level, or a whole-lesson part) so the author can jump to it.

var reviewItems = []string{"problem", "conditions", "method", "visuals", "hints", "self_test", "answer_policy"}

var findingTarget = regexp.MustCompile(`^(lesson|problem|conditions|method|answer|practice|layout|beat:[A-Za-z0-9_\-]{1,40}|hint:[1-9])$`)

type ReviewFinding struct {
	Target string `json:"target"`
	Item   string `json:"item"`
	Note   string `json:"note"`
}

type ReviewInput struct {
	Checklist map[string]string `json:"checklist"`
	Findings  []ReviewFinding   `json:"findings"`
}

func isReviewItem(k string) bool {
	for _, i := range reviewItems {
		if i == k {
			return true
		}
	}
	return false
}

// check validates the input against the decision and returns the stored JSON forms.
func (rv ReviewInput) check(decision, comment string) (string, string, error) {
	bad := func(msg string) (string, string, error) {
		return "", "", appErr(400, "INPUT_AMBIGUOUS", msg, "fix_review")
	}
	issue := false
	checklist := ""
	if rv.Checklist != nil {
		for k, v := range rv.Checklist {
			if !isReviewItem(k) {
				return bad("unknown checklist item " + k)
			}
			if v != "pass" && v != "issue" && v != "na" {
				return bad("checklist values are pass, issue or na")
			}
			issue = issue || v == "issue"
		}
		for _, k := range reviewItems {
			if _, ok := rv.Checklist[k]; !ok {
				return bad("checklist must cover " + strings.Join(reviewItems, ", "))
			}
		}
		b, _ := json.Marshal(rv.Checklist)
		checklist = string(b)
	}
	if len(rv.Findings) > 30 {
		return bad("at most 30 findings")
	}
	for i, f := range rv.Findings {
		f.Note = strings.TrimSpace(f.Note)
		if !findingTarget.MatchString(f.Target) || !isReviewItem(f.Item) {
			return bad(fmt.Sprintf("finding %d: unknown target or item", i+1))
		}
		if f.Note == "" || utf8.RuneCountInString(f.Note) > 300 {
			return bad(fmt.Sprintf("finding %d: note 1..300 characters", i+1))
		}
		rv.Findings[i] = f
	}
	findings := ""
	if len(rv.Findings) > 0 {
		b, _ := json.Marshal(rv.Findings)
		findings = string(b)
	}
	switch decision {
	case "approved":
		if issue || len(rv.Findings) > 0 {
			return "", "", appErr(409, "CHECKLIST_OPEN", "an approval cannot carry open checklist issues or findings", "reject_or_fix")
		}
	case "rejected":
		if !issue && len(rv.Findings) == 0 && strings.TrimSpace(comment) == "" {
			return bad("a rejection needs a finding, an issue item or a comment")
		}
	}
	return checklist, findings, nil
}

// checkFindingTargets confirms that beat and hint targets exist in this lesson revision's
// document, so a rejection never points the author at a step that is not there.
func (h *API) checkFindingTargets(c *gin.Context, tx *gorm.DB, a Actor, lessonID string, fs []ReviewFinding) error {
	need := false
	for _, f := range fs {
		if strings.HasPrefix(f.Target, "beat:") || strings.HasPrefix(f.Target, "hint:") {
			need = true
		}
	}
	if !need {
		return nil
	}
	ctx, cancel := ctx15(c)
	defer cancel()
	_, _, _, _, art, err := h.Svc.editBase(ctx, tx, a, lessonID)
	if err != nil {
		return err
	}
	raw, _, err := h.Svc.Client.Artifact(ctx, a.TenantID, art)
	if err != nil {
		return fmt.Errorf("%w: lesson artifact", ErrContract)
	}
	var doc struct {
		Beats []struct {
			ID string `json:"id"`
		} `json:"beats"`
		Hints []struct {
			Level int `json:"level"`
		} `json:"hints"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return fmt.Errorf("%w: lesson document", ErrContract)
	}
	have := map[string]bool{}
	for _, b := range doc.Beats {
		have["beat:"+b.ID] = true
	}
	for _, x := range doc.Hints {
		have["hint:"+strconv.Itoa(x.Level)] = true
	}
	for i, f := range fs {
		if (strings.HasPrefix(f.Target, "beat:") || strings.HasPrefix(f.Target, "hint:")) && !have[f.Target] {
			return appErr(400, "INPUT_AMBIGUOUS", fmt.Sprintf("finding %d: %s is not in this lesson revision", i+1, f.Target), "reload_revision")
		}
	}
	return nil
}
