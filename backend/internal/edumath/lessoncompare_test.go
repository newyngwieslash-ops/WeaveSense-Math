package edumath

import (
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

// TestLessonCompare covers DEV-073 on the real math service: the figure text is an editable class
// that still needs review, and the teacher's version comparison lists exactly the changed fields of
// two revisions of the same problem, under the same staff and tenant checks as the editor.
func TestLessonCompare(t *testing.T) {
	h := newHarness(t, startMathService(t))
	h.setupTeacher()
	pubID, _ := publishG1(t, h)
	var pub model.EduPublication
	h.db.First(&pub, "id = ?", pubID)
	l0 := pub.LessonRevisionID
	lr := "/api/edu/lesson-revisions/"

	c, out, raw, _ := h.call("u_alice", "GET", lr+l0+"/editable", nil)
	if c != 200 {
		t.Fatalf("editable: %d %s", c, raw)
	}
	ed := data(out)
	b0 := ed["beats"].([]any)[0].(map[string]any)["id"].(string)
	h0 := ed["lessonHash"].(string)

	// Figure text is its own class, needs review and is not inherited.
	fig := []map[string]any{{"target": "figure", "field": "prediction_prompt", "value": "先猜一猜"}}
	c, out, raw, _ = h.call("u_alice", "POST", lr+l0+"/edit-impact", map[string]any{"edits": fig})
	imp, _ := data(out)["impact"].(map[string]any)
	if c != 200 || string(mustJSON(imp["classes"])) != `["figure_text"]` || !strings.Contains(string(mustJSON(imp["invalidates"])), `"review"`) ||
		!strings.Contains(string(mustJSON(imp["keeps"])), `"figure_geometry"`) {
		t.Fatalf("figure impact: %d %s", c, raw)
	}
	// Not inherited even under the widest institution rule.
	if c, _, raw, _ := h.call("u_alice", "PUT", "/api/edu/policy", map[string]any{"reviewMode": "self_allowed", "consentMode": "guardian_required", "editReviewInherit": "presentation_only"}); c != 200 {
		t.Fatalf("set presentation_only: %d %s", c, raw)
	}
	if c, out, raw, _ := h.call("u_alice", "POST", lr+l0+"/edit-impact", map[string]any{"edits": fig}); c != 200 || data(out)["approvalInherited"] != false {
		t.Fatalf("figure text must not inherit review: %d %s", c, raw)
	}
	if c, out, _, _ := h.call("u_alice", "POST", lr+l0+"/edit-impact", map[string]any{"edits": []map[string]any{{"target": "figure", "field": "points", "value": "{}"}}}); c != 403 || errCode(out) != "POLICY_REVOKED" {
		t.Fatalf("figure geometry is not editable: %d %v", c, out)
	}

	c, out, raw, _ = h.call("u_alice", "POST", lr+l0+"/edits", map[string]any{"baseLessonHash": h0, "idempotencyKey": "cmp-1",
		"edits": []map[string]any{{"target": "beat", "id": b0, "field": "say", "value": "先看已知条件。"}}})
	if c != 202 {
		t.Fatalf("save: %d %s", c, raw)
	}
	done := h.waitJob("u_alice", data(out)["job"].(map[string]any)["id"].(string))
	c1, _ := done["lessonRevisionId"].(string)
	if done["status"] != "succeeded" || c1 == "" {
		t.Fatalf("edit job: %v", done)
	}

	c, out, raw, _ = h.call("u_alice", "GET", lr+l0+"/compare?with="+c1, nil)
	if c != 200 {
		t.Fatalf("compare: %d %s", c, raw)
	}
	d := data(out)
	ch := d["changes"].([]any)
	if d["same"] != false || len(ch) != 1 {
		t.Fatalf("exactly one change expected: %s", raw)
	}
	one := ch[0].(map[string]any)
	if one["section"] != "beat" || one["key"] != b0 || one["field"] != "say" || one["after"] != "先看已知条件。" || one["before"] == "先看已知条件。" {
		t.Fatalf("change: %v", one)
	}
	if d["from"].(map[string]any)["lessonRevisionId"] != l0 || d["to"].(map[string]any)["lessonRevisionId"] != c1 {
		t.Fatalf("sides: %s", raw)
	}
	if c, _, _, _ := h.call("u_alice", "GET", lr+l0+"/compare?with="+l0, nil); c != 400 {
		t.Fatalf("self compare: %d", c)
	}
	if c, _, _, _ := h.call("u_alice", "GET", lr+l0+"/compare", nil); c != 400 {
		t.Fatalf("missing with: %d", c)
	}
	if c, _, _, _ := h.call("u_alice", "GET", lr+l0+"/compare?with=nope", nil); c != 404 {
		t.Fatalf("unknown revision: %d", c)
	}
	if c, _, _, _ := h.call("u_bob", "GET", lr+l0+"/compare?with="+c1, nil); c != 403 && c != 404 {
		t.Fatalf("other users must not compare, got %d", c)
	}
}

// TestCompareLessonsFields checks the field diff itself: beats by id (changed, added, removed),
// hints by level, pronunciations, figure text, teaching parameters; formatting is not a change.
func TestCompareLessonsFields(t *testing.T) {
	a := `{"problem":{"text":"P"},"answer":{"plain":"1"},"beats":[{"id":"b1","title":"T","say":"S","board":["x"]},{"id":"b2","title":"U","say":"V","board":[]}],
	"hints":[{"level":1,"text":"h1"}],"pronunciations":{"棱":"léng"},"figure":{"explore":{"description":"d","prediction":{"id":"p","prompt":"q","options":["a","b"]}}},
	"meta":{"style":"standard"}}`
	b := `{"problem":{"text":"P"},"answer":{"plain":"1"},"beats":[{"id":"b1","title":"T","say":"S2","board":["x"]},{"id":"b3","title":"N","say":"M","board":["y"]}],
	"hints":[{"level":1,"text":"h1"},{"level":2,"text":"h2"}],"pronunciations":{},"figure":{"explore":{"description":"d","prediction":{"id":"p","prompt":"q","options":["a","c"]}}},
	"meta":{"style":"concise","target_minutes":3}}`
	got, err := compareLessons([]byte(a), []byte(b))
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, c := range got {
		keys = append(keys, c.Section+"/"+c.Key+"/"+c.Field)
	}
	want := "beat/b1/say beat/b3/title beat/b3/say beat/b3/board beat/b2/title beat/b2/say beat/b2/board hint/2/text pronunciation/棱/reading figure/prediction/options teaching//style teaching//targetMinutes"
	if strings.Join(keys, " ") != want {
		t.Fatalf("changes:\n got %s\nwant %s", strings.Join(keys, " "), want)
	}
	for _, c := range got {
		if c.Section == "beat" && c.Key == "b3" && c.Before != nil || c.Key == "b2" && c.After != nil || c.Section == "hint" && c.Before != nil {
			t.Fatalf("added/removed sides: %+v", c)
		}
	}
	if same, _ := compareLessons([]byte(a), []byte(a)); len(same) != 0 {
		t.Fatalf("identical lessons: %+v", same)
	}
}
