package edumath

import (
	"encoding/json"
	"html"
	"strings"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

// Content provenance (DEV-101). The math service writes how a lesson was made
// into its metadata (ai_generated, generator, edit_log); every format the
// platform hands out carries the same object, visibly and as machine-readable
// metadata. It is derived from the lesson document, never stored as an
// editable field, so review and teacher edits cannot remove it. Audio and
// video formats do not exist yet (TTS, DEV-022) and have no label here.

// Provenance mirrors mathsvc.lesson.provenance and adds who started the
// generation, from the platform's own job record.
type Provenance struct {
	Kind          string   `json:"kind"` // template_only | teacher_manual | model_assisted | unknown
	AIGenerated   *bool    `json:"ai_generated"`
	Generator     string   `json:"generator"`
	AIParts       []string `json:"ai_parts"`
	TeacherEdited bool     `json:"teacher_edited"`
	InitiatedBy   string   `json:"initiated_by,omitempty"` // teacher | agent (the build job's idempotency key)
	Label         string   `json:"label"`
}

var provenanceLabel = map[string]string{
	"deterministic-template": "讲解由程序按题族模板生成，平台未调用生成式人工智能撰写内容",
	"teacher-manual":         "讲解由老师人工编写，平台未调用生成式人工智能撰写内容",
}
var provenanceKind = map[string]string{"deterministic-template": "template_only", "teacher-manual": "teacher_manual"}
var aiPartLabel = map[string]string{"lesson": "讲解", "hints": "提示", "narration": "旁白", "audio": "配音", "intake": "题目识别", "figure": "图形"}

// provenanceOf reads the lesson's own record. "No generative AI" needs an
// explicit ai_generated=false for a known generator; a missing flag or an
// unreadable document is reported as unrecorded, and any recorded model part
// makes the label say so.
func provenanceOf(teacherLesson []byte) Provenance {
	var doc struct {
		Meta struct {
			AIGenerated *bool           `json:"ai_generated"`
			Generator   string          `json:"generator"`
			AIParts     []string        `json:"ai_parts"`
			EditLog     json.RawMessage `json:"edit_log"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(teacherLesson, &doc)
	m := doc.Meta
	p := Provenance{Generator: m.Generator, AIParts: m.AIParts}
	if p.Generator == "" {
		p.Generator = "unknown"
	}
	if p.AIParts == nil {
		p.AIParts = []string{}
	}
	log := strings.TrimSpace(string(m.EditLog))
	p.TeacherEdited = log != "" && log != "null" && log != "[]"
	yes, no := true, false
	switch {
	case m.AIGenerated != nil && *m.AIGenerated || len(p.AIParts) > 0:
		p.Kind, p.AIGenerated, p.Label = "model_assisted", &yes, "含人工智能生成内容"
		if len(p.AIParts) > 0 {
			names := make([]string, len(p.AIParts))
			for i, x := range p.AIParts {
				names[i] = x
				if l := aiPartLabel[x]; l != "" {
					names[i] = l
				}
			}
			p.Label += "（" + strings.Join(names, "、") + "）"
		}
	case m.AIGenerated != nil && provenanceLabel[p.Generator] != "":
		p.Kind, p.AIGenerated, p.Label = provenanceKind[p.Generator], &no, provenanceLabel[p.Generator]
	default:
		p.Kind, p.Label = "unknown", "生成方式未记录"
	}
	if p.TeacherEdited {
		p.Label += "；含老师修改"
	}
	return p
}

// lessonProvenance adds who started the generation: the root revision's build
// job, recorded with an "agent:" idempotency key when the teacher's assistant
// called the math tool. The assistant only starts a build of a teacher
// confirmed problem; it writes none of the content.
func (s *Service) lessonProvenance(tx *gorm.DB, tenant string, l model.EduLessonRevision, teacherLesson []byte) Provenance {
	p := provenanceOf(teacherLesson)
	root := l
	for i := 0; i < 100 && root.ParentLessonRevisionID != ""; i++ {
		var parent model.EduLessonRevision
		if tx.First(&parent, "id = ? AND tenant_id = ?", root.ParentLessonRevisionID, tenant).Error != nil {
			return p
		}
		root = parent
	}
	var j model.EduMathJob
	if root.JobID == "" || tx.First(&j, "id = ? AND tenant_id = ?", root.JobID, tenant).Error != nil {
		return p
	}
	p.InitiatedBy = "teacher"
	if strings.HasPrefix(j.IdemKey, "agent:") {
		p.InitiatedBy = "agent"
		p.Label += "；由智能助手发起生成，内容未由其撰写"
	}
	return p
}

// provenanceMeta is the implicit label for HTML formats.
func provenanceMeta(p Provenance) string {
	b, _ := json.Marshal(p)
	return `<meta name="content-provenance" content="` + html.EscapeString(string(b)) + `">`
}
