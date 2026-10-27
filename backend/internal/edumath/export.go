package edumath

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/model"
)

// Content and learning report export (DEV-103). An export is a self-contained
// HTML document (no scripts, no external resources) plus the same data as
// JSON, returned in the normal envelope so the page saves it as a file.
//
//   - Content export is bound to one published, immutable publication and
//     carries its manifest: lesson revision and hash, review record, problem
//     and solution revisions with their hashes, math state and evidence.
//     The source rights record is checked at export time: a linked source must
//     be active, not expired and allow export; the student version also needs
//     student use. Teacher original content (no record) is exportable.
//   - audience defaults to student: the student version is built from the
//     server-side student projection (never the teacher lesson), passes the
//     same answer guard as student reads, and lists which fields were left out
//     and why. The teacher version (answer, full explanation, gated hints,
//     self-test key) must be asked for explicitly.
//   - The learning report is aggregate only: counts per outcome, self-test and
//     help counts, never a student id, name, response or help text. Counts of
//     one or two are suppressed in the file because they could single out a
//     student. Aggregates need no access purpose; they are still audited.
//
// Neither export contains student personal data, and every export is
// audited (export.content, export.report).

const exportSuppressBelow = 3

func (h *API) registerExport(g *gin.RouterGroup) {
	g.GET("/publications/:id/export", h.exportPublication)
	g.GET("/assignments/:id/report-export", h.exportReport)
}

type exportField struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

func (h *API) exportPublication(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	audience := c.DefaultQuery("audience", "student")
	if audience != "student" && audience != "teacher" {
		reply(c, 400, nil, "audience must be student or teacher")
		return
	}
	var pub model.EduPublication
	var l model.EduLessonRevision
	var job model.EduMathJob
	if tx.First(&pub, "id = ? AND tenant_id = ?", c.Param("id"), a.TenantID).Error != nil ||
		tx.First(&l, "id = ? AND tenant_id = ?", pub.LessonRevisionID, a.TenantID).Error != nil ||
		tx.First(&job, "id = ? AND tenant_id = ?", l.JobID, a.TenantID).Error != nil || job.WorkspaceID != a.WorkspaceID {
		h.fail(c, ErrNotFound)
		return
	}
	if pub.State != "published" {
		h.fail(c, appErr(http.StatusConflict, "REVISION_CONFLICT", "only a published publication can be exported", "reload"))
		return
	}
	if pub.LessonHash != l.LessonHash {
		h.fail(c, appErr(http.StatusConflict, "REVISION_CONFLICT", "publication hash does not match the lesson revision", "reload"))
		return
	}
	var pr model.EduProblemRevision
	var p model.EduProblem
	var sol model.EduSolutionRevision
	var rr model.EduReviewRecord
	if tx.First(&pr, "id = ? AND tenant_id = ?", l.ProblemRevisionID, a.TenantID).Error != nil ||
		tx.First(&p, "id = ? AND tenant_id = ?", pr.ProblemID, a.TenantID).Error != nil ||
		tx.First(&sol, "id = ? AND tenant_id = ?", l.SolutionRevisionID, a.TenantID).Error != nil ||
		tx.First(&rr, "id = ? AND tenant_id = ?", pub.ReviewRecordID, a.TenantID).Error != nil {
		h.fail(c, ErrNotFound)
		return
	}
	rights, err := exportRights(tx, a.TenantID, l.ProblemRevisionID, audience)
	if err != nil {
		h.fail(c, err)
		return
	}
	var ta model.EduArtifact
	if tx.First(&ta, "id = ? AND tenant_id = ?", l.LessonArtifact, a.TenantID).Error != nil {
		h.fail(c, ErrNotFound)
		return
	}
	ctx, cancel := ctx15(c)
	defer cancel()
	teacherRaw, _, err := h.Svc.Client.Artifact(ctx, a.TenantID, ta)
	if err != nil {
		h.fail(c, err)
		return
	}
	var content map[string]any
	var omitted []exportField
	if audience == "student" {
		var sa model.EduArtifact
		if tx.First(&sa, "id = ? AND tenant_id = ? AND audience = ?", l.StudentArtifact, a.TenantID, "student").Error != nil {
			h.fail(c, ErrNotFound)
			return
		}
		raw, _, err := h.Svc.Client.Artifact(ctx, a.TenantID, sa)
		if err != nil {
			h.fail(c, err)
			return
		}
		if json.Unmarshal(raw, &content) != nil || StudentLeak(content, teacherRaw) {
			h.fail(c, ErrContract)
			return
		}
		omitted = []exportField{
			{"answer", "未开放的答案不进入学生版"},
			{"beats", "讲解正文在老师开放答案前不进入学生版，只保留段落标题"},
			{"gated_hints", "会透露答案的提示不进入学生版"},
			{"check_answer_key", "自测答案不进入学生版"},
			{"teacher_source", "教师源稿不默认给学生；需要时单独导出教师版"},
		}
	} else if json.Unmarshal(teacherRaw, &content) != nil {
		h.fail(c, ErrContract)
		return
	}
	meta, _ := content["meta"].(map[string]any)
	verification, _ := meta["verification"].(string)
	manifest := gin.H{
		"publicationId": pub.ID, "publishedAt": pub.CreatedAt, "lessonRevisionId": l.ID, "lessonHash": l.LessonHash,
		"reviewRecordId": rr.ID, "reviewDecision": rr.Decision, "selfReview": rr.SelfReview, "reviewedAt": rr.CreatedAt,
		"problemRevisionId": pr.ID, "problemRevision": pr.Revision, "problemInputHash": pr.InputHash,
		"solutionRevisionId": sol.ID, "solutionHash": sol.SolutionHash, "mathState": sol.MathState, "evidenceLevels": sol.EvidenceLevels,
		"verification": verification, "audience": audience, "exportedAt": h.Svc.now().UTC(),
	}
	title := p.Title
	if title == "" {
		title = "数学讲解"
	}
	doc := renderContentHTML(title, audience, manifest, rights, content, omitted)
	h.Svc.audit(tx, a, "export.content", pub.ID, "audience="+audience)
	reply(c, 200, gin.H{"filename": fmt.Sprintf("%s-%s.html", pub.ID, audience), "manifest": manifest, "rights": rights,
		"omitted": omitted, "html": doc}, "ok")
}

// exportRights applies the rights record at export time. No record means
// teacher original content.
func exportRights(tx *gorm.DB, tenant, problemRevisionID, audience string) (gin.H, error) {
	src, linked := rightsFor(tx, tenant, problemRevisionID)
	if !linked {
		return gin.H{"kind": "teacher_original", "registered": false, "note": "未登记来源，按教师原创处理"}, nil
	}
	ok := rightsLive(src, time.Now().UTC()) && src.AllowExport && (audience != "student" || src.AllowStudent)
	if !ok {
		return nil, appErr(http.StatusConflict, "RIGHTS_RESTRICTED", "the source rights do not allow export (expired, withdrawn or export not granted)", "check_source_rights")
	}
	out := gin.H{"registered": true, "sourceId": src.ID, "title": src.Title, "kind": src.Kind, "attribution": src.Attribution, "licenseRef": src.LicenseRef}
	if src.ExpiresAt != nil {
		out["expiresAt"] = src.ExpiresAt.UTC()
	}
	return out, nil
}

var exportCSS = `body{font-family:system-ui,sans-serif;max-width:46rem;margin:2rem auto;padding:0 1rem;line-height:1.6;color:#1c1917}` +
	`table{border-collapse:collapse}td,th{border:1px solid #d6d3d1;padding:.2rem .5rem;text-align:left;font-size:.85rem}` +
	`.note{color:#57534e;font-size:.9rem}`

func esc(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return html.EscapeString(x)
	case time.Time:
		return html.EscapeString(x.UTC().Format(time.RFC3339))
	default:
		return html.EscapeString(fmt.Sprint(x))
	}
}

func htmlHead(title string) *strings.Builder {
	b := &strings.Builder{}
	b.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">`)
	b.WriteString(`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">`)
	b.WriteString("<title>" + esc(title) + "</title><style>" + exportCSS + "</style></head><body>")
	return b
}

func kvTable(b *strings.Builder, caption string, rows [][2]any) {
	b.WriteString("<table><caption>" + esc(caption) + "</caption><tbody>")
	for _, r := range rows {
		if s := esc(r[1]); s != "" {
			b.WriteString("<tr><th scope=\"row\">" + esc(r[0]) + "</th><td>" + s + "</td></tr>")
		}
	}
	b.WriteString("</tbody></table>")
}

func strList(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := []string{}
		for _, s := range x {
			if t, ok := s.(string); ok {
				out = append(out, t)
			}
		}
		return out
	}
	return nil
}

var verificationLabel = map[string]string{ManualUnverified: "老师人工编写，未经自动数学验证"}

func renderContentHTML(title, audience string, man gin.H, rights gin.H, content map[string]any, omitted []exportField) string {
	b := htmlHead(title)
	label := "学生版"
	if audience == "teacher" {
		label = "教师版（含答案与完整讲解，不要直接发给学生）"
	}
	b.WriteString("<h1>" + esc(title) + "</h1><p class=\"note\">" + esc(label) + "</p>")
	if v, _ := man["verification"].(string); v != "" {
		if t := verificationLabel[v]; t != "" {
			b.WriteString("<p><strong>" + esc(t) + "</strong></p>")
		}
	}
	if pb, ok := content["problem"].(map[string]any); ok {
		b.WriteString("<h2>题目</h2><p>" + esc(pb["text"]) + "</p>")
	}
	if hs, ok := content["hints"].([]any); ok && len(hs) > 0 {
		b.WriteString("<h2>提示</h2><ol>")
		for _, x := range hs {
			m, _ := x.(map[string]any)
			if m == nil || (audience == "student" && m["gated"] == true) {
				continue
			}
			b.WriteString("<li>" + esc(m["text"]) + "</li>")
		}
		b.WriteString("</ol>")
	}
	if bs, ok := content["beats"].([]any); ok && len(bs) > 0 {
		b.WriteString("<h2>讲解</h2>")
		for _, x := range bs {
			m, _ := x.(map[string]any)
			if m == nil {
				continue
			}
			b.WriteString("<h3>" + esc(m["title"]) + "</h3>")
			if m["locked"] == true {
				b.WriteString("<p class=\"note\">老师开放答案后在平台查看。</p>")
				continue
			}
			for _, line := range strList(m["board"]) {
				b.WriteString("<p>" + esc(line) + "</p>")
			}
			if s, _ := m["say"].(string); s != "" {
				b.WriteString("<p class=\"note\">讲述：" + esc(s) + "</p>")
			}
		}
	}
	if ans, ok := content["answer"].(map[string]any); ok && audience == "teacher" {
		b.WriteString("<h2>答案</h2><p>" + esc(ans["plain"]) + "</p>")
	}
	if cq, ok := content["check_question"].(map[string]any); ok && cq["prompt"] != nil {
		b.WriteString("<h2>自测题</h2><p>" + esc(cq["prompt"]) + "</p>")
		if k, ok := content["check_answer_key"].(map[string]any); ok && audience == "teacher" {
			b.WriteString("<p>自测答案：" + esc(k["plain"]) + "</p>")
		}
	}
	b.WriteString("<h2>来源与许可</h2>")
	if rights["registered"] == true {
		kvTable(b, "来源权利（导出时复核）", [][2]any{{"来源", rights["title"]}, {"类型", rights["kind"]}, {"署名", rights["attribution"]}, {"许可", rights["licenseRef"]}, {"权利到期", rights["expiresAt"]}})
	} else {
		b.WriteString("<p>" + esc(rights["note"]) + "</p>")
	}
	b.WriteString("<p class=\"note\">由源铭振跃数学工作台导出。图形内核部分改写自 edulab（Apache-2.0），许可与 NOTICE 随软件分发。</p>")
	if len(omitted) > 0 {
		b.WriteString("<h2>未包含的内容</h2><ul>")
		for _, o := range omitted {
			b.WriteString("<li>" + esc(o.Reason) + "</li>")
		}
		b.WriteString("</ul>")
	}
	kvTable(b, "版本清单", [][2]any{{"发布", man["publicationId"]}, {"发布时间", man["publishedAt"]}, {"讲解修订", man["lessonRevisionId"]}, {"讲解哈希", man["lessonHash"]},
		{"审核记录", man["reviewRecordId"]}, {"审核时间", man["reviewedAt"]}, {"题目修订", man["problemRevisionId"]}, {"题目哈希", man["problemInputHash"]},
		{"解答修订", man["solutionRevisionId"]}, {"解答哈希", man["solutionHash"]}, {"数学状态", man["mathState"]}, {"证据层级", man["evidenceLevels"]}, {"导出时间", man["exportedAt"]}})
	b.WriteString("</body></html>")
	return b.String()
}

var outcomeLabel = []struct{ key, label string }{
	{"not_started", "未开始"}, {"attempted", "已尝试未完成"}, {"completed_independently", "独立完成"},
	{"completed_after_hints", "借助提示完成"}, {"pending_teacher", "待老师判定"},
}

// suppress hides counts of one or two in an exported file.
func suppress(n int) any {
	if n > 0 && n < exportSuppressBelow {
		return fmt.Sprintf("少于%d", exportSuppressBelow)
	}
	return n
}

func (h *API) exportReport(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	var as model.EduAssignment
	if tx.First(&as, "id = ? AND tenant_id = ?", c.Param("id"), a.TenantID).Error != nil {
		h.fail(c, ErrNotFound)
		return
	}
	k, err := classForStaff(tx, a, as.ClassID)
	if err != nil {
		h.fail(c, err)
		return
	}
	var pub model.EduPublication
	tx.First(&pub, "id = ? AND tenant_id = ?", as.PublicationID, a.TenantID)
	st := assignmentStats(tx, as)
	dist := gin.H{}
	for _, o := range outcomeLabel {
		dist[o.key] = suppress(st.Dist[o.key])
	}
	report := gin.H{
		"assignmentId": as.ID, "title": as.Title, "className": k.Name, "assignmentState": as.State,
		"publicationId": pub.ID, "lessonHash": pub.LessonHash, "answerPolicy": as.AnswerPolicy, "answerReleased": as.AnswerReleasedAt != nil,
		"students": st.Students, "distribution": dist,
		"selfTest":      gin.H{"independent": suppress(st.SelfIndependent), "independentCorrect": suppress(st.SelfCorrect)},
		"help":          gin.H{"open": suppress(int(st.OpenHelp)), "openReports": suppress(int(st.OpenReports))},
		"suppressBelow": exportSuppressBelow, "exportedAt": h.Svc.now().UTC(),
	}
	b := htmlHead(as.Title + " 学习报告")
	b.WriteString("<h1>" + esc(as.Title) + " 学习报告</h1><p class=\"note\">班级汇总，不含学生姓名、学号、作答或求助内容；1 至 2 人的格子显示为“少于3”。结果分布只是汇总，错误原因需对照学生作答核实。</p>")
	kvTable(b, "任务", [][2]any{{"班级", k.Name}, {"任务状态", as.State}, {"在读学生", st.Students}, {"答案已开放", map[bool]string{true: "是", false: "否"}[as.AnswerReleasedAt != nil]}})
	rows := [][2]any{}
	for _, o := range outcomeLabel {
		rows = append(rows, [2]any{o.label, dist[o.key]})
	}
	kvTable(b, "完成情况", rows)
	kvTable(b, "自测与求助", [][2]any{{"独立完成自测", report["selfTest"].(gin.H)["independent"]}, {"其中答对", report["selfTest"].(gin.H)["independentCorrect"]},
		{"未处理求助", report["help"].(gin.H)["open"]}, {"其中题目报错", report["help"].(gin.H)["openReports"]}})
	kvTable(b, "版本", [][2]any{{"发布", pub.ID}, {"讲解哈希", pub.LessonHash}, {"导出时间", report["exportedAt"]}})
	b.WriteString("</body></html>")
	h.Svc.audit(tx, a, "export.report", as.ID, "aggregate")
	reply(c, 200, gin.H{"filename": as.ID + "-report.html", "report": report, "html": b.String()}, "ok")
}
