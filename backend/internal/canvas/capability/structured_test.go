package capability

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func structuredValue(t *testing.T, raw string) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestStructuredPatchContracts(t *testing.T) {
	tests := []struct{ kind, patch string }{
		{"logic", `{"operator":"aggregate","groupBy":["region","substance"],"metrics":[{"field":"value","fn":"p95","as":"p95"},{"field":"","fn":"count","as":"n"}],"dedupeKeys":["id"],"limit":10}`},
		{"logic", `{"operator":"join","joinMode":"left","joinKeys":[{"left":"region","right":"area"},{"left":"substance","right":"drug"}]}`},
		{"logic", `{"operator":"compute","fieldPath":"","outputField":"delta","periodComparison":{"regionField":"region","substanceField":"substance","periodField":"period","baselinePeriodField":"baseline","valueField":"value"}}`},
		{"data-source", `{"sourceBindingKind":"mcp_tool","sourceBindingRefId":"ref","sourceBindingQuery":"query","sourceBindingSourceIds":["src"],"sourceBindingTimeWindow":{"kind":"custom","from":"2026-01-01","to":"2026-01-31","timezone":"UTC"},"sourceBindingToolName":"search","sourceBindingToolArgs":{"nested":{"tags":["x"],"limit":3}},"sourceBindingToolVersion":1}`},
		{"chart", `{"chartKind":"line","xKey":"region","series":["avg","p95"]}`},
		{"skill", `{"skillId":"skill","skillVersion":"1.0","skillHash":"sha256:abc"}`},
	}
	for _, tt := range tests {
		t.Run(tt.kind+tt.patch, func(t *testing.T) {
			d, _ := BuiltinRegistry().Resolve(tt.kind)
			node := map[string]any{"metadata": d.Metadata("")}
			if err := d.ApplyPatch(node, structuredValue(t, tt.patch)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStructuredPatchRejectsInvalidAndRemainsAtomic(t *testing.T) {
	tests := []struct{ kind, patch string }{
		{"logic", `{"groupBy":"region"}`},
		{"logic", `{"groupBy":[1]}`},
		{"logic", `{"metrics":[{"field":"value","fn":"median","as":"x"}]}`},
		{"logic", `{"metrics":[{"field":"","fn":"sum","as":"x"}]}`},
		{"logic", `{"metrics":[{"fn":"count","as":"x"}]}`},
		{"logic", `{"metrics":[{"field":"a","fn":"sum","as":"x"},{"field":"b","fn":"avg","as":"x"}]}`},
		{"logic", `{"metrics":[{"field":"a","fn":"sum","as":"x","evil":true}]}`},
		{"logic", `{"joinKeys":[{"left":"id"}]}`},
		{"logic", `{"joinKeys":[{"left":"id","right":false}]}`},
		{"logic", `{"joinMode":"outer"}`},
		{"logic", `{"dedupeKeys":[{}]}`},
		{"logic", `{"limit":0}`},
		{"logic", `{"limit":1.5}`},
		{"logic", `{"aggregateFn":"sum"}`},
		{"logic", `{"aggregateField":"value"}`},
		{"logic", `{"rightKey":"id"}`},
		{"logic", `{"periodComparison":{"regionField":"r"}}`},
		{"logic", `{"periodComparison":{"regionField":" ","substanceField":"s","periodField":"p","baselinePeriodField":"b","valueField":"v"}}`},
		{"logic", `{"periodComparison":{"regionField":"r","substanceField":"s","periodField":"p","baselinePeriodField":"b","valueField":"v"}}`},
		{"data-source", `{"sourceBindingKind":"unknown"}`},
		{"data-source", `{"sourceBindingQuery":{}}`},
		{"data-source", `{"sourceBindingSourceIds":[1]}`},
		{"data-source", `{"sourceBindingToolArgs":[]}`},
		{"data-source", `{"sourceBindingTimeWindow":{"kind":"custom","from":"2026-02-01","to":"2026-01-01"}}`},
		{"data-source", `{"sourceBindingTimeWindow":{"kind":"custom","from":"bad","to":"also bad"}}`},
		{"data-source", `{"sourceBindingTimeWindow":{"kind":"7d","evil":true}}`},
		{"data-source", `{"sourceBindingTimeWindow":{"kind":"year"}}`},
		{"chart", `{"series":[1]}`},
		{"chart", `{"chartKind":"pie"}`},
		{"skill", `{"skillVersion":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.kind+tt.patch, func(t *testing.T) {
			d, _ := BuiltinRegistry().Resolve(tt.kind)
			node := map[string]any{"title": "before", "metadata": d.Metadata("before")}
			before, _ := json.Marshal(node)
			patch := structuredValue(t, tt.patch)
			patch["title"] = "must not leak"
			if err := d.ApplyPatch(node, patch); err == nil {
				t.Fatal("invalid patch accepted")
			}
			after, _ := json.Marshal(node)
			if string(before) != string(after) {
				t.Fatalf("failed patch mutated node: %s", after)
			}
		})
	}
}

func TestStructuredProjectionPreservesNull(t *testing.T) {
	d, _ := BuiltinRegistry().Resolve("data-source")
	value := structuredValue(t, `{"sourceBinding":{"toolArgs":{"filter":null,"values":[null,1]}}}`)
	got, truncated, handled := d.ProjectStructuredField("dataSource", value, 200)
	if !handled || truncated || !reflect.DeepEqual(got, value) {
		t.Fatalf("null changed without truncation: %#v (%v)", got, truncated)
	}
}

func TestStructuredProjectionBoundsAndAllowlist(t *testing.T) {
	d, _ := BuiltinRegistry().Resolve("data-source")
	deep := map[string]any{"value": "leaf"}
	for i := 0; i < 12; i++ {
		deep = map[string]any{"child": deep}
	}
	value := map[string]any{"sourceBinding": map[string]any{"kind": "mcp_tool", "secret": "must not leak", "toolArgs": deep}, "unknown": "hidden"}
	got, truncated, handled := d.ProjectStructuredField("dataSource", value, 200)
	if !handled || !truncated {
		t.Fatal("deep registered projection was not bounded")
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "must not leak") || strings.Contains(string(raw), "hidden") || strings.Contains(string(raw), "leaf") {
		t.Fatalf("unsafe projection: %s", raw)
	}
	args := map[string]any{}
	for i := 0; i < 100; i++ {
		args[fmt.Sprint(i)] = strings.Repeat("x", 1000)
	}
	got, truncated, _ = d.ProjectStructuredField("dataSource", map[string]any{"sourceBinding": map[string]any{"toolArgs": args}}, 200)
	raw, _ = json.Marshal(got)
	if !truncated || len(raw) > 8000 {
		t.Fatalf("large arguments not bounded: %d", len(raw))
	}
	wide := map[string]any{}
	for i := 0; i < 32; i++ {
		wide[fmt.Sprint(i)] = args
	}
	got, truncated, _ = d.ProjectStructuredField("dataSource", map[string]any{"sourceBinding": map[string]any{"toolArgs": wide}}, 200)
	raw, _ = json.Marshal(got)
	if !truncated || len(raw) > 18000 {
		t.Fatalf("nested breadth bypasses total budget: %d", len(raw))
	}
}

func TestStructuredNilCollectionsRejected(t *testing.T) {
	for _, tt := range []struct {
		kind, key string
		value     any
	}{{"logic", "groupBy", []any(nil)}, {"logic", "metrics", []map[string]any(nil)}, {"data-source", "sourceBindingToolArgs", map[string]any(nil)}} {
		d, _ := BuiltinRegistry().Resolve(tt.kind)
		if d.ValidatePatch(map[string]any{tt.key: tt.value}) == nil {
			t.Fatalf("null %s accepted", tt.key)
		}
	}
}

func TestStructuredTypedMetricsCannotBypassSemantics(t *testing.T) {
	d, _ := BuiltinRegistry().Resolve("logic")
	patch := map[string]any{"metrics": []map[string]any{{"field": "a", "fn": "sum", "as": "x"}, {"field": "b", "fn": "avg", "as": "x"}}}
	if d.ValidatePatch(patch) == nil {
		t.Fatal("typed metrics bypassed duplicate validation")
	}
}

func TestStructuredSchemaHashAndClone(t *testing.T) {
	input := testDescriptor()
	input.PatchFields["tags"] = structuredField("metadata.tags", "Tags", 40, stringArraySchema())
	input.PatchFields["mode"] = structuredField("metadata.mode", "Mode", 41, enumSchema("left", "inner"))
	registry, err := NewRegistry([]Descriptor{input})
	if err != nil {
		t.Fatal(err)
	}
	hash := registry.Hash()
	input.PatchFields["tags"].Schema["items"].(map[string]any)["minLength"] = 3
	if registry.Hash() != hash {
		t.Fatal("registry retained schema input")
	}
	other, err := NewRegistry([]Descriptor{input})
	if err != nil {
		t.Fatal(err)
	}
	if other.Hash() == hash {
		t.Fatal("schema changes absent from hash")
	}
	resolved, _ := registry.Resolve("custom")
	resolved.PatchFields["tags"].Schema["items"].(map[string]any)["minLength"] = 4
	resolved.PatchFields["mode"].Schema["enum"].([]string)[0] = "outer"
	listed := registry.List()
	listed[0].PatchFields["tags"].Schema["items"].(map[string]any)["minLength"] = 5
	if registry.Hash() != hash {
		t.Fatal("Resolve/List leaked schema")
	}
	for _, schema := range []map[string]any{{"type": "array"}, {"type": "array", "items": map[string]any{"type": "object"}}, {"type": "object"}} {
		input.PatchFields["tags"] = structuredField("metadata.tags", "Tags", 40, schema)
		if _, err := NewRegistry([]Descriptor{input}); err == nil {
			t.Fatalf("unbounded shape accepted: %#v", schema)
		}
	}
}

func TestStructuredCandidateChecksStoredConfigAtomically(t *testing.T) {
	d, _ := BuiltinRegistry().Resolve("logic")
	comparison := structuredValue(t, `{"regionField":"region","substanceField":"substance","periodField":"period","baselinePeriodField":"baseline","valueField":"value"}`)
	node := map[string]any{"metadata": map[string]any{"logic": map[string]any{"expression": "value*2", "fieldPath": ""}}}
	before, _ := json.Marshal(node)
	if d.ApplyPatch(node, map[string]any{"periodComparison": comparison, "title": "bad"}) == nil {
		t.Fatal("stored expression conflict accepted")
	}
	after, _ := json.Marshal(node)
	if string(before) != string(after) {
		t.Fatal("candidate conflict partially applied")
	}
	for _, output := range []string{"value", "value_status"} {
		c := cloneJSONValue(comparison).(map[string]any)
		c["valueField"] = output
		if d.ApplyPatch(node, map[string]any{"expression": "", "periodComparison": c, "outputField": "value"}) == nil {
			t.Fatal("output collision accepted")
		}
	}
	if err := d.ApplyPatch(node, map[string]any{"expression": "", "periodComparison": comparison, "outputField": "delta"}); err != nil {
		t.Fatal(err)
	}
	if d.ApplyPatch(node, map[string]any{"expression": "new expression"}) == nil {
		t.Fatal("stored comparison conflict accepted")
	}
}

func TestStructuredPatchDoesNotRetainCallerValues(t *testing.T) {
	d, _ := BuiltinRegistry().Resolve("logic")
	patch := structuredValue(t, `{"groupBy":["region"]}`)
	node := map[string]any{"metadata": d.Metadata("")}
	if err := d.ApplyPatch(node, patch); err != nil {
		t.Fatal(err)
	}
	patch["groupBy"].([]any)[0] = "mutated"
	if !reflect.DeepEqual(node["metadata"].(map[string]any)["logic"].(map[string]any)["groupBy"], []any{"region"}) {
		t.Fatal("patch retained caller-owned array")
	}
}
