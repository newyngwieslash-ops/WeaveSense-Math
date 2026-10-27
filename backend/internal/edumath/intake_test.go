package edumath

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTextIntake (DEV-031, E51): a typed statement becomes a structured draft that still needs
// the teacher's confirmation; conventions outside a family are refused with a reason and a next
// action; nothing is stored or charged; students and unbound users cannot call it. Real math service.
func TestTextIntake(t *testing.T) { runTextIntake(t, newHarness(t, startMathService(t))) }

func TestPGTextIntake(t *testing.T) {
	runTextIntake(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

func runTextIntake(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	const path = "/api/edu/problem-drafts/normalize"
	text := "正四棱锥 P-ABCD 的底面边长为 2，高为 3，E 为 PC 的中点。求直线 BE 与平面 PAC 所成角的正弦值。"
	var before int64
	h.db.Table("edu_problem_revisions").Count(&before)
	var credits0 int64
	h.db.Table("credit_accounts").Select("available_microcredits").Where("user_id = ?", "u_alice").Scan(&credits0)

	code, out, raw, _ := h.call("u_alice", "POST", path, map[string]any{"text": text})
	if code != 200 {
		t.Fatalf("normalize: %d %s", code, raw)
	}
	d := data(out)
	st, _ := d["structured"].(map[string]any)
	sum, _ := d["summary"].(map[string]any)
	if d["state"] != "needs_confirmation" || d["family_id"] != "SOLID-ANGLE-02" || st["family_id"] != "SOLID-ANGLE-02" || sum["target"] == "" {
		t.Fatalf("draft: %s", raw)
	}
	if p, _ := st["params"].(map[string]any); p["a"] != "2" || p["h"] != "3" {
		t.Fatalf("params: %s", raw)
	}
	var after, credits1 int64
	h.db.Table("edu_problem_revisions").Count(&after)
	h.db.Table("credit_accounts").Select("available_microcredits").Where("user_id = ?", "u_alice").Scan(&credits1)
	if after != before || credits1 != credits0 {
		t.Fatalf("a draft must not store a revision or charge: revisions %d->%d credits %d->%d", before, after, credits0, credits1)
	}

	// The draft goes through the normal path: a revision that needs confirmation, then analysis.
	code, out, raw, _ = h.call("u_alice", "POST", "/api/edu/problems", map[string]any{"familyId": d["family_id"], "title": "识别的题目", "structured": st})
	if code != 201 {
		t.Fatalf("create from draft: %d %s", code, raw)
	}
	rev := data(out)["revision"].(map[string]any)
	if rev["state"] != "needs_confirmation" {
		t.Fatalf("draft revision must need confirmation: %v", rev)
	}
	h.call("u_alice", "POST", "/api/edu/problem-revisions/"+rev["id"].(string)+"/confirm", map[string]any{"inputHash": rev["inputHash"]})
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/problem-revisions/"+rev["id"].(string)+"/analyze", map[string]any{"idempotencyKey": "in-an"})
	if j := h.waitJob("u_alice", data(out)["id"].(string)); j["status"] != "succeeded" {
		t.Fatalf("analysis of the confirmed draft: %v", j)
	}

	// Refusals carry the service's reason and next action.
	refusals := []struct{ text, code, next, word string }{
		{"已知椭圆 x²/4 + y²/3 = 1，F 为其左焦点，过 F 的直线与椭圆交于 A、B 两点。求弦长 |AB| 的取值范围。", "UNSUPPORTED_FAMILY", "manual_authoring", "左焦点"},
		{"正方体 ABCD-EFGH 的棱长为 2。求直线 EC 与底面 ABCD 所成角的正弦值。", "UNSUPPORTED_FAMILY", "manual_authoring", "图形字母"},
		{"正方体 ABCD-A1B1C1D1 的棱长为 2，M 为 AB 中点，求直线 A1C 与底面 ABCD 所成角的正弦值。", "INPUT_AMBIGUOUS", "teacher_confirm_input", "结构化表单"},
		{"求数列 1, 3, 5 的前 10 项和", "UNSUPPORTED_FAMILY", "manual_authoring", "不属于"},
		{"正方体 ABCD-A1B1C1D1 的棱长为 0，求直线 A1C 与底面 ABCD 所成角的正弦值。", "CONDITION_CONFLICT", "teacher_fix_conditions", ""},
	}
	for _, r := range refusals {
		code, out, raw, _ := h.call("u_alice", "POST", path, map[string]any{"text": r.text})
		e, _ := data(out)["error"].(map[string]any)
		msg, _ := e["reason"].(string)
		if r.word == "" {
			msg = ""
		}
		if code != 422 || e["code"] != r.code || e["next_action"] != r.next || !strings.Contains(msg, r.word) {
			t.Fatalf("%q: want 422 %s/%s containing %q, got %d %s", r.text, r.code, r.next, r.word, code, raw)
		}
	}

	// Shape and size are checked before the service is called.
	for _, body := range []any{map[string]any{"text": ""}, map[string]any{"text": strings.Repeat("正", 601)}, map[string]any{"text": text, "familyId": "SOLID-ANGLE-02"}, "x"} {
		if code, _, raw, _ := h.call("u_alice", "POST", path, body); code != 400 {
			t.Fatalf("bad body %v must be 400, got %d %s", body, code, raw)
		}
	}
	if code, _, _, _ := h.call("u_bob", "POST", path, map[string]any{"text": text}, "X-Edu-Tenant", tenant); code != 403 && code != 404 {
		t.Fatalf("a non-teacher must not normalize, got %d", code)
	}
	if code, _, _, _ := h.call("u_carol", "POST", path, map[string]any{"text": text}); code != 403 && code != 404 {
		t.Fatalf("an unbound user must not normalize, got %d", code)
	}
}

// TestNormalizeContract: a draft that is not needs_confirmation, or whose family does not match
// its structured problem, is a contract error, never shown as admitted.
func TestNormalizeContract(t *testing.T) {
	for _, body := range []string{
		`{"state":"confirmed","family_id":"SOLID-ANGLE-01","structured":{"family_id":"SOLID-ANGLE-01"},"summary":{"object":"o","conditions":["c"],"target":"t","unit":"","method":"m","method_stated":false},"problem_text":"p"}`,
		`{"state":"needs_confirmation","family_id":"SOLID-ANGLE-01","structured":{"family_id":"SOLID-ANGLE-02"},"summary":{"object":"o","conditions":["c"],"target":"t","unit":"","method":"m","method_stated":false},"problem_text":"p"}`,
		`{"state":"needs_confirmation","family_id":"SOLID-ANGLE-01","structured":{"family_id":"SOLID-ANGLE-01"},"summary":{"object":"o","conditions":["c"],"target":"t","unit":"","method":"m","method_stated":false},"problem_text":"p","extra":1}`,
		`not json`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		_, err := (Client{BaseURL: srv.URL, Key: []byte(testKey)}).Normalize(context.Background(), "t1", "u1", "x")
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), ErrContract.Error()) {
			t.Fatalf("%s: want contract error, got %v", body, err)
		}
	}
}
