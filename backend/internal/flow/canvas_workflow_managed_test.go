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

func managedSkillBlueprintFixture(t *testing.T, skillID string, doc map[string]any) (CanvasWorkflowManagedRunner, CanvasWorkflowExecutions, model.CanvasWorkflowExecution) {
	t.Helper()
	outputRaw, err := json.Marshal(substanceOutput(t, skillID))
	if err != nil {
		t.Fatal(err)
	}
	tools, req, _ := canvasSkillFixture(t, string(outputRaw), http.StatusOK)
	executions := *req.Executions
	scope := Scope{TenantID: model.PersonalTenantID(req.UserID), WorkspaceID: model.PersonalWorkspaceID(req.UserID)}
	sk, ok := FindSkill(skillID, "")
	if !ok {
		t.Fatalf("skill %s not found", skillID)
	}
	if _, err := InstallSkill(tools.DB, scope, req.UserID, sk.ID, sk.Version); err != nil {
		t.Fatal(err)
	}
	authority := executions.Authority.(*canvasSkillTestAuthority)
	authority.skill = sk.Ref()

	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/evidence/resolve":
			yes := true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": []CanvasWorkflowEvidence{
					{ResolvedEvidence: ResolvedEvidence{EvidenceID: "fixture-monitor", Revision: 2, DocumentID: "fixture-document", SourceID: "monitor", SnippetOK: &yes, URLOK: &yes}, CharEnd: 1},
					{ResolvedEvidence: ResolvedEvidence{EvidenceID: "fixture-factor", Revision: 1, DocumentID: "fixture-document", SourceID: "factor", SnippetOK: &yes, URLOK: &yes}, CharEnd: 1},
					{ResolvedEvidence: ResolvedEvidence{EvidenceID: "fixture-precursor", Revision: 1, DocumentID: "fixture-document", SourceID: "precursor", SnippetOK: &yes, URLOK: &yes}, CharEnd: 1},
					{ResolvedEvidence: ResolvedEvidence{EvidenceID: "fixture-policy", Revision: 1, DocumentID: "fixture-document", SourceID: "policy", SnippetOK: &yes, URLOK: &yes}, CharEnd: 1},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(engine.Close)
	tools.Engine = EngineClient{Cfg: Config{EngineURL: engine.URL, SigningKey: key, Issuer: "flow-platform", Audience: "loom-engine", TTL: MaxTokenTTL}}

	for _, raw := range doc["nodes"].([]any) {
		node := raw.(map[string]any)
		if node["type"] == "skill" {
			meta := ensureMap(node, "metadata")
			meta["skillId"] = sk.ID
			meta["skillVersion"] = sk.Version
			meta["skillHash"] = sk.Hash
		}
	}
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	canvasID := "blueprint-" + skillID
	if err := tools.DB.Create(&model.CanvasProject{
		ID:          canvasID,
		UserID:      req.UserID,
		Revision:    1,
		PayloadJSON: string(rawDoc),
	}).Error; err != nil {
		t.Fatal(err)
	}
	rev := int64(1)
	run, err := executions.Admit(context.Background(), CanvasWorkflowAdmission{
		UserID:           req.UserID,
		TenantID:         scope.TenantID,
		WorkspaceID:      scope.WorkspaceID,
		CanvasID:         canvasID,
		ExpectedRevision: &rev,
		ExecutionKey:     "t20-" + skillID,
		BudgetMicro:      1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	return CanvasWorkflowManagedRunner{
		Tools:     tools,
		WorkerID:  "t20-" + skillID,
		MaxTokens: 4096,
		Timeout:   5 * time.Second,
	}, executions, run
}

func committedManagedNodes(t *testing.T, executions CanvasWorkflowExecutions, canvasID string) map[string]map[string]any {
	t.Helper()
	var project model.CanvasProject
	if err := executions.DB.First(&project, "id = ?", canvasID).Error; err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(project.PayloadJSON), &doc); err != nil {
		t.Fatal(err)
	}
	nodes := map[string]map[string]any{}
	for _, raw := range doc["nodes"].([]any) {
		node := raw.(map[string]any)
		nodes[node["id"].(string)] = node
	}
	return nodes
}

func TestT20IndustryBlueprintsManagedRun(t *testing.T) {
	t.Run("wastewater", func(t *testing.T) {
		measurementsRaw := `[
			{"region_code":"fixture-region-A","substance_name":"fixture-substance","period":"2026-02","baseline_period":"2026-01","period_role":"current","concentration_ng_l":50,"flow_m3_day":100000,"population":500000,"excretion_factor":2.3,"excretion_factor_source":{"version":"fixture-v1","citations":[{"evidence_id":"fixture-not-real-evidence","revision":1}]}},
			{"region_code":"fixture-region-B","substance_name":"fixture-substance","period":"2026-02","baseline_period":"2026-01","period_role":"current","concentration_ng_l":60,"flow_m3_day":100000,"population":500000,"excretion_factor":2.3,"excretion_factor_source":{"version":"fixture-v1","citations":[{"evidence_id":"fixture-not-real-evidence","revision":1}]}},
			{"region_code":"fixture-region-C","substance_name":"fixture-substance","period":"2026-02","baseline_period":"2026-01","period_role":"current","concentration_ng_l":70,"flow_m3_day":100000,"population":500000,"excretion_factor":2.3,"excretion_factor_source":{"version":"fixture-v1","citations":[{"evidence_id":"fixture-not-real-evidence","revision":1}]}}
		]`
		baselineRaw := `[
			{"region_code":"fixture-region-A","substance_name":"fixture-substance","period":"2026-01","baseline_period":"2025-12","period_role":"baseline","consumption":10,"excretion_factor_source":{"version":"fixture-v1","citations":[{"evidence_id":"fixture-not-real-evidence","revision":1}]}},
			{"region_code":"fixture-region-B","substance_name":"fixture-substance","period":"2026-01","baseline_period":"2025-12","period_role":"baseline","consumption":0,"excretion_factor_source":{"version":"fixture-v1","citations":[{"evidence_id":"fixture-not-real-evidence","revision":1}]}}
		]`
		measurements := managedSource("measurements", measurementsRaw)
		baseline := managedSource("baseline", baselineRaw)
		compute := map[string]any{
			"id":   "compute",
			"type": "logic",
			"metadata": map[string]any{
				"logic": map[string]any{
					"operator":    "compute",
					"expression":  "concentration_ng_l * flow_m3_day * excretion_factor / population",
					"outputField": "consumption",
				},
			},
		}
		aggregate := map[string]any{
			"id":   "aggregate",
			"type": "logic",
			"metadata": map[string]any{
				"logic": map[string]any{
					"operator": "aggregate",
					"groupBy":  []any{"region_code", "substance_name", "period", "baseline_period", "period_role", "excretion_factor_source.version"},
					"metrics":  []any{map[string]any{"field": "consumption", "fn": "avg", "as": "consumption"}},
				},
			},
		}
		comparison := map[string]any{
			"id":   "comparison",
			"type": "logic",
			"metadata": map[string]any{
				"logic": map[string]any{
					"operator":    "compute",
					"outputField": "change_pct",
					"periodComparison": map[string]any{
						"regionField":         "region_code",
						"substanceField":      "substance_name",
						"periodField":         "period",
						"baselinePeriodField": "baseline_period",
						"valueField":          "consumption",
					},
				},
			},
		}
		current := map[string]any{
			"id":   "current",
			"type": "logic",
			"metadata": map[string]any{
				"logic": map[string]any{
					"operator":     "filter",
					"fieldPath":    "period_role",
					"comparator":   "equals",
					"compareValue": "current",
				},
			},
		}
		rank := map[string]any{
			"id":   "rank",
			"type": "logic",
			"metadata": map[string]any{
				"logic": map[string]any{
					"operator":   "sort_limit",
					"sortField":  "consumption",
					"sortOrder":  "desc",
					"limit":      10,
					"dedupeKeys": []any{"region_code", "substance_name", "period", "excretion_factor_source.version"},
				},
			},
		}
		chart := managedSink("chart")
		md := managedSink("markdown")
		md["type"] = "markdown"

		doc := map[string]any{
			"nodes": []any{measurements, baseline, compute, aggregate, comparison, current, rank, chart, md},
			"connections": []any{
				managedEdge("e1", "measurements", "compute"),
				managedEdge("e2", "compute", "aggregate"),
				managedEdge("e3", "aggregate", "comparison"),
				managedEdge("e4", "baseline", "comparison"),
				managedEdge("e5", "comparison", "current"),
				managedEdge("e6", "current", "rank"),
				managedEdge("e7", "rank", "chart"),
				managedEdge("e8", "rank", "markdown"),
			},
		}
		runner, executions, run := managedFixture(t, doc)
		if err := runner.Run(context.Background(), executions, run); err != nil {
			t.Fatalf("wastewater blueprint run failed: %v", err)
		}
		nodes := committedManagedNodes(t, executions, run.CanvasID)
		for _, sinkID := range []string{"chart", "markdown"} {
			meta := nodes[sinkID]["metadata"].(map[string]any)
			rows, ok := meta["computed"].([]any)
			if !ok || len(rows) != 3 {
				t.Fatalf("%s expected 3 ranked rows, got %+v", sinkID, meta["computed"])
			}
			byRegion := map[string]map[string]any{}
			for _, raw := range rows {
				row := raw.(map[string]any)
				byRegion[row["region_code"].(string)] = row
			}
			if rows[0].(map[string]any)["region_code"] != "fixture-region-C" || rows[1].(map[string]any)["region_code"] != "fixture-region-B" || rows[2].(map[string]any)["region_code"] != "fixture-region-A" {
				t.Fatalf("%s unexpected ranking order: %+v", sinkID, rows)
			}
			rowA := byRegion["fixture-region-A"]
			if rowA["change_pct"] != float64(130) || rowA["change_pct_status"] != "ok" {
				t.Fatalf("fixture-region-A unexpected change_pct: %+v", rowA)
			}
			rowB := byRegion["fixture-region-B"]
			if rowB["change_pct"] != nil || rowB["change_pct_status"] != "zero_baseline" {
				t.Fatalf("fixture-region-B expected nil + zero_baseline, got %+v", rowB)
			}
			rowC := byRegion["fixture-region-C"]
			if rowC["change_pct"] != nil || rowC["change_pct_status"] != "missing_baseline" {
				t.Fatalf("fixture-region-C expected nil + missing_baseline, got %+v", rowC)
			}
		}
	})

	t.Run("nps", func(t *testing.T) {
		inputJSON, err := json.Marshal([]any{substanceInput(t, "nps-substitution-screening")})
		if err != nil {
			t.Fatal(err)
		}
		inputNode := managedSource("input", string(inputJSON))
		skillNode := map[string]any{
			"id":       "skill",
			"type":     "skill",
			"metadata": map[string]any{},
		}
		extractNode := map[string]any{
			"id":   "extract",
			"type": "logic",
			"metadata": map[string]any{
				"logic": map[string]any{
					"operator":  "json_extract",
					"fieldPath": "emerging_substances",
				},
			},
		}
		aggregateNode := map[string]any{
			"id":   "aggregate",
			"type": "logic",
			"metadata": map[string]any{
				"logic": map[string]any{
					"operator": "aggregate",
					"groupBy":  []any{"substance_name", "category"},
					"metrics":  []any{map[string]any{"field": "substance_name", "fn": "count", "as": "record_count"}},
				},
			},
		}
		chartNode := managedSink("chart")
		mdNode := managedSink("markdown")
		mdNode["type"] = "markdown"

		doc := map[string]any{
			"nodes": []any{inputNode, skillNode, extractNode, aggregateNode, chartNode, mdNode},
			"connections": []any{
				managedEdge("e1", "input", "skill"),
				managedEdge("e2", "skill", "extract"),
				managedEdge("e3", "extract", "aggregate"),
				managedEdge("e4", "aggregate", "chart"),
				managedEdge("e5", "aggregate", "markdown"),
			},
		}
		runner, executions, run := managedSkillBlueprintFixture(t, "nps-substitution-screening", doc)
		if err := runner.Run(context.Background(), executions, run); err != nil {
			t.Fatalf("nps blueprint run failed: %v", err)
		}
		nodes := committedManagedNodes(t, executions, run.CanvasID)
		for _, sinkID := range []string{"chart", "markdown"} {
			meta := nodes[sinkID]["metadata"].(map[string]any)
			rows, ok := meta["computed"].([]any)
			if !ok || len(rows) != 1 {
				t.Fatalf("%s expected 1 aggregated row, got %+v", sinkID, meta["computed"])
			}
			row := rows[0].(map[string]any)
			if row["substance_name"] != "fixture-analyte" || row["category"] != "unconfirmed" || row["record_count"] != float64(1) {
				t.Fatalf("%s unexpected nps aggregated row: %+v", sinkID, row)
			}
		}
	})

	t.Run("precursor", func(t *testing.T) {
		precursorSource := map[string]any{
			"id":   "precursor",
			"type": "data-source",
			"metadata": map[string]any{
				"dataSource": map[string]any{
					"format":  "intel",
					"rawText": `[{"region_code":"fixture-region","precursor_name":"fixture-precursor","related_substance":"fixture-substance","period":"2026-02","event_at":"2026-02-03T08:00:00Z","quantity":120,"quantity_unit":"kg","anomaly_type":"fixture-anomaly","citations":[{"evidence_id":"fixture-precursor","revision":1}]}]`,
				},
			},
		}
		monitoringSource := map[string]any{
			"id":   "monitoring",
			"type": "data-source",
			"metadata": map[string]any{
				"dataSource": map[string]any{
					"format":  "intel",
					"rawText": `[{"region_code":"fixture-region","substance_name":"fixture-substance","period":"2026-02","observed_at":"2026-02-10T00:00:00Z","concentration_ng_l":50,"citations":[{"evidence_id":"fixture-monitor","revision":2}]}]`,
				},
			},
		}
		joinNode := map[string]any{
			"id":   "join",
			"type": "logic",
			"metadata": map[string]any{
				"logic": map[string]any{
					"operator": "join",
					"joinMode": "left",
					"joinKeys": []any{
						map[string]any{"left": "region_code", "right": "region_code"},
						map[string]any{"left": "related_substance", "right": "substance_name"},
						map[string]any{"left": "period", "right": "period"},
					},
				},
			},
		}
		coChart := managedSink("co-chart")

		inputJSON, err := json.Marshal([]any{substanceInput(t, "precursor-substance-correlation")})
		if err != nil {
			t.Fatal(err)
		}
		inputNode := managedSource("input", string(inputJSON))
		skillNode := map[string]any{
			"id":       "skill",
			"type":     "skill",
			"metadata": map[string]any{},
		}
		extractNode := map[string]any{
			"id":   "extract",
			"type": "logic",
			"metadata": map[string]any{
				"logic": map[string]any{
					"operator":  "json_extract",
					"fieldPath": "correlations",
				},
			},
		}
		rankNode := map[string]any{
			"id":   "rank",
			"type": "logic",
			"metadata": map[string]any{
				"logic": map[string]any{
					"operator":   "sort_limit",
					"sortField":  "lag_days",
					"sortOrder":  "desc",
					"limit":      10,
					"dedupeKeys": []any{},
				},
			},
		}
		chartNode := managedSink("chart")
		mdNode := managedSink("markdown")
		mdNode["type"] = "markdown"

		doc := map[string]any{
			"nodes": []any{precursorSource, monitoringSource, joinNode, coChart, inputNode, skillNode, extractNode, rankNode, chartNode, mdNode},
			"connections": []any{
				map[string]any{"id": "e1", "fromNodeId": "precursor", "toNodeId": "join", "toHandleId": "input_a"},
				map[string]any{"id": "e2", "fromNodeId": "monitoring", "toNodeId": "join", "toHandleId": "input_b"},
				managedEdge("e3", "join", "co-chart"),
				managedEdge("e4", "input", "skill"),
				managedEdge("e5", "skill", "extract"),
				managedEdge("e6", "extract", "rank"),
				managedEdge("e7", "rank", "chart"),
				managedEdge("e8", "rank", "markdown"),
			},
		}
		runner, executions, run := managedSkillBlueprintFixture(t, "precursor-substance-correlation", doc)
		if err := runner.Run(context.Background(), executions, run); err != nil {
			t.Fatalf("precursor blueprint run failed: %v", err)
		}
		nodes := committedManagedNodes(t, executions, run.CanvasID)
		coMeta := nodes["co-chart"]["metadata"].(map[string]any)
		coRows, ok := coMeta["computed"].([]any)
		if !ok || len(coRows) != 1 {
			t.Fatalf("co-chart expected 1 joined row, got %+v", coMeta["computed"])
		}
		coRow := coRows[0].(map[string]any)
		if coRow["region_code"] != "fixture-region" || coRow["precursor_name"] != "fixture-precursor" || coRow["concentration_ng_l"] != float64(50) {
			t.Fatalf("co-chart unexpected joined row: %+v", coRow)
		}
		for _, sinkID := range []string{"chart", "markdown"} {
			meta := nodes[sinkID]["metadata"].(map[string]any)
			rows, ok := meta["computed"].([]any)
			if !ok || len(rows) != 1 {
				t.Fatalf("%s expected 1 correlation row, got %+v", sinkID, meta["computed"])
			}
			row := rows[0].(map[string]any)
			if row["region_code"] != "R1" || row["precursor_name"] != "fixture-precursor" || row["related_substance"] != "fixture-analyte" || row["lag_days"] != float64(0) {
				t.Fatalf("%s unexpected precursor correlation row: %+v", sinkID, row)
			}
		}
	})
}
