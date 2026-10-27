package edumath

import (
	"encoding/json"
	"html"
	"strings"
)

// Content provenance (DEV-101). The math service writes how a lesson was made
// into its metadata (ai_generated, generator, edit_log); every format the
// platform hands out carries the same object, visibly and as machine-readable
// metadata. It is derived from the lesson document, never stored as an
// editable field, so review and teacher edits cannot remove it. Audio and
// video formats do not exist yet (TTS, DEV-022) and have no label here.

// Provenance mirrors mathsvc.lesson.provenance.
type Provenance struct {
	AIGenerated   bool   `json:"ai_generated"`
	Generator     string `json:"generator"`
	TeacherEdited bool   `json:"teacher_edited"`
	Label         string `json:"label"`
}

var provenanceLabel = map[string]string{
	"deterministic-template": "由程序按题族模板自动生成，未使用生成式人工智能",
	"teacher-manual":         "老师人工编写，未使用生成式人工智能",
}

// provenanceOf reads the teacher lesson's metadata; a document that cannot be
// read is labelled as unrecorded rather than guessed.
func provenanceOf(teacherLesson []byte) Provenance {
	var doc struct {
		Meta struct {
			AIGenerated bool            `json:"ai_generated"`
			Generator   string          `json:"generator"`
			EditLog     json.RawMessage `json:"edit_log"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(teacherLesson, &doc)
	p := Provenance{AIGenerated: doc.Meta.AIGenerated, Generator: doc.Meta.Generator}
	if p.Generator == "" {
		p.Generator = "unknown"
	}
	log := strings.TrimSpace(string(doc.Meta.EditLog))
	p.TeacherEdited = log != "" && log != "null" && log != "[]"
	switch {
	case p.AIGenerated:
		p.Label = "含人工智能生成内容"
	case provenanceLabel[p.Generator] != "":
		p.Label = provenanceLabel[p.Generator]
	default:
		p.Label = "生成方式未记录"
	}
	if p.TeacherEdited {
		p.Label += "；含老师修改"
	}
	return p
}

// provenanceMeta is the implicit label for HTML formats.
func provenanceMeta(p Provenance) string {
	b, _ := json.Marshal(p)
	return `<meta name="content-provenance" content="` + html.EscapeString(string(b)) + `">`
}
