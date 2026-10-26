package app

import (
	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
	"reflect"
	"testing"
)

func TestT17WorkflowToolSchemaAndPermission(t *testing.T) {
	for _, permission := range []string{"auto", "request_approval", "read_only", "unknown", ""} {
		req := agentTestRequest()
		req.ContextScope = []string{"canvas"}
		req.PermissionMode = permission
		var run, apply map[string]any
		for _, tool := range cloudAgentTools(req) {
			fn := tool["function"].(map[string]any)
			if fn["name"] == "canvas_run_workflow" {
				run = fn["parameters"].(map[string]any)
			}
			if fn["name"] == "canvas_apply_ops" {
				apply = fn["parameters"].(map[string]any)
			}
		}
		allowed := permission == "auto" || permission == "request_approval"
		if (run != nil) != allowed {
			t.Fatalf("workflow tool permission %q: %#v", permission, run)
		}
		if !allowed {
			continue
		}
		props := run["properties"].(map[string]any)
		if !reflect.DeepEqual(props["ops"], apply["properties"].(map[string]any)["ops"]) {
			t.Fatal("workflow ops diverge from canvas_apply_ops")
		}
		raw, _ := json.Marshal(run)
		var schema jsonschema.Schema
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{`{"expected_revision":1,"ops":[],"run":{"budget_micro":1}}`, `{"expected_revision":0,"ops":[],"run":{"budget_micro":100,"refresh_bindings":true,"save_report":true,"report_title":"report"}}`} {
			var value any
			_ = json.Unmarshal([]byte(text), &value)
			if err := resolved.Validate(value); err != nil {
				t.Fatal(err)
			}
		}
		for _, text := range []string{`{"expected_revision":1,"run":{"budget_micro":1}}`, `{"expected_revision":1,"ops":[],"run":{}}`, `{"expected_revision":-1,"ops":[],"run":{"budget_micro":1}}`, `{"expected_revision":1,"ops":[],"run":{"budget_micro":1.5}}`, `{"expected_revision":1,"ops":[],"run":{"budget_micro":1},"approval":true}`, `{"expected_revision":1,"ops":[],"run":{"budget_micro":1},"execution_key":"client"}`} {
			var value any
			_ = json.Unmarshal([]byte(text), &value)
			if resolved.Validate(value) == nil {
				t.Fatalf("invalid workflow args accepted: %s", text)
			}
		}
	}
}
