package flow

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/google/jsonschema-go/jsonschema"
	"yingce/backend/internal/canvas/capability"
)

// CanvasWorkflowSourceResult is a candidate snapshot, never a mutation of the
// canvas. The caller may commit LastSyncedAt only with its successful node write.
// Truncated snapshots deliberately have no LastSyncedAt.
type CanvasWorkflowSourceResult struct {
	Items        []any                        `json:"items"`
	Coverage     CanvasWorkflowSourceCoverage `json:"coverage"`
	Evidence     []CanvasWorkflowEvidence     `json:"evidence"`
	ItemEvidence [][]CanvasWorkflowEvidence   `json:"item_evidence"`
	Errors       []CanvasWorkflowSourceError  `json:"errors"`
	LastSyncedAt string                       `json:"lastSyncedAt,omitempty"`
	RunID        string                       `json:"run_id,omitempty"`
}
type CanvasWorkflowSourceCoverage struct {
	Truncated   bool                        `json:"truncated"`
	SearchPages int                         `json:"search_pages"`
	ReadPages   int                         `json:"read_pages"`
	Documents   int                         `json:"documents"`
	Window      *CanvasWorkflowSourceWindow `json:"window,omitempty"`
	Reasons     []string                    `json:"reasons"`
}
type CanvasWorkflowSourceWindow struct {
	Kind     string `json:"kind"`
	From     string `json:"from"`
	To       string `json:"to"`
	Timezone string `json:"timezone"`
}
type CanvasWorkflowEvidence struct {
	ResolvedEvidence
	TenantID    string `json:"tenant_id"`
	WorkspaceID string `json:"workspace_id"`
	CharStart   int    `json:"char_start"`
	CharEnd     int    `json:"char_end"`
}
type CanvasWorkflowSourceError struct {
	Reason string `json:"reason"`
}

func (e CanvasWorkflowSourceError) Error() string { return "canvas workflow source: " + e.Reason }
func sourceError(reason string) error             { return CanvasWorkflowSourceError{Reason: reason} }

const sourceMaxSearchPages = 10
const sourceMaxReadPages = 10
const sourceMaxDocuments = 100

// ResolveCanvasWorkflowSource is the runner integration boundary. DB is needed
// for current authorization even on replay; Model is intentionally not called.
func (t Tools) ResolveCanvasWorkflowSource(ctx context.Context, ss SubjectScope, userID string, node map[string]any, executionKey string, now time.Time) (result CanvasWorkflowSourceResult, err error) {
	result = CanvasWorkflowSourceResult{Items: []any{}, Evidence: []CanvasWorkflowEvidence{}, ItemEvidence: [][]CanvasWorkflowEvidence{}, Errors: []CanvasWorkflowSourceError{}, RunID: t.RunID, Coverage: CanvasWorkflowSourceCoverage{Reasons: []string{}}}
	defer func() {
		if err != nil {
			var issue CanvasWorkflowSourceError
			if !errors.As(err, &issue) {
				issue = CanvasWorkflowSourceError{Reason: "source_failed"}
			}
			result.Errors = append(result.Errors, issue)
			result.Items = []any{}
			result.Evidence = []CanvasWorkflowEvidence{}
			result.ItemEvidence = [][]CanvasWorkflowEvidence{}
			result.LastSyncedAt = ""
		} else if !result.Coverage.Truncated {
			result.LastSyncedAt = now.UTC().Format(time.RFC3339Nano)
		}
	}()
	if ctx.Err() != nil {
		return result, sourceError("cancelled")
	}
	if now.IsZero() || strings.TrimSpace(executionKey) == "" {
		return result, sourceError("invalid_execution")
	}
	if t.DB == nil || userID == "" || ss.Scope.TenantID == "" || ss.Scope.WorkspaceID == "" {
		return result, sourceError("scope_forbidden")
	}
	current, e := ResolveScope(t.DB.WithContext(ctx), userID, ss.Scope.WorkspaceID)
	if e != nil || current.TenantID != ss.Scope.TenantID {
		return result, sourceError("scope_forbidden")
	}
	if ss.Subject.Kind != "" {
		resolved, e := ResolveSubject(t.DB.WithContext(ctx), userID, ss.Subject)
		if e != nil || resolved.Scope.TenantID != current.TenantID || resolved.Scope.WorkspaceID != current.WorkspaceID {
			return result, sourceError("subject_forbidden")
		}
	}
	ss.Scope = current
	if sourceString(node, "type") != "data-source" {
		return result, sourceError("invalid_source_node")
	}
	meta, _ := node["metadata"].(map[string]any)
	ds, _ := meta["dataSource"].(map[string]any)
	if ds == nil {
		return result, sourceError("invalid_source_config")
	}
	binding, _ := ds["sourceBinding"].(map[string]any)
	if ds["sourceBinding"] != nil && binding == nil {
		return result, sourceError("invalid_source_binding")
	}
	kind := sourceString(binding, "kind")
	if kind == "" {
		kind = "static"
	}
	window, e := sourceWindow(binding["timeWindow"], now)
	if e != nil {
		return result, e
	}
	result.Coverage.Window = window
	switch kind {
	case "static":
		raw, ok := ds["rawText"].(string)
		if ds["rawText"] != nil && !ok {
			return result, sourceError("invalid_raw_text")
		}
		result.Items, e = sourceStrictItems(raw, sourceString(ds, "format"), sourceString(ds, "fileName"))
		if e != nil {
			return result, e
		}
		// A machine reference is never trusted merely because it was saved in JSON.
		var refs []HandoffCitation
		if ds["evidence"] != nil {
			if e = decodeInto(ds["evidence"], &refs); e != nil {
				return result, sourceError("invalid_evidence")
			}
		}
		result.Evidence, e = t.verifyHandoffEvidence(ctx, ss, userID, refs)
		if e != nil {
			return result, e
		}
		for range result.Items {
			result.ItemEvidence = append(result.ItemEvidence, result.Evidence)
		}
	case "intel_query", "topic":
		queries := []string{sourceString(binding, "query")}
		sources, e := sourceStringList(binding["sourceIds"])
		if e != nil {
			return result, e
		}
		if kind == "topic" {
			id := sourceString(binding, "refId")
			if id == "" {
				return result, sourceError("topic_id_required")
			}
			topic, e := GetTopic(t.DB.WithContext(ctx), ss.Scope, id)
			if e != nil {
				return result, sourceError("topic_forbidden")
			}
			queries = append([]string{topic.Question}, topic.Keywords...)
			sources = topic.SourceIDs
		}
		unique := []string{}
		seen := map[string]bool{}
		for _, q := range queries {
			q = strings.TrimSpace(q)
			if q != "" && !seen[q] {
				seen[q] = true
				unique = append(unique, q)
			}
		}
		if len(unique) == 0 {
			return result, sourceError("query_required")
		}
		e = t.sourceDocuments(ctx, ss, userID, node, executionKey, unique, sources, &result)
		if e != nil {
			return result, e
		}
	case "monitor":
		// Actual recorded hits only: the engine's scoped, paged hit API is the
		// single truth. Monitor configuration/preview is never treated as hits.
		if t.Engine == nil {
			return result, sourceError("monitor_hits_unavailable")
		}
		monitorID := sourceString(binding, "refId")
		if monitorID == "" {
			return result, sourceError("monitor_id_required")
		}
		if e = t.sourceMonitorHits(ctx, ss, userID, monitorID, &result); e != nil {
			return result, e
		}
	case "mcp_tool":
		e = t.sourceMCP(ctx, ss, userID, node, executionKey, binding, &result)
		if e != nil {
			return result, e
		}
	default:
		return result, sourceError("unsupported_source_kind")
	}
	return result, nil
}

func sourceString(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return strings.TrimSpace(v)
}
func sourceStringList(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	var values []string
	if err := decodeInto(v, &values); err != nil {
		return nil, sourceError("invalid_source_ids")
	}
	for _, s := range values {
		if strings.TrimSpace(s) == "" {
			return nil, sourceError("invalid_source_ids")
		}
	}
	return values, nil
}

func sourceStrictItems(raw, format string, fileName ...string) ([]any, error) {
	items, err := wfParseRawItems(raw, format, fileName...)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []any{}
	}
	return items, nil
}

func sourceWindow(v any, now time.Time) (*CanvasWorkflowSourceWindow, error) {
	if v == nil {
		return nil, nil
	}
	var window map[string]any
	if decodeInto(v, &window) != nil || window == nil {
		return nil, sourceError("invalid_time_window")
	}
	from, to, err := capability.ResolveTimeWindow(window, now)
	if err != nil {
		return nil, sourceError(err.Error())
	}
	timezone := sourceString(window, "timezone")
	if timezone == "" {
		timezone = "UTC"
	}
	return &CanvasWorkflowSourceWindow{
		Kind: sourceString(window, "kind"), Timezone: timezone,
		From: from.UTC().Format(time.RFC3339Nano), To: to.UTC().Format(time.RFC3339Nano),
	}, nil
}

func (t Tools) sourceHTTP(ctx context.Context, ss SubjectScope, userID, method, path, key string, body any, out any) error {
	if t.Engine == nil {
		return sourceError("engine_unavailable")
	}
	var raw []byte
	var e error
	if body != nil {
		raw, e = json.Marshal(body)
		if e != nil {
			return sourceError("invalid_request")
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	status, data, e := t.Engine.Do(cctx, ss.Scope, Actor{Type: "user", ID: userID}, method, path, key, raw)
	if e != nil {
		return sourceError("engine_unavailable")
	}
	if status < 200 || status >= 300 {
		if status == 401 || status == 403 || status == 404 {
			return sourceError("source_forbidden")
		}
		return sourceError("engine_http_error")
	}
	var env struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &env) != nil || env.Code == nil || *env.Code != 0 || len(env.Data) == 0 || string(env.Data) == "null" {
		return sourceError("invalid_engine_response")
	}
	if json.Unmarshal(env.Data, out) != nil {
		return sourceError("invalid_engine_response")
	}
	return nil
}
func sourceTruncate(result *CanvasWorkflowSourceResult, reason string) {
	result.Coverage.Truncated = true
	for _, r := range result.Coverage.Reasons {
		if r == reason {
			return
		}
	}
	result.Coverage.Reasons = append(result.Coverage.Reasons, reason)
}

type sourceHit struct {
	DocumentID string `json:"document_id"`
	Revision   int    `json:"revision"`
	SourceID   string `json:"source_id"`
}

func (t Tools) sourceDocuments(ctx context.Context, ss SubjectScope, userID string, node map[string]any, key string, queries, sources []string, result *CanvasWorkflowSourceResult) error {
	seen := map[string]bool{}
	for _, query := range queries {
		offset := 0
		for {
			if result.Coverage.SearchPages >= sourceMaxSearchPages {
				sourceTruncate(result, "search_page_limit")
				return nil
			}
			body := map[string]any{"query": query, "source_ids": sources, "offset": offset, "limit": 50, "vector": false}
			if w := result.Coverage.Window; w != nil {
				body["from"] = w.From
				body["to"] = w.To
			}
			var page struct {
				Items *[]sourceHit `json:"items"`
				Next  *int         `json:"next_offset"`
			}
			if e := t.sourceHTTP(ctx, ss, userID, http.MethodPost, "/v1/documents/search", "", body, &page); e != nil {
				return e
			}
			result.Coverage.SearchPages++
			if page.Items == nil || len(*page.Items) > 50 || (page.Next != nil && *page.Next <= offset) {
				return sourceError("invalid_search_page")
			}
			for _, hit := range *page.Items {
				if hit.DocumentID == "" || hit.Revision <= 0 {
					return sourceError("invalid_document_reference")
				}
				id := hit.DocumentID + ":" + strconv.Itoa(hit.Revision)
				if seen[id] {
					continue
				}
				seen[id] = true
				if result.Coverage.Documents >= sourceMaxDocuments {
					sourceTruncate(result, "document_limit")
					return nil
				}
				item, ev, e := t.sourceReadDocument(ctx, ss, userID, node, key, hit, result)
				if e != nil {
					return e
				}
				result.Items = append(result.Items, item)
				result.Evidence = append(result.Evidence, ev)
				result.ItemEvidence = append(result.ItemEvidence, []CanvasWorkflowEvidence{ev})
				result.Coverage.Documents++
			}
			if page.Next == nil {
				break
			}
			offset = *page.Next
		}
	}
	return nil
}

func (t Tools) sourceReadDocument(ctx context.Context, ss SubjectScope, userID string, node map[string]any, key string, hit sourceHit, result *CanvasWorkflowSourceResult) (map[string]any, CanvasWorkflowEvidence, error) {
	var zero CanvasWorkflowEvidence
	offset := 0
	total := -1
	var text strings.Builder
	title, link := "", ""
	truncated := false
	for page := 0; page < sourceMaxReadPages; page++ {
		var read struct {
			DocumentID string `json:"document_id"`
			Revision   int    `json:"revision"`
			Offset     int    `json:"offset"`
			Text       string `json:"text"`
			Title      string `json:"title"`
			URL        string `json:"url"`
			Coverage   *struct {
				Total     int  `json:"total_chars"`
				Returned  int  `json:"returned_chars"`
				Truncated bool `json:"truncated"`
				Next      *int `json:"next_offset"`
			} `json:"coverage"`
		}
		e := t.sourceHTTP(ctx, ss, userID, http.MethodPost, "/v1/documents/read", "", map[string]any{"document_id": hit.DocumentID, "revision": hit.Revision, "offset": offset, "max_chars": 20000}, &read)
		if e != nil {
			return nil, zero, e
		}
		result.Coverage.ReadPages++
		c := read.Coverage
		if c == nil || read.DocumentID != hit.DocumentID || read.Revision != hit.Revision || read.Offset != offset || c.Returned != sourceUTF16Len(read.Text) || c.Returned > 20000 || c.Total < offset+c.Returned || (total >= 0 && c.Total != total) {
			return nil, zero, sourceError("invalid_document_page")
		}
		if c.Truncated != (offset+c.Returned < c.Total) || (c.Truncated && (c.Next == nil || *c.Next != offset+c.Returned || c.Returned == 0)) || (!c.Truncated && c.Next != nil) {
			return nil, zero, sourceError("invalid_document_cursor")
		}
		total = c.Total
		text.WriteString(read.Text)
		title = read.Title
		link = read.URL
		truncated = c.Truncated
		if !c.Truncated {
			break
		}
		offset = *c.Next
	}
	if truncated {
		sourceTruncate(result, "document_read_limit")
	}
	ev, e := t.sourceDocumentEvidence(ctx, ss, userID, node, key, hit, text.String())
	if e != nil {
		return nil, zero, e
	}
	return map[string]any{"document_id": hit.DocumentID, "revision": hit.Revision, "source_id": hit.SourceID, "title": title, "url": link, "text": text.String(), "coverage": map[string]any{"total_chars": total, "returned_chars": sourceUTF16Len(text.String()), "truncated": truncated}, "evidence": []CanvasWorkflowEvidence{ev}}, ev, nil
}
func sourceUTF16Len(s string) int { return len(utf16.Encode([]rune(s))) }
func sourceQuote(s string) string {
	n := 0
	var b strings.Builder
	for _, r := range s {
		size := 1
		if r > 0xffff {
			size = 2
		}
		if n+size > 1000 {
			break
		}
		b.WriteRune(r)
		n += size
	}
	return b.String()
}
func sourceExecutionKey(parts ...string) string {
	raw, _ := json.Marshal(parts)
	return fmt.Sprintf("canvas-source:%x", sha256.Sum256(raw))
}

func (t Tools) sourceDocumentEvidence(ctx context.Context, ss SubjectScope, userID string, node map[string]any, key string, hit sourceHit, text string) (CanvasWorkflowEvidence, error) {
	quote := sourceQuote(text)
	end := sourceUTF16Len(quote)
	idem := sourceExecutionKey(ss.Scope.TenantID, ss.Scope.WorkspaceID, userID, t.RunID, key, sourceString(node, "id"), hit.DocumentID, strconv.Itoa(hit.Revision), quote)
	var created struct {
		Created *int `json:"created"`
		Items   []struct {
			EvidenceID  string `json:"evidence_id"`
			TenantID    string `json:"tenant_id"`
			WorkspaceID string `json:"workspace_id"`
			DocumentID  string `json:"document_id"`
			Revision    int    `json:"revision"`
			Key         string `json:"idempotency_key"`
			Start       int    `json:"char_start"`
			End         int    `json:"char_end"`
			Quote       string `json:"quote"`
		} `json:"items"`
	}
	e := t.sourceHTTP(ctx, ss, userID, http.MethodPost, "/v1/evidence", idem, map[string]any{"created_by": userID, "items": []any{map[string]any{"document_id": hit.DocumentID, "revision": hit.Revision, "char_start": 0, "char_end": end, "quote": quote}}}, &created)
	if e != nil {
		return CanvasWorkflowEvidence{}, e
	}
	// The create response is the authoritative receipt: consume it directly
	// instead of scanning the (capped, newest-first) evidence list, which can
	// miss the fresh row once a document accumulates more rows than the cap.
	if created.Created == nil || *created.Created < 0 || *created.Created > 1 || len(created.Items) != 1 {
		return CanvasWorkflowEvidence{}, sourceError("invalid_evidence_receipt")
	}
	row := created.Items[0]
	if row.EvidenceID == "" || row.Key != idem+":0" || row.TenantID != ss.Scope.TenantID || row.WorkspaceID != ss.Scope.WorkspaceID || row.DocumentID != hit.DocumentID || row.Revision != hit.Revision || row.Start != 0 || row.End != end || row.Quote != quote {
		return CanvasWorkflowEvidence{}, sourceError("evidence_mismatch")
	}
	refs, e := t.verifyHandoffEvidence(ctx, ss, userID, []HandoffCitation{{DocumentID: hit.DocumentID, EvidenceID: row.EvidenceID, Revision: hit.Revision}})
	if e != nil {
		return CanvasWorkflowEvidence{}, e
	}
	if len(refs) != 1 || refs[0].Quote != quote || refs[0].CharStart != 0 || refs[0].CharEnd != end {
		return CanvasWorkflowEvidence{}, sourceError("evidence_mismatch")
	}
	return refs[0], nil
}

const sourceMaxMonitorHits = 500

type monitorHitRow struct {
	HitID          string `json:"hit_id"`
	MonitorID      string `json:"monitor_id"`
	MonitorVersion int64  `json:"monitor_version"`
	ScheduleID     string `json:"schedule_id"`
	OperationID    string `json:"operation_id"`
	JobID          string `json:"job_id"`
	SourceID       string `json:"source_id"`
	DocumentID     string `json:"document_id"`
	Revision       int    `json:"revision"`
	MatchedAt      string `json:"matched_at"`
	ScheduledAt    string `json:"scheduled_at"`
}

type monitorHitsPage struct {
	Items      []monitorHitRow `json:"items"`
	NextOffset *int            `json:"next_offset"`
}

// sourceMonitorHits returns only the monitor's actually recorded hits from the
// engine's scoped, paged API; a monitor's configuration or preview is never a
// hit list. Items cite evidence already recorded against the hit document
// revision; revisions without evidence stay citation-empty instead of
// fabricating references.
func (t Tools) sourceMonitorHits(ctx context.Context, ss SubjectScope, userID, monitorID string, result *CanvasWorkflowSourceResult) error {
	evidenceByRevision := map[string][]CanvasWorkflowEvidence{}
	evidenceSeen := map[string]bool{}
	documents := map[string]bool{}
	var follow func(offset int) error
	follow = func(offset int) error {
		path := "/v1/monitors/" + url.PathEscape(monitorID) + "/hits?limit=50&offset=" + strconv.Itoa(offset)
		if result.Coverage.Window != nil {
			path += "&from=" + url.QueryEscape(result.Coverage.Window.From) + "&to=" + url.QueryEscape(result.Coverage.Window.To)
		}
		var page monitorHitsPage
		if e := t.sourceHTTP(ctx, ss, userID, http.MethodGet, path, "", nil, &page); e != nil {
			return e
		}
		for _, hit := range page.Items {
			if len(result.Items) >= sourceMaxMonitorHits {
				sourceTruncate(result, "monitor_hits_truncated")
				return nil
			}
			documents[hit.DocumentID] = true
			cacheKey := hit.DocumentID + "\x00" + strconv.Itoa(hit.Revision)
			evidence, cached := evidenceByRevision[cacheKey]
			if !cached {
				evidence = t.monitorRevisionEvidence(ctx, ss, userID, hit)
				evidenceByRevision[cacheKey] = evidence
				for _, item := range evidence {
					if !evidenceSeen[item.EvidenceID] {
						evidenceSeen[item.EvidenceID] = true
						result.Evidence = append(result.Evidence, item)
					}
				}
			}
			citations := []any{}
			for _, item := range evidence {
				citations = append(citations, map[string]any{"evidence_id": item.EvidenceID, "revision": item.Revision, "chunk_index": item.ChunkIndex})
			}
			result.Items = append(result.Items, map[string]any{
				"hit_id": hit.HitID, "monitor_id": hit.MonitorID, "monitor_version": hit.MonitorVersion, "schedule_id": hit.ScheduleID,
				"operation_id": hit.OperationID, "job_id": hit.JobID, "source_id": hit.SourceID, "document_id": hit.DocumentID,
				"revision": hit.Revision, "matched_at": hit.MatchedAt, "scheduled_at": hit.ScheduledAt, "citations": citations,
			})
			result.ItemEvidence = append(result.ItemEvidence, evidence)
		}
		if page.NextOffset != nil && *page.NextOffset > offset {
			return follow(*page.NextOffset)
		}
		return nil
	}
	if e := follow(0); e != nil {
		return e
	}
	result.Coverage.Documents = len(documents)
	return nil
}

func (t Tools) monitorRevisionEvidence(ctx context.Context, ss SubjectScope, userID string, hit monitorHitRow) []CanvasWorkflowEvidence {
	var revision struct {
		Evidence []struct {
			ID        string `json:"id"`
			CharStart int    `json:"char_start"`
			CharEnd   int    `json:"char_end"`
			Quote     string `json:"quote"`
		} `json:"evidence"`
	}
	if e := t.sourceHTTP(ctx, ss, userID, http.MethodGet, "/v1/documents/"+url.PathEscape(hit.DocumentID)+"/revisions/"+strconv.Itoa(hit.Revision), "", nil, &revision); e != nil {
		return []CanvasWorkflowEvidence{}
	}
	out := []CanvasWorkflowEvidence{}
	for _, row := range revision.Evidence {
		if row.ID == "" {
			continue
		}
		out = append(out, CanvasWorkflowEvidence{ResolvedEvidence: ResolvedEvidence{EvidenceID: row.ID, DocumentID: hit.DocumentID, SourceID: hit.SourceID, Revision: hit.Revision, Quote: row.Quote}, TenantID: ss.Scope.TenantID, WorkspaceID: ss.Scope.WorkspaceID, CharStart: row.CharStart, CharEnd: row.CharEnd})
	}
	return out
}

func sourceValidateSchema(schema, value any) error {
	raw, e := json.Marshal(schema)
	if e != nil {
		return sourceError("invalid_mcp_schema")
	}
	var s jsonschema.Schema
	if json.Unmarshal(raw, &s) != nil {
		return sourceError("invalid_mcp_schema")
	}
	resolved, e := s.Resolve(nil)
	if e != nil {
		return sourceError("invalid_mcp_schema")
	}
	raw, e = json.Marshal(value)
	if e != nil {
		return sourceError("invalid_mcp_value")
	}
	var normalized any
	if json.Unmarshal(raw, &normalized) != nil || resolved.Validate(normalized) != nil {
		return sourceError("mcp_schema_validation_failed")
	}
	return nil
}
func (t Tools) sourceMCP(ctx context.Context, ss SubjectScope, userID string, node map[string]any, key string, b map[string]any, result *CanvasWorkflowSourceResult) error {
	if t.MCP == nil || t.MCP.DB == nil {
		return sourceError("mcp_unavailable")
	}
	if !hasScope(ss.Scope.Role, "intel.mcp.use") {
		return sourceError("mcp_forbidden")
	}
	id, tool := sourceString(b, "refId"), sourceString(b, "toolName")
	args, ok := b["toolArgs"].(map[string]any)
	if id == "" || tool == "" || !ok || b["outputSchema"] == nil {
		return sourceError("invalid_mcp_binding")
	}
	if w := result.Coverage.Window; w != nil {
		fields, ok := b["windowArguments"].(map[string]any)
		from, to := sourceString(fields, "from"), sourceString(fields, "to")
		if !ok || from == "" || to == "" || from == to {
			return sourceError("mcp_window_mapping_required")
		}
		copyArgs := make(map[string]any, len(args)+2)
		for k, v := range args {
			copyArgs[k] = v
		}
		if (args[from] != nil && args[from] != w.From) || (args[to] != nil && args[to] != w.To) {
			return sourceError("mcp_window_argument_conflict")
		}
		copyArgs[from], copyArgs[to] = w.From, w.To
		args = copyArgs
	}
	var version int
	if decodeInto(b["toolVersion"], &version) != nil || version <= 0 || sourceString(b, "schemaHash") == "" {
		return sourceError("mcp_schema_pin_required")
	}
	check := func() error {
		conn, e := t.MCP.get(ss.Scope, id)
		if e != nil || conn.Status != "approved" {
			return sourceError("mcp_not_approved")
		}
		view := t.MCP.View(conn)
		ap, ok := view.Approved[tool]
		allowed := false
		for _, name := range view.AllowedTools {
			if name == tool {
				allowed = true
			}
		}
		if !ok || !allowed {
			return sourceError("mcp_tool_denied")
		}
		if conn.Version != version || ap.Hash != sourceString(b, "schemaHash") {
			return sourceError("mcp_schema_drift")
		}
		return sourceValidateSchema(ap.InputSchema, args)
	}
	if e := check(); e != nil {
		return e
	}
	idem := sourceExecutionKey(ss.Scope.TenantID, ss.Scope.WorkspaceID, userID, t.RunID, key, sourceString(node, "id"), id, tool)
	out, truncated, e := t.MCP.ExecuteKeyed(ctx, ss.Scope, userID, id, tool, args, idem)
	if e != nil {
		return sourceError("mcp_execution_failed")
	}
	// ExecuteKeyed can replay without checking the connection; check on both sides.
	if e = check(); e != nil {
		return e
	}
	if truncated {
		sourceTruncate(result, "mcp_output_limit")
		return sourceError("mcp_output_truncated")
	}
	var value any
	if json.Unmarshal([]byte(out), &value) != nil {
		return sourceError("invalid_mcp_output")
	}
	if e = sourceValidateSchema(b["outputSchema"], value); e != nil {
		return e
	}
	result.Items, e = sourceStrictItems(out, "json")
	if e != nil {
		return e
	}
	for range result.Items {
		result.ItemEvidence = append(result.ItemEvidence, []CanvasWorkflowEvidence{})
	}
	return nil
}
