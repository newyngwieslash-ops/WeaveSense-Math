package edumath

import (
	"errors"
	"testing"
)

func okReport() renderCheckReport {
	f := false
	return renderCheckReport{SchemaVersion: renderCheckSchema, Checker: "render-check/1.0.0", Renderer: "preview-svg/1.1.0", Scope: "presentation",
		MathVerification: &f, Environment: "isolated_subprocess_empty_env", State: "warnings", Blocking: []string{}, Warnings: []string{"label_occlusion"},
		Checks: []renderCheckItem{{ID: "clean_reproduction", Status: "pass"}, {ID: "label_occlusion", Status: "warn", Count: 2, Codes: []string{"label_overlap"}},
			{ID: "audio_sync", Status: "not_applicable"}},
		TeacherHTMLSHA: "sha256:t", StudentHTMLSHA: "sha256:s"}
}

func TestRenderCheckReportValidation(t *testing.T) {
	if err := okReport().validate("sha256:t", "sha256:s"); err != nil {
		t.Fatalf("valid report refused: %v", err)
	}
	tr := true
	cases := map[string]func(r *renderCheckReport){
		"claims math verification": func(r *renderCheckReport) { r.MathVerification = &tr },
		"no math flag":             func(r *renderCheckReport) { r.MathVerification = nil },
		"wrong scope":              func(r *renderCheckReport) { r.Scope = "math" },
		"other preview":            func(r *renderCheckReport) { r.TeacherHTMLSHA = "sha256:x" },
		"state hides a failure": func(r *renderCheckReport) {
			r.Checks = append(r.Checks, renderCheckItem{ID: "no_host_paths", Status: "fail"})
		},
		"passed with warnings": func(r *renderCheckReport) { r.State = "passed" },
		"unknown status":       func(r *renderCheckReport) { r.Checks[0].Status = "ok" },
		"duplicate check":      func(r *renderCheckReport) { r.Checks = append(r.Checks, r.Checks[0]) },
		"text in code":         func(r *renderCheckReport) { r.Checks[1].Codes = []string{"答案 2√22/11"} },
		"phantom blocking":     func(r *renderCheckReport) { r.Blocking = []string{"clean_reproduction"}; r.State = "failed" },
	}
	for name, mut := range cases {
		r := okReport()
		r.Checks = append([]renderCheckItem(nil), r.Checks...)
		mut(&r)
		if err := r.validate("sha256:t", "sha256:s"); !errors.Is(err, ErrContract) {
			t.Fatalf("%s: want contract error, got %v", name, err)
		}
	}
	r := okReport()
	r.Checks = append(r.Checks, renderCheckItem{ID: "no_host_paths", Status: "fail", Count: 1, Codes: []string{"host_path"}})
	r.Blocking, r.State = []string{"no_host_paths"}, "failed"
	if err := r.validate("sha256:t", "sha256:s"); err != nil {
		t.Fatalf("consistent failed report refused: %v", err)
	}
	if got := mergeBlocking(`["math_not_verified"]`, []string{"render_check:no_host_paths"}); got != `["math_not_verified","render_check:no_host_paths"]` {
		t.Fatalf("merge: %s", got)
	}
	if mergeBlocking("", nil) != "" {
		t.Fatal("no codes must stay empty")
	}
}
