package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

// CanvasWorkflowManagedRunner executes an already admitted frozen canvas. Model,
// source and report work use their existing managed adapters, not a second engine.
type CanvasWorkflowManagedRunner struct {
	Tools           Tools
	WorkerID        string
	NodeBudgetMicro int64
	MaxTokens       int
	Timeout         time.Duration
}

type CanvasWorkflowManagedResult struct {
	Status             string                               `json:"status"`
	RunID              string                               `json:"run_id"`
	CanvasID           string                               `json:"canvas_id"`
	Revision           int64                                `json:"revision"`
	WrittenPayloadHash string                               `json:"written_payload_hash,omitempty"`
	NodeSnapshots      map[string]CanvasNodeSnapshot        `json:"node_snapshots"`
	Nodes              map[string]CanvasWorkflowManagedNode `json:"nodes"`
	ReportID           string                               `json:"report_id,omitempty"`
	ReportNodeID       string                               `json:"report_node_id,omitempty"`
	ConversationID     string                               `json:"conversation_id,omitempty"`
	ReportPayload      map[string]any                       `json:"report_payload,omitempty"`
	Error              string                               `json:"error,omitempty"`
}

type CanvasWorkflowManagedNode struct {
	Items      []any                          `json:"items"`
	TrueItems  []any                          `json:"true_items,omitempty"`
	FalseItems []any                          `json:"false_items,omitempty"`
	Evidence   []CanvasWorkflowEvidence       `json:"evidence,omitempty"`
	Coverage   []CanvasWorkflowSourceCoverage `json:"coverage,omitempty"`
	Source     *CanvasWorkflowSourceResult    `json:"source,omitempty"`
	Skill      *CanvasWorkflowSkillResult     `json:"skill,omitempty"`
	Status     string                         `json:"status"`
	Error      string                         `json:"error,omitempty"`
}

type managedCanvasEdge struct{ from, to, fromPort, toPort string }
type managedCanvasGraph struct {
	doc      map[string]any
	nodes    map[string]map[string]any
	order    []string
	incoming map[string][]managedCanvasEdge
}

func preflightManagedCanvas(raw string) (managedCanvasGraph, error) {
	graph := managedCanvasGraph{nodes: map[string]map[string]any{}, incoming: map[string][]managedCanvasEdge{}}
	invalid := func(reason string) (managedCanvasGraph, error) {
		return graph, fmt.Errorf("%w: %s", ErrInvalid, reason)
	}
	if json.Unmarshal([]byte(raw), &graph.doc) != nil || graph.doc == nil {
		return invalid("invalid canvas JSON")
	}
	nodes, ok := graph.doc["nodes"].([]any)
	if !ok || len(nodes) > 1000 {
		return invalid("nodes must be a bounded array")
	}
	original := []string{}
	for _, raw := range nodes {
		n, ok := raw.(map[string]any)
		if !ok {
			return invalid("invalid node")
		}
		id := sourceString(n, "id")
		if id == "" || graph.nodes[id] != nil {
			return invalid("empty or duplicate node ID")
		}
		graph.nodes[id] = n
		original = append(original, id)
	}
	edges, ok := graph.doc["connections"].([]any)
	if !ok || len(edges) > 10000 {
		return invalid("connections must be a bounded array")
	}
	edgeIDs := map[string]bool{}
	pairs := map[string]bool{}
	for _, raw := range edges {
		e, ok := raw.(map[string]any)
		if !ok {
			return invalid("invalid connection")
		}
		id, from, to := sourceString(e, "id"), sourceString(e, "fromNodeId"), sourceString(e, "toNodeId")
		if id == "" || edgeIDs[id] {
			return invalid("empty or duplicate connection ID")
		}
		edgeIDs[id] = true
		if graph.nodes[from] == nil || graph.nodes[to] == nil {
			return invalid("dangling connection")
		}
		relation := sourceString(e, "relation")
		switch relation {
		case "storyboard-output", "storyboard-asset-reference", "batch-output", "batch-input":
			continue
		case "":
		default:
			return invalid("unknown connection relation")
		}
		fromPort, toPort := sourceString(e, "fromHandleId"), sourceString(e, "toHandleId")
		signature := strings.Join([]string{from, to, fromPort, toPort}, "\x00")
		if pairs[signature] {
			return invalid("duplicate data connection")
		}
		pairs[signature] = true
		source, target := graph.nodes[from], graph.nodes[to]
		if !managedCanvasType(sourceString(source, "type")) || !managedCanvasType(sourceString(target, "type")) {
			return invalid("unsupported node on data edge")
		}
		if sourceString(target, "type") == "data-source" {
			return invalid("data source has no input port")
		}
		if fromPort != "" && fromPort != "output" && fromPort != "source" {
			if sourceString(source, "type") != "logic" || wfLogicOperator(source) != "if_else" || (fromPort != "true" && fromPort != "false") {
				return invalid("invalid source port")
			}
		}
		if toPort != "" && toPort != "input" && toPort != "target" {
			if sourceString(target, "type") != "logic" || !wfIsDualInput(wfLogicOperator(target)) || (toPort != "input_a" && toPort != "input_b") {
				return invalid("invalid target port")
			}
		}
		graph.incoming[to] = append(graph.incoming[to], managedCanvasEdge{from, to, fromPort, toPort})
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("%w: dataflow cycle", ErrInvalid)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, edge := range graph.incoming[id] {
			if err := visit(edge.from); err != nil {
				return err
			}
		}
		state[id] = 2
		if managedCanvasType(sourceString(graph.nodes[id], "type")) {
			graph.order = append(graph.order, id)
		}
		return nil
	}
	for _, id := range original {
		if err := visit(id); err != nil {
			return graph, err
		}
	}
	return graph, nil
}
func managedCanvasType(kind string) bool {
	switch kind {
	case "data-source", "logic", "skill", "chart", "markdown", "text":
		return true
	}
	return false
}

func (r CanvasWorkflowManagedRunner) Run(ctx context.Context, executions CanvasWorkflowExecutions, admitted model.CanvasWorkflowExecution) (retErr error) {
	if executions.DB == nil || r.Tools.DB != executions.DB || admitted.ID == "" || admitted.UserID == "" {
		return ErrInvalid
	}
	run, err := executions.Get(ctx, admitted.UserID, admitted.ID)
	if err != nil {
		return err
	}
	if run.Status == "succeeded" {
		return nil
	}
	if run.Status == "cancelled" || run.Status == "cancel_requested" {
		return context.Canceled
	}
	if run.Status == "failed" {
		return fmt.Errorf("%w: workflow already failed", ErrCanvasWorkflowConflict)
	}
	worker := r.WorkerID
	if worker == "" {
		worker = NewID("canvas_worker_")
	}
	// A unique lease owner per invocation prevents same-host concurrent calls from
	// sharing a fencing token. Renew never creates a lease after it has been lost.
	worker += ":" + NewID("")
	token, err := executions.Acquire(ctx, run.UserID, run.ID, worker)
	if err != nil {
		return err
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	leases := RunLeases{DB: executions.DB, Now: executions.Now}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-callCtx.Done():
				return
			case <-ticker.C:
				if leases.Renew(run.ID, worker, token) != nil {
					cancel()
					return
				}
				current, e := executions.Get(callCtx, run.UserID, run.ID)
				if e != nil || current.Status == "cancel_requested" || current.Status == "cancelled" {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		close(stop)
		wg.Wait()
		if retErr != nil && !errors.Is(retErr, ErrModelUnknownOutcome) {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			cleanupErr := executions.CloseFailedRun(cleanupCtx, run.UserID, run.ID, worker, token)
			cleanupCancel()
			retErr = errors.Join(retErr, cleanupErr)
		}
		leases.Release(run.ID, worker, token)
	}()
	cp, err := DecodeCanvasWorkflowCheckpoint(run)
	if err != nil {
		return err
	}
	if cp.Nodes == nil {
		cp.Nodes = map[string]CanvasWorkflowNodeCheckpoint{}
	}
	result := CanvasWorkflowManagedResult{Status: "running", RunID: run.ID, CanvasID: run.CanvasID, Revision: run.CanvasRevision, Nodes: map[string]CanvasWorkflowManagedNode{}, NodeSnapshots: map[string]CanvasNodeSnapshot{}}
	if len(cp.Result) > 0 {
		if err := json.Unmarshal(cp.Result, &result); err != nil {
			return err
		}
	}
	if result.Nodes == nil {
		result.Nodes = map[string]CanvasWorkflowManagedNode{}
	}
	if result.NodeSnapshots == nil {
		result.NodeSnapshots = map[string]CanvasNodeSnapshot{}
	}
	graph, err := preflightManagedCanvas(run.CanvasJSON)
	if err != nil {
		return r.managedFailure(executions, run, worker, token, cp, &result, err)
	}
	saveCheckpoint := func(stage string) error {
		current, e := executions.Get(callCtx, run.UserID, run.ID)
		if e != nil {
			return e
		}
		persisted, e := DecodeCanvasWorkflowCheckpoint(current)
		if e != nil {
			return e
		}
		// PrepareModelNode may have added identity while the adapter was running.
		for id, node := range persisted.Nodes {
			local, ok := cp.Nodes[id]
			if !ok {
				cp.Nodes[id] = node
				continue
			}
			if local.TaskKey == "" {
				local.TaskKey = node.TaskKey
				local.RequestHash = node.RequestHash
				cp.Nodes[id] = local
			}
		}
		cp.Stage = stage
		cp.Result, _ = json.Marshal(result)
		next, e := executions.Checkpoint(callCtx, run.UserID, run.ID, worker, token, current.Revision, cp)
		if e == nil {
			run = next
		}
		return e
	}
	if cp.Stage != "saving_result" && cp.Stage != "saving_report" {
		var failed error
		for _, id := range graph.order {
			if err := callCtx.Err(); err != nil {
				return r.managedFailure(executions, run, worker, token, cp, &result, err)
			}
			current, e := executions.Get(callCtx, run.UserID, run.ID)
			if e != nil {
				return e
			}
			if current.Status == "cancel_requested" || current.Status == "cancelled" {
				return context.Canceled
			}
			if e := executions.DB.Transaction(func(tx *gorm.DB) error { return executions.fence(tx, run.ID, worker, token) }); e != nil {
				return e
			}
			if prior, ok := cp.Nodes[id]; ok && (prior.Status == "succeeded" || prior.Status == "blocked" || prior.Status == "failed") {
				var node CanvasWorkflowManagedNode
				if json.Unmarshal(prior.Result, &node) != nil {
					return ErrInvalid
				}
				result.Nodes[id] = node
				if prior.Status != "succeeded" {
					failed = fmt.Errorf("node %s: %s", id, prior.Error)
				}
				continue
			}
			node := graph.nodes[id]
			edges := graph.incoming[id]
			var a, b []any
			var evidence []CanvasWorkflowEvidence
			var coverage []CanvasWorkflowSourceCoverage
			blocked := false
			for idx, edge := range edges {
				up := result.Nodes[edge.from]
				if up.Status != "succeeded" {
					blocked = true
					continue
				}
				items := up.Items
				if sourceString(graph.nodes[edge.from], "type") == "logic" && wfLogicOperator(graph.nodes[edge.from]) == "if_else" {
					if edge.fromPort == "false" {
						items = up.FalseItems
					} else {
						items = up.TrueItems
					}
				}
				if wfIsDualInput(wfLogicOperator(node)) && (edge.toPort == "input_b" || (edge.toPort == "" && idx > 0)) {
					b = append(b, items...)
				} else {
					a = append(a, items...)
				}
				evidence = mergeManagedEvidence(evidence, up.Evidence...)
				coverage = append(coverage, up.Coverage...)
			}
			started := time.Now()
			output := CanvasWorkflowManagedNode{Items: []any{}, Status: "succeeded", Evidence: evidence, Coverage: coverage}
			var nodeErr error
			if blocked {
				output.Status = "blocked"
				nodeErr = fmt.Errorf("upstream node failed")
			} else {
				tools := r.Tools
				tools.RunID = run.ID
				scope, e := ResolveScope(executions.DB, run.UserID, run.WorkspaceID)
				if e != nil {
					return e
				}
				ss := SubjectScope{Scope: scope, Subject: AgentSubject{Kind: SubjectWorkspace, ID: run.WorkspaceID}}
				switch sourceString(node, "type") {
				case "data-source":
					source, e := tools.ResolveCanvasWorkflowSource(callCtx, ss, run.UserID, node, run.ID+":"+id, run.CreatedAt)
					nodeErr = e
					output.Source = &source
					output.Items = source.Items
					output.Evidence = source.Evidence
					output.Coverage = []CanvasWorkflowSourceCoverage{source.Coverage}
					if e == nil {
						for i, item := range output.Items {
							if row, ok := item.(map[string]any); ok && i < len(source.ItemEvidence) {
								refs := []any{}
								for _, ev := range source.ItemEvidence[i] {
									refs = append(refs, map[string]any{"evidence_id": ev.EvidenceID, "revision": ev.Revision, "chunk_index": ev.ChunkIndex})
								}
								if len(refs) > 0 {
									row["citations"] = wfMergeProvenanceValue(row["citations"], refs)
								}
							}
						}
						if len(output.Evidence) == 0 {
							output.Evidence = extractManagedInlineEvidence(ss.Scope, node, output.Items)
						}
					}
				case "logic":
					evaluated := wfEvalLogicNode(node, a, b)
					output.Items = evaluated.outputItems
					output.TrueItems = evaluated.trueItems
					output.FalseItems = evaluated.falseItems
					if evaluated.errText != "" {
						nodeErr = errors.New(evaluated.errText)
					}
				case "skill":
					meta := ensureMap(node, "metadata")
					approved := SkillRef{ID: sourceString(meta, "skillId"), Version: sourceString(meta, "skillVersion"), Hash: sourceString(meta, "skillHash")}
					var input any = a
					if len(a) == 1 {
						input = a[0]
					}
					modelID := ""
					if gateway, ok := tools.Model.(*OpenAICompatGateway); ok {
						modelID = gateway.Model
					}
					budget := r.NodeBudgetMicro
					if budget <= 0 {
						budget = run.BudgetMicro
						count := int64(0)
						for _, candidate := range graph.nodes {
							if sourceString(candidate, "type") == "skill" {
								count++
							}
						}
						if count > 0 {
							budget /= count
						}
					}
					maxTokens := r.MaxTokens
					if maxTokens <= 0 {
						maxTokens = 4096
					}
					material := MaterialScope{}
					for _, ev := range evidence {
						if ev.DocumentID != "" {
							material.DocumentIDs = append(material.DocumentIDs, ev.DocumentID)
						}
					}
					if object, ok := input.(map[string]any); ok {
						if materialValue, ok := object["material_scope"]; ok {
							_ = decodeInto(materialValue, &material)
						}
					}
					skill, e := tools.ExecuteCanvasWorkflowSkill(callCtx, CanvasWorkflowSkillRequest{Executions: &executions, WorkerID: worker, FencingToken: token, UserID: run.UserID, RunID: run.ID, NodeID: id, ModelID: modelID,
						ApprovedSkill: approved, Input: input, PreviousOutput: meta["skillOutput"], MaterialScope: material, BudgetMicro: budget, MaxTokens: maxTokens, TaskID: cp.Nodes[id].TaskID})
					output.Skill = &skill
					nodeErr = e
					prior := cp.Nodes[id]
					prior.TaskID = skill.TaskID
					prior.TaskKey = skill.TaskKey
					prior.RequestHash = skill.RequestHash
					prior.PromptHash = skill.PromptHash
					cp.Nodes[id] = prior
					if e == nil {
						output.Items = []any{skill.Output}
					}
					if errors.Is(e, ErrModelUnknownOutcome) || skill.TaskStatus == StatusReconciliation || (e != nil && (skill.TaskStatus == "running" || skill.TaskStatus == "queued" || skill.TaskStatus == StatusRetryWait)) {
						output.Status = "waiting_external"
						output.Error = SanitizeCanvasWorkflowError(e.Error())
						result.Nodes[id] = output
						stored := cp.Nodes[id]
						stored.Status = output.Status
						stored.Error = output.Error
						stored.Result, _ = json.Marshal(output)
						cp.Nodes[id] = stored
						result.Status = "waiting_external"
						result.Error = output.Error
						cp.Error = output.Error
						if err := saveCheckpoint("waiting_external"); err != nil {
							return err
						}
						return e
					}
				case "text", "markdown", "chart":
					output.Items = a
				}
			}
			if nodeErr != nil {
				if output.Status != "blocked" {
					output.Status = "failed"
				}
				output.Error = SanitizeCanvasWorkflowError(nodeErr.Error())
				output.Items = []any{}
				failed = fmt.Errorf("node %s: %w", id, nodeErr)
			}
			if output.Items == nil {
				output.Items = []any{}
			}
			result.Nodes[id] = output
			snapshot := CanvasNodeSnapshot{RunID: run.ID, ExecutedAt: executions.now().Format(time.RFC3339Nano), DurationMs: time.Since(started).Milliseconds(), InputCount: len(a) + len(b), OutputCount: len(output.Items), Status: managedSnapshotStatus(output.Status), Error: output.Error}
			snapshot.SampleItems = output.Items
			if len(snapshot.SampleItems) > 5 {
				snapshot.SampleItems = snapshot.SampleItems[:5]
			}
			result.NodeSnapshots[id] = snapshot
			stored := cp.Nodes[id]
			stored.Status = output.Status
			stored.Error = output.Error
			stored.Result, _ = json.Marshal(output)
			cp.Nodes[id] = stored
			if err := saveCheckpoint("running"); err != nil {
				return err
			}
		}
		if failed != nil {
			return r.managedFailure(executions, run, worker, token, cp, &result, failed)
		}
		result.Status = "saving_result"
		if err := saveCheckpoint("saving_result"); err != nil {
			return err
		}
	}
	if result.Revision == run.CanvasRevision {
		if err := r.commitManagedCanvas(callCtx, executions, run, worker, token, graph, &result); err != nil {
			return err
		}
		if err := saveCheckpoint("saving_result"); err != nil {
			return err
		}
	}
	var admission CanvasWorkflowAdmission
	if err := json.Unmarshal([]byte(run.RequestJSON), &admission); err != nil {
		return err
	}
	if admission.SaveReport && result.ReportID == "" {
		if result.ReportPayload == nil {
			payload, nodeID, err := managedCanvasReport(graph, result, admission.ReportTitle)
			if err != nil {
				result.Status = "partial"
				result.Error = SanitizeCanvasWorkflowError(err.Error())
				cp.Error = SanitizeCanvasWorkflowError(err.Error())
				_ = saveCheckpoint("saving_report")
				return err
			}
			result.ReportPayload = payload
			result.ReportNodeID = nodeID
			ss := SubjectScope{Scope: Scope{TenantID: run.TenantID, WorkspaceID: run.WorkspaceID}, Subject: AgentSubject{Kind: SubjectWorkspace, ID: run.WorkspaceID}}
			conv, err := (Conversations{DB: executions.DB}).Create(run.UserID, ss, "Canvas workflow report")
			if err != nil {
				return err
			}
			result.ConversationID = conv.ID
			if err := saveCheckpoint("saving_report"); err != nil {
				return err
			}
		}
		conv, ss, err := (Conversations{DB: executions.DB}).Get(run.UserID, result.ConversationID)
		if err != nil {
			return err
		}
		tools := r.Tools
		tools.RunID = run.ID
		saved, err := (CanvasReportSaver{Tools: tools}).Save(callCtx, conv, ss, run.UserID, run.ID, result.ReportNodeID, result.ReportPayload)
		if err != nil {
			result.Status = "partial"
			result.Error = SanitizeCanvasWorkflowError(err.Error())
			cp.Error = SanitizeCanvasWorkflowError(err.Error())
			if saveErr := saveCheckpoint("saving_report"); saveErr != nil {
				return saveErr
			}
			return err
		}
		result.ReportID = saved.ReportID
	}
	result.Status = "succeeded"
	result.Error = ""
	cp.Error = ""
	cp.ReportID = result.ReportID
	return saveCheckpoint("succeeded")
}

func (r CanvasWorkflowManagedRunner) managedFailure(executions CanvasWorkflowExecutions, run model.CanvasWorkflowExecution, worker string, token int64, cp CanvasWorkflowCheckpoint, result *CanvasWorkflowManagedResult, cause error) error {
	// Persist diagnostics with a short independent context after request cancellation;
	// fencing/current authority still apply, so cancelled or superseded runs cannot write.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	current, err := executions.Get(ctx, run.UserID, run.ID)
	if err != nil {
		return errors.Join(cause, err)
	}
	if current.Status == "cancel_requested" || current.Status == "cancelled" {
		return cause
	}
	result.Status = "failed"
	result.Error = SanitizeCanvasWorkflowError(cause.Error())
	cp.Error = SanitizeCanvasWorkflowError(cause.Error())
	cp.Stage = "failed"
	cp.Result, _ = json.Marshal(result)
	_, err = executions.Checkpoint(ctx, run.UserID, run.ID, worker, token, current.Revision, cp)
	return errors.Join(cause, err)
}

func (r CanvasWorkflowManagedRunner) commitManagedCanvas(ctx context.Context, executions CanvasWorkflowExecutions, run model.CanvasWorkflowExecution, worker string, token int64, graph managedCanvasGraph, result *CanvasWorkflowManagedResult) error {
	for id, value := range result.Nodes {
		node := graph.nodes[id]
		if node == nil {
			continue
		}
		meta := ensureMap(node, "metadata")
		meta["content"] = ""
		if len(value.Items) > 0 {
			meta["content"] = wfFormatItems(value.Items)
		}
		meta["computed"] = value.Items
		trueItems, falseItems := value.TrueItems, value.FalseItems
		if trueItems == nil {
			trueItems = []any{}
		}
		if falseItems == nil {
			falseItems = []any{}
		}
		meta["computedBranches"] = map[string]any{"true": trueItems, "false": falseItems}
		if value.Skill != nil {
			meta["skillOutput"] = value.Skill.Output
		}
		meta["derived"] = map[string]any{"runId": run.ID, "canvasRevision": run.CanvasRevision, "status": value.Status, "evidence": value.Evidence, "coverage": value.Coverage}
		meta["lastRunSnapshot"] = result.NodeSnapshots[id]
		if value.Source != nil {
			ds := ensureMap(meta, "dataSource")
			if value.Source.LastSyncedAt != "" {
				ds["lastSyncedAt"] = value.Source.LastSyncedAt
			} else {
				delete(ds, "lastSyncedAt")
			}
		}
	}
	graph.doc["workflowRunId"] = run.ID
	graph.doc["updatedAt"] = run.CreatedAt.UTC().Format(time.RFC3339Nano)
	if err := ValidateCanvasWorkflowPayload(graph.doc); err != nil {
		return err
	}
	raw, err := json.Marshal(graph.doc)
	if err != nil {
		return err
	}
	// The authority re-derives this hash from the persisted payload through a
	// map round-trip; typed values (struct field order) break byte equality
	// with that canonical form, so the hash is computed from it as well.
	var canonical map[string]any
	if err := json.Unmarshal(raw, &canonical); err != nil {
		return err
	}
	canonicalHash := hashJSON(canonical)
	err = executions.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fresh, e := executions.get(ctx, tx, run.UserID, run.ID)
		if e != nil {
			return e
		}
		if fresh.Status == "cancel_requested" || fresh.Status == "cancelled" {
			return context.Canceled
		}
		if e := executions.fence(tx, run.ID, worker, token); e != nil {
			return e
		}
		var project model.CanvasProject
		if e := tx.First(&project, "id = ? AND user_id = ?", run.CanvasID, run.UserID).Error; e != nil {
			return e
		}
		updated := tx.Model(&model.CanvasProject{}).Where("id = ? AND user_id = ? AND revision = ?", run.CanvasID, run.UserID, run.CanvasRevision).Updates(map[string]any{"payload_json": string(raw), "revision": run.CanvasRevision + 1, "updated_at": executions.now()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errCanvasWorkflowRevisionConflict
		}
		persisted, e := DecodeCanvasWorkflowCheckpoint(fresh)
		if e != nil {
			return e
		}
		written := *result
		written.Revision = run.CanvasRevision + 1
		written.WrittenPayloadHash = canonicalHash
		persisted.Stage = "saving_result"
		persisted.Result, e = json.Marshal(written)
		if e != nil {
			return e
		}
		if _, e = executions.save(tx, fresh, persisted); e != nil {
			return e
		}
		return nil
	})
	if err == nil {
		result.Revision = run.CanvasRevision + 1
		result.WrittenPayloadHash = canonicalHash
	}
	return err
}

func managedCanvasReport(graph managedCanvasGraph, result CanvasWorkflowManagedResult, title string) (map[string]any, string, error) {
	for i := len(graph.order) - 1; i >= 0; i-- {
		id := graph.order[i]
		node := result.Nodes[id]
		for _, item := range node.Items {
			value, ok := item.(map[string]any)
			if !ok || value["claims"] == nil || value["material_scope"] == nil {
				continue
			}
			if title == "" {
				title = sourceString(value, "title")
			}
			if title == "" {
				title = "Canvas analysis"
			}
			body := sourceString(value, "body")
			if body == "" {
				body = wfFormatItems([]any{value})
			}
			payload := map[string]any{"title": title, "body": body, "claims": value["claims"], "material_scope": value["material_scope"], "coverage": value["coverage"], "structured": value}
			return payload, id, nil
		}
	}
	return nil, "", fmt.Errorf("%w: no evidence-bound structured report output", ErrInvalid)
}

// CancelCanvasWorkflow persists intent and invalidates an in-flight runner's
// fence. Uncertain model calls retain their reservation for reconciliation.
func CancelCanvasWorkflow(ctx context.Context, executions CanvasWorkflowExecutions, userID, runID string) error {
	if executions.DB == nil {
		return ErrInvalid
	}
	return executions.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		run, err := executions.get(ctx, tx, userID, runID)
		if err != nil {
			return err
		}
		if run.Status == "succeeded" || run.Status == "failed" || run.Status == "cancelled" {
			return nil
		}
		cp, err := DecodeCanvasWorkflowCheckpoint(run)
		if err != nil {
			return err
		}
		var tasks []model.FlowModelTask
		if err := tx.Joins("JOIN flow_budget_reservations ON flow_budget_reservations.id = flow_model_tasks.reservation_id").Where("flow_budget_reservations.parent_id = ?", run.ReservationID).Find(&tasks).Error; err != nil {
			return err
		}
		uncertain := false
		for _, task := range tasks {
			switch task.Status {
			case "queued", StatusRetryWait:
				update := tx.Model(&model.FlowModelTask{}).Where("id = ? AND status IN ?", task.ID, []string{"queued", StatusRetryWait}).Updates(map[string]any{"status": "failed", "last_error": "workflow cancelled", "updated_at": executions.now()})
				if update.Error != nil {
					return update.Error
				}
				if update.RowsAffected != 1 {
					uncertain = true
					continue
				}
				if _, err := (Billing{DB: tx}).Release(task.ReservationID); err != nil {
					return err
				}
			case "running", StatusReconciliation:
				uncertain = true
			}
		}
		cp.Stage = "cancelled"
		if uncertain {
			cp.Stage = "cancel_requested"
		}
		cp.Error = "workflow cancelled"
		if len(cp.Result) > 0 {
			var result CanvasWorkflowManagedResult
			if json.Unmarshal(cp.Result, &result) == nil {
				result.Status = cp.Stage
				result.Error = cp.Error
				cp.Result, _ = json.Marshal(result)
			}
		}
		if _, err := executions.save(tx, run, cp); err != nil {
			return err
		}
		// Preserve the lease row: deleting/recreating it could reset its fencing token.
		return tx.Model(&model.FlowRunLease{}).Where("run_id = ?", runID).Updates(map[string]any{"fencing_token": gorm.Expr("fencing_token + 1"), "owner": "cancelled", "expires_at": executions.now(), "updated_at": executions.now()}).Error
	})
}

func managedSnapshotStatus(nodeStatus string) string {
	switch nodeStatus {
	case "succeeded":
		return "success"
	case "blocked":
		return "skipped"
	case "failed":
		return "error"
	default:
		return nodeStatus
	}
}

func mergeManagedEvidence(existing []CanvasWorkflowEvidence, incoming ...CanvasWorkflowEvidence) []CanvasWorkflowEvidence {
	seen := make(map[string]bool, len(existing)+len(incoming))
	out := make([]CanvasWorkflowEvidence, 0, len(existing)+len(incoming))
	add := func(ev CanvasWorkflowEvidence) {
		if strings.TrimSpace(ev.EvidenceID) == "" || ev.Revision <= 0 {
			return
		}
		key := fmt.Sprintf("%s\x00%d\x00%d", ev.EvidenceID, ev.Revision, ev.ChunkIndex)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, ev)
	}
	for _, ev := range existing {
		add(ev)
	}
	for _, ev := range incoming {
		add(ev)
	}
	return out
}

func extractManagedInlineEvidence(scope Scope, node map[string]any, items []any) []CanvasWorkflowEvidence {
	type inlineCitation struct {
		EvidenceID string `json:"evidence_id"`
		Revision   int    `json:"revision"`
		DocumentID string `json:"document_id"`
		SourceID   string `json:"source_id"`
		ChunkIndex int    `json:"chunk_index"`
		Title      string `json:"title"`
		Quote      string `json:"quote"`
		Snippet    string `json:"snippet"`
		URL        string `json:"url"`
		CharStart  int    `json:"char_start"`
		CharEnd    int    `json:"char_end"`
	}
	var collected []CanvasWorkflowEvidence
	var walk func(v any, depth int)
	walk = func(v any, depth int) {
		if depth > 32 || v == nil {
			return
		}
		switch val := v.(type) {
		case map[string]any:
			if _, ok := val["evidence_id"]; ok {
				var c inlineCitation
				if decodeInto(val, &c) == nil && strings.TrimSpace(c.EvidenceID) != "" && c.Revision > 0 {
					quote := c.Quote
					if quote == "" {
						quote = c.Snippet
					}
					collected = append(collected, CanvasWorkflowEvidence{
						ResolvedEvidence: ResolvedEvidence{
							EvidenceID: strings.TrimSpace(c.EvidenceID),
							DocumentID: strings.TrimSpace(c.DocumentID),
							SourceID:   strings.TrimSpace(c.SourceID),
							Revision:   c.Revision,
							ChunkIndex: c.ChunkIndex,
							Title:      strings.TrimSpace(c.Title),
							Quote:      quote,
							URL:        strings.TrimSpace(c.URL),
						},
						TenantID:    scope.TenantID,
						WorkspaceID: scope.WorkspaceID,
						CharStart:   c.CharStart,
						CharEnd:     c.CharEnd,
					})
				}
			}
			for _, child := range val {
				walk(child, depth+1)
			}
		case []any:
			for _, child := range val {
				walk(child, depth+1)
			}
		}
	}
	walk(items, 0)
	if meta, ok := node["metadata"].(map[string]any); ok {
		if intel, ok := meta["intelSource"].(map[string]any); ok {
			walk(intel["citations"], 0)
		}
		if ds, ok := meta["dataSource"].(map[string]any); ok {
			if prov, ok := ds["provenance"].(map[string]any); ok {
				walk(prov["citations"], 0)
			}
		}
	}
	return mergeManagedEvidence(nil, collected...)
}
