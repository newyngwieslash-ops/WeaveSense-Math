package edumath

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// Catalog search for reuse (DEV-081, AC-33): a teacher finds lessons of the current workspace by
// title text and problem family, sees whether each is approved or published, and reopens it in the
// author page to assign or edit (edits always create a new revision). Only titles, family, state
// and ids are returned, never answers or artifacts. A teacher may favorite lessons; favorites are private
// to the teacher, hold only ids, and are filtered with favorites=1.

type catalogItem struct {
	LessonRevisionID string    `json:"lessonRevisionId"`
	ProblemID        string    `json:"problemId"`
	Title            string    `json:"title"`
	FamilyID         string    `json:"familyId"`
	State            string    `json:"state"`
	PublicationID    string    `json:"publicationId,omitempty"`
	PublicationState string    `json:"publicationState,omitempty"`
	Mine             bool      `json:"mine"`
	Favorite         bool      `json:"favorite"`
	CreatedAt        time.Time `json:"createdAt"`
}

var catalogStates = map[string][]string{
	"":          {"ready_for_review", "approved", "rejected"},
	"approved":  {"approved"},
	"review":    {"ready_for_review"},
	"rejected":  {"rejected"},
	"published": {"approved"},
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (h *API) catalog(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	family := strings.TrimSpace(c.Query("family"))
	stateKey := c.Query("state")
	favOnly := c.Query("favorites")
	if favOnly != "" && favOnly != "1" {
		h.fail(c, appErr(http.StatusBadRequest, "INVALID_QUERY", "favorites 只能是 1", "去掉或改为 favorites=1"))
		return
	}
	states, known := catalogStates[stateKey]
	if len([]rune(q)) > 60 || len(family) > 40 || !known {
		h.fail(c, appErr(http.StatusBadRequest, "INVALID_QUERY", "检索词至多 60 字，状态只能是 approved、review、rejected 或 published", "缩短检索词或更换筛选"))
		return
	}
	var rows []struct {
		ID, ProblemID, Title, FamilyID, State, CreatedBy, PubID, PubState, FavID string
		CreatedAt                                                                time.Time
	}
	qq := tx.Table("edu_lesson_revisions AS l").
		Select("l.id AS id, p.id AS problem_id, p.title AS title, p.family_id AS family_id, l.state AS state, l.created_by AS created_by, l.created_at AS created_at, pub.id AS pub_id, pub.state AS pub_state, f.id AS fav_id").
		Joins("JOIN edu_math_jobs j ON j.id = l.job_id AND j.tenant_id = l.tenant_id").
		Joins("JOIN edu_problem_revisions r ON r.id = l.problem_revision_id AND r.tenant_id = l.tenant_id").
		Joins("JOIN edu_problems p ON p.id = r.problem_id AND p.tenant_id = l.tenant_id").
		Joins("LEFT JOIN edu_publications pub ON pub.lesson_revision_id = l.id AND pub.tenant_id = l.tenant_id").
		Joins("LEFT JOIN edu_favorites f ON f.lesson_revision_id = l.id AND f.tenant_id = l.tenant_id AND f.workspace_id = ? AND f.user_id = ?", a.WorkspaceID, a.UserID).
		Where("l.tenant_id = ? AND j.workspace_id = ? AND l.state IN ?", a.TenantID, a.WorkspaceID, states)
	if q != "" {
		qq = qq.Where(`LOWER(p.title) LIKE ? ESCAPE '\'`, "%"+strings.ToLower(likeEscape(q))+"%")
	}
	if family != "" {
		qq = qq.Where("p.family_id = ?", family)
	}
	if favOnly == "1" {
		qq = qq.Where("f.id IS NOT NULL")
	}
	if stateKey == "published" {
		qq = qq.Where("pub.state = ?", "published")
	}
	if err := qq.Order("l.created_at DESC").Limit(50).Scan(&rows).Error; err != nil {
		h.fail(c, err)
		return
	}
	items := make([]catalogItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, catalogItem{LessonRevisionID: r.ID, ProblemID: r.ProblemID, Title: r.Title, FamilyID: r.FamilyID, State: r.State,
			PublicationID: r.PubID, PublicationState: r.PubState, Mine: r.CreatedBy == a.UserID, Favorite: r.FavID != "", CreatedAt: r.CreatedAt})
	}
	reply(c, http.StatusOK, gin.H{"items": items, "limit": 50}, "ok")
}

// setFavorite bookmarks a lesson revision for the calling teacher. The lesson must be readable by the
// caller in the current workspace (same staff checks as opening it); repeating the call is a no-op.
func (h *API) setFavorite(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	l, ok := h.staffLesson(c, tx, a, c.Param("id"))
	if !ok {
		return
	}
	var n int64
	if err := tx.Model(&model.EduFavorite{}).Where("tenant_id = ? AND user_id = ? AND lesson_revision_id = ?", a.TenantID, a.UserID, l.ID).Count(&n).Error; err != nil {
		h.fail(c, err)
		return
	}
	if n == 0 {
		var cnt int64
		tx.Model(&model.EduFavorite{}).Where("tenant_id = ? AND user_id = ?", a.TenantID, a.UserID).Count(&cnt)
		if cnt >= maxFavorites {
			h.fail(c, appErr(http.StatusUnprocessableEntity, "PARAMETER_LIMIT", "收藏已达上限", "先取消一些不再使用的收藏"))
			return
		}
		f := model.EduFavorite{ID: flow.NewID("efv_"), TenantID: a.TenantID, WorkspaceID: a.WorkspaceID, UserID: a.UserID, LessonRevisionID: l.ID}
		if err := tx.Create(&f).Error; err != nil {
			h.fail(c, err)
			return
		}
	}
	reply(c, http.StatusOK, gin.H{"lessonRevisionId": l.ID, "favorite": true}, "ok")
}

// unsetFavorite removes the caller's own bookmark only; removing a missing one is a no-op.
func (h *API) unsetFavorite(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if err := tx.Where("tenant_id = ? AND user_id = ? AND lesson_revision_id = ?", a.TenantID, a.UserID, id).Delete(&model.EduFavorite{}).Error; err != nil {
		h.fail(c, err)
		return
	}
	reply(c, http.StatusOK, gin.H{"lessonRevisionId": id, "favorite": false}, "ok")
}

const maxFavorites = 500
