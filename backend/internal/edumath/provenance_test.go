package edumath

import (
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

const templateLabel = "由程序按题族模板自动生成，未使用生成式人工智能"

// TestContentProvenance covers DEV-101 on the real math service: the student JSON, the student and
// teacher preview pages, both export files and the teacher editor carry the same provenance,
// visibly and as metadata; review keeps it and a teacher edit adds "含老师修改" without removing
// the generator.
func TestContentProvenance(t *testing.T) {
	h := newHarness(t, startMathService(t))
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	var pub model.EduPublication
	h.db.First(&pub, "id = ?", pubID)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二(8)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "来源", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-prov"})
	stu := []string{"X-Edu-Tenant", tenant}

	c, out, raw, _ := h.call("u_bob", "GET", "/api/edu/student/assignments/"+asID, nil, stu...)
	if c != 200 {
		t.Fatalf("student view: %d %s", c, raw)
	}
	d := data(out)
	p := d["provenance"].(map[string]any)
	if p["label"] != templateLabel || p["generator"] != "deterministic-template" || p["ai_generated"] != false || p["teacher_edited"] != false {
		t.Fatalf("student provenance: %v", p)
	}
	if lp, _ := d["lesson"].(map[string]any)["meta"].(map[string]any)["provenance"].(map[string]any); lp == nil || lp["label"] != templateLabel {
		t.Fatalf("student projection must carry the same provenance: %s", raw)
	}
	code, body, _ := fetch(h, "", d["preview"].(map[string]any)["url"].(string))
	if code != 200 || !strings.Contains(body, "data-provenance>"+templateLabel+"<") || !strings.Contains(body, `<meta name="content-provenance" content="`) {
		t.Fatalf("student preview page: %d %.300s", code, body)
	}
	for _, aud := range []string{"student", "teacher"} {
		c, out, raw, _ := h.call("u_alice", "GET", "/api/edu/publications/"+pubID+"/export?audience="+aud, nil)
		if c != 200 {
			t.Fatalf("%s export: %d %s", aud, c, raw)
		}
		doc := data(out)["html"].(string)
		man := data(out)["manifest"].(map[string]any)
		if !strings.Contains(doc, "data-provenance>"+templateLabel+"<") || !strings.Contains(doc, `<meta name="content-provenance" content="{&#34;ai_generated&#34;:false`) ||
			man["provenance"].(map[string]any)["label"] != templateLabel {
			t.Fatalf("%s export provenance: %.400s %v", aud, doc, man["provenance"])
		}
	}

	// A teacher edit keeps the generator and adds the edit.
	lr := "/api/edu/lesson-revisions/"
	_, out, _, _ = h.call("u_alice", "GET", lr+pub.LessonRevisionID+"/editable", nil)
	ed := data(out)
	if ed["provenance"].(map[string]any)["label"] != templateLabel {
		t.Fatalf("editor provenance: %v", ed["provenance"])
	}
	b0 := ed["beats"].([]any)[0].(map[string]any)["id"].(string)
	c, out, raw, _ = h.call("u_alice", "POST", lr+pub.LessonRevisionID+"/edits", map[string]any{"baseLessonHash": ed["lessonHash"], "idempotencyKey": "prov-1",
		"edits": []map[string]any{{"target": "beat", "id": b0, "field": "title", "value": "换个说法"}}})
	if c != 202 {
		t.Fatalf("edit: %d %s", c, raw)
	}
	done := h.waitJob("u_alice", data(out)["job"].(map[string]any)["id"].(string))
	_, out, _, _ = h.call("u_alice", "GET", lr+done["lessonRevisionId"].(string)+"/editable", nil)
	if p := data(out)["provenance"].(map[string]any); p["label"] != templateLabel+"；含老师修改" || p["generator"] != "deterministic-template" || p["teacher_edited"] != true {
		t.Fatalf("edited provenance: %v", p)
	}
}

func TestProvenanceOf(t *testing.T) {
	for _, tc := range []struct{ doc, label string }{
		{`{"meta":{"ai_generated":false,"generator":"deterministic-template"}}`, templateLabel},
		{`{"meta":{"generator":"teacher-manual","edit_log":[{"class":"wording"}]}}`, "老师人工编写，未使用生成式人工智能；含老师修改"},
		{`{"meta":{"ai_generated":true,"generator":"model-x","edit_log":[]}}`, "含人工智能生成内容"},
		{`{"meta":{}}`, "生成方式未记录"},
		{`not json`, "生成方式未记录"},
	} {
		if got := provenanceOf([]byte(tc.doc)).Label; got != tc.label {
			t.Errorf("%s: %q, want %q", tc.doc, got, tc.label)
		}
	}
}
