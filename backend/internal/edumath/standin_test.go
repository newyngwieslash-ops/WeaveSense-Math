package edumath

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

func TestErrIfStandinDoc(t *testing.T) {
	ok := []string{
		``,
		`{}`,
		`{"meta":{}}`,
		`{"meta":{"ai_generated":true,"ai_parts":["narration","audio"]}}`,
		`{"meta":{"media_standin":null}}`,
		`{"meta":{"media_standin":{}}}`,
		`not json`,
	}
	for _, doc := range ok {
		if err := errIfStandinDoc([]byte(doc)); err != nil {
			t.Errorf("accepted lesson rejected: %s (%v)", doc, err)
		}
	}
	bad := []string{
		`{"meta":{"standin":true}}`,
		`{"meta":{"media_standin":{"provider":"fake-standin"}}}`,
		`{"meta":{"provider":"fake-standin"}}`,
		`{"meta":{"ai_parts":["narration","audio","standin"]}}`,
		`{"meta":{"standin":true,"ai_parts":["standin"],"media_standin":{"provider":"fake-standin"}}}`,
	}
	for _, doc := range bad {
		err := errIfStandinDoc([]byte(doc))
		if err == nil {
			t.Errorf("stand-in lesson accepted: %s", doc)
			continue
		}
		ae, ok := err.(*AppError)
		if !ok || ae.Code != "STANDIN_MEDIA" || ae.Status != 409 {
			t.Errorf("%s: got %v", doc, err)
		}
	}
}

func TestProvenanceMarksStandin(t *testing.T) {
	p := provenanceOf([]byte(`{"meta":{"ai_generated":true,"generator":"fake-standin","ai_parts":["narration","audio","standin"],"standin":true,"media_standin":{"provider":"fake-standin"}}}`))
	if !p.Standin || p.Kind != "model_assisted" {
		t.Fatalf("standin=%v kind=%s", p.Standin, p.Kind)
	}
	if !strings.Contains(p.Label, "本地替身") || !strings.Contains(p.Label, "不可作为正式课件发布") {
		t.Fatalf("label %q", p.Label)
	}
	if provenanceOf([]byte(`{"meta":{"ai_generated":false,"generator":"deterministic-template"}}`)).Standin {
		t.Fatal("plain template marked stand-in")
	}
}

func TestProblemImageOCRStandinOff(t *testing.T) {
	h := newHarness(t, startMathService(t)) // MATH_MEDIA_STANDIN unset, default off
	_ = h.setupTeacher()
	src := h.mustSource("u_alice")
	var buf bytes.Buffer
	if err := png.Encode(&buf, testPicture(32, 24)); err != nil {
		t.Fatal(err)
	}
	c, out, raw := h.uploadImage("u_alice", buf.Bytes(), "q.png", "image/png", src)
	if c != 201 {
		t.Fatalf("upload: %d %s", c, raw)
	}
	id := data(out)["id"].(string)
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/problem-images/"+id+"/ocr", nil)
	if c == 200 {
		t.Fatalf("OCR succeeded with stand-in off: %s", raw)
	}
	if code := errCode(out); code != "PROVIDER_UNAVAILABLE" {
		t.Fatalf("ocr with stand-in off: %d code=%s %s", c, code, raw)
	}
}

func TestHtmlLooksStandin(t *testing.T) {
	if htmlLooksStandin([]byte(`<p>普通预览</p>`)) || htmlLooksStandin(nil) {
		t.Fatal("plain html marked stand-in")
	}
	if !htmlLooksStandin([]byte(`<p class="prov">本地替身媒体，不可作为正式课件发布</p>`)) {
		t.Fatal("label missed")
	}
	if !htmlLooksStandin([]byte(`<meta name="content-provenance" content="{&#34;standin&#34;:true}">`)) &&
		!htmlLooksStandin([]byte(`<meta name="content-provenance" content='{"standin":true}'>`)) {
		t.Fatal("json standin flag missed")
	}
}

func TestFileRefusesStandinForStudentKeepsTeacherPreview(t *testing.T) {
	teacherLesson := []byte(`{"meta":{"standin":true,"ai_parts":["standin"],"ai_generated":true,"generator":"fake-standin"}}`)
	studentLesson := []byte(`{"meta":{"ai_generated":false,"generator":"deterministic-template"}}`)
	teacherHTML := []byte(`<html><p class="prov" data-provenance>教师预览</p></html>`)
	studentHTML := []byte(`<html><p class="prov" data-provenance>学生预览</p></html>`)
	cleanHTML := []byte(`<html><p class="prov" data-provenance>正常学生预览</p></html>`)
	audio := []byte{0xff, 0xfb, 0x90, 0x00} // not a real MP3; bytes only

	type blob struct {
		kind, audience, media string
		data                  []byte
	}
	ma := func(n byte) string { return "ma_" + strings.Repeat(string("0123456789abcdef"[n]), 32) }
	blobs := map[string]blob{
		ma(0): {kind: "lesson", audience: "teacher", media: "application/json", data: teacherLesson},
		ma(1): {kind: "lesson", audience: "student", media: "application/json", data: studentLesson},
		ma(2): {kind: "preview_html", audience: "teacher", media: "text/html; charset=utf-8", data: teacherHTML},
		ma(3): {kind: "preview_html_student", audience: "student", media: "text/html; charset=utf-8", data: studentHTML},
		ma(4): {kind: "audio", audience: "teacher", media: "audio/mpeg", data: audio},
		ma(5): {kind: "preview_html_student", audience: "student", media: "text/html; charset=utf-8", data: cleanHTML},
		ma(6): {kind: "lesson", audience: "teacher", media: "application/json", data: []byte(`{"meta":{"ai_generated":false,"generator":"deterministic-template"}}`)},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/artifacts/")
		b, ok := blobs[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		sum := sha256.Sum256(b.data)
		digest := "sha256:" + hex.EncodeToString(sum[:])
		w.Header().Set("Content-Type", b.media)
		w.Header().Set("X-Artifact-Sha256", digest)
		w.Header().Set("X-Artifact-Kind", b.kind)
		w.Header().Set("X-Artifact-Audience", b.audience)
		w.WriteHeader(200)
		_, _ = w.Write(b.data)
	}))
	t.Cleanup(srv.Close)

	h := newHarness(t, srv.URL)
	tenant := h.setupTeacher()
	ws := model.PersonalWorkspaceID("u_alice")

	putArt := func(id, svc, job, kind, audience, media string, raw []byte) model.EduArtifact {
		sum := sha256.Sum256(raw)
		art := model.EduArtifact{
			ID: id, TenantID: tenant, JobID: job, ServiceArtifactID: svc,
			Kind: kind, Audience: audience, MediaType: media,
			SHA256: "sha256:" + hex.EncodeToString(sum[:]), Size: int64(len(raw)), CreatedAt: time.Now().UTC(),
		}
		if err := h.db.Create(&art).Error; err != nil {
			t.Fatal(err)
		}
		return art
	}

	jobStandin := "emj_standin1"
	jobClean := "emj_clean1"
	if err := h.db.Create(&model.EduMathJob{ID: jobStandin, TenantID: tenant, WorkspaceID: ws, Kind: "lesson_build", IdemKey: "standin", InputHash: "sha256:" + strings.Repeat("a", 64), Status: "succeeded", Settlement: "settled", CreatedBy: "u_alice"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.db.Create(&model.EduMathJob{ID: jobClean, TenantID: tenant, WorkspaceID: ws, Kind: "lesson_build", IdemKey: "clean", InputHash: "sha256:" + strings.Repeat("b", 64), Status: "succeeded", Settlement: "settled", CreatedBy: "u_alice"}).Error; err != nil {
		t.Fatal(err)
	}

	ta := putArt("ear_tlesson", ma(0), jobStandin, "lesson", "teacher", "application/json", teacherLesson)
	sa := putArt("ear_slesson", ma(1), jobStandin, "lesson", "student", "application/json", studentLesson)
	th := putArt("ear_thtml", ma(2), jobStandin, "preview_html", "teacher", "text/html; charset=utf-8", teacherHTML)
	sh := putArt("ear_shtml", ma(3), jobStandin, "preview_html_student", "student", "text/html; charset=utf-8", studentHTML)
	au := putArt("ear_taudio", ma(4), jobStandin, "audio", "teacher", "audio/mpeg", audio)
	ch := putArt("ear_chtml", ma(5), jobClean, "preview_html_student", "student", "text/html; charset=utf-8", cleanHTML)
	_ = putArt("ear_clesson", ma(6), jobClean, "lesson", "teacher", "application/json", blobs[ma(6)].data)

	revS := model.EduLessonRevision{ID: "elr_standin", TenantID: tenant, JobID: jobStandin, LessonArtifact: ta.ID, StudentArtifact: sa.ID, PreviewArtifact: th.ID, StudentPreview: sh.ID, LessonHash: "sha256:" + strings.Repeat("c", 64), State: "approved", CreatedBy: "u_alice", CreatedAt: time.Now().UTC()}
	revC := model.EduLessonRevision{ID: "elr_clean", TenantID: tenant, JobID: jobClean, LessonArtifact: "ear_clesson", StudentPreview: ch.ID, LessonHash: "sha256:" + strings.Repeat("d", 64), State: "approved", CreatedBy: "u_alice", CreatedAt: time.Now().UTC()}
	if err := h.db.Create(&revS).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.db.Create(&revC).Error; err != nil {
		t.Fatal(err)
	}
	revRec := model.EduReviewRecord{ID: "erv_standin", TenantID: tenant, LessonRevisionID: revS.ID, LessonHash: revS.LessonHash, ReviewerID: "u_alice", Decision: "approved", CreatedAt: time.Now().UTC()}
	revRecC := model.EduReviewRecord{ID: "erv_clean", TenantID: tenant, LessonRevisionID: revC.ID, LessonHash: revC.LessonHash, ReviewerID: "u_alice", Decision: "approved", CreatedAt: time.Now().UTC()}
	if err := h.db.Create(&revRec).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.db.Create(&revRecC).Error; err != nil {
		t.Fatal(err)
	}
	pubS := model.EduPublication{ID: "epb_standin", TenantID: tenant, LessonRevisionID: revS.ID, LessonHash: revS.LessonHash, ReviewRecordID: revRec.ID, State: "published", PublishedBy: "u_alice", CreatedAt: time.Now().UTC()}
	pubC := model.EduPublication{ID: "epb_clean", TenantID: tenant, LessonRevisionID: revC.ID, LessonHash: revC.LessonHash, ReviewRecordID: revRecC.ID, State: "published", PublishedBy: "u_alice", CreatedAt: time.Now().UTC()}
	if err := h.db.Create(&pubS).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.db.Create(&pubC).Error; err != nil {
		t.Fatal(err)
	}

	code, out, raw, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "替身取件班"})
	if code != 201 {
		t.Fatalf("class: %d %s", code, raw)
	}
	classID := data(out)["id"].(string)
	code, out, raw, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubS.ID, "title": "替身课后"})
	if code != 201 {
		t.Fatalf("assignment standin: %d %s", code, raw)
	}
	code, out, raw, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubC.ID, "title": "正常课后"})
	if code != 201 {
		t.Fatalf("assignment clean: %d %s", code, raw)
	}
	code, out, raw, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	if code != 201 {
		t.Fatalf("invite: %d %s", code, raw)
	}
	invite := data(out)["code"]
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": invite}); c != 200 {
		t.Fatalf("join: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-e100"}); c != 200 {
		t.Fatalf("consent: %d %s", c, raw)
	}

	mustFetch := func(uid, artID string) (int, map[string]any, []byte) {
		t.Helper()
		u, _ := h.svc.SignedURL(artID, uid, time.Minute)
		path, err := url.Parse(u)
		if err != nil {
			t.Fatal(err)
		}
		c, out, raw, _ := h.call("", "GET", path.RequestURI(), nil)
		return c, out, raw
	}

	// Teacher preview of stand-in HTML, lesson JSON and audio stays available.
	for _, id := range []string{th.ID, ta.ID, au.ID, sh.ID} {
		c, _, raw := mustFetch("u_alice", id)
		if c != 200 {
			t.Fatalf("teacher preview %s: %d %s", id, c, raw)
		}
	}
	// Student / public signed URL must not serve stand-in preview or lesson, even with a valid signature.
	for _, id := range []string{sh.ID, sa.ID} {
		c, out, raw := mustFetch("u_bob", id)
		if c != 409 || errCode(out) != "STANDIN_MEDIA" {
			t.Fatalf("student stand-in file %s: %d code=%s %s", id, c, errCode(out), raw)
		}
	}
	// A published lesson that is not a stand-in still serves to the student.
	c, _, raw := mustFetch("u_bob", ch.ID)
	if c != 200 {
		t.Fatalf("clean student preview: %d %s", c, raw)
	}
}
