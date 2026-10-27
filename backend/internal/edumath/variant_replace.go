package edumath

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// Teacher's own item replacement in a variant set (DEV-078).
//
// A teacher replaces one item of an existing set with their own numbers in the same family. The rights come
// from a source asset of the workspace that is active, not expired and allows student and derivative use.
// The math service re-normalizes, re-solves and independently verifies every item of the new list (the
// prompt is rendered from the family template, never from teacher free text). The result is a new set with a
// new hash that needs its own review; the earlier set, its reviews and any assignment using it are unchanged.
// If the teacher's item fails a check or repeats an answer it is not silently dropped: the new set is
// rejected with a blocking error naming the reason.

var paramKeyRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,15}$`)
var paramValRE = regexp.MustCompile(`^[0-9a-zA-Z_+\-*/^().√ ]{1,32}$`)

type replaceItem struct {
	Params map[string]string `json:"params"`
	Source string            `json:"source"`
}

// gradingScope states what the automatic judge covers for an item (shown to teachers before assigning).
func gradingScope(answerFormat string) gin.H {
	if answerFormat == "exact" || answerFormat == "" {
		return gin.H{"auto": true, "covers": "final_answer", "equivalence": "exact_value", "decimal": "needs_exact", "steps": "not_graded", "unit": "checked_when_expected"}
	}
	return gin.H{"auto": false, "covers": "none", "steps": "not_graded"}
}

func cleanParams(in map[string]string) (map[string]string, string, error) {
	if len(in) < 1 || len(in) > 6 {
		return nil, "", appErr(400, "PARAMETER_LIMIT", "params must have 1..6 entries", "")
	}
	out := map[string]string{}
	keys := make([]string, 0, len(in))
	for k, v := range in {
		v = strings.TrimSpace(v)
		if !paramKeyRE.MatchString(k) || !paramValRE.MatchString(v) {
			return nil, "", appErr(400, "INPUT_AMBIGUOUS", "parameter "+k+" is not a short numeric expression", "teacher_confirm_input")
		}
		out[k] = v
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + out[k] + ";")
	}
	return out, b.String(), nil
}

// StartVariantReplacement dispatches a variant_set job whose item list is the base set with itemID replaced.
func (s *Service) StartVariantReplacement(ctx context.Context, tx *gorm.DB, a Actor, setID, itemID string, params map[string]string, sourceAssetID, note, idemKey string) (model.EduMathJob, model.EduVariantReplacement, error) {
	var rep model.EduVariantReplacement
	if !a.Has("teacher") {
		return model.EduMathJob{}, rep, ErrNoRole
	}
	var base model.EduVariantSet
	if tx.First(&base, "id = ? AND tenant_id = ? AND workspace_id = ?", setID, a.TenantID, a.WorkspaceID).Error != nil {
		return model.EduMathJob{}, rep, ErrNotFound
	}
	var l model.EduLessonRevision
	if tx.First(&l, "id = ? AND tenant_id = ?", base.LessonRevisionID, a.TenantID).Error != nil {
		return model.EduMathJob{}, rep, ErrNotFound
	}
	r, _, err := s.revision(tx, a, l.ProblemRevisionID)
	if err != nil {
		return model.EduMathJob{}, rep, err
	}
	clean, canon, err := cleanParams(params)
	if err != nil {
		return model.EduMathJob{}, rep, err
	}
	if len(note) > 200 {
		return model.EduMathJob{}, rep, appErr(400, "PARAMETER_LIMIT", "note <= 200 bytes", "")
	}
	inputHash := hashOf("variant_replace", base.ID, base.SetHash, itemID, canon, sourceAssetID)
	var prior model.EduMathJob
	if idemKey != "" && tx.First(&prior, "tenant_id = ? AND kind = ? AND idem_key = ?", a.TenantID, "variant_set", idemKey).Error == nil {
		if prior.InputHash != inputHash {
			return prior, rep, appErr(409, "REVISION_CONFLICT", "idempotency key reused with a different input", "use_new_key")
		}
		tx.First(&rep, "job_id = ? AND tenant_id = ?", prior.ID, a.TenantID)
		return prior, rep, nil
	}
	if base.BlockingErrors != "" {
		return model.EduMathJob{}, rep, appErr(409, "VALIDATION_FAILED", "the base set has blocking errors; generate a new set instead", "teacher_fix")
	}
	if l.BlockingErrors != "" || l.State == "rejected" || l.State == "withdrawn" {
		return model.EduMathJob{}, rep, appErr(409, "VALIDATION_FAILED", "lesson revision has blocking errors or was withdrawn", "teacher_fix")
	}
	if err := rightsAllow(tx, a.TenantID, l.ProblemRevisionID, "derivative"); err != nil {
		return model.EduMathJob{}, rep, err
	}
	var src model.EduSourceAsset
	if sourceAssetID == "" || tx.First(&src, "id = ? AND tenant_id = ? AND workspace_id = ?", sourceAssetID, a.TenantID, a.WorkspaceID).Error != nil {
		return model.EduMathJob{}, rep, appErr(409, "RIGHTS_RESTRICTED", "an own item needs a source asset of this workspace that records its rights", "check_source_rights")
	}
	if !rightsLive(src, time.Now().UTC()) || !src.AllowStudent || !src.AllowDerivative {
		return model.EduMathJob{}, rep, appErr(409, "RIGHTS_RESTRICTED", "the source rights do not allow student and derivative use (expired, withdrawn or not granted)", "check_source_rights")
	}
	var items []variantItem
	if json.Unmarshal([]byte(base.ItemsJSON), &items) != nil || len(items) == 0 {
		return model.EduMathJob{}, rep, ErrContract
	}
	list := make([]replaceItem, 0, len(items))
	found := false
	for _, it := range items {
		src := it.Source
		if src == "" {
			src = "generated"
		}
		if it.ID == itemID {
			found = true
			list = append(list, replaceItem{Params: clean, Source: "teacher_own"})
			continue
		}
		list = append(list, replaceItem{Params: it.Params, Source: src})
	}
	if !found {
		return model.EduMathJob{}, rep, appErr(400, "INPUT_AMBIGUOUS", "item "+itemID+" is not in this set", "reload_revision")
	}
	pj, _ := json.Marshal(clean)
	lj, _ := json.Marshal(list)
	rep = model.EduVariantReplacement{ID: flow.NewID("evx_"), TenantID: a.TenantID, WorkspaceID: a.WorkspaceID, BaseSetID: base.ID, BaseSetHash: base.SetHash,
		ItemID: itemID, ParamsJSON: string(pj), ItemsJSON: string(lj), SourceAssetID: src.ID, Note: strings.TrimSpace(note), CreatedBy: a.UserID, CreatedAt: s.now()}
	rep.JobID = "pending:" + rep.ID
	if err := tx.Create(&rep).Error; err != nil {
		return model.EduMathJob{}, rep, err
	}
	var problem map[string]any
	_ = json.Unmarshal([]byte(r.Structured), &problem)
	var anyList []any
	_ = json.Unmarshal(lj, &anyList)
	payload := map[string]any{"problem": problem, "problem_hash": r.InputHash, "count": len(list), "items": anyList}
	opts, _ := json.Marshal(map[string]any{"count": len(list), "replacementId": rep.ID})
	job, err := s.startJobWith(ctx, tx, a, "variant_set", idemKey, inputHash, r.ID, l.JobID, string(opts), payload)
	if err != nil {
		tx.Delete(&rep)
		return job, rep, err
	}
	tx.Model(&rep).Update("job_id", job.ID)
	rep.JobID = job.ID
	s.audit(tx, a, "variants.replace", rep.ID, base.ID+" item="+itemID+" source="+src.ID)
	return job, rep, nil
}

type variantJobOptions struct {
	Count         int    `json:"count"`
	ReplacementID string `json:"replacementId"`
}

// variantJobPayload restores the kind specific payload for a resend under the same key.
func variantJobPayload(tx *gorm.DB, job model.EduMathJob, payload map[string]any) error {
	var opt variantJobOptions
	if json.Unmarshal([]byte(job.Options), &opt) != nil || opt.Count < 1 {
		return fmt.Errorf("%w: variant_set job options", ErrContract)
	}
	payload["count"] = opt.Count
	if opt.ReplacementID != "" {
		var rep model.EduVariantReplacement
		if tx.First(&rep, "id = ? AND tenant_id = ?", opt.ReplacementID, job.TenantID).Error != nil {
			return ErrNotFound
		}
		var list []any
		if json.Unmarshal([]byte(rep.ItemsJSON), &list) != nil {
			return ErrContract
		}
		payload["items"] = list
	}
	return nil
}

// replacementOf returns the replacement behind a variant_set job, if any, and the blocking reasons for
// teacher items the service dropped (an own item is never silently left out).
func replacementOf(tx *gorm.DB, job model.EduMathJob, dropped json.RawMessage, got int) (model.EduVariantReplacement, []string, bool) {
	var rep model.EduVariantReplacement
	var opt variantJobOptions
	if json.Unmarshal([]byte(job.Options), &opt) != nil || opt.ReplacementID == "" {
		return rep, nil, false
	}
	if tx.First(&rep, "id = ? AND tenant_id = ?", opt.ReplacementID, job.TenantID).Error != nil {
		return rep, []string{"replacement_missing"}, true
	}
	var ds []struct {
		Reason string `json:"reason"`
		Source string `json:"source"`
	}
	_ = json.Unmarshal(dropped, &ds)
	var blocking []string
	for _, d := range ds {
		who := "item"
		if d.Source == "teacher_own" {
			who = "teacher_item"
		}
		blocking = append(blocking, who+"_dropped:"+d.Reason)
	}
	if len(blocking) == 0 && got < opt.Count {
		blocking = append(blocking, "item_count_short")
	}
	return rep, blocking, true
}

func (h *API) replaceVariantItem(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	var in struct {
		ItemID         string            `json:"itemId"`
		Params         map[string]string `json:"params"`
		SourceAssetID  string            `json:"sourceAssetId"`
		Note           string            `json:"note"`
		IdempotencyKey string            `json:"idempotencyKey"`
	}
	if c.ShouldBindJSON(&in) != nil || in.ItemID == "" {
		reply(c, 400, nil, "itemId, params and sourceAssetId required")
		return
	}
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		key = in.IdempotencyKey
	}
	var vs model.EduVariantSet
	if tx.First(&vs, "id = ? AND tenant_id = ? AND workspace_id = ?", c.Param("id"), a.TenantID, a.WorkspaceID).Error != nil {
		h.fail(c, ErrNotFound)
		return
	}
	if _, ok := h.staffLesson(c, tx, a, vs.LessonRevisionID); !ok {
		return
	}
	ctx, cancel := ctx15(c)
	defer cancel()
	j, rep, err := h.Svc.StartVariantReplacement(ctx, tx, a, vs.ID, in.ItemID, in.Params, in.SourceAssetID, in.Note, key)
	if err != nil {
		h.fail(c, err)
		return
	}
	out := jobOut(j)
	out["replacementId"] = rep.ID
	reply(c, 202, out, "accepted")
}
