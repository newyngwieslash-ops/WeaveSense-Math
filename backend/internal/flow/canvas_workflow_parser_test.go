package flow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestT05bStrictParser(t *testing.T) {
	check := func(t *testing.T, raw, format, name, want string) {
		t.Helper()
		items, err := wfParseRawItems(raw, format, name)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := json.Marshal(items)
		var value any
		if err = json.Unmarshal([]byte(want), &value); err != nil {
			t.Fatal(err)
		}
		expected, _ := json.Marshal(value)
		if string(got) != string(expected) {
			t.Fatalf("got %s want %s", got, expected)
		}
	}
	t.Run("csv tsv multiline bom crlf empty cells and ids", func(t *testing.T) {
		for _, delimiter := range []string{",", "\t"} {
			name := "rows.csv"
			if delimiter == "\t" {
				name = " ROWS.TSV "
			}
			raw := "\ufeffid" + delimiter + "note" + delimiter + "value" + delimiter + "empty\r\n001" + delimiter + "\"line1\r\nline2" + delimiter + "\"\"quoted\"\"\"" + delimiter + "23" + delimiter + "\r\n002" + delimiter + "\" padded \"" + delimiter + "27.6" + delimiter + "\r\n"
			want, _ := json.Marshal([]any{map[string]any{"id": "001", "note": "line1\nline2" + delimiter + "\"quoted\"", "value": 23, "empty": ""}, map[string]any{"id": "002", "note": " padded ", "value": 27.6, "empty": ""}})
			check(t, raw, "csv", name, string(want))
		}
		check(t, "a\tb", "csv", "x.tsv", `[]`)
		check(t, "\ufeff\r\n", "csv", "x.csv", `[]`)
	})
	t.Run("invalid delimited records fail without text or partial rows", func(t *testing.T) {
		for _, raw := range []string{"\ta\tb\t", "a\ta\n1\t2", "a\t a \n1\t2", "a\tb\n1", "a\tb\n1\t2\t3", "a\tb\n\"unclosed\tx", "a\tb\nx\"bad\ty", "a\tb\n\"x\"bad\ty"} {
			items, err := wfParseRawItems(raw, "csv", "x.tsv")
			if err == nil || len(items) != 0 {
				t.Fatalf("bad accepted %q: %+v %v", raw, items, err)
			}
		}
		_, err := wfParseRawItems("a,b\n1,2\n3", "csv")
		if err == nil || !strings.Contains(err.Error(), "3") {
			t.Fatalf("missing line number: %v", err)
		}
	})
	t.Run("json and intel same envelopes and strict shapes", func(t *testing.T) {
		for _, format := range []string{"json", "intel"} {
			for _, key := range []string{"items", "rows", "data", "records", "list", "documents", "results"} {
				check(t, `{"`+key+`":[{"id":"001","value":23}]}`, format, "x.json", `[{"id":"001","value":23}]`)
			}
			check(t, "\ufeff{\"id\":\"001\",\"value\":27.6}", format, "x.json", `[{"id":"001","value":27.6}]`)
			for _, raw := range []string{"null", "42", "false", `"text"`, "{broken", `[{"a":1},]`, `{"v":1e999}`, "plain intelligence"} {
				items, err := wfParseRawItems(raw, format)
				if err == nil || len(items) != 0 {
					t.Fatalf("bad %s accepted %q", format, raw)
				}
			}
		}
	})
	t.Run("jsonl values retain rows and physical error lines", func(t *testing.T) {
		check(t, "\ufeff\r\n{\"items\":[1,2]}\r\n\r\n[1,\"二\"]\n\"text\"\n42\nfalse\nnull", "json", " ROWS.JSONL ", `[{"items":[1,2]},{"items":[1,"二"],"text":"[1,\"二\"]"},"text",42,false,""]`)
		for _, raw := range []string{"{}\n\n{bad}", "{}\n\n{\"v\":1e999}"} {
			items, err := wfParseRawItems(raw, "json", "x.jsonl")
			if err == nil || !strings.Contains(err.Error(), "3") || len(items) != 0 {
				t.Fatalf("jsonl failure: %v %+v", err, items)
			}
		}
	})
}
