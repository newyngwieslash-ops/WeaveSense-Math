package edumath

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"yingce/backend/internal/model"
)

// errStandinMedia is returned when a lesson (or its media) was produced by the
// local MATH_MEDIA_STANDIN fake. Silent MP3, a minimal MP4 and OCR drafts are
// for automatic tests only and must not be approved, published or sent to students.
var errStandinMedia = appErr(http.StatusConflict, "STANDIN_MEDIA", "stand-in audio, captions or video cannot be approved, published or sent to students", "use_real_provider")

func standinDoc(flag bool, mediaStandin json.RawMessage, provider string, parts []string) bool {
	if flag || provider == "fake-standin" {
		return true
	}
	s := strings.TrimSpace(string(mediaStandin))
	if s != "" && s != "null" && s != "{}" {
		return true
	}
	for _, p := range parts {
		if p == "standin" {
			return true
		}
	}
	return false
}

func errIfStandinDoc(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var doc struct {
		Meta struct {
			Standin      bool            `json:"standin"`
			MediaStandin json.RawMessage `json:"media_standin"`
			Provider     string          `json:"provider"`
			AIParts      []string        `json:"ai_parts"`
		} `json:"meta"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	m := doc.Meta
	if standinDoc(m.Standin, m.MediaStandin, m.Provider, m.AIParts) {
		return errStandinMedia
	}
	return nil
}

func (s *Service) rejectStandinRevision(tx *gorm.DB, tenant string, l model.EduLessonRevision) error {
	if s == nil || !s.Client.Configured() || l.LessonArtifact == "" {
		return nil
	}
	var ta model.EduArtifact
	if tx.First(&ta, "id = ? AND tenant_id = ?", l.LessonArtifact, tenant).Error != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	raw, _, err := s.Client.Artifact(ctx, tenant, ta)
	if err != nil {
		return err
	}
	return errIfStandinDoc(raw)
}

// htmlLooksStandin reports a preview page that already labels itself as a local stand-in.
func htmlLooksStandin(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	if bytes.Contains(raw, []byte("本地替身")) {
		return true
	}
	return bytes.Contains(raw, []byte(`"standin":true`)) || bytes.Contains(raw, []byte(`"standin": true`))
}

func staffMayPreviewStandin(db *gorm.DB, art model.EduArtifact, userID string) bool {
	var job model.EduMathJob
	if db.First(&job, "id = ? AND tenant_id = ?", art.JobID, art.TenantID).Error != nil {
		return false
	}
	rs, err := rolesIn(db, art.TenantID, userID, job.WorkspaceID)
	if err != nil || !(rs["teacher"] || rs["reviewer"] || rs["org_admin"]) {
		return false
	}
	var n int64
	db.Model(&model.WorkspaceMembership{}).Where("workspace_id = ? AND user_id = ?", job.WorkspaceID, userID).Count(&n)
	return n > 0
}

func relatedLessonRevisions(db *gorm.DB, art model.EduArtifact) []model.EduLessonRevision {
	var revs []model.EduLessonRevision
	db.Where("tenant_id = ? AND (lesson_artifact = ? OR student_artifact = ? OR preview_artifact = ? OR student_preview = ? OR render_check_artifact = ? OR job_id = ?)",
		art.TenantID, art.ID, art.ID, art.ID, art.ID, art.ID, art.JobID).Find(&revs)
	return revs
}

func (s *Service) fileIsStandin(ctx context.Context, db *gorm.DB, art model.EduArtifact, raw []byte) bool {
	if errIfStandinDoc(raw) != nil || htmlLooksStandin(raw) {
		return true
	}
	if s == nil || db == nil {
		return false
	}
	seen := map[string]bool{}
	checkTeacher := func(id string) bool {
		if id == "" || seen[id] {
			return false
		}
		seen[id] = true
		var ta model.EduArtifact
		if db.First(&ta, "id = ? AND tenant_id = ?", id, art.TenantID).Error != nil {
			return false
		}
		if !s.Client.Configured() {
			return false
		}
		doc, _, err := s.Client.Artifact(ctx, art.TenantID, ta)
		return err == nil && errIfStandinDoc(doc) != nil
	}
	for _, l := range relatedLessonRevisions(db, art) {
		if checkTeacher(l.LessonArtifact) {
			return true
		}
	}
	if art.JobID == "" {
		return false
	}
	var sibs []model.EduArtifact
	db.Where("tenant_id = ? AND job_id = ? AND id <> ?", art.TenantID, art.JobID, art.ID).Find(&sibs)
	for _, sib := range sibs {
		if sib.Kind != "lesson" && !strings.Contains(sib.MediaType, "json") {
			continue
		}
		if checkTeacher(sib.ID) {
			return true
		}
	}
	return false
}

// refuseStandinFile rejects a signed fetch of stand-in media or a stand-in
// lesson/preview for anyone who is not staff previewing in their workspace.
// A valid signature does not override this: previously issued student or
// public links must not keep serving silent MP3, minimal MP4 or stand-in HTML.
func (s *Service) refuseStandinFile(ctx context.Context, db *gorm.DB, art model.EduArtifact, userID string, raw []byte) error {
	if !s.fileIsStandin(ctx, db, art, raw) {
		return nil
	}
	if staffMayPreviewStandin(db, art, userID) {
		return nil
	}
	return errStandinMedia
}
