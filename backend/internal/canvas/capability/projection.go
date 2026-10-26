package capability

import (
	"encoding/json"
	"sort"
	"strings"
)

// ProjectStructuredField exposes only registered patch paths under a declared
// read field. Invalid stored contracts are omitted, never forwarded as metadata.
func (d Descriptor) ProjectStructuredField(key string, value any, textLimit int) (any, bool, bool) {
	prefix := "metadata." + key
	out := map[string]any{}
	handled, truncated := false, false
	budget := 16000
	keys := make([]string, 0, len(d.PatchFields))
	for key := range d.PatchFields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, fieldKey := range keys {
		field := d.PatchFields[fieldKey]
		if field.Path == prefix && (field.Kind == "array" || field.Kind == "object") {
			if validateFieldSchema(field, value) != nil {
				return nil, false, true
			}
			safe, cut := boundedProjection(value, textLimit, 0, &budget)
			return safe, cut, true
		}
		if !strings.HasPrefix(field.Path, prefix+".") {
			continue
		}
		handled = true
		parts := strings.Split(strings.TrimPrefix(field.Path, prefix+"."), ".")
		current := value
		for _, part := range parts {
			object, ok := current.(map[string]any)
			if !ok {
				current = nil
				break
			}
			current = object[part]
		}
		if current == nil || validateFieldSchema(field, current) != nil {
			continue
		}
		safe, cut := boundedProjection(current, textLimit, 0, &budget)
		truncated = truncated || cut
		if safe == nil {
			continue
		}
		target := out
		for _, part := range parts[:len(parts)-1] {
			child, ok := target[part].(map[string]any)
			if !ok {
				child = map[string]any{}
				target[part] = child
			}
			target = child
		}
		target[parts[len(parts)-1]] = safe
	}
	return out, truncated, handled
}

// Bound every collection and nesting level, including the explicitly open MCP
// argument bag. Sorting object keys makes truncation deterministic.
func boundedProjection(value any, textLimit, depth int, budget *int) (any, bool) {
	if depth >= 6 || *budget < 32 {
		return nil, true
	}
	*budget -= 32
	switch v := value.(type) {
	case string:
		runes := []rune(v)
		limit := min(textLimit, 200)
		cut := len(runes) > limit
		if cut {
			v = string(runes[:limit])
		}
		raw, _ := json.Marshal(v)
		if len(raw) > *budget {
			return nil, true
		}
		*budget -= len(raw)
		return v, cut
	case float64, bool:
		return v, false
	case []any:
		out := make([]any, 0, min(len(v), 32))
		cut := len(v) > 32
		for _, item := range v[:min(len(v), 32)] {
			safe, truncated := boundedProjection(item, textLimit, depth+1, budget)
			cut = cut || truncated
			if truncated && safe == nil {
				break
			}
			out = append(out, safe)
		}
		return out, cut
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := map[string]any{}
		cut := len(keys) > 32
		for _, key := range keys[:min(len(keys), 32)] {
			if len([]rune(key)) > 200 {
				cut = true
				continue
			}
			encodedKey, _ := json.Marshal(key)
			if len(encodedKey) > *budget {
				cut = true
				break
			}
			*budget -= len(encodedKey)
			safe, truncated := boundedProjection(v[key], textLimit, depth+1, budget)
			cut = cut || truncated
			if !truncated || safe != nil {
				out[key] = safe
			}
		}
		return out, cut
	default:
		return nil, false
	}
}
