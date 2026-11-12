package edumath

import (
	"errors"
	"fmt"
	"testing"
)

// TestMapVendorOutcome (DEV-058, E96): a fake vendor covering the contract
// codes; unknown results are never blindly retried as a paid call, even when
// the vendor marks them retryable. Real vendor codes still need U8.
func TestMapVendorOutcome(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		in     VendorOutcome
		code   string
		action string
		paid   bool
	}{
		{"unknown after send ignores vendor retryable", VendorOutcome{Provider: "fake", Code: "RATE_LIMITED", Retryable: true, Sent: true, Definite: false}, "OUTCOME_UNKNOWN", actionReconcile, false},
		{"transport unknown", VendorOutcome{Provider: "fake", Sent: true, Definite: false, HTTPStatus: 0}, "OUTCOME_UNKNOWN", actionReconcile, false},
		{"explicit OUTCOME_UNKNOWN", VendorOutcome{Provider: "fake", Code: "OUTCOME_UNKNOWN", Sent: true, Definite: true}, "OUTCOME_UNKNOWN", actionReconcile, false},
		{"timeout undetermined is a new-key retry, not the same call", VendorOutcome{Provider: "fake", Code: "TIMEOUT_UNDETERMINED", Sent: true, Definite: true}, "TIMEOUT_UNDETERMINED", actionRetryNew, true},
		{"rate limited definite", VendorOutcome{Provider: "fake", Code: "RATE_LIMITED", Retryable: true, Sent: true, Definite: true}, "RATE_LIMITED", actionRetryLater, true},
		{"http 429 without code", VendorOutcome{Provider: "fake", HTTPStatus: 429, Sent: true, Definite: true}, "RATE_LIMITED", actionRetryLater, true},
		{"http 503", VendorOutcome{Provider: "fake", HTTPStatus: 503, Sent: true, Definite: true}, "PROVIDER_UNAVAILABLE", actionRetryLater, true},
		{"validation", VendorOutcome{Provider: "fake", Code: "VALIDATION_FAILED", Sent: true, Definite: true}, "VALIDATION_FAILED", actionDoNotRetry, false},
		{"budget", VendorOutcome{Provider: "fake", Code: "BUDGET_EXCEEDED", Sent: true, Definite: true}, "BUDGET_EXCEEDED", actionDoNotRetry, false},
		{"never sent is safe to retry", VendorOutcome{Provider: "fake", Code: "PROVIDER_UNAVAILABLE", Sent: false, Definite: true}, "PROVIDER_UNAVAILABLE", actionRetryLater, true},
		{"never sent, no code", VendorOutcome{Provider: "fake", Sent: false, Definite: false}, "PROVIDER_UNAVAILABLE", actionRetryLater, true},
		{"unmapped definite code is not retried", VendorOutcome{Provider: "fake", Code: "WEIRD_VENDOR_X", Sent: true, Definite: true}, "INTERNAL", actionContact, false},
		{"internal definite may start a new key", VendorOutcome{Provider: "fake", Code: "INTERNAL", Retryable: true, Sent: true, Definite: true}, "INTERNAL", actionRetryNew, true},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			d := MapVendorOutcome(cs.in)
			if d.PlatformCode != cs.code || d.Action != cs.action || d.PaidRetry != cs.paid {
				t.Fatalf("got %+v, want code=%s action=%s paid=%v", d, cs.code, cs.action, cs.paid)
			}
			if !cs.in.Definite && cs.in.Sent && d.PaidRetry {
				t.Fatalf("paid retry on unknown outcome: %+v", d)
			}
		})
	}

	if d := MapTransportError(fmt.Errorf("%w: timeout", ErrServiceSent), true); d.Action != actionReconcile || d.PaidRetry {
		t.Fatalf("ErrServiceSent must reconcile without paid retry: %+v", d)
	}
	if d := MapTransportError(errors.New("connection refused"), false); d.PlatformCode != "PROVIDER_UNAVAILABLE" || !d.PaidRetry {
		t.Fatalf("unsent transport error: %+v", d)
	}
	if d := MapTransportError(nil, false); d.Action != "" {
		t.Fatalf("nil error: %+v", d)
	}

	// Every math-service contract code in errors.py is mapped when definite.
	for _, code := range []string{"INPUT_AMBIGUOUS", "CONDITION_CONFLICT", "UNSUPPORTED_FAMILY", "PARAMETER_LIMIT",
		"METHOD_MISMATCH", "VALIDATION_FAILED", "RENDER_FAILED", "BUDGET_EXCEEDED", "RATE_LIMITED",
		"PROVIDER_UNAVAILABLE", "OUTCOME_UNKNOWN", "REVISION_CONFLICT", "POLICY_REVOKED", "TIMEOUT_UNDETERMINED", "INTERNAL"} {
		d := MapVendorOutcome(VendorOutcome{Provider: "math", Code: code, Sent: true, Definite: true})
		if d.PlatformCode != code {
			t.Fatalf("%s mapped to %s", code, d.PlatformCode)
		}
		if code == "OUTCOME_UNKNOWN" && d.PaidRetry {
			t.Fatalf("OUTCOME_UNKNOWN must not allow a paid retry")
		}
	}
}
