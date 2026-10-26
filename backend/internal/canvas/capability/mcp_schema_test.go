package capability

import (
	"strings"
	"testing"
)

func TestMCPBindingSchemaContract(t *testing.T) {
	d, _ := BuiltinRegistry().Resolve("data-source")
	good := structuredValue(t, `{"sourceBindingToolVersion":1,"sourceBindingSchemaHash":"approved-hash","sourceBindingWindowArguments":{"from":"start","to":"end"},"sourceBindingOutputSchema":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}}}`)
	if err := d.ValidatePatch(good); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"sourceBindingToolVersion":"v1"}`, `{"sourceBindingToolVersion":0}`, `{"sourceBindingToolVersion":1.5}`,
		`{"sourceBindingWindowArguments":{"from":"same","to":"same"}}`, `{"sourceBindingWindowArguments":{"from":"","to":"end"}}`, `{"sourceBindingWindowArguments":{"from":"start","to":"end","extra":true}}`,
		`{"sourceBindingOutputSchema":{"type":"not-a-type"}}`, `{"sourceBindingOutputSchema":{"type":"object","additionalProperties":{"$ref":"https://example.test/schema"}}}`,
		`{"sourceBindingOutputSchema":{"type":"array","items":{"$ref":"https://example.test/schema"}}}`, `{"sourceBindingOutputSchema":[]}`,
	} {
		if d.ValidatePatch(structuredValue(t, raw)) == nil {
			t.Fatalf("invalid MCP contract accepted: %s", raw)
		}
	}
	big := map[string]any{"type": "string", "description": strings.Repeat("x", 17000)}
	if d.ValidatePatch(map[string]any{"sourceBindingOutputSchema": big}) == nil {
		t.Fatal("oversize schema accepted")
	}
	deep := map[string]any{"type": "string"}
	for i := 0; i < 14; i++ {
		deep = map[string]any{"type": "array", "items": deep}
	}
	if d.ValidatePatch(map[string]any{"sourceBindingOutputSchema": deep}) == nil {
		t.Fatal("deep schema accepted")
	}
}
