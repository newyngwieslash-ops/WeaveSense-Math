package flow

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func substancePack(t *testing.T) SkillPack {
	t.Helper()
	packs, err := SkillPacks()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range packs {
		if p.ID == "substance-precursor-intel" {
			return p
		}
	}
	t.Fatal("substance pack missing")
	return SkillPack{}
}

func substanceJSON(t *testing.T, text string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Synthetic records below are contract fixtures, never model results or live evidence.
func substanceOutput(t *testing.T, id string) map[string]any {
	t.Helper()
	out := substanceJSON(t, `{"title":"Synthetic contract fixture","period":{"start":"2026-01-01","end":"2026-01-31","granularity":"month"},"claims":[{"text":"Fixture observation","kind":"source_statement","citations":[{"evidence_id":"fixture-monitor","revision":2}]}],"material_scope":{"document_ids":["fixture-document"]},"coverage":{"covered_regions":["R1"],"uncovered_regions":["R2"],"failed_sources":[],"truncated_evidence_ids":[],"limitations":["Synthetic fixture only"]},"gaps":[]}`)
	rows := map[string]string{
		"regional-wastewater-modeling":    `{"regional_metrics":[{"region_code":"R1","substance_name":"fixture-analyte","period":{"start":"2026-01-01","end":"2026-01-31","granularity":"month"},"consumption_mg_1000p":23,"unit":"mg/day/1000人","excretion_factor":2.3,"excretion_factor_source":{"version":"fixture-v1","citations":[{"evidence_id":"fixture-factor","revision":1}]},"risk_level":"unknown","comparison_status":"missing_baseline","kind":"model_inference","citations":[{"evidence_id":"fixture-monitor","revision":2},{"evidence_id":"fixture-factor","revision":1}]}]}`,
		"nps-substitution-screening":      `{"emerging_substances":[{"substance_name":"fixture-analyte","category":"unconfirmed","detected_regions":["R1"],"first_detected_at":"2026-01-02","co_use_features":[],"substitution_status":"insufficient_evidence","legal_status":{"status":"unknown"},"kind":"source_statement","citations":[{"evidence_id":"fixture-monitor","revision":2}]}]}`,
		"precursor-substance-correlation": `{"correlations":[{"region_code":"R1","precursor_name":"fixture-precursor","related_substance":"fixture-analyte","window":{"start":"2026-01-01","end":"2026-01-31","granularity":"month"},"lag_days":0,"association_status":"candidate","alert_level":"unassessed","precursor_citations":[{"evidence_id":"fixture-precursor","revision":1}],"monitoring_citations":[{"evidence_id":"fixture-monitor","revision":2}],"kind":"model_inference","citations":[{"evidence_id":"fixture-precursor","revision":1},{"evidence_id":"fixture-monitor","revision":2}]}]}`,
		"regional-drug-situation-brief":   `{"body":"Synthetic evidence-limited summary","regional_metrics":[],"emerging_substances":[],"correlations":[],"recommendations":["Collect missing regional observations"]}`,
	}
	for k, v := range substanceJSON(t, rows[id]) {
		out[k] = v
	}
	return out
}

func substanceInput(t *testing.T, id string) map[string]any {
	t.Helper()
	in := substanceJSON(t, `{"period":{"start":"2026-01-01","end":"2026-01-31","granularity":"month"},"material_scope":{"document_ids":["fixture-document"]},"coverage":{"covered_regions":["R1"],"uncovered_regions":[],"failed_sources":[],"truncated_evidence_ids":[],"limitations":[]}}`)
	rows := map[string]string{
		"regional-wastewater-modeling":    `{"monitoring_records":[{"region_code":"R1","substance_name":"fixture-analyte","sample_start":"2026-01-01","sample_end":"2026-01-31","concentration_ng_l":50,"flow_m3_day":100000,"population":500000,"excretion_factor":2.3,"excretion_factor_source":{"version":"fixture-v1","citations":[{"evidence_id":"fixture-factor","revision":1}]},"citations":[{"evidence_id":"fixture-monitor","revision":2}]}],"baseline_records":[]}`,
		"nps-substitution-screening":      `{"detection_records":[{"substance_name":"fixture-analyte","region_code":"R1","detected_at":"2026-01-02","category":"unconfirmed","co_use_features":[],"citations":[{"evidence_id":"fixture-monitor","revision":2}]}],"traditional_metrics":[],"control_materials":[]}`,
		"precursor-substance-correlation": `{"monitoring_records":[{"region_code":"R1","substance_name":"fixture-analyte","observed_at":"2026-01-02","citations":[{"evidence_id":"fixture-monitor","revision":2}]}],"precursor_records":[{"region_code":"R1","precursor_name":"fixture-precursor","related_substance":"fixture-analyte","event_at":"2026-01-03","quantity":10,"quantity_unit":"kg","anomaly_type":"reported_flow","citations":[{"evidence_id":"fixture-precursor","revision":1}]}],"association_config":{"version":"fixture-v1","max_lag_days":7,"citations":[{"evidence_id":"fixture-policy","revision":1}]}}`,
		"regional-drug-situation-brief":   `{"analyses":[],"requested_regions":["R1","R2"]}`,
	}
	for k, v := range substanceJSON(t, rows[id]) {
		in[k] = v
	}
	return in
}

func TestSubstancePrecursorPackSchemas(t *testing.T) {
	p := substancePack(t)
	if p.Kind != "industry" || len(p.Skills) != 4 {
		t.Fatalf("kind/skills changed: %s/%d", p.Kind, len(p.Skills))
	}
	rowKeys := map[string]string{"regional-wastewater-modeling": "regional_metrics", "nps-substitution-screening": "emerging_substances", "precursor-substance-correlation": "correlations"}
	for _, skill := range p.Skills {
		t.Run(skill.ID, func(t *testing.T) {
			var s jsonschema.Schema
			if err := json.Unmarshal(skill.OutputSchema, &s); err != nil {
				t.Fatal(err)
			}
			r, err := s.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			valid := substanceOutput(t, skill.ID)
			if err := r.Validate(valid); err != nil {
				t.Fatalf("valid output rejected: %v", err)
			}
			for _, field := range []string{"title", "claims", "period", "material_scope", "coverage", "gaps"} {
				t.Run("requires_"+field, func(t *testing.T) {
					bad := substanceOutput(t, skill.ID)
					delete(bad, field)
					if r.Validate(bad) == nil {
						t.Fatalf("missing %s accepted", field)
					}
				})
			}
			for _, field := range []string{"covered_regions", "uncovered_regions", "failed_sources", "truncated_evidence_ids", "limitations"} {
				bad := substanceOutput(t, skill.ID)
				delete(bad["coverage"].(map[string]any), field)
				if r.Validate(bad) == nil {
					t.Errorf("coverage without %s accepted", field)
				}
			}
			for _, kind := range []string{"source_statement", "cross_source_verified", "model_inference"} {
				good := substanceOutput(t, skill.ID)
				good["claims"].([]any)[0].(map[string]any)["kind"] = kind
				if err := r.Validate(good); err != nil {
					t.Errorf("existing claim kind %s rejected: %v", kind, err)
				}
			}
			for _, mutation := range []string{"empty_claims", "uncited", "bad_kind", "empty_evidence", "zero_revision", "fractional_revision", "missing_revision", "row_uncited"} {
				t.Run(mutation, func(t *testing.T) {
					bad := substanceOutput(t, skill.ID)
					claim := bad["claims"].([]any)[0].(map[string]any)
					citation := claim["citations"].([]any)[0].(map[string]any)
					switch mutation {
					case "empty_claims":
						bad["claims"] = []any{}
					case "uncited":
						claim["citations"] = []any{}
					case "bad_kind":
						claim["kind"] = "verified"
					case "empty_evidence":
						citation["evidence_id"] = ""
					case "zero_revision":
						citation["revision"] = float64(0)
					case "fractional_revision":
						citation["revision"] = 1.5
					case "missing_revision":
						delete(citation, "revision")
					case "row_uncited":
						key, ok := rowKeys[skill.ID]
						if !ok {
							return
						}
						delete(bad[key].([]any)[0].(map[string]any), "citations")
					}
					if r.Validate(bad) == nil {
						t.Fatal("invalid output accepted")
					}
				})
			}
			input, ok := s.Defs["input_requirements"]
			if !ok {
				t.Fatal("missing input_requirements in existing output_schema $defs")
			}
			inputRoot := jsonschema.Schema{Ref: "#/$defs/input_requirements", Defs: s.Defs}
			ri, err := inputRoot.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := ri.Validate(substanceInput(t, skill.ID)); err != nil {
				t.Fatalf("valid input rejected: %v", err)
			}
			for _, field := range input.Required {
				bad := substanceInput(t, skill.ID)
				delete(bad, field)
				if ri.Validate(bad) == nil {
					t.Errorf("input without %s accepted", field)
				}
			}
			if skill.ID == "regional-wastewater-modeling" {
				for _, field := range []string{"concentration_ng_l", "flow_m3_day", "population", "excretion_factor", "excretion_factor_source", "sample_start", "sample_end", "citations"} {
					bad := substanceInput(t, skill.ID)
					delete(bad["monitoring_records"].([]any)[0].(map[string]any), field)
					if ri.Validate(bad) == nil {
						t.Errorf("monitor without %s accepted", field)
					}
				}
				for _, field := range []string{"population", "flow_m3_day", "excretion_factor"} {
					bad := substanceInput(t, skill.ID)
					bad["monitoring_records"].([]any)[0].(map[string]any)[field] = float64(0)
					if ri.Validate(bad) == nil {
						t.Errorf("zero %s accepted", field)
					}
				}
			}
		})
	}
}

func TestSubstancePrecursorPackNoInventedAssessment(t *testing.T) {
	for _, skill := range substancePack(t).Skills {
		var s jsonschema.Schema
		if err := json.Unmarshal(skill.OutputSchema, &s); err != nil {
			t.Fatal(err)
		}
		r, err := s.Resolve(nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(skill.ID, func(t *testing.T) {
			out := substanceOutput(t, skill.ID)
			switch skill.ID {
			case "regional-wastewater-modeling":
				row := out["regional_metrics"].([]any)[0].(map[string]any)
				row["risk_level"] = "high"
				if r.Validate(out) == nil {
					t.Error("risk without threshold provenance accepted")
				}
				row["risk_level"] = "unknown"
				row["mom_pct"] = float64(0)
				if r.Validate(out) == nil {
					t.Error("fabricated change with no baseline accepted")
				}
			case "nps-substitution-screening":
				row := out["emerging_substances"].([]any)[0].(map[string]any)
				row["legal_status"] = map[string]any{"status": "documented", "value": "controlled"}
				if r.Validate(out) == nil {
					t.Error("legal assertion without jurisdiction/version/effective date/citations accepted")
				}
				row["legal_status"] = map[string]any{"status": "unknown"}
				row["substitution_status"] = "hypothesis"
				if r.Validate(out) == nil {
					t.Error("substitution hypothesis mislabeled as source fact accepted")
				}
			case "precursor-substance-correlation":
				row := out["correlations"].([]any)[0].(map[string]any)
				row["alert_level"] = "urgent"
				if r.Validate(out) == nil {
					t.Error("alert without threshold provenance accepted")
				}
				row["alert_level"] = "unassessed"
				delete(row, "precursor_citations")
				if r.Validate(out) == nil {
					t.Error("one-sided correlation accepted")
				}
			case "regional-drug-situation-brief":
				delete(out, "body")
				if r.Validate(out) == nil {
					t.Error("empty brief accepted")
				}
			}
		})
	}
}

func TestSubstancePrecursorPackFormulaAndMethods(t *testing.T) {
	p := substancePack(t)
	var glossary map[string]string
	if err := json.Unmarshal(p.Glossary, &glossary); err != nil {
		t.Fatal(err)
	}
	formula := "concentration_ng_l*flow_m3_day*excretion_factor/population"
	if !strings.Contains(glossary["consumption_mg_1000p"], formula) || strings.Contains(glossary["consumption_mg_1000p"], "人口(千人)") {
		t.Error("glossary must use population in persons, not thousands")
	}
	for _, key := range []string{"monitoring_fields", "precursor_fields", "substance_aliases", "unit_normalization", "excretion_factor_source", "control_materials", "evidence_labels", "threshold_source", "coverage"} {
		if glossary[key] == "" {
			t.Errorf("missing glossary contract %s", key)
		}
	}
	if !strings.Contains(glossary["excretion_factor"], "不默认") {
		t.Error("factor must not default to 1")
	}
	for _, s := range p.Skills {
		text := s.Method + strings.Join(s.Steps, " ")
		for _, term := range []string{"input_requirements", "evidence_id", "revision", "source_statement", "cross_source_verified", "model_inference", "coverage", "失败"} {
			if !strings.Contains(text, term) {
				t.Errorf("%s missing method contract %s", s.ID, term)
			}
		}
		if s.ID == "regional-wastewater-modeling" && (!strings.Contains(text, formula) || !strings.Contains(text, "23")) {
			t.Error("method formula/sample mismatch")
		}
	}
}

func TestSubstancePrecursorPackEvaluationCases(t *testing.T) {
	var samples []struct {
		ID       string `json:"id"`
		SkillID  string `json:"skill_id"`
		Question string `json:"question"`
		Expect   string `json:"expect"`
	}
	if err := json.Unmarshal(substancePack(t).EvalSamples, &samples); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, s := range samples {
		if s.ID == "" || s.Question == "" || s.Expect == "" {
			t.Error("incomplete evaluation case")
		}
		if _, ok := FindSkill(s.SkillID, ""); !ok {
			t.Errorf("unknown evaluation skill %q", s.SkillID)
		}
		if seen[s.ID] {
			t.Errorf("duplicate evaluation %s", s.ID)
		}
		seen[s.ID] = true
	}
	for _, id := range []string{"wbe-unit-23", "missing-evidence", "wrong-revision", "missing-factor", "wrong-factor-version", "cross-period", "missing-baseline", "zero-baseline", "missing-threshold", "same-source", "unknown-legal-status", "correlation-not-causation", "coverage-gaps"} {
		if !seen[id] {
			t.Errorf("missing evaluation %s", id)
		}
	}
}

func TestSubstancePrecursorPackEvidenceResolution(t *testing.T) {
	// Exercise the existing production citation validator, not a pack-specific replacement.
	resolved := []ResolvedEvidence{
		{EvidenceID: "fixture-monitor", DocumentID: "fixture-monitor-doc", SourceID: "fixture-lab", Revision: 2},
		{EvidenceID: "fixture-precursor", DocumentID: "fixture-precursor-doc", SourceID: "fixture-audit", Revision: 3},
		{EvidenceID: "fixture-reprint", DocumentID: "fixture-copy-doc", SourceID: "fixture-lab", Revision: 1, Truncated: true},
	}
	scope := MaterialScope{SourceIDs: []string{"fixture-lab", "fixture-audit"}}
	cases := []struct {
		name, kind string
		citations  []Citation
		wantError  bool
	}{
		{"source fact", "source_statement", []Citation{{EvidenceID: "fixture-monitor", Revision: 2}}, false},
		{"two sources", "cross_source_verified", []Citation{{EvidenceID: "fixture-monitor", Revision: 2}, {EvidenceID: "fixture-precursor", Revision: 3}}, false},
		{"inference", "model_inference", []Citation{{EvidenceID: "fixture-monitor", Revision: 2}}, false},
		{"missing evidence", "source_statement", nil, true},
		{"invented evidence", "source_statement", []Citation{{EvidenceID: "invented", Revision: 2}}, true},
		{"wrong revision", "source_statement", []Citation{{EvidenceID: "fixture-monitor", Revision: 1}}, true},
		{"same source", "cross_source_verified", []Citation{{EvidenceID: "fixture-monitor", Revision: 2}, {EvidenceID: "fixture-reprint", Revision: 1}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ValidateCitations([]ReportClaim{{Text: "Synthetic contract", Kind: c.kind, Citations: c.citations}}, scope, resolved)
			if errors.Is(err, ErrCitation) != c.wantError {
				t.Fatalf("want rejection=%v, got %v", c.wantError, err)
			}
		})
	}
	truncated, err := ValidateCitations([]ReportClaim{{Text: "Partial observation", Kind: "source_statement", Citations: []Citation{{EvidenceID: "fixture-reprint", Revision: 1}}}}, scope, resolved)
	if err != nil || !reflect.DeepEqual(truncated, []string{"fixture-reprint"}) {
		t.Fatalf("truncation not disclosed: %v %v", truncated, err)
	}
}

func TestSubstancePrecursorPackInstallAndFreeze(t *testing.T) {
	tools, _, _, ss := setupP3(t)
	for _, s := range substancePack(t).Skills {
		if _, err := InstallSkill(tools.DB, ss.Scope, "u_alice", s.ID, s.Version); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadSkillForRun(tools.DB, ss.Scope, "substance-contract-run", s.ID)
		if err != nil || loaded.Hash != s.Hash || string(loaded.OutputSchema) != string(s.OutputSchema) {
			t.Fatalf("skill lost on load: %s %v", s.ID, err)
		}
	}
}

func TestSubstancePrecursorPackEnergyStorageRegression(t *testing.T) {
	s, ok := FindSkill("storage-market-brief", "1.0.0")
	if !ok || s.PackID != "energy-storage" {
		t.Fatal("energy-storage skill lost")
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(s.OutputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	r, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(substanceJSON(t, `{"title":"Storage","claims":[{"text":"Price","kind":"source_statement","citations":[{"evidence_id":"fixture-storage","revision":1}]}]}`)); err != nil {
		t.Fatal(err)
	}
	if len(industryPackFiles(t)["energy-storage/manifest.json"]) == 0 {
		t.Fatal("energy-storage export lost")
	}
}

func TestSubstancePrecursorPackToolLoadComplete(t *testing.T) {
	tools, _, conv, ss := setupP3(t)
	for _, skill := range substancePack(t).Skills {
		t.Run(skill.ID, func(t *testing.T) {
			if _, err := InstallSkill(tools.DB, ss.Scope, "u_alice", skill.ID, skill.Version); err != nil {
				t.Fatal(err)
			}
			result, err := tools.Invoke(context.Background(), conv, ss, "u_alice", "load-complete-"+skill.ID, "intel_load_skill", map[string]any{"skill_id": skill.ID}, "")
			if err != nil || result.Status != "succeeded" {
				t.Fatalf("load: %v %+v", err, result)
			}
			if result.Truncated || strings.Contains(string(result.Output), "truncated_text") {
				t.Fatalf("skill truncated at real tool boundary: %d bytes", len(result.Output))
			}
			var loaded Skill
			if err := json.Unmarshal(result.Output, &loaded); err != nil {
				t.Fatal(err)
			}
			if loaded.ID != skill.ID || loaded.Hash != skill.Hash {
				t.Fatalf("lost identity: %+v", loaded.Ref())
			}
			var got, want any
			if err := json.Unmarshal(loaded.OutputSchema, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(skill.OutputSchema, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(loaded.Steps, skill.Steps) {
				t.Fatal("tool lost method/schema")
			}
			t.Logf("complete tool payload: %d bytes", len(result.Output))
		})
	}
}

func substanceInputSchema(t *testing.T, id string) *jsonschema.Resolved {
	t.Helper()
	skill, ok := FindSkill(id, "")
	if !ok {
		t.Fatal(id)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(skill.OutputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	root := jsonschema.Schema{Ref: "#/$defs/input_requirements", Defs: schema.Defs}
	resolved, err := root.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestSubstancePrecursorPackBriefAnalysisRows(t *testing.T) {
	schema := substanceInputSchema(t, "regional-drug-situation-brief")
	keys := map[string]string{"regional-wastewater-modeling": "regional_metrics", "nps-substitution-screening": "emerging_substances", "precursor-substance-correlation": "correlations"}
	for id, key := range keys {
		t.Run(id, func(t *testing.T) {
			makeInput := func() (map[string]any, map[string]any) {
				in := substanceInput(t, "regional-drug-situation-brief")
				row := substanceOutput(t, id)
				row["skill_id"] = id
				row["skill_version"] = "1.1.0"
				in["analyses"] = []any{row}
				return in, row
			}
			in, _ := makeInput()
			if err := schema.Validate(in); err != nil {
				t.Fatalf("valid analysis rejected: %v", err)
			}
			for _, badValue := range []any{"not an array", []any{"not a row"}} {
				in, row := makeInput()
				row[key] = badValue
				if schema.Validate(in) == nil {
					t.Errorf("invalid %s accepted: %v", key, badValue)
				}
			}
			in, row := makeInput()
			delete(row, key)
			if schema.Validate(in) == nil {
				t.Errorf("missing skill-specific %s accepted", key)
			}
			in, row = makeInput()
			delete(row[key].([]any)[0].(map[string]any), "citations")
			if schema.Validate(in) == nil {
				t.Error("analysis row without citations accepted")
			}
			in, row = makeInput()
			for otherID, otherKey := range keys {
				if otherID != id {
					row[otherKey] = []any{}
					break
				}
			}
			if schema.Validate(in) == nil {
				t.Error("analysis with another skill's output accepted")
			}
		})
	}
}

func TestSubstancePrecursorPackOfficialControlMaterials(t *testing.T) {
	schema := substanceInputSchema(t, "nps-substitution-screening")
	material := func() map[string]any {
		return substanceJSON(t, `{"substance_name":"fixture-analyte","cas_no":"fixture-cas","value":"fixture-status","jurisdiction":"fixture-jurisdiction","effective_from":"2026-01-01","material_source":{"version":"fixture-v1","citations":[{"evidence_id":"fixture-official","revision":1}]}}`)
	}
	in := substanceInput(t, "nps-substitution-screening")
	if err := schema.Validate(in); err != nil {
		t.Fatalf("no material must allow []: %v", err)
	}
	in["control_materials"] = []any{material()}
	if err := schema.Validate(in); err != nil {
		t.Fatalf("valid official material rejected: %v", err)
	}
	in["control_materials"] = []any{map[string]any{"status": "unknown"}}
	if schema.Validate(in) == nil {
		t.Error("unknown output state accepted as official input material")
	}
	for _, field := range []string{"substance_name", "value", "jurisdiction", "effective_from", "material_source"} {
		m := material()
		delete(m, field)
		in["control_materials"] = []any{m}
		if schema.Validate(in) == nil {
			t.Errorf("official material without %s accepted", field)
		}
	}
	m := material()
	m["status"] = "unknown"
	in["control_materials"] = []any{m}
	if schema.Validate(in) == nil {
		t.Error("unknown material carrying fields accepted")
	}
}
