package flow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"yingce/backend/internal/model"
)

var sourceNow = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func sourceSetup(t *testing.T) (Tools, SubjectScope) {
	t.Helper()
	db := testDB(t)
	sc, err := ResolveScope(db, "u_alice", "")
	if err != nil {
		t.Fatal(err)
	}
	return Tools{DB: db, RunID: "run-source"}, SubjectScope{Scope: sc, Subject: AgentSubject{Kind: SubjectWorkspace, ID: sc.WorkspaceID}}
}

func sourceNode(kind, format, raw string) map[string]any {
	return map[string]any{"id": "n1", "type": "data-source", "metadata": map[string]any{"content": "STALE CACHE", "dataSource": map[string]any{"format": format, "rawText": raw, "sourceBinding": map[string]any{"kind": kind, "lastSyncedAt": "old"}}}}
}
func sourceDS(n map[string]any) map[string]any {
	return n["metadata"].(map[string]any)["dataSource"].(map[string]any)
}
func sourceBinding(n map[string]any) map[string]any {
	return sourceDS(n)["sourceBinding"].(map[string]any)
}
func sourceResolve(t *testing.T, tools Tools, ss SubjectScope, n map[string]any) (CanvasWorkflowSourceResult, error) {
	t.Helper()
	before, _ := json.Marshal(n)
	out, err := tools.ResolveCanvasWorkflowSource(context.Background(), ss, "u_alice", n, "exec-source", sourceNow)
	after, _ := json.Marshal(n)
	if string(before) != string(after) {
		t.Fatal("resolver mutated input, including sync time")
	}
	if err != nil && (out.LastSyncedAt != "" || len(out.Errors) == 0 || len(out.Items) != 0 || len(out.Evidence) != 0) {
		t.Fatalf("failure published data/time or lacks machine error: %+v %v", out, err)
	}
	return out, err
}

func TestCanvasWorkflowSourceStaticStrictRawOnly(t *testing.T) {
	tools, ss := sourceSetup(t)
	for _, tc := range []struct {
		name, format, raw string
		want              int
		fail              bool
	}{
		{"json", "json", `[{"v":1}]`, 1, false}, {"empty", "json", "", 0, false},
		{"invalid-json", "json", `[{"v":]`, 0, true}, {"scalar", "json", `42`, 0, true},
		{"csv", "csv", "name,value\na,2", 1, false}, {"ragged-csv", "csv", "a,b\nx", 0, true},
		{"duplicate-header", "csv", "a,a\nx,y", 0, true}, {"text", "txt", "original", 1, false},
		{"intel-json", "intel", `[{"v":2}]`, 1, false}, {"bad-intel-json", "intel", `[{broken`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := sourceNode("static", tc.format, tc.raw)
			out, err := sourceResolve(t, tools, ss, n)
			if (err != nil) != tc.fail || len(out.Items) != tc.want {
				t.Fatalf("out=%+v err=%v", out, err)
			}
			if !tc.fail && out.LastSyncedAt != sourceNow.Format(time.RFC3339Nano) {
				t.Fatal("missing successful snapshot time")
			}
		})
	}
}

func TestCanvasWorkflowSourceScopeAndMissingMonitorFailClosed(t *testing.T) {
	tools, ss := sourceSetup(t)
	n := sourceNode("monitor", "json", `[{"stale":true}]`)
	sourceBinding(n)["refId"] = "monitor1"
	out, err := sourceResolve(t, tools, ss, n)
	if err == nil || out.Errors[0].Reason != "monitor_hits_unavailable" {
		t.Fatalf("%+v %v", out, err)
	}
	n = sourceNode("static", "txt", "raw")
	ss.Scope.TenantID = "other"
	if _, err = sourceResolve(t, tools, ss, n); err == nil {
		t.Fatal("cross-tenant scope accepted")
	}
	_, ss = sourceSetupForExisting(t, tools)
	tools.DB.Where("workspace_id = ? AND user_id = ?", ss.Scope.WorkspaceID, "u_alice").Delete(&model.WorkspaceMembership{})
	if _, err = sourceResolve(t, tools, ss, n); err == nil {
		t.Fatal("revoked membership accepted")
	}
}
func sourceSetupForExisting(t *testing.T, tools Tools) (Tools, SubjectScope) {
	sc, err := ResolveScope(tools.DB, "u_alice", "")
	if err != nil {
		t.Fatal(err)
	}
	return tools, SubjectScope{Scope: sc}
}

// This server implements the actual Loom envelope, offset pagination, UTF-16
// read coverage, evidence create/list/resolve protocols, not an EngineCaller stub.
func sourceEngine(t *testing.T, ss SubjectScope, handle func(*http.Request, map[string]any) (int, any)) EngineClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := Verify(key, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), "loom-engine", time.Now())
		if err != nil || claims.TenantID != ss.Scope.TenantID || claims.WorkspaceID != ss.Scope.WorkspaceID || claims.Actor.ID != "u_alice" {
			t.Errorf("bad scoped token: %+v %v", claims, err)
			w.WriteHeader(403)
			return
		}
		var body map[string]any
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
		}
		code, data := handle(r, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data, "msg": "ok"})
	}))
	t.Cleanup(srv.Close)
	return EngineClient{Cfg: Config{EngineURL: srv.URL, SigningKey: key, Issuer: "flow-platform", Audience: "loom-engine", TTL: time.Minute}, HTTP: srv.Client()}
}

func TestCanvasWorkflowSourceIntelPagesRevisionAndEvidence(t *testing.T) {
	tools, ss := sourceSetup(t)
	searches, reads := 0, 0
	var evidenceKey string
	tools.Engine = sourceEngine(t, ss, func(r *http.Request, b map[string]any) (int, any) {
		switch r.URL.Path {
		case "/v1/documents/search":
			searches++
			if b["query"] != "market" || b["from"] != "2026-10-18T12:00:00Z" || b["to"] != "2026-10-25T12:00:00Z" || b["vector"] != false {
				t.Errorf("wrong query: %#v", b)
			}
			if searches == 1 {
				return 200, map[string]any{"items": []any{map[string]any{"document_id": "d1", "revision": 3, "source_id": "s1"}}, "next_offset": 50}
			}
			if b["offset"] != float64(50) {
				t.Errorf("offset not advanced: %#v", b)
			}
			return 200, map[string]any{"items": []any{}, "next_offset": nil}
		case "/v1/documents/read":
			reads++
			if b["revision"] != float64(3) {
				t.Error("revision not pinned")
			}
			offset := int(b["offset"].(float64))
			text := "A😀"
			trunc := true
			var next any = 3
			if offset == 3 {
				text = "B"
				trunc = false
				next = nil
			}
			return 200, map[string]any{"document_id": "d1", "revision": 3, "offset": offset, "text": text, "title": "Doc", "coverage": map[string]any{"total_chars": 4, "returned_chars": lenUTF16ForTest(text), "truncated": trunc, "next_offset": next}}
		case "/v1/evidence":
			if r.Method == http.MethodPost {
				evidenceKey = r.Header.Get("X-Idempotency-Key")
				if evidenceKey == "" {
					t.Error("no evidence idempotency key")
				}
				return 201, map[string]any{"created": 1, "items": []any{map[string]any{"evidence_id": "ev-real", "idempotency_key": evidenceKey + ":0", "char_start": 0, "char_end": 4, "quote": "A😀B", "tenant_id": ss.Scope.TenantID, "workspace_id": ss.Scope.WorkspaceID, "document_id": "d1", "revision": 3}}}
			}
			t.Errorf("evidence list scan must not be used: %s %s", r.Method, r.URL)
			return 404, nil
		case "/v1/evidence/resolve":
			return 200, []any{map[string]any{"evidence_id": "ev-real", "document_id": "d1", "revision": 3, "source_id": "s1", "quote": "A😀B", "snippet_ok": true, "url_ok": true, "char_start": 0, "char_end": 4}}
		default:
			t.Errorf("unexpected protocol %s %s", r.Method, r.URL)
			return 404, nil
		}
	})
	n := sourceNode("intel_query", "intel", "stale")
	sourceBinding(n)["query"] = "market"
	sourceBinding(n)["timeWindow"] = map[string]any{"kind": "7d", "timezone": "UTC"}
	out, err := sourceResolve(t, tools, ss, n)
	if err != nil || searches != 2 || reads != 2 || len(out.Items) != 1 || len(out.Evidence) != 1 {
		t.Fatalf("%+v %v requests=%d/%d", out, err, searches, reads)
	}
	item := out.Items[0].(map[string]any)
	if item["text"] != "A😀B" || out.Evidence[0].EvidenceID != "ev-real" || out.Evidence[0].Revision != 3 || item["evidence"] == nil {
		t.Fatalf("not real material/evidence: %+v", out)
	}
}
func lenUTF16ForTest(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}

func TestCanvasWorkflowSourceEmptyErrorsAndPaginationCap(t *testing.T) {
	for _, mode := range []string{"empty", "http-error", "bad-envelope", "repeat-offset", "cap"} {
		t.Run(mode, func(t *testing.T) {
			tools, ss := sourceSetup(t)
			calls := 0
			tools.Engine = sourceEngine(t, ss, func(r *http.Request, b map[string]any) (int, any) {
				calls++
				if mode == "http-error" {
					return 403, nil
				}
				if mode == "bad-envelope" {
					return 200, map[string]any{"unexpected": true}
				}
				var next any
				if mode == "repeat-offset" {
					next = 0
				}
				if mode == "cap" {
					next = int(b["offset"].(float64)) + 50
				}
				return 200, map[string]any{"items": []any{}, "next_offset": next}
			})
			n := sourceNode("intel_query", "intel", "stale")
			sourceBinding(n)["query"] = "q"
			out, err := sourceResolve(t, tools, ss, n)
			if mode == "empty" {
				if err != nil || len(out.Items) != 0 || out.Coverage.Truncated {
					t.Fatalf("not true empty: %+v %v", out, err)
				}
			} else if mode == "cap" {
				if err != nil || !out.Coverage.Truncated || calls > 20 || out.LastSyncedAt != "" {
					t.Fatalf("cap not disclosed: %+v %v calls=%d", out, err, calls)
				}
			} else if err == nil {
				t.Fatalf("accepted %s", mode)
			}
		})
	}
}

func TestCanvasWorkflowSourceTopicUsesConditionsNotNotes(t *testing.T) {
	tools, ss := sourceSetup(t)
	title, question, notes := "not material", "supply", "FAKE NOTES"
	keywords := []string{"price", "risk"}
	ids := []string{"s1"}
	top, err := CreateTopic(tools.DB, ss.Scope, "u_alice", TopicInput{Title: &title, Question: &question, Notes: &notes, Keywords: &keywords, SourceIDs: &ids}, sourceNow)
	if err != nil {
		t.Fatal(err)
	}
	queries := []string{}
	tools.Engine = sourceEngine(t, ss, func(r *http.Request, b map[string]any) (int, any) {
		if r.URL.Path != "/v1/documents/search" {
			t.Error("not searching materials")
		}
		queries = append(queries, b["query"].(string))
		if !reflect.DeepEqual(b["source_ids"], []any{"s1"}) {
			t.Errorf("source scope lost: %#v", b)
		}
		return 200, map[string]any{"items": []any{}, "next_offset": nil}
	})
	n := sourceNode("topic", "intel", "cached notes")
	sourceBinding(n)["refId"] = top.ID
	out, err := sourceResolve(t, tools, ss, n)
	if err != nil || len(out.Items) != 0 || !reflect.DeepEqual(queries, []string{"supply", "price", "risk"}) {
		t.Fatalf("%+v %v queries=%v", out, err, queries)
	}
	bob, _ := ResolveScope(tools.DB, "u_bob", "")
	ss.Scope = bob
	if _, err := tools.ResolveCanvasWorkflowSource(context.Background(), ss, "u_bob", n, "x", sourceNow); err == nil {
		t.Fatal("foreign topic accepted")
	}
}

func TestCanvasWorkflowSourceWindow(t *testing.T) {
	tools, ss := sourceSetup(t)
	for _, tc := range []struct {
		name     string
		w        map[string]any
		from, to string
		fail     bool
	}{
		{"30d", map[string]any{"kind": "30d", "timezone": "Asia/Shanghai"}, "2026-09-25T12:00:00Z", "2026-10-25T12:00:00Z", false},
		{"custom", map[string]any{"kind": "custom", "timezone": "Asia/Shanghai", "from": "2026-10-01T00:00:00", "to": "2026-10-02T00:00:00"}, "2026-09-30T16:00:00Z", "2026-10-01T16:00:00Z", false},
		{"reverse", map[string]any{"kind": "custom", "timezone": "UTC", "from": "2026-10-02", "to": "2026-10-01"}, "", "", true},
		{"zone", map[string]any{"kind": "7d", "timezone": "Bad/Zone"}, "", "", true},
		{"gap", map[string]any{"kind": "custom", "timezone": "America/New_York", "from": "2026-03-08T02:30:00", "to": "2026-03-09T00:00:00"}, "", "", true},
		{"fold", map[string]any{"kind": "custom", "timezone": "America/New_York", "from": "2026-11-01T01:30:00", "to": "2026-11-02T00:00:00"}, "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tools.Engine = sourceEngine(t, ss, func(_ *http.Request, b map[string]any) (int, any) {
				if b["from"] != tc.from || b["to"] != tc.to {
					t.Errorf("window: %#v", b)
				}
				return 200, map[string]any{"items": []any{}, "next_offset": nil}
			})
			n := sourceNode("intel_query", "intel", "")
			sourceBinding(n)["query"] = "q"
			sourceBinding(n)["timeWindow"] = tc.w
			_, err := sourceResolve(t, tools, ss, n)
			if (err != nil) != tc.fail {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestCanvasWorkflowSourceMCPProtocolAndRevocation(t *testing.T) {
	tools, ss := sourceSetup(t)
	calls := 0
	output := `[{"value":2}]`
	schema := map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}, "from": map[string]any{"type": "string"}, "to": map[string]any{"type": "string"}}, "required": []string{"q"}, "additionalProperties": false}
	server := mcp.NewServer(&mcp.Implementation{Name: "source-test", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "lookup", Description: "lookup", InputSchema: schema}, func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls++
		if !strings.Contains(string(r.Params.Arguments), `"q":"hello"`) {
			t.Error("arguments changed")
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: output}}}, nil
	})
	srv := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer srv.Close()
	tools.MCP = &MCPGateway{DB: tools.DB, AllowInsecure: true}
	c, err := tools.MCP.Create(ss.Scope, "u_alice", MCPConnectionInput{Name: "source", Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := tools.MCP.Approve(context.Background(), ss.Scope, c.ID, "operator", []string{"lookup"})
	if err != nil {
		t.Fatal(err)
	}
	n := sourceNode("mcp_tool", "json", "")
	b := sourceBinding(n)
	b["refId"] = c.ID
	b["toolName"] = "lookup"
	b["toolArgs"] = map[string]any{"q": "hello"}
	b["toolVersion"] = approved.Version
	b["schemaHash"] = approved.Approved["lookup"].Hash
	b["outputSchema"] = map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"value"}}}
	out, err := sourceResolve(t, tools, ss, n)
	if err != nil || len(out.Items) != 1 || calls != 1 {
		t.Fatalf("%+v %v calls=%d", out, err, calls)
	}
	if _, err = sourceResolve(t, tools, ss, n); err != nil || calls != 1 {
		t.Fatalf("not idempotent: %v calls=%d", err, calls)
	}
	b["timeWindow"] = map[string]any{"kind": "7d", "timezone": "UTC"}
	if _, err = sourceResolve(t, tools, ss, n); err == nil {
		t.Error("MCP silently ignored requested time window")
	}
	delete(b, "timeWindow")
	for _, tc := range []struct {
		name, output string
		fail         bool
	}{
		{"empty", `[]`, false}, {"bad-json", `not JSON`, true}, {"bad-schema", `[{"wrong":2}]`, true},
	} {
		output = tc.output
		n["id"] = tc.name
		result, callErr := sourceResolve(t, tools, ss, n)
		if (callErr != nil) != tc.fail || (!tc.fail && len(result.Items) != 0) {
			t.Errorf("%s: %+v %v", tc.name, result, callErr)
		}
	}
	n["id"] = "n1"
	beforeCalls := calls
	b["toolArgs"] = map[string]any{"q": 7}
	if _, err = sourceResolve(t, tools, ss, n); err == nil {
		t.Error("invalid input accepted")
	}
	b["toolArgs"] = map[string]any{"q": "hello"}
	b["toolVersion"] = approved.Version + 1
	if _, err = sourceResolve(t, tools, ss, n); err == nil {
		t.Error("wrong schema version accepted")
	}
	b["toolVersion"] = approved.Version
	if calls != beforeCalls {
		t.Fatal("invalid binding executed tool")
	}
	if err = tools.MCP.Revoke(ss.Scope, c.ID, "revoked"); err != nil {
		t.Fatal(err)
	}
	if _, err = sourceResolve(t, tools, ss, n); err == nil {
		t.Fatal("cached result bypassed current authorization")
	}
	if calls != beforeCalls {
		t.Fatal("revoked tool executed")
	}
}

func TestCanvasWorkflowSourceMonitorHitsPagedWithRevisionEvidence(t *testing.T) {
	tools, ss := sourceSetup(t)
	hitsPages := map[string]int{}
	revisionReads := map[string]int{}
	tools.Engine = sourceEngine(t, ss, func(r *http.Request, b map[string]any) (int, any) {
		switch {
		case r.URL.Path == "/v1/monitors/mon1/hits":
			offset := r.URL.Query().Get("offset")
			hitsPages[offset]++
			if r.URL.Query().Get("from") == "" || r.URL.Query().Get("to") == "" {
				t.Error("hit query missing bound window")
			}
			if offset == "0" {
				return 200, map[string]any{"items": []any{
					map[string]any{"hit_id": "hit-1", "monitor_id": "mon1", "monitor_version": 3, "schedule_id": "sc1", "operation_id": "op1", "job_id": "job1", "source_id": "s1", "document_id": "d1", "revision": 3, "matched_at": "2026-10-20T00:00:00Z", "scheduled_at": "2026-10-20T00:00:00Z"},
					map[string]any{"hit_id": "hit-2", "monitor_id": "mon1", "monitor_version": 3, "schedule_id": "sc1", "operation_id": "op1", "job_id": "job2", "source_id": "s1", "document_id": "d2", "revision": 1, "matched_at": "2026-10-21T00:00:00Z", "scheduled_at": "2026-10-21T00:00:00Z"},
					map[string]any{"hit_id": "hit-3", "monitor_id": "mon1", "monitor_version": 3, "schedule_id": "sc1", "operation_id": "op1", "job_id": "job3", "source_id": "s1", "document_id": "d1", "revision": 3, "matched_at": "2026-10-22T00:00:00Z", "scheduled_at": "2026-10-22T00:00:00Z"},
				}, "next_offset": 50}
			}
			if offset == "50" {
				return 200, map[string]any{"items": []any{}, "next_offset": nil}
			}
			t.Errorf("unexpected hit offset %s", offset)
			return 400, nil
		case r.URL.Path == "/v1/documents/d1/revisions/3":
			revisionReads["d1:3"]++
			return 200, map[string]any{"document_id": "d1", "revision": 3, "evidence": []any{map[string]any{"id": "ev-hit", "char_start": 0, "char_end": 4, "quote": "hit quote"}}}
		case r.URL.Path == "/v1/documents/d2/revisions/1":
			revisionReads["d2:1"]++
			return 200, map[string]any{"document_id": "d2", "revision": 1, "evidence": []any{}}
		default:
			t.Errorf("unexpected protocol %s %s", r.Method, r.URL)
			return 404, nil
		}
	})
	n := sourceNode("monitor", "json", `[{"stale":true}]`)
	sourceBinding(n)["refId"] = "mon1"
	sourceBinding(n)["timeWindow"] = map[string]any{"kind": "7d", "timezone": "UTC"}
	out, err := sourceResolve(t, tools, ss, n)
	if err != nil {
		t.Fatal(err)
	}
	if len(hitsPages) != 2 || hitsPages["0"] != 1 || hitsPages["50"] != 1 {
		t.Fatalf("pagination not followed: %v", hitsPages)
	}
	if revisionReads["d1:3"] != 1 || revisionReads["d2:1"] != 1 {
		t.Fatalf("revision evidence not deduped per (document,revision): %v", revisionReads)
	}
	if len(out.Items) != 3 || out.Coverage.Documents != 2 {
		t.Fatalf("items/documents: %+v", out)
	}
	first := out.Items[0].(map[string]any)
	if first["hit_id"] != "hit-1" || first["document_id"] != "d1" || first["revision"] != 3 || first["matched_at"] == "" {
		t.Fatalf("hit record fields lost: %+v", first)
	}
	citations, _ := first["citations"].([]any)
	if len(citations) != 1 || citations[0].(map[string]any)["evidence_id"] != "ev-hit" {
		t.Fatalf("citations not from revision evidence: %+v", first["citations"])
	}
	second := out.Items[1].(map[string]any)
	if second["hit_id"] != "hit-2" || len(second["citations"].([]any)) != 0 {
		t.Fatalf("no-evidence revision must stay citation-empty: %+v", second)
	}
	if len(out.ItemEvidence) != 3 || len(out.ItemEvidence[0]) != 1 || len(out.ItemEvidence[1]) != 0 {
		t.Fatalf("item evidence not aligned: %+v", out.ItemEvidence)
	}
	if out.Coverage.Truncated || out.LastSyncedAt == "" {
		t.Fatalf("complete sync must stamp time: %+v", out.Coverage)
	}
}

func TestCanvasWorkflowSourceMonitorZeroHitsAndMissingMonitor(t *testing.T) {
	tools, ss := sourceSetup(t)
	tools.Engine = sourceEngine(t, ss, func(r *http.Request, b map[string]any) (int, any) {
		if r.URL.Path == "/v1/monitors/mon-empty/hits" {
			return 200, map[string]any{"items": []any{}, "next_offset": nil}
		}
		if r.URL.Path == "/v1/monitors/mon-gone/hits" {
			return 404, nil
		}
		t.Errorf("unexpected protocol %s %s", r.Method, r.URL)
		return 404, nil
	})
	n := sourceNode("monitor", "json", `[{"stale":true}]`)
	sourceBinding(n)["refId"] = "mon-empty"
	out, err := sourceResolve(t, tools, ss, n)
	if err != nil || len(out.Items) != 0 || out.LastSyncedAt == "" || out.Coverage.Truncated {
		t.Fatalf("zero hits must be an empty success: %+v %v", out, err)
	}
	sourceBinding(n)["refId"] = "mon-gone"
	if _, err = sourceResolve(t, tools, ss, n); err == nil || err.(CanvasWorkflowSourceError).Reason != "source_forbidden" {
		t.Fatalf("missing monitor accepted: %v", err)
	}
	sourceBinding(n)["refId"] = ""
	if _, err = sourceResolve(t, tools, ss, n); err == nil || err.(CanvasWorkflowSourceError).Reason != "monitor_id_required" {
		t.Fatalf("missing monitor id accepted: %v", err)
	}
}
