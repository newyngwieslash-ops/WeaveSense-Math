package flow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

func managedFixture(t *testing.T, doc map[string]any) (CanvasWorkflowManagedRunner, CanvasWorkflowExecutions, model.CanvasWorkflowExecution) {
	t.Helper()
	executions, admission, _ := workflowExecutionSetup(t)
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := executions.DB.Model(&model.CanvasProject{}).Where("id = ?", admission.CanvasID).Update("payload_json", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	record, err := executions.Admit(context.Background(), admission)
	if err != nil {
		t.Fatal(err)
	}
	return CanvasWorkflowManagedRunner{Tools: Tools{DB: executions.DB}, WorkerID: "managed-test"}, executions, record
}
func managedSource(id, raw string) map[string]any {
	return map[string]any{"id": id, "type": "data-source", "metadata": map[string]any{"dataSource": map[string]any{"format": "json", "rawText": raw}, "content": "old"}}
}
func managedSink(id string) map[string]any {
	return map[string]any{"id": id, "type": "chart", "metadata": map[string]any{"content": "stale", "computed": []any{9}}}
}
func managedEdge(id, from, to string) map[string]any {
	return map[string]any{"id": id, "fromNodeId": from, "toNodeId": to}
}

func TestManagedCanvasPreflightBeforeAnyNode(t *testing.T) {
	for _, name := range []string{"duplicate_node", "duplicate_edge", "dangling", "cycle", "bad_port", "unknown_relation"} {
		t.Run(name, func(t *testing.T) {
			a, b := managedSource("a", "[1]"), managedSink("b")
			edge := managedEdge("e", "a", "b")
			nodes := []any{a, b}
			edges := []any{edge}
			switch name {
			case "duplicate_node":
				nodes = append(nodes, managedSource("a", "[2]"))
			case "duplicate_edge":
				edges = append(edges, managedEdge("e", "a", "b"))
			case "dangling":
				edge["fromNodeId"] = "missing"
			case "cycle":
				a["type"] = "text"
				edges = append(edges, managedEdge("e2", "b", "a"))
			case "bad_port":
				edge["fromHandleId"] = "false"
			case "unknown_relation":
				edge["relation"] = "imaginary"
			}
			runner, executions, run := managedFixture(t, map[string]any{"nodes": nodes, "connections": edges})
			if err := runner.Run(context.Background(), executions, run); !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected graph rejection, got %v", err)
			}
			saved, err := executions.Get(context.Background(), run.UserID, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			cp, _ := DecodeCanvasWorkflowCheckpoint(saved)
			if saved.Status != "failed" || len(cp.Nodes) != 0 {
				t.Fatalf("preflight executed nodes: %+v", cp)
			}
			var project model.CanvasProject
			executions.DB.First(&project, "id = ?", run.CanvasID)
			if project.Revision != run.CanvasRevision {
				t.Fatal("invalid graph mutated canvas")
			}
		})
	}
}

func TestManagedCanvasFailureBlocksSuccessors(t *testing.T) {
	runner, executions, run := managedFixture(t, map[string]any{"nodes": []any{managedSource("a", "{"), managedSink("b")}, "connections": []any{managedEdge("e", "a", "b")}})
	if err := runner.Run(context.Background(), executions, run); err == nil {
		t.Fatal("invalid source succeeded")
	}
	stored, err := executions.Get(context.Background(), run.UserID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	cp, _ := DecodeCanvasWorkflowCheckpoint(stored)
	if stored.Status != "failed" || cp.Nodes["a"].Status != "failed" || cp.Nodes["b"].Status != "blocked" {
		t.Fatalf("failure not propagated: %+v", cp)
	}
}

func TestManagedCanvasCancelPersistsAndFences(t *testing.T) {
	runner, executions, run := managedFixture(t, map[string]any{"nodes": []any{managedSource("a", "[]")}, "connections": []any{}})
	token, err := executions.Acquire(context.Background(), run.UserID, run.ID, "old-worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := CancelCanvasWorkflow(context.Background(), executions, run.UserID, run.ID); err != nil {
		t.Fatal(err)
	}
	saved, err := executions.Get(context.Background(), run.UserID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != "cancelled" {
		t.Fatalf("not cancelled: %s", saved.Status)
	}
	if err := runner.Run(context.Background(), executions, run); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled runner executed: %v", err)
	}
	cp, _ := DecodeCanvasWorkflowCheckpoint(saved)
	cp.Stage = "running"
	if _, err := executions.Checkpoint(context.Background(), run.UserID, run.ID, "old-worker", token, saved.Revision, cp); err == nil {
		t.Fatal("stale worker committed")
	}
}

func TestManagedCanvasSkillReportPartialSaveOnlyAndCAS(t *testing.T) {
	for _, mode := range []string{"partial", "cas", "cancel", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			tools, req, _ := canvasSkillFixture(t, "", 200)
			old := req.Executions
			var modelCalls, reportCalls atomic.Int32
			output, _ := json.Marshal(substanceOutput(t, "regional-wastewater-modeling"))
			var activeRun model.CanvasWorkflowExecution
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				modelCalls.Add(1)
				switch mode {
				case "cas":
					old.DB.Model(&model.CanvasProject{}).Where("id = ?", activeRun.CanvasID).Updates(map[string]any{"revision": 2, "payload_json": `{"nodes":[],"connections":[],"user_edit":true}`})
				case "cancel":
					if err := CancelCanvasWorkflow(context.Background(), *old, activeRun.UserID, activeRun.ID); err != nil {
						t.Errorf("cancel: %v", err)
					}
				case "unknown":
					w.WriteHeader(503)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(output)}}}, "usage": map[string]any{"total_tokens": 100}})
			}))
			defer gateway.Close()
			gw := tools.Model.(*OpenAICompatGateway)
			gw.URL = gateway.URL
			gw.HTTP = gateway.Client()
			authority := old.Authority.(*canvasSkillTestAuthority)
			authority.route["flow_model_gateway"] = map[string]any{"model": gw.Model, "url_hash": canvasSkillDigest([]byte(gw.URL)), "pricing_hash": hashJSON(gw.MicroPer1KTok), "micro_per_1k_tokens": gw.MicroPer1KTok, "status_url_hash": canvasSkillDigest([]byte(""))}
			engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/evidence/resolve":
					yes := true
					json.NewEncoder(w).Encode(map[string]any{"data": []CanvasWorkflowEvidence{{ResolvedEvidence: ResolvedEvidence{EvidenceID: "fixture-monitor", Revision: 2, DocumentID: "fixture-document", SourceID: "monitor", SnippetOK: &yes, URLOK: &yes}, CharEnd: 1}, {ResolvedEvidence: ResolvedEvidence{EvidenceID: "fixture-factor", Revision: 1, DocumentID: "fixture-document", SourceID: "factor", SnippetOK: &yes, URLOK: &yes}, CharEnd: 1}}})
				case "/v1/reports":
					if r.Method == http.MethodGet {
						json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
						return
					}
					if reportCalls.Add(1) == 1 {
						w.WriteHeader(503)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"report_id": "rpt_managed"}})
				default:
					w.WriteHeader(404)
				}
			}))
			defer engine.Close()
			tools.Engine = EngineClient{Cfg: Config{EngineURL: engine.URL, SigningKey: key, Issuer: "flow-platform", Audience: "loom-engine", TTL: MaxTokenTTL}}
			input, _ := json.Marshal(req.Input)
			source := managedSource("source", string(input))
			skill := map[string]any{"id": "skill-node", "type": "skill", "metadata": map[string]any{"skillId": req.ApprovedSkill.ID, "skillVersion": req.ApprovedSkill.Version, "skillHash": req.ApprovedSkill.Hash}}
			sink := managedSink("report")
			sink["type"] = "markdown"
			doc := map[string]any{"nodes": []any{source, skill, sink}, "connections": []any{managedEdge("a", "source", "skill-node"), managedEdge("b", "skill-node", "report")}}
			raw, _ := json.Marshal(doc)
			old.DB.Model(&model.CanvasProject{}).Where("id = ?", "skill-canvas").Update("payload_json", string(raw))
			rev := int64(1)
			run, err := old.Admit(context.Background(), CanvasWorkflowAdmission{UserID: req.UserID, TenantID: model.PersonalTenantID(req.UserID), WorkspaceID: model.PersonalWorkspaceID(req.UserID), CanvasID: "skill-canvas", ExpectedRevision: &rev, ExecutionKey: "managed-integration", BudgetMicro: 1_000_000, SaveReport: mode == "partial"})
			if err != nil {
				t.Fatal(err)
			}
			activeRun = run
			runner := CanvasWorkflowManagedRunner{Tools: tools, WorkerID: "managed-integration", MaxTokens: 4096, Timeout: 5 * time.Second}
			err = runner.Run(context.Background(), *old, run)
			saved, e := old.Get(context.Background(), run.UserID, run.ID)
			if e != nil {
				t.Fatal(e)
			}
			cp, _ := DecodeCanvasWorkflowCheckpoint(saved)
			var result CanvasWorkflowManagedResult
			json.Unmarshal(cp.Result, &result)
			if err == nil {
				t.Fatal("expected interrupted/partial run")
			}
			switch mode {
			case "partial":
				if saved.Status != "saving_report" || result.Status != "partial" || result.Nodes["report"].Items[0].(map[string]any)["regional_metrics"] == nil {
					t.Fatalf("lost full output/partial: %+v", result)
				}
				if err := runner.Run(context.Background(), *old, run); err != nil {
					t.Fatal(err)
				}
				done, _ := old.Get(context.Background(), run.UserID, run.ID)
				cp, _ = DecodeCanvasWorkflowCheckpoint(done)
				if done.Status != "succeeded" || cp.ReportID != "rpt_managed" || modelCalls.Load() != 1 || reportCalls.Load() != 2 {
					t.Fatalf("save-only replay lost: %s calls=%d/%d", done.Status, modelCalls.Load(), reportCalls.Load())
				}
			case "cas":
				if !errors.Is(err, errCanvasWorkflowRevisionConflict) {
					t.Fatalf("CAS conflict missing: %v", err)
				}
			case "cancel":
				if saved.Status != "cancel_requested" && saved.Status != "cancelled" {
					t.Fatalf("lost cancellation: %s", saved.Status)
				}
			case "unknown":
				if saved.Status != "waiting_external" {
					t.Fatalf("unknown not parked: %s %v", saved.Status, err)
				}
				if err := runner.Run(context.Background(), *old, run); !errors.Is(err, ErrModelUnknownOutcome) {
					t.Fatalf("unknown resume: %v", err)
				}
				if modelCalls.Load() != 1 {
					t.Fatal("unknown graph replay recharged")
				}
			}
		})
	}
}

func TestManagedCanvasEmptyClearsStaleAndReplays(t *testing.T) {
	runner, executions, run := managedFixture(t, map[string]any{"nodes": []any{managedSource("a", "[]"), managedSink("b")}, "connections": []any{managedEdge("e", "a", "b")}})
	if err := runner.Run(context.Background(), executions, run); err != nil {
		t.Fatal(err)
	}
	var project model.CanvasProject
	executions.DB.First(&project, "id = ?", run.CanvasID)
	var doc map[string]any
	json.Unmarshal([]byte(project.PayloadJSON), &doc)
	for _, node := range doc["nodes"].([]any) {
		meta := node.(map[string]any)["metadata"].(map[string]any)
		if meta["content"] != "" {
			t.Fatalf("stale output: %+v", meta)
		}
	}
	if project.Revision != run.CanvasRevision+1 {
		t.Fatal("no canvas CAS")
	}
	if err := runner.Run(context.Background(), executions, run); err != nil {
		t.Fatal(err)
	}
	executions.DB.First(&project, "id = ?", run.CanvasID)
	if project.Revision != run.CanvasRevision+1 {
		t.Fatal("replay committed twice")
	}
}

func TestT14ManagedDAGEndToEndEvidencePropagation(t *testing.T) {
	sourceA := map[string]any{
		"id":   "src-a",
		"type": "data-source",
		"metadata": map[string]any{
			"dataSource": map[string]any{
				"format":  "json",
				"rawText": `[{"region_code":"CN-GD","substance_name":"ephedrine","period":"2026-02","quantity":120,"citations":[{"evidence_id":"ev-precursor-1","revision":1}]}]`,
			},
		},
	}
	sourceB := map[string]any{
		"id":   "src-b",
		"type": "data-source",
		"metadata": map[string]any{
			"dataSource": map[string]any{
				"format":  "json",
				"rawText": `[{"region_code":"CN-GD","substance_name":"ephedrine","period":"2026-02","concentration_ng_l":45,"citations":[{"evidence_id":"ev-monitor-2","revision":3}]}]`,
			},
		},
	}
	joinNode := map[string]any{
		"id":   "logic-join",
		"type": "logic",
		"metadata": map[string]any{
			"logic": map[string]any{
				"operator": "join",
				"joinMode": "inner",
				"joinKeys": []any{
					map[string]any{"left": "region_code", "right": "region_code"},
					map[string]any{"left": "substance_name", "right": "substance_name"},
					map[string]any{"left": "period", "right": "period"},
				},
			},
		},
	}
	aggNode := map[string]any{
		"id":   "logic-agg",
		"type": "logic",
		"metadata": map[string]any{
			"logic": map[string]any{
				"operator": "aggregate",
				"groupBy":  []any{"region_code", "substance_name"},
				"metrics": []any{
					map[string]any{"field": "quantity", "fn": "sum", "as": "total_quantity"},
					map[string]any{"field": "concentration_ng_l", "fn": "avg", "as": "avg_conc"},
				},
			},
		},
	}
	rankNode := map[string]any{
		"id":   "logic-rank",
		"type": "logic",
		"metadata": map[string]any{
			"logic": map[string]any{
				"operator":  "sort_limit",
				"sortField": "total_quantity",
				"sortOrder": "desc",
				"limit":     5,
			},
		},
	}
	chartNode := managedSink("sink-chart")
	mdNode := managedSink("sink-md")
	mdNode["type"] = "markdown"

	doc := map[string]any{
		"nodes": []any{sourceA, sourceB, joinNode, aggNode, rankNode, chartNode, mdNode},
		"connections": []any{
			map[string]any{"id": "e1", "fromNodeId": "src-a", "toNodeId": "logic-join", "toHandleId": "input_a"},
			map[string]any{"id": "e2", "fromNodeId": "src-b", "toNodeId": "logic-join", "toHandleId": "input_b"},
			managedEdge("e3", "logic-join", "logic-agg"),
			managedEdge("e4", "logic-agg", "logic-rank"),
			managedEdge("e5", "logic-rank", "sink-chart"),
			managedEdge("e6", "logic-rank", "sink-md"),
		},
	}

	runner, executions, run := managedFixture(t, doc)
	if err := runner.Run(context.Background(), executions, run); err != nil {
		t.Fatalf("managed run failed: %v", err)
	}

	var project model.CanvasProject
	if err := executions.DB.First(&project, "id = ?", run.CanvasID).Error; err != nil {
		t.Fatal(err)
	}
	var committed map[string]any
	if err := json.Unmarshal([]byte(project.PayloadJSON), &committed); err != nil {
		t.Fatal(err)
	}

	nodesByID := map[string]map[string]any{}
	for _, raw := range committed["nodes"].([]any) {
		node := raw.(map[string]any)
		nodesByID[node["id"].(string)] = node
	}

	for _, id := range []string{"logic-join", "logic-agg", "logic-rank", "sink-chart", "sink-md"} {
		meta := nodesByID[id]["metadata"].(map[string]any)
		derived, ok := meta["derived"].(map[string]any)
		if !ok || derived["status"] != "succeeded" {
			t.Fatalf("node %s missing succeeded derived metadata: %+v", id, meta)
		}
		evidenceList, ok := derived["evidence"].([]any)
		if !ok || len(evidenceList) != 2 {
			t.Fatalf("node %s expected 2 propagated evidence citations, got: %+v", id, derived["evidence"])
		}
		snap, ok := meta["lastRunSnapshot"].(map[string]any)
		if !ok || snap["status"] != "success" {
			t.Fatalf("node %s missing success lastRunSnapshot: %+v", id, snap)
		}
	}
}
