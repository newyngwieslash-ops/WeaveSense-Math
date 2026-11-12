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
