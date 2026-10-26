package flow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"yingce/backend/internal/model"
)

// reportEvidenceClaims covers nested method/threshold sources and structured
// rows too; validating only top-level prose would permit fabricated factors.
func reportEvidenceClaims(a map[string]any) ([]ReportClaim, error) {
	var claims []ReportClaim
	if err := decodeInto(a["claims"], &claims); err != nil {
		return nil, fmt.Errorf("%w: claims: %v", ErrToolArgs, err)
	}
	if len(claims) == 0 || len(claims) > 100 {
		return nil, fmt.Errorf("%w: claims: 1..100 required", ErrCitation)
	}
	var walk func(any, string, bool) error
	walk = func(value any, path string, row bool) error {
		switch v := value.(type) {
		case []any:
			for i, x := range v {
				if err := walk(x, fmt.Sprintf("%s[%d]", path, i), true); err != nil {
					return err
				}
			}
		case map[string]any:
			if strings.Contains(path,".claims[") { var claim ReportClaim; if err:=decodeInto(v,&claim);err!=nil{return err}; claims=append(claims,claim);return nil }
			raw, has := v["citations"]
			required := row || strings.HasSuffix(path, "_source") || strings.HasSuffix(path, "_config")
			if has || required {
				var cites []Citation
				if err := decodeInto(raw, &cites); err != nil || len(cites) == 0 {
					return fmt.Errorf("%w: %s: row/source citations required", ErrCitation, path)
				}
				kind := argStr(v, "kind")
				if kind == "" {
					kind = "source_statement"
				}
				claims = append(claims, ReportClaim{Text: path, Kind: kind, Citations: cites})
			}
			for k, x := range v {
				if k != "citations" && k != "coverage" && k != "gaps" && k != "material_scope" {
					if err := walk(x, path+"."+k, false); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := walk(a["structured"], "structured", false); err != nil {
		return nil, err
	}
	return claims, nil
}

func prepareManagedReport(t Tools, ctx context.Context, conv model.FlowAgentConversation, ss SubjectScope, userID string, a map[string]any) (map[string]any, error) {
	claims, err := reportEvidenceClaims(a)
	if err != nil {
		return nil, err
	}
	var scope MaterialScope
	if err := decodeInto(a["material_scope"], &scope); err != nil {
		return nil, fmt.Errorf("%w: material_scope", ErrToolArgs)
	}
	ids := []string{}
	for _, c := range claims {
		for _, ct := range c.Citations {
			ids = append(ids, ct.EvidenceID)
		}
	}
	resolved, err := t.resolveEvidence(ctx, ss, userID, ids)
	if err != nil {
		return nil, err
	}
	truncated, err := ValidateCitations(claims, scope, resolved)
	if err != nil {
		CountCitationRejected("report_save")
		return nil, err
	}
	if strings.TrimSpace(argStr(a, "title")) == "" || strings.TrimSpace(argStr(a, "body")) == "" {
		return nil, fmt.Errorf("%w: report title and body required", ErrToolArgs)
	}
	skill, err := skillRefFor(t, conv, ss, a)
	if err != nil {
		return nil, err
	}
	if rawIDs, exists := a["skill_ids"]; exists {
		var ids []string
		if err := decodeInto(rawIDs, &ids); err != nil || len(ids) == 0 || len(ids) > 32 {
			return nil, fmt.Errorf("%w: skill_ids requires 1..32 installed skill IDs", ErrToolArgs)
		}
		refs := []any{}
		seen := map[string]bool{}
		if id := argStr(a, "skill_id"); id != "" {
			ids = append([]string{id}, ids...)
		}
		for _, id := range ids {
			if strings.TrimSpace(id) == "" {
				return nil, fmt.Errorf("%w: empty skill ID", ErrToolArgs)
			}
			if seen[id] {
				continue
			}
			pinned, err := LoadSkillForRun(t.DB, ss.Scope, t.runKey(conv), id)
			if err != nil {
				return nil, err
			}
			refs = append(refs, pinned.Ref())
			seen[id] = true
		}
		combined := map[string]any{}
		if err := decodeInto(skill, &combined); err != nil {
			return nil, err
		}
		combined["refs"] = refs
		skill = combined
	}
	out := map[string]any{}
	for k, v := range a {
		if k != "skill_id" && k != "skill_ids" {
			out[k] = v
		}
	}
	out["skill"] = skill
	coverage := map[string]any{}
	if a["coverage"] != nil {
		if err := decodeInto(a["coverage"], &coverage); err != nil || coverage == nil {
			return nil, fmt.Errorf("%w: coverage object required", ErrToolArgs)
		}
	}
	if coverage["_managed_report"] != nil {
		return nil, fmt.Errorf("%w: reserved report metadata", ErrToolArgs)
	}
	seen := map[string]bool{}
	var merged []string
	var existing []string
	_ = decodeInto(coverage["truncated_citations"], &existing)
	for _, id := range append(existing, truncated...) {
		if !seen[id] {
			merged = append(merged, id)
			seen[id] = true
		}
	}
	coverage["truncated_citations"] = merged
	coverage["truncated"] = coverage["truncated"] == true || len(merged) > 0
	out["coverage"] = coverage
	return out, nil
}

// A report write is re-enterable because Loom checks the same key AND payload.
// Unknown/5xx outcomes use the existing persisted tool reconciliation state.
func saveManagedReport(t Tools, ctx context.Context, conv model.FlowAgentConversation, ss SubjectScope, userID string, a map[string]any) (any, error) {
	key, _ := ctx.Value(callKeyCtx{}).(string)
	if key == "" {
		return nil, fmt.Errorf("%w: report save key required", ErrToolArgs)
	}
	var row model.FlowAgentToolCall
	callID := strings.TrimPrefix(key, "tool:"+conv.ID+":")
	if err := t.DB.Where("tenant_id = ? AND conversation_id = ? AND call_id = ?", conv.TenantID, conv.ID, callID).First(&row).Error; err != nil {
		return nil, err
	}
	var stored map[string]any
	if err := json.Unmarshal([]byte(row.Arguments), &stored); err != nil {
		return nil, err
	}
	payload, frozen := stored["_prepared_report"].(map[string]any)
	if !frozen {
		var err error
		payload, err = prepareManagedReport(t, ctx, conv, ss, userID, a)
		if err != nil {
			return nil, err
		}
		stored["_prepared_report"] = payload
		frozenArgs, err := json.Marshal(stored)
		if err != nil {
			return nil, err
		}
		if err := t.DB.Model(&row).Update("arguments", string(frozenArgs)).Error; err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if row.Status == "unknown" {
		// Match Loom's sorted-key JSON hash without HTML escaping. A mismatch
		// fails closed; only an explicit scoped 404 permits a same-key retry.
		var canonical bytes.Buffer
		encoder := json.NewEncoder(&canonical)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(payload); err != nil {
			return nil, err
		}
		sum := sha256.Sum256(bytes.TrimSuffix(canonical.Bytes(), []byte("\n")))
		path := "/v1/reports/by-key?" + url.Values{"key": {key}, "request_hash": {hex.EncodeToString(sum[:])}}.Encode()
		status, prior, err := t.Engine.Do(cctx, ss.Scope, Actor{Type: "user", ID: userID}, http.MethodGet, path, "", nil)
		if err != nil || status >= 500 || status == 429 {
			return nil, fmt.Errorf("%w: report reconciliation failed (status %d): %v", ErrMCPUnknownOutcome, status, err)
		}
		if status == http.StatusOK {
			var found struct {
				Data struct {
					ReportID string `json:"report_id"`
				} `json:"data"`
			}
			if json.Unmarshal(prior, &found) != nil || !strings.HasPrefix(found.Data.ReportID, "rpt_") {
				return nil, fmt.Errorf("%w: invalid report reconciliation response", ErrMCPUnknownOutcome)
			}
			return map[string]any{"report_id": found.Data.ReportID, "replayed": true}, nil
		}
		if status != http.StatusNotFound {
			return nil, fmt.Errorf("report reconciliation returned %d", status)
		}
	}
	status, body, err := t.Engine.Do(cctx, ss.Scope, Actor{Type: "user", ID: userID}, http.MethodPost, "/v1/reports", key, raw)
	if err != nil {
		return nil, fmt.Errorf("%w: report engine unavailable: %v", ErrMCPUnknownOutcome, err)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
		Msg  string          `json:"msg"`
	}
	_ = json.Unmarshal(body, &env)
	if status < 200 || status >= 300 {
		failure := fmt.Errorf("report engine returned %d: %s", status, env.Msg)
		if status >= 500 || status == 429 {
			return nil, fmt.Errorf("%w: %v", ErrMCPUnknownOutcome, failure)
		}
		return nil, failure
	}
	if len(env.Data) > 0 {
		body = env.Data
	}
	var saved struct {
		ReportID string `json:"report_id"`
		Replayed bool   `json:"replayed"`
	}
	if json.Unmarshal(body, &saved) != nil || !strings.HasPrefix(saved.ReportID, "rpt_") {
		return nil, fmt.Errorf("%w: engine did not return a flow report_id", ErrMCPUnknownOutcome)
	}
	return saved, nil
}
