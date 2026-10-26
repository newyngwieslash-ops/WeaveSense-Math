package capability

import "testing"

func TestTimeWindowMatchesSourceResolver(t *testing.T) {
	d, _ := BuiltinRegistry().Resolve("data-source")
	for _, raw := range []string{
		`{"kind":"custom"}`, `{"kind":"custom","from":"2026-01-01"}`, `{"kind":"custom","from":"2026-01-01","to":"2026-01-01"}`,
		`{"kind":"7d","from":"2026-01-01"}`, `{"kind":"30d","to":"2026-01-31"}`,
		`{"kind":"custom","timezone":"America/New_York","from":"2026-03-08T02:30:00","to":"2026-03-09T03:30:00"}`,
		`{"kind":"custom","timezone":"America/New_York","from":"2026-11-01T01:30:00","to":"2026-11-02T03:30:00"}`,
	} {
		if d.ValidatePatch(map[string]any{"sourceBindingTimeWindow": structuredValue(t, raw)}) == nil {
			t.Fatalf("invalid time window accepted: %s", raw)
		}
	}
	good := structuredValue(t, `{"kind":"custom","timezone":"America/New_York","from":"2026-01-01T10:00:00","to":"2026-01-02T10:00:00"}`)
	if err := d.ValidatePatch(map[string]any{"sourceBindingTimeWindow": good}); err != nil {
		t.Fatal(err)
	}
}
