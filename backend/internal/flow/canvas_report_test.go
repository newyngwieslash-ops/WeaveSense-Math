package flow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"yingce/backend/internal/model"
)

func t16Payload() map[string]any {
	return map[string]any{"title": "Industry", "body": "# Evidence report\nFull body", "claims": []any{claim("source_statement", cite("ev_ok", 1))}, "material_scope": map[string]any{"source_ids": []any{"src_1"}}, "coverage": map[string]any{"regions": []any{"A"}, "truncated": true}, "structured": map[string]any{"regional_metrics": []any{map[string]any{"region": "A", "citations": []any{cite("ev_ok", 1)}, "excretion_factor_source": map[string]any{"version": "v1", "citations": []any{cite("ev_trunc", 1)}}}}}}
}

func TestT16ReportAdapterHTTPRetryAndSQLiteReplay(t *testing.T) {
	tools, _, conv, ss := setupP3(t)
	calls := 0
	var bodies [][]byte
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/evidence/resolve" {
			json.NewEncoder(w).Encode(map[string]any{"data": []ResolvedEvidence{{EvidenceID: "ev_ok", DocumentID: "d1", SourceID: "src_1", Revision: 1}, {EvidenceID: "ev_trunc", DocumentID: "d1", SourceID: "src_1", Revision: 1, Truncated: true}}})
			return
		}
		if r.URL.Path == "/v1/reports/by-key" {
			w.WriteHeader(404)
			io.WriteString(w, `{"msg":"not found"}`)
			return
		}
		if r.URL.Path != "/v1/reports" {
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"data":[]}`)
			return
		}
		calls++
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, b)
		keys = append(keys, r.Header.Get("X-Idempotency-Key"))
		if calls == 1 {
			w.WriteHeader(503)
			io.WriteString(w, `{"msg":"storage unavailable"}`)
			return
		}
		io.WriteString(w, `{"data":{"report_id":"rpt_real"}}`)
	}))
	defer srv.Close()
	tools.Engine = EngineClient{Cfg: Config{EngineURL: srv.URL, SigningKey: key, Issuer: "flow-platform", Audience: "loom-engine", TTL: MaxTokenTTL}}
	saver := CanvasReportSaver{Tools: tools}
	p := t16Payload()
	a, err := saver.Save(context.Background(), conv, ss, "u_alice", "run_16", "markdown", p)
	if err == nil || a.Status != "partial" || !strings.Contains(a.Error, "503") {
		t.Fatalf("first save must be partial with real failure: %+v %v", a, err)
	}
	b, err := saver.Save(context.Background(), conv, ss, "u_alice", "run_16", "markdown", p)
	if err != nil || b.ReportID != "rpt_real" || b.Status != "succeeded" {
		t.Fatalf("save-only retry: %+v %v", b, err)
	}
	// Recreate the adapter: idempotency lives in SQLite tool calls, not memory.
	c, err := (CanvasReportSaver{Tools: tools}).Save(context.Background(), conv, ss, "u_alice", "run_16", "markdown", p)
	if err != nil || c.ReportID != b.ReportID || calls != 2 {
		t.Fatalf("persisted replay: %+v %v calls=%d", c, err, calls)
	}
	if keys[0] == "" || keys[0] != keys[1] || !reflect.DeepEqual(bodies[0], bodies[1]) {
		t.Fatal("retry changed frozen request")
	}
	var sent map[string]any
	json.Unmarshal(bodies[1], &sent)
	cov := sent["coverage"].(map[string]any)
	if sent["structured"] == nil || cov["regions"] == nil || cov["truncated"] != true || cov["truncated_citations"] == nil {
		t.Fatalf("lost structured data or coverage: %s", bodies[1])
	}
	p["body"] = "changed"
	_, err = saver.Save(context.Background(), conv, ss, "u_alice", "run_16", "markdown", p)
	if !errors.Is(err, ErrToolConflict) {
		t.Fatalf("same key different payload: %v", err)
	}
	var n int64
	tools.DB.Model(&model.FlowAgentToolCall{}).Count(&n)
	if n != 1 {
		t.Fatalf("expected one persistent tool call, got %d", n)
	}
}

func TestT16ReportPinsMultipleSkills(t *testing.T) {
 tools,fe,conv,ss:=setupP3(t)
 for _,id:=range []string{"periodic-brief","storage-market-brief"} {if _,err:=InstallSkill(tools.DB,ss.Scope,"u_alice",id,"");err!=nil {t.Fatal(err)}}
 p:=t16Payload();p["skill_ids"]=[]any{"periodic-brief","storage-market-brief"}
 result,err:=(CanvasReportSaver{Tools:tools}).Save(context.Background(),conv,ss,"u_alice","run-multi","md",p)
 if err!=nil || result.Status!="succeeded"{t.Fatalf("multi skill save: %+v %v",result,err)}
 refs:=fe.reports[0]["skill"].(map[string]any)["refs"].([]any)
 if len(refs)!=2 {t.Fatalf("lost pinned skill refs: %+v",refs)}
}

func TestT16RejectsNestedAndRowCitations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"factor revision", func(p map[string]any) {
			p["structured"] = map[string]any{"factor_source": map[string]any{"version": "v1", "citations": []any{cite("ev_ok", 9)}}}
		}},
		{"row no evidence", func(p map[string]any) {
			p["structured"] = map[string]any{"regional_metrics": []any{map[string]any{"region": "A", "value": 2}}}
		}},
		{"factor no evidence", func(p map[string]any) {
			p["structured"] = map[string]any{"factor_source": map[string]any{"version": "v1"}}
		}},
		{"empty claims", func(p map[string]any) { p["claims"] = []any{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tools, fe, conv, ss := setupP3(t)
			p := t16Payload()
			tc.mutate(p)
			r, e := (CanvasReportSaver{Tools: tools}).Save(context.Background(), conv, ss, "u_alice", "run", "md", p)
			if e == nil || r.Status != "partial" || len(fe.reports) != 0 {
				t.Fatalf("must reject without save: %+v %v", r, e)
			}
		})
	}
}
