package edumath

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// DEV-031 text intake: a typed statement becomes a structured draft that the
// teacher must still confirm. The math service matches the whole statement
// against one family's fixed grammar (never by resemblance) and runs the
// family's own admission checks; refusals carry a reason and a next action
// (structured form or a teacher written lesson). Nothing is stored and no math
// is solved, so nothing is charged; the draft only fills the authoring form.

const (
	maxIntakeRunes = 600
	maxIntakeBody  = 8 << 10
	maxDraftBytes  = 64 << 10
)

// Draft is the service's proposal. State is always needs_confirmation.
type Draft struct {
	State       string         `json:"state"`
	FamilyID    string         `json:"family_id"`
	Structured  map[string]any `json:"structured"`
	Summary     DraftSummary   `json:"summary"`
	ProblemText string         `json:"problem_text"`
}

type DraftSummary struct {
	Object       string   `json:"object"`
	Conditions   []string `json:"conditions"`
	Target       string   `json:"target"`
	Unit         string   `json:"unit"`
	Method       string   `json:"method"`
	MethodStated bool     `json:"method_stated"`
}

func (d Draft) validate() error {
	fid, _ := d.Structured["family_id"].(string)
	if d.State != "needs_confirmation" || d.FamilyID == "" || fid != d.FamilyID || d.Summary.Object == "" || d.Summary.Target == "" ||
		len(d.Summary.Conditions) == 0 || d.Summary.Method == "" || d.ProblemText == "" {
		return fmt.Errorf("%w: draft", ErrContract)
	}
	return nil
}

// Normalize asks the math service for a draft (synchronous, isolated subprocess on the service).
func (c Client) Normalize(ctx context.Context, tenant, actor, text string) (Draft, error) {
	var d Draft
	st, data, _, err := c.doLimit(ctx, http.MethodPost, "/v1/normalize", tenant, actor, map[string]any{"text": text}, maxDraftBytes)
	if err != nil {
		return d, err
	}
	if st != http.StatusOK {
		var e struct {
			Error *ServiceError `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != nil && e.Error.Code != "" {
			return d, e.Error
		}
		return d, fmt.Errorf("%w: normalize http %d", ErrContract, st)
	}
	if err := strictJSON(data, &d); err != nil {
		return d, fmt.Errorf("%w: %v", ErrContract, err)
	}
	return d, d.validate()
}

// intakeRefusal turns a service refusal into a teacher facing error with its reason.
// intakeRefusal shapes a service refusal for the teacher. reason is the service's fixed Chinese
// explanation (details.reason) and is the only service text passed through; it is capped in length.
func intakeRefusal(se *ServiceError) gin.H {
	next := se.NextAction
	if next == "" {
		next = "teacher_confirm_input"
	}
	e := gin.H{"code": se.Code, "message": se.Message, "next_action": next}
	if r, ok := se.Details["reason"].(string); ok && r != "" && utf8.RuneCountInString(r) <= 200 {
		e["reason"] = r
		e["message"] = r
	}
	return gin.H{"error": e}
}

func (h *API) normalizeDraft(c *gin.Context) {
	a, _, ok := h.staff(c)
	if !ok {
		return
	}
	if !a.Has("teacher") {
		h.fail(c, ErrNoRole)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxIntakeBody+1))
	if err != nil || len(raw) > maxIntakeBody {
		h.fail(c, appErr(400, "PARAMETER_LIMIT", "body too large", "teacher_fix"))
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.More() {
		h.fail(c, appErr(400, "INPUT_AMBIGUOUS", "body must be {text}", "teacher_fix"))
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" || !utf8.ValidString(in.Text) || utf8.RuneCountInString(in.Text) > maxIntakeRunes {
		h.fail(c, appErr(400, "PARAMETER_LIMIT", fmt.Sprintf("text must be 1 to %d characters", maxIntakeRunes), "teacher_fix"))
		return
	}
	ctx, cancel := ctx15(c)
	defer cancel()
	d, err := h.Svc.Client.Normalize(ctx, a.TenantID, a.UserID, in.Text)
	var se *ServiceError
	switch {
	case errors.As(err, &se):
		reply(c, http.StatusUnprocessableEntity, intakeRefusal(se), se.Code)
		return
	case errors.Is(err, ErrContract):
		h.fail(c, err)
		return
	case err != nil:
		h.fail(c, appErr(http.StatusServiceUnavailable, "PROVIDER_UNAVAILABLE", "math service unavailable", "retry_later"))
		return
	}
	reply(c, 200, d, "ok")
}
