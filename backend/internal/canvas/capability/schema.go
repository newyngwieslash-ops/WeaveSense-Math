package capability

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

// JSONSchema is the single constraint source for model tools and server validation.
func (f PatchField) JSONSchema() map[string]any {
	schema := map[string]any{}
	if f.Schema != nil {
		schema = cloneJSONValue(f.Schema).(map[string]any)
	}
	schema["type"] = f.Kind
	if f.MaxRunes > 0 {
		schema["maxLength"] = f.MaxRunes
	}
	if f.Limit > 0 {
		schema["minimum"] = -f.Limit
		schema["maximum"] = f.Limit
	}
	return schema
}

func resolveFieldSchema(f PatchField) (*jsonschema.Resolved, error) {
	raw, err := json.Marshal(f.JSONSchema())
	if err != nil {
		return nil, err
	}
	var schema jsonschema.Schema
	if err = json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	return schema.Resolve(nil)
}

func checkFieldSchema(f PatchField) error {
	if f.Schema != nil {
		if kind, ok := f.Schema["type"]; ok && kind != f.Kind {
			return fmt.Errorf("schema type conflicts with field kind")
		}
	}
	if err := checkSchemaShape(f.JSONSchema()); err != nil {
		return err
	}
	_, err := resolveFieldSchema(f)
	return err
}

// Arrays always name their items; objects are closed unless explicitly declared as
// a JSON argument bag. No external references or implicit arbitrary objects.
func checkSchemaShape(s map[string]any) error {
	if _, ok := s["$ref"]; ok {
		return fmt.Errorf("schema references are not supported")
	}
	switch s["type"] {
	case "array":
		item, ok := s["items"].(map[string]any)
		if !ok {
			return fmt.Errorf("array requires an item schema")
		}
		return checkSchemaShape(item)
	case "object":
		if _, ok := s["additionalProperties"].(bool); !ok {
			return fmt.Errorf("object requires explicit additionalProperties policy")
		}
		if props, ok := s["properties"].(map[string]any); ok {
			for _, property := range props {
				child, ok := property.(map[string]any)
				if !ok {
					return fmt.Errorf("invalid property schema")
				}
				if err := checkSchemaShape(child); err != nil {
					return err
				}
			}
		}
	case "string", "number", "integer", "boolean":
	default:
		return fmt.Errorf("schema requires a supported type")
	}
	return nil
}

func validateFieldSchema(f PatchField, value any) error {
	if err := checkFieldSchema(f); err != nil {
		return err
	}
	// Validate the persisted JSON shape, not Go's treatment of typed nil maps/slices.
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return err
	}
	resolved, err := resolveFieldSchema(f)
	if err != nil {
		return err
	}
	return resolved.Validate(normalized)
}

func cloneJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = cloneJSONValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneJSONValue(item)
		}
		return out
	case []string:
		return append([]string(nil), v...)
	default:
		return value
	}
}

func normalizeJSONMap(value map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func validatePatchSemantics(kind string, patch map[string]any) error {
	// Validate the same JSON representation used by persistence, including typed Go slices.
	patch, err := normalizeJSONMap(patch)
	if err != nil {
		return err
	}
	if kind == "logic" {
		if values, ok := patch["metrics"].([]any); ok {
			seen := map[string]bool{}
			for _, value := range values {
				metric := value.(map[string]any)
				name := metric["as"].(string)
				if seen[name] {
					return fmt.Errorf("metrics.as must be unique: %s", name)
				}
				seen[name] = true
				if metric["fn"] != "count" && strings.TrimSpace(metric["field"].(string)) == "" {
					return fmt.Errorf("non-count metric requires field")
				}
			}
		}
	}
	if kind == "data-source" {
		if window, ok := patch["sourceBindingTimeWindow"].(map[string]any); ok {
			if err := validateTimeWindow(window); err != nil {
				return err
			}
		}
		if mapping, ok := patch["sourceBindingWindowArguments"].(map[string]any); ok && mapping["from"] == mapping["to"] {
			return fmt.Errorf("windowArguments.from and to must differ")
		}
		if schema, ok := patch["sourceBindingOutputSchema"].(map[string]any); ok {
			return validateOutputSchema(schema)
		}
	}
	return nil
}

func validateOutputSchema(value map[string]any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) > 16384 {
		return fmt.Errorf("outputSchema exceeds 16 KiB")
	}
	var check func(any, int) error
	check = func(value any, depth int) error {
		if depth > 12 {
			return fmt.Errorf("outputSchema exceeds nesting limit")
		}
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if key == "$ref" || key == "$dynamicRef" {
					if ref, ok := child.(string); !ok || !strings.HasPrefix(ref, "#") {
						return fmt.Errorf("outputSchema remote references are forbidden")
					}
				}
				if err := check(child, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range v {
				if err := check(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := check(value, 0); err != nil {
		return err
	}
	if err := validateSchemaDeclarations(value); err != nil {
		return err
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return fmt.Errorf("invalid outputSchema: %w", err)
	}
	_, err = schema.Resolve(nil)
	return err
}

// jsonschema-go resolves references and regexes but does not yet check type names
// against the meta-schema. Check declarations only; instance evaluation stays in it.
func validateSchemaDeclarations(value any) error {
	if _, ok := value.(bool); ok {
		return nil
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("outputSchema child must be a schema")
	}
	if types, exists := schema["type"]; exists {
		values, ok := types.([]any)
		if !ok {
			values = []any{types}
		}
		if len(values) == 0 {
			return fmt.Errorf("outputSchema type cannot be empty")
		}
		seen := map[string]bool{}
		for _, value := range values {
			name, ok := value.(string)
			if !ok || seen[name] {
				return fmt.Errorf("invalid outputSchema type")
			}
			seen[name] = true
			switch name {
			case "null", "boolean", "object", "array", "number", "string", "integer":
			default:
				return fmt.Errorf("invalid outputSchema type %q", name)
			}
		}
	}
	for _, key := range []string{"minLength", "maxLength", "minItems", "maxItems", "minProperties", "maxProperties", "minContains", "maxContains"} {
		if value, ok := schema[key].(float64); ok && (value < 0 || value != float64(int64(value))) {
			return fmt.Errorf("invalid outputSchema %s", key)
		}
	}
	if value, ok := schema["multipleOf"].(float64); ok && value <= 0 {
		return fmt.Errorf("outputSchema multipleOf must be positive")
	}
	for _, key := range []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"} {
		if entries, ok := schema[key].(map[string]any); ok {
			for _, child := range entries {
				if err := validateSchemaDeclarations(child); err != nil {
					return err
				}
			}
		}
	}
	for _, key := range []string{"items", "contains", "additionalProperties", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "not", "if", "then", "else"} {
		if child, exists := schema[key]; exists {
			if err := validateSchemaDeclarations(child); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		if children, ok := schema[key].([]any); ok {
			for _, child := range children {
				if err := validateSchemaDeclarations(child); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateTimeWindow(window map[string]any) error {
	_, _, err := ResolveTimeWindow(window, time.Now())
	return err
}

// ResolveTimeWindow is shared by registration and source execution. Returned bounds
// are instants; callers may format them in UTC without reinterpreting local time.
func ResolveTimeWindow(window map[string]any, now time.Time) (time.Time, time.Time, error) {
	invalid := func(reason string) (time.Time, time.Time, error) {
		return time.Time{}, time.Time{}, fmt.Errorf("%s", reason)
	}
	for key, value := range window {
		switch key {
		case "kind", "from", "to", "timezone":
		default:
			return invalid("invalid_time_window")
		}
		if text, ok := value.(string); !ok || strings.TrimSpace(text) == "" {
			return invalid("invalid_time_window")
		}
	}
	zone, _ := window["timezone"].(string)
	if zone == "" {
		zone = "UTC"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return invalid("invalid_timezone")
	}
	var from, to time.Time
	switch window["kind"] {
	case "7d", "30d":
		if _, ok := window["from"]; ok {
			return invalid("relative_window_has_bounds")
		}
		if _, ok := window["to"]; ok {
			return invalid("relative_window_has_bounds")
		}
		days := 7
		if window["kind"] == "30d" {
			days = 30
		}
		to = now
		from = now.In(loc).AddDate(0, 0, -days)
	case "custom":
		text, _ := window["from"].(string)
		from, err = parseWindowTime(text, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		text, _ = window["to"].(string)
		to, err = parseWindowTime(text, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	default:
		return invalid("invalid_window_kind")
	}
	if !from.Before(to) {
		return invalid("invalid_window_order")
	}
	return from, to, nil
}

func parseWindowTime(text string, loc *time.Location) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return parsed, nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02"} {
		parsed, err := time.ParseInLocation(layout, text, loc)
		if err != nil {
			continue
		}
		if parsed.In(loc).Format(layout) != text {
			return time.Time{}, fmt.Errorf("nonexistent_local_time")
		}
		if layout != "2006-01-02" {
			for _, delta := range []time.Duration{-2 * time.Hour, -time.Hour, -30 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour} {
				if parsed.Add(delta).In(loc).Format(layout) == text {
					return time.Time{}, fmt.Errorf("ambiguous_local_time")
				}
			}
		}
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("invalid_window_time")
}

func nonemptyStringSchema() map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "pattern": `\S`}
}
func enumSchema(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}
func arraySchema(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}
func stringArraySchema() map[string]any { return arraySchema(nonemptyStringSchema()) }
func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func structuredField(path, label string, order int, schema map[string]any) PatchField {
	return PatchField{Path: path, Label: label, Order: order, Kind: schema["type"].(string), Schema: schema}
}
func setFieldSchema(fields map[string]PatchField, key string, schema map[string]any) {
	field := fields[key]
	field.Schema = schema
	fields[key] = field
}

func validateCandidate(kind string, node map[string]any) error {
	if kind != "logic" {
		return nil
	}
	metadata, _ := node["metadata"].(map[string]any)
	logic, _ := metadata["logic"].(map[string]any)
	comparison, ok := logic["periodComparison"].(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range []string{"expression", "fieldPath"} {
		if text, _ := logic[key].(string); strings.TrimSpace(text) != "" {
			return fmt.Errorf("periodComparison conflicts with %s; clear it in the same patch", key)
		}
	}
	output, _ := logic["outputField"].(string)
	if output != "" {
		for _, value := range comparison {
			if value == output || value == output+"_status" {
				return fmt.Errorf("compute outputField or derived status conflicts with periodComparison input")
			}
		}
	}
	return nil
}
