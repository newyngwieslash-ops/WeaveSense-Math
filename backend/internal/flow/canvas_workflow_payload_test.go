package flow

import (
	"errors"
	"strings"
	"testing"
)

func TestCanvasWorkflowPayloadRejectsCredentials(t *testing.T) {
	cases := []any{
		map[string]any{"metadata": map[string]any{"apiKey": "review-secret"}},
		map[string]any{"headers": map[string]any{"Authorization": "Bearer review-secret"}},
		map[string]any{"Cookie": "session=review-secret"},
		map[string]any{"client_secret": "review-secret"},
		map[string]any{"private-key": "review-secret"},
		"https://user:review-secret@example.com/data",
		"https://example.com/data?token=review-secret",
		"source https://example.com/data?X-Amz-Signature=review-secret next",
		"https://example.com/data?api%5Fkey=review-secret",
		"https://example.com/#access_token=review-secret",
		"Authorization: Bearer review-secret",
		"Cookie: session=review-secret",
		"apiKey=review-secret",
		"sk-abcdefghijklmnopqrstuvwxyz123456",
		`{"metadata":{"apiKey":"review-secret"}}`,
	}
	for i, v := range cases {
		if e := ValidateCanvasWorkflowPayload(v); !errors.Is(e, ErrCanvasWorkflowSensitivePayload) {
			t.Errorf("case %d not rejected: %v", i, e)
		} else if strings.Contains(e.Error(), "review-secret") {
			t.Fatal("error disclosed credential")
		}
	}
}

func TestCanvasWorkflowPayloadAllowsBusinessData(t *testing.T) {
	for _, v := range []any{
		map[string]any{"source_url": "https://example.com/articles?q=energy&limit=10", "text": "API keys must not be shared; token count is 42", "token_count": 42},
		"https://example.com/path?keyword=apiKey",
		"The report says authorization was declined. Cookies are a food category.",
		map[string]any{"credential_ref": "vault:item-123", "apiKey": ""},
		`{"report":{"title":"Normal business text"}}`,
	} {
		if e := ValidateCanvasWorkflowPayload(v); e != nil {
			t.Fatalf("safe data rejected: %v", e)
		}
	}
}

func TestCanvasWorkflowPayloadBounds(t *testing.T) {
	if e := ValidateCanvasWorkflowPayload(strings.Repeat("x", CanvasWorkflowPayloadMaxBytes)); !errors.Is(e, ErrCanvasWorkflowPayloadLimit) {
		t.Fatalf("oversize: %v", e)
	}
	var v any = "leaf"
	for i := 0; i < CanvasWorkflowPayloadMaxDepth+1; i++ {
		v = map[string]any{"nested": v}
	}
	if e := ValidateCanvasWorkflowPayload(v); !errors.Is(e, ErrCanvasWorkflowPayloadLimit) {
		t.Fatalf("depth: %v", e)
	}
	if e := ValidateCanvasWorkflowPayload(make([]any, CanvasWorkflowPayloadMaxValues+1)); !errors.Is(e, ErrCanvasWorkflowPayloadLimit) {
		t.Fatalf("values: %v", e)
	}
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	if e := ValidateCanvasWorkflowPayload(cyclic); e == nil || strings.Contains(e.Error(), "self") {
		t.Fatalf("invalid JSON must return generic error: %v", e)
	}
}

func TestCanvasWorkflowErrorSanitization(t *testing.T) {
	for _, s := range []string{"request failed Authorization: Bearer review-secret", "upstream https://user:review-secret@example.com/path", "request failed apiKey=review-secret", "Cookie: session=review-secret", `{"error":{"apiKey":"review-secret"}}`} {
		out := SanitizeCanvasWorkflowError(s)
		if strings.Contains(out, "review-secret") || out == "" {
			t.Fatalf("unsafe error: %q", out)
		}
	}
	safe := "source unavailable: https://example.com/articles?q=energy"
	if out := SanitizeCanvasWorkflowError(safe); out != safe {
		t.Fatalf("business error changed: %q", out)
	}
	if len(SanitizeCanvasWorkflowError(strings.Repeat("x", CanvasWorkflowPayloadMaxBytes+1))) > 1024 {
		t.Fatal("error not bounded")
	}
}
