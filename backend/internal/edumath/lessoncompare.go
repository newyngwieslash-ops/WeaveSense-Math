package edumath

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
)

// lessonCompare is the teacher's version comparison (DEV-073): two lesson
// revisions of the same problem, compared field by field. Both revisions go
// through the same staff, tenant and problem checks as the editor; the reply is
// teacher scope only and is never shown to students.
//
//	GET /lesson-revisions/:id/compare?with=<lessonRevisionId>
func (h *API) lessonCompare(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	other := c.Query("with")
	if other == "" || other == c.Param("id") {
		h.fail(c, appErr(400, "INPUT_AMBIGUOUS", "pick a different revision to compare with", "pick_revision"))
		return
	}
	ctx, cancel := ctx15(c)
	defer cancel()
	l1, r1, _, _, art1, err := h.Svc.editBase(ctx, tx, a, c.Param("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	l2, r2, _, _, art2, err := h.Svc.editBase(ctx, tx, a, other)
	if err != nil {
		h.fail(c, err)
		return
	}
	if r1.ProblemID != r2.ProblemID {
		h.fail(c, appErr(409, "REVISION_CONFLICT", "these revisions belong to different problems", "pick_revision"))
		return
	}
	raw1, _, err1 := h.Svc.Client.Artifact(ctx, a.TenantID, art1)
	raw2, _, err2 := h.Svc.Client.Artifact(ctx, a.TenantID, art2)
	if err1 != nil || err2 != nil {
		h.fail(c, fmt.Errorf("%w: lesson artifact", ErrContract))
		return
	}
	changes, err := compareLessons(raw1, raw2)
	if err != nil {
		h.fail(c, fmt.Errorf("%w: lesson document", ErrContract))
		return
	}
	side := func(id, hash, state string, rev int) gin.H {
		return gin.H{"lessonRevisionId": id, "lessonHash": hash, "state": state, "problemRevision": rev}
	}
	reply(c, 200, gin.H{
		"from":    side(l1.ID, l1.LessonHash, l1.State, r1.Revision),
		"to":      side(l2.ID, l2.LessonHash, l2.State, r2.Revision),
		"same":    l1.LessonHash == l2.LessonHash,
		"changes": changes,
	}, "ok")
}

// LessonChange is one field that differs; Before or After is null when the
// field exists on one side only (a beat added or removed).
type LessonChange struct {
	Section string `json:"section"` // problem, answer, beat, hint, figure, pronunciation, teaching, layout
	Key     string `json:"key"`     // beat id, hint level, term, or field name
	Field   string `json:"field"`
	Before  any    `json:"before"`
	After   any    `json:"after"`
}

type lessonDoc struct {
	Problem map[string]any `json:"problem"`
	Answer  map[string]any `json:"answer"`
	Beats   []struct {
		ID    string   `json:"id"`
		Title string   `json:"title"`
		Say   string   `json:"say"`
		Board []string `json:"board"`
	} `json:"beats"`
	Hints []struct {
		Level int    `json:"level"`
		Text  string `json:"text"`
	} `json:"hints"`
	Pronunciations map[string]string `json:"pronunciations"`
	Figure         struct {
		Explore map[string]any `json:"explore"`
	} `json:"figure"`
	Meta struct {
		Goal          string `json:"goal"`
		Style         string `json:"style"`
		TargetMinutes int    `json:"target_minutes"`
		Layout        any    `json:"layout"`
	} `json:"meta"`
}

// compareLessons returns the changes in a stable order. Values are compared by
// their JSON form, so formatting does not count as a change.
func compareLessons(rawA, rawB []byte) ([]LessonChange, error) {
	var a, b lessonDoc
	if err := json.Unmarshal(rawA, &a); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rawB, &b); err != nil {
		return nil, err
	}
	out := []LessonChange{}
	add := func(sec, key, field string, x, y any, hx, hy bool) {
		var bx, by any
		if hx {
			bx = x
		}
		if hy {
			by = y
		}
		if hx == hy && sameJSON(x, y) {
			return
		}
		out = append(out, LessonChange{sec, key, field, bx, by})
	}
	for _, f := range []string{"text", "plain", "latex"} {
		add("problem", "", f, a.Problem[f], b.Problem[f], a.Problem[f] != nil, b.Problem[f] != nil)
		add("answer", "", f, a.Answer[f], b.Answer[f], a.Answer[f] != nil, b.Answer[f] != nil)
	}
	// Beats by id, in the newer order, then removed ones.
	ia := map[string]int{}
	for i, x := range a.Beats {
		ia[x.ID] = i
	}
	seen := map[string]bool{}
	for _, y := range b.Beats {
		seen[y.ID] = true
		i, ok := ia[y.ID]
		if !ok {
			add("beat", y.ID, "title", nil, y.Title, false, true)
			add("beat", y.ID, "say", nil, y.Say, false, true)
			add("beat", y.ID, "board", nil, y.Board, false, true)
			continue
		}
		x := a.Beats[i]
		add("beat", y.ID, "title", x.Title, y.Title, true, true)
		add("beat", y.ID, "say", x.Say, y.Say, true, true)
		add("beat", y.ID, "board", x.Board, y.Board, true, true)
	}
	for _, x := range a.Beats {
		if !seen[x.ID] {
			add("beat", x.ID, "title", x.Title, nil, true, false)
			add("beat", x.ID, "say", x.Say, nil, true, false)
			add("beat", x.ID, "board", x.Board, nil, true, false)
		}
	}
	ha, hb := map[int]string{}, map[int]string{}
	levels := map[int]bool{}
	for _, x := range a.Hints {
		ha[x.Level] = x.Text
		levels[x.Level] = true
	}
	for _, y := range b.Hints {
		hb[y.Level] = y.Text
		levels[y.Level] = true
	}
	for _, lv := range sortedInts(levels) {
		x, hx := ha[lv]
		y, hy := hb[lv]
		add("hint", strconv.Itoa(lv), "text", x, y, hx, hy)
	}
	terms := map[string]bool{}
	for k := range a.Pronunciations {
		terms[k] = true
	}
	for k := range b.Pronunciations {
		terms[k] = true
	}
	for _, k := range sortedKeys(terms) {
		x, hx := a.Pronunciations[k]
		y, hy := b.Pronunciations[k]
		add("pronunciation", k, "reading", x, y, hx, hy)
	}
	ea, eb := a.Figure.Explore, b.Figure.Explore
	add("figure", "", "description", ea["description"], eb["description"], ea["description"] != nil, eb["description"] != nil)
	pa, _ := ea["prediction"].(map[string]any)
	pb, _ := eb["prediction"].(map[string]any)
	for _, f := range []string{"prompt", "options"} {
		add("figure", "prediction", f, pa[f], pb[f], pa[f] != nil, pb[f] != nil)
	}
	add("teaching", "", "goal", a.Meta.Goal, b.Meta.Goal, true, true)
	add("teaching", "", "style", a.Meta.Style, b.Meta.Style, true, true)
	add("teaching", "", "targetMinutes", a.Meta.TargetMinutes, b.Meta.TargetMinutes, true, true)
	add("layout", "", "layout", a.Meta.Layout, b.Meta.Layout, a.Meta.Layout != nil, b.Meta.Layout != nil)
	return out, nil
}

func sameJSON(x, y any) bool {
	bx, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return string(bx) == string(by)
}

func sortedInts(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
