package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestT08PatchSchemaIncludesStructuredItems(t *testing.T) {
	schema := cloudAgentPatchSchema()
	props := schema["properties"].(map[string]any)
	for _, key := range []string{"groupBy", "metrics", "joinKeys", "dedupeKeys", "series", "sourceBindingSourceIds"} {
		property, ok := props[key].(map[string]any)
		if !ok || property["type"] != "array" || property["items"] == nil {
			t.Fatalf("%s lacks typed array items: %#v", key, property)
		}
	}
	raw, _ := json.Marshal(schema)
	var s jsonschema.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, patch := range []map[string]any{
		{"groupBy": []any{1.0}}, {"metrics": []any{map[string]any{"field": "a", "fn": "sum", "as": "x", "unknown": true}}},
		{"sourceBindingTimeWindow": map[string]any{"kind": "7d", "unknown": true}},
	} {
		if resolved.Validate(patch) == nil {
			t.Fatalf("published schema accepted invalid patch: %#v", patch)
		}
	}
}

func TestT08StructuredReadbackDoesNotExposeUnknownMetadata(t *testing.T) {
	doc := map[string]any{"nodes": []map[string]any{{"id": "logic", "type": "logic", "metadata": map[string]any{"logic": map[string]any{
		"operator": "aggregate", "groupBy": []any{"region"}, "storageKey": "secret", "metrics": []any{map[string]any{"field": "v", "fn": "sum", "as": "total", "secret": "private"}},
	}}}}}
	result, err := cloudAgentCanvasState(nil, "user", "canvas", doc, 0, []string{"logic"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	item := result.(map[string]any)["nodes"].([]any)[0].(map[string]any)
	logic, ok := item["logic"].(map[string]any)
	if !ok {
		t.Fatal("registered structured root missing")
	}
	if logic["storageKey"] != nil || !reflect.DeepEqual(logic["groupBy"], []any{"region"}) {
		t.Fatalf("unknown metadata leaked or known value lost: %#v", logic)
	}
	if metrics, ok := logic["metrics"].([]any); ok {
		for _, metric := range metrics {
			if metric.(map[string]any)["secret"] != nil {
				t.Fatal("nested unknown field leaked")
			}
		}
	}
}

func TestT08CanvasApplyOpsPersistsStructuredContracts(t *testing.T) {
	s, _, args := agentMediaFixture(t)
	run, state := agentMediaRun(t, s, args, "auto")
	cases := []struct {
		id, kind string
		patch    map[string]any
	}{
		{"t08-source", "data-source", map[string]any{"sourceBindingKind": "intel_query", "sourceBindingRefId": "ref", "sourceBindingQuery": "query", "sourceBindingSourceIds": []any{"src"}, "sourceBindingTimeWindow": map[string]any{"kind": "custom", "from": "2026-01-01", "to": "2026-01-31"}, "sourceBindingToolName": "search", "sourceBindingToolArgs": map[string]any{"filters": []any{"one"}, "filter": nil, "values": []any{nil, 1.0}}, "sourceBindingToolVersion": 1.0, "sourceBindingSchemaHash": "approved-hash", "sourceBindingOutputSchema": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "sourceBindingWindowArguments": map[string]any{"from": "start", "to": "end"}}},
		{"t08-logic", "logic", map[string]any{"operator": "aggregate", "groupBy": []any{"region", "substance"}, "metrics": []any{map[string]any{"field": "value", "fn": "p95", "as": "p95"}, map[string]any{"field": "", "fn": "count", "as": "n"}}, "joinKeys": []any{map[string]any{"left": "region", "right": "area"}}, "joinMode": "left", "dedupeKeys": []any{"id"}, "limit": 10.0}},
		{"t08-compute", "logic", map[string]any{"operator": "compute", "fieldPath": "", "outputField": "delta", "periodComparison": map[string]any{"regionField": "region", "substanceField": "substance", "periodField": "period", "baselinePeriodField": "baseline", "valueField": "value"}}},
		{"t08-chart", "chart", map[string]any{"chartKind": "line", "xKey": "region", "series": []any{"p95", "n"}}},
		{"t08-skill", "skill", map[string]any{"skillId": "skill", "skillVersion": "v1", "skillHash": "sha256:test"}},
	}
	ops := []map[string]any{}
	for _, tt := range cases {
		ops = append(ops, map[string]any{"type": "add_node", "id": tt.id, "nodeType": tt.kind}, map[string]any{"type": "update_node", "id": tt.id, "patch": tt.patch})
	}
	call := cloudAgentCall{ID: "t08-create"}
	call.Function.Name = "canvas_apply_ops"
	raw, _ := json.Marshal(map[string]any{"snapshotHash": args.SnapshotHash, "ops": ops})
	call.Function.Arguments = string(raw)
	state.Calls = []cloudAgentCall{call}
	if err := s.advanceCloudAgentTool(run, &state); err != nil {
		t.Fatal(err)
	}
	saved, err := s.repo.CanvasProjectForUser("user", "agent-canvas")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := creationDocument(saved.PayloadJSON)
	if err != nil {
		t.Fatal(err)
	}
	nodes := map[string]map[string]any{}
	for _, node := range creationMaps(doc["nodes"]) {
		nodes[stringValue(node["id"])] = node
	}
	for _, tt := range cases {
		node := nodes[tt.id]
		if node == nil {
			t.Fatalf("canvas_apply_ops did not persist %s: %+v", tt.id, state.Events)
		}
		descriptor, _ := canvasCapabilityRegistry.Resolve(tt.kind)
		for key, want := range tt.patch {
			var got any = node
			for _, part := range strings.Split(descriptor.PatchFields[key].Path, ".") {
				obj, ok := got.(map[string]any)
				if !ok {
					t.Fatalf("missing object in %s.%s", tt.id, key)
				}
				got = obj[part]
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("persisted %s.%s = %#v, want %#v", tt.id, key, got, want)
			}
		}
	}
	read := cloudAgentCall{ID: "t08-read"}
	read.Function.Name = "canvas_get_state"
	read.Function.Arguments = `{"nodeIds":["t08-source","t08-logic","t08-compute","t08-chart","t08-skill"]}`
	readback, err := cloudAgentReadTool(s.repo, "user", &state, read, s)
	if err != nil {
		t.Fatal(err)
	}
	projected := map[string]map[string]any{}
	for _, node := range creationMaps(readback.(map[string]any)["nodes"]) {
		projected[stringValue(node["id"])] = node
	}
	for _, tt := range cases {
		descriptor, _ := canvasCapabilityRegistry.Resolve(tt.kind)
		for key, want := range tt.patch {
			var got any = projected[tt.id]
			for _, part := range strings.Split(strings.TrimPrefix(descriptor.PatchFields[key].Path, "metadata."), ".") {
				object, ok := got.(map[string]any)
				if !ok {
					t.Fatalf("readback lost %s.%s: %#v", tt.id, key, got)
				}
				got = object[part]
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("readback %s.%s=%#v, want %#v", tt.id, key, got, want)
			}
		}
	}
	// A rejected second call must not save its preceding valid operation either.
	before := saved.PayloadJSON
	run, _ = s.repo.CloudAgent("user", run.ID)
	state, _ = cloudAgentDecode(run)
	state.CallIndex = 0
	call.ID = "t08-invalid"
	raw, _ = json.Marshal(map[string]any{"snapshotHash": cloudAgentCanvasHash(doc), "ops": []map[string]any{
		{"type": "update_node", "id": "t08-chart", "patch": map[string]any{"title": "must not save"}},
		{"type": "update_node", "id": "t08-logic", "patch": map[string]any{"metrics": []any{map[string]any{"field": "a", "fn": "sum", "as": "duplicate"}, map[string]any{"field": "b", "fn": "avg", "as": "duplicate"}}}},
	}})
	call.Function.Arguments = string(raw)
	state.Calls = []cloudAgentCall{call}
	if err := s.advanceCloudAgentTool(run, &state); err != nil {
		t.Fatal(err)
	}
	last := state.Events[len(state.Events)-1]
	if last.Type != "tool_failed" || !strings.Contains(stringValue(last.Payload["text"]), "metrics.as") {
		t.Fatalf("expected duplicate metric rejection, got %+v", last)
	}
	saved, err = s.repo.CanvasProjectForUser("user", "agent-canvas")
	if err != nil {
		t.Fatal(err)
	}
	if saved.PayloadJSON != before {
		t.Fatal("invalid canvas_apply_ops partially persisted")
	}
}
