package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"yingce/backend/internal/model"
)

func TestT03ReviewContracts(t *testing.T) {
	eval := func(cfg map[string]any, a, b []any) wfEvalResult {
		return wfEvalLogicNode(map[string]any{"metadata": map[string]any{"logic": cfg}}, a, b)
	}
	t.Run("count rows even field missing", func(t *testing.T) {
		r := eval(map[string]any{"operator": "aggregate", "metrics": []any{map[string]any{"field": "v", "fn": "count", "as": "n"}}}, []any{map[string]any{"v": 0}, map[string]any{"v": nil}, map[string]any{}}, nil)
		if r.errText != "" || r.outputItems[0].(map[string]any)["n"] != 3 {
			t.Fatalf("count: %+v", r)
		}
	})
	t.Run("nested path collisions", func(t *testing.T) {
		r := eval(map[string]any{"operator": "join", "joinKeys": []any{map[string]any{"left": "id", "right": "id"}}}, []any{map[string]any{"id": 1, "v": 23, "right": map[string]any{"v": 10}}}, []any{map[string]any{"id": 1, "v": 27.6}})
		if r.errText != "" {
			t.Fatal(r.errText)
		}
		row := r.outputItems[0]
		if wfReadField(row, "right.v") != 10 || wfReadField(row, "right.right.v") != 27.6 {
			t.Fatalf("shadowed nested path: %+v", row)
		}
	})
	t.Run("unicode codepoint sort", func(t *testing.T) {
		r := eval(map[string]any{"operator": "sort_limit", "sortField": "name", "dedupeKeys": []any{"id"}, "limit": 1}, []any{map[string]any{"id": 1, "name": ""}, map[string]any{"id": 1, "name": "𐀀"}}, nil)
		if r.errText != "" || r.outputItems[0].(map[string]any)["name"] != "𐀀" {
			t.Fatalf("unicode order: %+v", r)
		}
	})
	t.Run("canonical evidence ignores null", func(t *testing.T) {
		r := eval(map[string]any{"operator": "aggregate"}, []any{map[string]any{"citations": []any{map[string]any{"evidence_id": "e", "revision": 1}, nil}}, map[string]any{"citations": []any{nil, map[string]any{"revision": 1, "evidence_id": "e"}}}, map[string]any{"citations": nil}}, nil)
		if r.errText != "" {
			t.Fatal(r.errText)
		}
		data, _ := json.Marshal(r.outputItems[0].(map[string]any)["citations"])
		if string(data) != `[{"evidence_id":"e","revision":1}]` {
			t.Fatalf("evidence: %s", data)
		}
	})
}

func TestT03T04Contracts(t *testing.T) {
	eval := func(logic map[string]any, a, b []any) wfEvalResult {
		return wfEvalLogicNode(map[string]any{"metadata": map[string]any{"logic": logic}}, a, b)
	}
	metric := func(field, fn, alias string) map[string]any {
		return map[string]any{"field": field, "fn": fn, "as": alias}
	}
	metrics := []any{metric("", "count", "count"), metric("v", "sum", "sum"), metric("v", "avg", "avg"), metric("v", "min", "min"), metric("v", "max", "max"), metric("v", "p95", "p95")}
	assert := func(t *testing.T, r wfEvalResult, want string) {
		t.Helper()
		if r.errText != "" {
			t.Fatalf("unexpected error: %s", r.errText)
		}
		got, err := json.Marshal(r.outputItems)
		if err != nil {
			t.Fatal(err)
		}
		var expected any
		if err := json.Unmarshal([]byte(want), &expected); err != nil {
			t.Fatal(err)
		}
		normalized, _ := json.Marshal(expected)
		if string(got) != string(normalized) {
			t.Fatalf("got %s want %s", got, normalized)
		}
	}
	t.Run("multi metrics exact values and missing", func(t *testing.T) {
		cfg := map[string]any{"operator": "aggregate", "groupBy": []any{}, "metrics": metrics}
		assert(t, eval(cfg, []any{map[string]any{"v": 23}, map[string]any{"v": 27.6}, map[string]any{"v": nil}, map[string]any{"v": ""}, map[string]any{}}, nil), `[{"count":5,"sum":50.6,"avg":25.3,"min":23,"max":27.6,"p95":27.6}]`)
		assert(t, eval(cfg, nil, nil), `[{"count":0,"sum":null,"avg":null,"min":null,"max":null,"p95":null}]`)
		cfg["groupBy"] = []any{"region"}
		assert(t, eval(cfg, nil, nil), `[]`)
		cfg["groupBy"] = []any{}
		assert(t, eval(cfg, []any{map[string]any{"v": nil}, map[string]any{"v": " "}}, nil), `[{"count":2,"sum":null,"avg":null,"min":null,"max":null,"p95":null}]`)
	})
	t.Run("nearest rank and typed groups", func(t *testing.T) {
		rows := []any{}
		for i := 1; i <= 20; i++ {
			rows = append(rows, map[string]any{"v": i})
		}
		assert(t, eval(map[string]any{"operator": "aggregate", "metrics": []any{metric("v", "p95", "p")}}, rows, nil), `[{"p":19}]`)
		assert(t, eval(map[string]any{"operator": "aggregate", "groupBy": []any{"a"}, "metrics": []any{metric("", "count", "n")}}, []any{map[string]any{"a": 1}, map[string]any{"a": "1"}}, nil), `[{"a":1,"n":1},{"a":"1","n":1}]`)
	})
	t.Run("invalid contracts and values", func(t *testing.T) {
		for _, patch := range []map[string]any{{"groupBy": "region"}, {"groupBy": []any{""}}, {"groupBy": []any{"region", "region"}}, {"metrics": []any{}}, {"metrics": []any{metric("v", "median", "m")}}, {"metrics": []any{metric("", "sum", "s")}}, {"metrics": []any{metric("v", "sum", "region")}}, {"metrics": []any{metric("v", "sum", "citations")}}, {"metrics": []any{metric("v", "sum", "s"), metric("v", "sum", "s")}}} {
			cfg := map[string]any{"operator": "aggregate", "groupBy": []any{"region"}, "metrics": metrics}
			for k, v := range patch {
				cfg[k] = v
			}
			if r := eval(cfg, nil, nil); r.errText == "" {
				t.Fatalf("invalid accepted: %+v", cfg)
			}
		}
		for _, v := range []any{"bad", false, map[string]any{}, []any{}, math.NaN(), math.Inf(1), "1e309"} {
			r := eval(map[string]any{"operator": "aggregate", "metrics": metrics}, []any{map[string]any{"v": 23}, map[string]any{"v": v}}, nil)
			if r.errText == "" || len(r.outputItems) != 0 {
				t.Fatalf("invalid numeric accepted: %+v", r)
			}
		}
	})
	t.Run("composite join collision evidence and downstream access", func(t *testing.T) {
		cfg := map[string]any{"operator": "join", "joinMode": "left", "joinKeys": []any{map[string]any{"left": "region", "right": "region"}, map[string]any{"left": "substance", "right": "substance"}, map[string]any{"left": "period", "right": "period"}}}
		a := []any{map[string]any{"region": "A", "substance": "X", "period": "P", "v": 23, "right.v": "reserved", "citations": []any{"left"}}, map[string]any{"region": "A", "substance": "X"}}
		b := []any{map[string]any{"region": "A", "substance": "X", "period": "P", "v": 27.6, "citations": []any{"right"}, "metadata": map[string]any{"evidence": []any{"proof"}}}, map[string]any{"region": "A", "substance": "X", "period": "P", "v": 30}, map[string]any{"region": "A", "substance": "X"}}
		r := eval(cfg, a, b)
		if r.errText != "" || len(r.outputItems) != 3 {
			t.Fatalf("join: %+v", r)
		}
		m := r.outputItems[0].(map[string]any)
		if m["v"] != 23 || m["right.v"] != "reserved" || m["right.right.v"] != 27.6 {
			t.Fatalf("collision: %+v", m)
		}
		ev, _ := json.Marshal(m["citations"])
		if string(ev) != `["left","right"]` {
			t.Fatalf("evidence lost: %+v", m)
		}
		c := eval(map[string]any{"operator": "compute", "expression": "right.right.v", "outputField": "copied"}, r.outputItems[:1], nil)
		if c.errText != "" || c.outputItems[0].(map[string]any)["copied"] != 27.6 {
			t.Fatalf("downstream: %+v", c)
		}
		f := eval(map[string]any{"operator": "filter", "fieldPath": "right.right.v", "comparator": "gt", "compareValue": "25"}, r.outputItems[:1], nil)
		if len(f.outputItems) != 1 {
			t.Fatalf("filter: %+v", f)
		}
		cfg["joinMode"] = "inner"
		if r = eval(cfg, a, b); len(r.outputItems) != 2 {
			t.Fatalf("inner: %+v", r)
		}
		assert(t, eval(map[string]any{"operator": "join", "joinKeys": []any{map[string]any{"left": "id", "right": "id"}}}, []any{map[string]any{"id": 1}, map[string]any{}}, []any{map[string]any{"id": "1"}, map[string]any{}}), `[]`)
		for _, patch := range []map[string]any{{"joinKeys": []any{}}, {"joinKeys": []any{map[string]any{"left": "id", "right": ""}}}, {"joinMode": "outer"}} {
			bad := map[string]any{}
			for k, v := range cfg {
				bad[k] = v
			}
			for k, v := range patch {
				bad[k] = v
			}
			if eval(bad, nil, nil).errText == "" {
				t.Fatalf("invalid join accepted: %+v", bad)
			}
		}
	})
	t.Run("stable sort dedupe then top n", func(t *testing.T) {
		rows := []any{map[string]any{"id": "a", "score": 23}, map[string]any{"id": "a", "score": 27.6}, map[string]any{"id": "b", "score": 27.6}, map[string]any{"score": nil, "tag": 1}, map[string]any{"score": "", "tag": 2}}
		cfg := map[string]any{"operator": "sort_limit", "sortField": "score", "dedupeKeys": []any{"id"}, "limit": 3}
		assert(t, eval(cfg, rows, nil), `[{"id":"a","score":27.6},{"id":"b","score":27.6},{"score":null,"tag":1}]`)
		cfg["sortOrder"] = "asc"
		cfg["limit"] = 10
		assert(t, eval(cfg, rows, nil), `[{"id":"a","score":23},{"id":"b","score":27.6},{"score":null,"tag":1},{"score":"","tag":2}]`)
		for _, patch := range []map[string]any{{"dedupeKeys": "id"}, {"dedupeKeys": []any{""}}, {"dedupeKeys": []any{"id", "id"}}, {"sortField": " "}, {"sortOrder": "bad"}} {
			bad := map[string]any{"operator": "sort_limit"}
			for k, v := range patch {
				bad[k] = v
			}
			if eval(bad, nil, nil).errText == "" {
				t.Fatalf("invalid sort accepted: %+v", bad)
			}
		}
	})
}

func TestT03T04ProvenanceAndBoundary(t *testing.T) {
	eval := func(cfg map[string]any, items []any) wfEvalResult {
		return wfEvalLogicNode(map[string]any{"metadata": map[string]any{"logic": cfg}}, items, nil)
	}
	cfg := map[string]any{"operator": "aggregate", "groupBy": []any{"region", "substance"}, "metrics": []any{map[string]any{"field": "v", "fn": "sum", "as": "total"}}}
	rows := []any{map[string]any{"region": "A", "substance": "X", "v": 23, "citations": []any{"a"}, "metadata": map[string]any{"evidence": []any{"e1"}}}, map[string]any{"region": "A", "substance": "X", "v": 27.6, "citations": []any{"b"}, "metadata": map[string]any{"evidence": []any{"e2"}}}}
	r := eval(cfg, rows)
	if r.errText != "" || len(r.outputItems) != 1 {
		t.Fatalf("aggregate: %+v", r)
	}
	row := r.outputItems[0].(map[string]any)
	if row["total"] != 50.6 {
		t.Fatal(row)
	}
	encoded, _ := json.Marshal(row["metadata"])
	if string(encoded) != `{"evidence":["e1","e2"]}` {
		t.Fatalf("lost evidence: %s", encoded)
	}
	cfg["groupBy"] = []any{}
	r = eval(cfg, []any{map[string]any{"v": 1e308}})
	if r.errText != "" || r.outputItems[0].(map[string]any)["total"] != 1e308 {
		t.Fatalf("large finite: %+v", r)
	}
	if eval(cfg, []any{map[string]any{"v": 1e308}, map[string]any{"v": 1e308}}).errText == "" {
		t.Fatal("overflow accepted")
	}
	r = eval(map[string]any{"operator": "sort_limit", "sortField": "v", "dedupeKeys": []any{"id"}, "limit": 1}, []any{map[string]any{"id": 1, "v": 23, "citations": []any{"a"}}, map[string]any{"id": 1, "v": 27.6, "citations": []any{"b"}}})
	if r.errText != "" {
		t.Fatal(r.errText)
	}
	encoded, _ = json.Marshal(r.outputItems)
	if string(encoded) != `[{"citations":["b","a"],"id":1,"v":27.6}]` {
		t.Fatalf("dedupe provenance: %s", encoded)
	}
}

func TestT02ReviewedFieldAndZeroContract(t *testing.T) {
	for _, expr := range []string{"readings.length", "readings.0"} {
		if _, err := wfEvalArithmetic(map[string]any{"readings": []any{23}}, expr); err == "" {
			t.Fatalf("array path accepted: %s", expr)
		}
	}
	for _, out := range []any{nil, "", " "} {
		cfg := map[string]any{"operator": "compute", "expression": "x * 2"}
		if out != nil {
			cfg["outputField"] = out
		}
		r := wfEvalLogicNode(map[string]any{"metadata": map[string]any{"logic": cfg}}, []any{map[string]any{"x": 2, "computed_value": 99}}, nil)
		if r.errText == "" || len(r.outputItems) != 0 {
			t.Fatalf("default collision accepted: %+v", r)
		}
	}
	cfg := map[string]any{"operator": "compute", "outputField": "change", "periodComparison": map[string]any{"regionField": "region", "substanceField": "substance", "periodField": "period", "baselinePeriodField": "baseline", "valueField": "amount"}}
	row := func(period, baseline, amount float64) any {
		return map[string]any{"region": "A", "substance": "X", "period": period, "baseline": baseline, "amount": amount}
	}
	node := map[string]any{"metadata": map[string]any{"logic": cfg}}
	r := wfEvalLogicNode(node, []any{row(1, 0, 23), row(math.Copysign(0, -1), -1, 20)}, nil)
	if r.errText != "" || r.outputItems[0].(map[string]any)["change"] != float64(15) {
		t.Fatalf("negative zero baseline mismatch: %+v", r)
	}
	if r = wfEvalLogicNode(node, []any{row(0, -1, 20), row(math.Copysign(0, -1), -1, 20)}, nil); r.errText == "" {
		t.Fatal("duplicate zero accepted")
	}
	for _, op := range []string{"aggregate", "join"} {
		logic := map[string]any{"operator": op, "groupBy": []any{"period"}, "metrics": []any{map[string]any{"field": "", "fn": "count", "as": "n"}}, "joinKeys": []any{map[string]any{"left": "period", "right": "period"}}}
		r := wfEvalLogicNode(map[string]any{"metadata": map[string]any{"logic": logic}}, []any{row(0, -1, 20), row(math.Copysign(0, -1), -1, 20)}, []any{row(0, -1, 23)})
		want := 1
		if op == "join" {
			want = 2
		}
		if r.errText != "" || len(r.outputItems) != want {
			t.Fatalf("%s negative-zero mismatch: %+v", op, r)
		}
	}
}

func TestT01aLogicContract(t *testing.T) {
	eval := func(logic map[string]any, items []any) wfEvalResult {
		return wfEvalLogicNode(map[string]any{"metadata": map[string]any{"logic": logic}}, items, nil)
	}
	for _, op := range []any{"unknown", "", " ", nil, 1, "toString"} {
		t.Run(fmt.Sprintf("operator_%v", op), func(t *testing.T) {
			r := eval(map[string]any{"operator": op, "fieldPath": "score"}, []any{map[string]any{"score": 2}})
			if !strings.Contains(r.errText, "operator") || len(r.outputItems) != 0 {
				t.Fatalf("invalid operator accepted: %+v", r)
			}
		})
	}
	for _, limit := range []any{"2", nil, false, 0, -1, 1.5, math.NaN(), math.Inf(1), float64(9007199254740992)} {
		t.Run(fmt.Sprintf("limit_%v", limit), func(t *testing.T) {
			r := eval(map[string]any{"operator": "sort_limit", "limit": limit}, []any{1, 2})
			if !strings.Contains(r.errText, "limit") || len(r.outputItems) != 0 {
				t.Fatalf("invalid limit accepted: %+v", r)
			}
		})
	}
	for _, cfg := range []map[string]any{{}, {"operator": "sort_limit"}, {"operator": "sort_limit", "limit": 1}} {
		if r := eval(cfg, []any{1}); r.errText != "" {
			t.Fatalf("valid/default rejected: %+v", r)
		}
	}
	if r := eval(map[string]any{"operator": "compute", "expression": "score * 2"}, []any{map[string]any{"score": 2, "computed_value": 1}}); r.errText == "" || len(r.outputItems) != 0 {
		t.Fatalf("omitted outputField collision accepted: %+v", r)
	}
	items := []any{map[string]any{"score": 2}, map[string]any{"score": 3, "result": nil}}
	r := eval(map[string]any{"operator": "compute", "expression": "score * 2", "outputField": "result"}, items)
	if !strings.Contains(r.errText, "outputField") || len(r.outputItems) != 0 {
		t.Fatalf("duplicate alias accepted: %+v", r)
	}
	r = eval(map[string]any{"operator": "compute", "expression": "score * 2", "outputField": "result"}, items[:1])
	if r.errText != "" || len(r.outputItems) != 1 || r.outputItems[0].(map[string]any)["result"] != float64(4) {
		t.Fatalf("valid compute rejected: %+v", r)
	}
}

func TestT01aRunRequestContract(t *testing.T) {
	db := testDB(t)
	router := gin.New()
	registerCanvasWorkflowRoutes(router.Group(""), db, func(c *gin.Context) (Scope, *model.User, bool) { return Scope{}, &model.User{ID: "u_alice"}, true })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/canvas-workflows/canvas/run", strings.NewReader(`{"expected_revision":1,"execution_key":"k"}`)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unsafe legacy REST still registered: %d", w.Code)
	}
	for _, raw := range []string{`{}`, `{"expected_revision":1}`, `{"expected_revision":1,"execution_key":"k","target_node_id":"x"}`, `{"expected_revision":1,"execution_key":"k","refresh_bindings":"yes"}`} {
		if _, err := decodeCanvasWorkflowRunOptions(strings.NewReader(raw)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid request accepted: %v", err)
		}
	}
}
func TestT01aDirectAndToolAdmission(t *testing.T) {
	rev := int64(1)
	if _, err := ExecuteCanvasWorkflow(context.Background(), nil, Scope{}, "u_alice", "canvas", CanvasWorkflowRunOptions{ExpectedRevision: &rev, ExecutionKey: "k"}, time.Now()); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("unmanaged direct execution: %v", err)
	}
	for _, tool := range Catalog {
		if tool.Name == "intel_run_canvas_workflow" {
			t.Fatal("unapproved legacy run tool still exposed")
		}
	}
}

func TestExecuteCanvasWorkflowSubstancePrecursorDAG(t *testing.T) {
	db := testDB(t)
	sc, err := ResolveScope(db, "u_alice", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

	canvasDoc := map[string]any{
		"id":    "canvas_drug_dag",
		"title": "华南与西南区域毒情污水折算与前体关联模型",
		"nodes": []any{
			map[string]any{
				"id":    "ds_wbe",
				"type":  "data-source",
				"title": "污水监测原始数据",
				"metadata": map[string]any{
					"dataSource": map[string]any{
						"format": "intel",
						"rawText": `[
							{"id":"w1","region":"华南A区","substance":"甲基苯丙胺","concentration_ng_l":50,"flow_m3_day":100000,"excretion_factor":2.3,"excretion_factor_source":{"version":"fixture-v1","citations":[{"evidence_id":"fixture-not-real-evidence","revision":1}]},"population":500000},
							{"id":"w2","region":"华南A区","substance":"甲基苯丙胺","concentration_ng_l":70,"flow_m3_day":100000,"excretion_factor":2.3,"excretion_factor_source":{"version":"fixture-v1","citations":[{"evidence_id":"fixture-not-real-evidence","revision":1}]},"population":500000},
							{"id":"w3","region":"西南B区","substance":"氯胺酮","concentration_ng_l":20,"flow_m3_day":80000,"excretion_factor":3.0,"excretion_factor_source":{"version":"fixture-v1","citations":[{"evidence_id":"fixture-not-real-evidence","revision":1}]},"population":400000}
						]`,
					},
				},
			},
			map[string]any{
				"id":    "ds_precursor",
				"type":  "data-source",
				"title": "易制毒化学品异常流向",
				"metadata": map[string]any{
					"dataSource": map[string]any{
						"format": "intel",
						"rawText": `[
							{"region":"华南A区","precursor":"1-苯基-2-丙酮","anomaly_lots":4},
							{"region":"西南B区","precursor":"邻氯苯基环戊基酮","anomaly_lots":1}
						]`,
					},
				},
			},
			map[string]any{
				"id":    "logic_compute",
				"type":  "logic",
				"title": "折算千人消费量",
				"metadata": map[string]any{
					"logic": map[string]any{
						"operator":    "compute",
						"expression":  "wbe_consumption_mg_1000p",
						"outputField": "consumption_mg_1000p",
					},
				},
			},
			map[string]any{
				"id":    "logic_join",
				"type":  "logic",
				"title": "污水与前体关联拼接",
				"metadata": map[string]any{
					"logic": map[string]any{
						"operator": "join",
						"joinKeys": []any{map[string]any{"left": "region", "right": "region"}},
						"joinMode": "inner",
					},
				},
			},
			map[string]any{
				"id":    "logic_agg",
				"type":  "logic",
				"title": "按区域聚合均值",
				"metadata": map[string]any{
					"logic": map[string]any{
						"operator": "aggregate",
						"groupBy":  []any{"region"},
						"metrics":  []any{map[string]any{"field": "consumption_mg_1000p", "fn": "avg", "as": "avg_consumption"}, map[string]any{"field": "", "fn": "count", "as": "count"}},
					},
				},
			},
			map[string]any{
				"id":    "logic_top",
				"type":  "logic",
				"title": "高风险区域 Top-1",
				"metadata": map[string]any{
					"logic": map[string]any{
						"operator":  "sort_limit",
						"sortField": "avg_consumption",
						"sortOrder": "desc",
						"limit":     1,
					},
				},
			},
			map[string]any{
				"id":    "chart_risk",
				"type":  "chart",
				"title": "区域负荷对比图",
				"metadata": map[string]any{
					"chartKind": "bar",
				},
			},
			map[string]any{
				"id":       "md_brief",
				"type":     "markdown",
				"title":    "区域毒情研判快报",
				"metadata": map[string]any{},
			},
		},
		"connections": []any{
			map[string]any{"id": "c1", "fromNodeId": "ds_wbe", "toNodeId": "logic_compute"},
			map[string]any{"id": "c2", "fromNodeId": "logic_compute", "toNodeId": "logic_join", "toHandleId": "input_a"},
			map[string]any{"id": "c3", "fromNodeId": "ds_precursor", "toNodeId": "logic_join", "toHandleId": "input_b"},
			map[string]any{"id": "c4", "fromNodeId": "logic_join", "toNodeId": "logic_agg"},
			map[string]any{"id": "c5", "fromNodeId": "logic_agg", "toNodeId": "logic_top"},
			map[string]any{"id": "c6", "fromNodeId": "logic_agg", "toNodeId": "chart_risk"},
			map[string]any{"id": "c7", "fromNodeId": "logic_top", "toNodeId": "md_brief"},
		},
	}
	rawDoc, err := json.Marshal(canvasDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CanvasProject{
		ID:          "canvas_drug_dag",
		UserID:      "u_alice",
		Title:       "华南与西南区域毒情污水折算与前体关联模型",
		PayloadJSON: string(rawDoc),
		Revision:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(&model.CanvasWorkflowExecution{}); err != nil {
		t.Fatal(err)
	}
	fund(t, Billing{DB: db}, "u_alice", 10000)
	if _, err := (Billing{DB: db}).Account(db, sc.TenantID); err != nil {
		t.Fatal(err)
	}
	executions := CanvasWorkflowExecutions{DB: db, Authority: &workflowTestAuthority{approved: true}}
	revision := int64(1)
	run, err := executions.Admit(context.Background(), CanvasWorkflowAdmission{UserID: "u_alice", TenantID: sc.TenantID, WorkspaceID: sc.WorkspaceID, CanvasID: "canvas_drug_dag", ExpectedRevision: &revision, ExecutionKey: "substance-precursor-test", BudgetMicro: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if err := (CanvasWorkflowManagedRunner{Tools: Tools{DB: db}, WorkerID: "numeric-dag"}).Run(context.Background(), executions, run); err != nil {
		t.Fatalf("managed numeric DAG: %v", err)
	}
	stored, err := executions.Get(context.Background(), "u_alice", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := DecodeCanvasWorkflowCheckpoint(stored)
	if err != nil {
		t.Fatal(err)
	}
	var res CanvasWorkflowManagedResult
	if err := json.Unmarshal(checkpoint.Result, &res); err != nil {
		t.Fatal(err)
	}
	if res.Revision != 2 || len(res.NodeSnapshots) != 8 || res.Status != "succeeded" {
		t.Fatalf("unexpected managed result %+v", res)
	}
	if res.NodeSnapshots["logic_compute"].OutputCount != 3 {
		t.Fatalf("expected 3 computed items, got %+v", res.NodeSnapshots["logic_compute"])
	}
	for index, want := range []float64{23, 32.2, 12} {
		if got := res.NodeSnapshots["logic_compute"].SampleItems[index].(map[string]any)["consumption_mg_1000p"]; got != want {
			t.Fatalf("compute sample %d = %v, want %v", index, got, want)
		}
	}
	if got := res.NodeSnapshots["logic_agg"].SampleItems[0].(map[string]any)["avg_consumption"]; got != float64(27.6) {
		t.Fatalf("regional mean = %v, want 27.6", got)
	}
	if res.NodeSnapshots["logic_agg"].OutputCount != 2 {
		t.Fatalf("expected 2 aggregated regions, got %+v", res.NodeSnapshots["logic_agg"])
	}
	if res.NodeSnapshots["logic_top"].OutputCount != 1 {
		t.Fatalf("expected 1 top risk region, got %+v", res.NodeSnapshots["logic_top"])
	}

	var updated model.CanvasProject
	if err := db.First(&updated, "id = ?", "canvas_drug_dag").Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(updated.PayloadJSON, "27.6") || !strings.Contains(updated.PayloadJSON, "lastRunSnapshot") {
		t.Fatalf("expected computed 27.6 and lastRunSnapshot in updated canvas payload: %s", updated.PayloadJSON)
	}
}

func TestT02ComputeContract(t *testing.T) {
	source := map[string]any{"version": "fixture-v1", "citations": []any{map[string]any{"evidence_id": "fixture-not-real-evidence", "revision": 1}}}
	sample := map[string]any{"concentration_ng_l": 50, "flow_m3_day": 100000, "population": 500000, "excretion_factor": 2.3, "excretion_factor_source": source}
	for _, expr := range []string{"wbe_consumption_mg_1000p", "concentration_ng_l * flow_m3_day * excretion_factor / population"} {
		if v, err := wfEvalArithmetic(sample, expr); err != "" || v != 23 {
			t.Errorf("units: %v %s", v, err)
		}
	}
	if v, err := wfEvalArithmetic(map[string]any{"nested": map[string]any{"value": "2.5e1"}}, "-(nested.value + 5) / 2"); err != "" || v != -15 {
		t.Errorf("valid: %v %s", v, err)
	}
	for _, v := range []any{nil, "", " ", "bad", "0x10", true, []any{}, map[string]any{}, math.NaN(), math.Inf(1), "Infinity", "1e309"} {
		if _, err := wfEvalArithmetic(map[string]any{"x": v}, "x + 1"); err == "" {
			t.Errorf("invalid field accepted: %#v", v)
		}
	}
	for _, expr := range []string{"", "x / 0", "x +", "(x", "x)", "x 2", "x..a", "x @ 2", "x * huge * huge", "huge * huge / huge"} {
		if _, err := wfEvalArithmetic(map[string]any{"x": 2, "huge": 1e308}, expr); err == "" {
			t.Errorf("invalid expression accepted: %s", expr)
		}
	}
	for _, field := range []string{"excretion_factor", "excretion_factor_source"} {
		cp := map[string]any{}
		for k, v := range sample {
			cp[k] = v
		}
		delete(cp, field)
		if _, err := wfEvalArithmetic(cp, "wbe_consumption_mg_1000p"); err == "" {
			t.Errorf("missing %s accepted", field)
		}
	}
	for _, v := range []any{"source", nil, map[string]any{}, map[string]any{"version": "v1", "citations": []any{}}, map[string]any{"version": "v1", "citations": []any{map[string]any{"evidence_id": "fixture", "revision": 0}}}} {
		cp := map[string]any{}
		for k, v := range sample {
			cp[k] = v
		}
		cp["excretion_factor_source"] = v
		if _, err := wfEvalArithmetic(cp, "wbe_consumption_mg_1000p"); err == "" {
			t.Errorf("bad source accepted: %#v", v)
		}
	}
	eval := func(cfg map[string]any, items []any) wfEvalResult {
		return wfEvalLogicNode(map[string]any{"metadata": map[string]any{"logic": cfg}}, items, nil)
	}
	if r := eval(map[string]any{"operator": "compute", "expression": "x * 2"}, []any{map[string]any{"x": 2}, map[string]any{"x": "bad"}}); r.errText == "" || len(r.outputItems) != 0 {
		t.Errorf("partial output: %+v", r)
	}
	if r := eval(map[string]any{"operator": "compute"}, []any{sample}); r.errText == "" {
		t.Error("implicit formula accepted")
	}
}

func TestT02PeriodComparison(t *testing.T) {
	comparison := map[string]any{"regionField": "region", "substanceField": "substance", "periodField": "period", "baselinePeriodField": "baseline", "valueField": "amount"}
	cfg := map[string]any{"operator": "compute", "outputField": "change", "periodComparison": comparison}
	eval := func(cfg map[string]any, items []any) wfEvalResult {
		return wfEvalLogicNode(map[string]any{"metadata": map[string]any{"logic": cfg}}, items, nil)
	}
	row := func(period, baseline, amount, region any, substance string) map[string]any {
		return map[string]any{"region": region, "substance": substance, "period": period, "baseline": baseline, "amount": amount}
	}
	items := []any{row("2026-10", "2026-09", 23, "A", "X"), row("2026-09", "2026-08", 20, "A", "X"), row("2026-09", "2026-08", 100, "B", "X"), row("2026-09", "2026-08", 200, "A", "Y"), row("2025-10", "2025-09", 10, "A", "X"), row("2026-10-yoy", "2025-10", 23, "A", "X"), row("zero", "none", 0, "A", "X"), row("after-zero", "zero", 23, "A", "X")}
	r := eval(cfg, items)
	if r.errText != "" || len(r.outputItems) != len(items) {
		t.Fatalf("comparison: %+v", r)
	}
	for _, tc := range []struct {
		index  int
		value  any
		status string
	}{{0, float64(15), "ok"}, {1, nil, "missing_baseline"}, {5, float64(130), "ok"}, {7, nil, "zero_baseline"}} {
		got := r.outputItems[tc.index].(map[string]any)
		if got["change"] != tc.value || got["change_status"] != tc.status {
			t.Errorf("row %d: %+v", tc.index, got)
		}
	}
	for _, items := range [][]any{{row(1, "1", 23, "A", "X")}, {row("now", "old", 23, 1, "X"), row("old", "none", 10, "1", "X")}} {
		r := eval(cfg, items)
		if r.errText != "" || r.outputItems[0].(map[string]any)["change_status"] != "missing_baseline" {
			t.Errorf("key types: %+v", r)
		}
	}
	for _, bad := range []any{nil, map[string]any{}, map[string]any{"regionField": "region", "substanceField": "substance", "periodField": "", "baselinePeriodField": "baseline", "valueField": "amount"}} {
		if r := eval(map[string]any{"operator": "compute", "periodComparison": bad}, nil); r.errText == "" {
			t.Errorf("bad config accepted: %#v", bad)
		}
	}
	for _, items := range [][]any{
		{row("now", "old", 23, "A", "X"), row("old", "none", 20, "A", "X"), row("old", "none", 20, "A", "X")},
		{row("now", "old", 23, "A", "X"), row("old", "none", math.Inf(1), "A", "X")},
		{row("now", "old", 23, nil, "X")}, {row("now", "", 23, "A", "X")},
		{map[string]any{"region": "A", "substance": "X", "period": "now", "baseline": "old", "amount": 23, "change_status": "old"}},
	} {
		if r := eval(cfg, items); r.errText == "" || len(r.outputItems) != 0 {
			t.Errorf("invalid comparison accepted: %+v", r)
		}
	}
	if r := eval(map[string]any{"operator": "compute", "periodComparison": comparison, "expression": "amount"}, nil); r.errText == "" {
		t.Error("ambiguous config accepted")
	}
}

func TestT02FactorSourceAndRounding(t *testing.T) {
	invalid := []any{nil, map[string]any{}, "source", map[string]any{"version": " ", "citations": []any{map[string]any{"evidence_id": "fixture", "revision": 1}}}, map[string]any{"version": "v1", "citations": []any{nil}}, map[string]any{"version": "v1", "citations": []any{map[string]any{"evidence_id": " ", "revision": 1}}}}
	for _, revision := range []any{0, -1, 1.5, "1", math.Inf(1), float64(9007199254740992)} {
		invalid = append(invalid, map[string]any{"version": "v1", "citations": []any{map[string]any{"evidence_id": "fixture", "revision": revision}}})
	}
	for _, source := range invalid {
		if _, err := wfEvalArithmetic(map[string]any{"excretion_factor": 2.3, "excretion_factor_source": source}, "excretion_factor"); err == "" {
			t.Errorf("invalid source accepted: %#v", source)
		}
	}
	for _, tc := range []struct{ input, want float64 }{{-0.00005, 0}, {1e308, 1e308}} {
		if got, err := wfEvalArithmetic(map[string]any{"x": tc.input}, "x"); err != "" || got != tc.want {
			t.Errorf("rounding: %v %s", got, err)
		}
	}
}
