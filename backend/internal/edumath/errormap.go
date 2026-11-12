package edumath

import (
	"errors"
	"strings"
)

// VendorOutcome is one error report from a vendor adapter or the fake vendor
// used in contract tests (DEV-058, E96). Sent means the request may have left
// this process; Definite means we have a definitive success or failure body.
type VendorOutcome struct {
	Provider   string
	Code       string
	HTTPStatus int
	Retryable  bool
	Sent       bool
	Definite   bool
	Message    string
}

// RetryDecision is the platform mapping of a vendor outcome. PaidRetry is never
// true when the request may have been accepted but the result is unknown
// (hard constraint 5): those cases are reconciled, never blindly retried.
type RetryDecision struct {
	PlatformCode string `json:"platformCode"`
	Action       string `json:"action"`
	PaidRetry    bool   `json:"paidRetry"`
	NextAction   string `json:"nextAction"`
	Reason       string `json:"reason"`
}

const (
	actionReconcile  = "reconcile"
	actionRetryLater = "retry_later"
	actionRetryNew   = "retry_new_key"
	actionDoNotRetry = "do_not_retry"
	actionContact    = "contact_admin"
)

// known vendor / platform codes and how a *definite* outcome is handled.
// Unknown codes with a definite body fall through to contact_admin.
var definiteCodeAction = map[string]RetryDecision{
	"INPUT_AMBIGUOUS":      {PlatformCode: "INPUT_AMBIGUOUS", Action: actionDoNotRetry, NextAction: "teacher_confirm_input"},
	"CONDITION_CONFLICT":   {PlatformCode: "CONDITION_CONFLICT", Action: actionDoNotRetry, NextAction: "teacher_fix_conditions"},
	"UNSUPPORTED_FAMILY":   {PlatformCode: "UNSUPPORTED_FAMILY", Action: actionDoNotRetry, NextAction: "manual_authoring"},
	"PARAMETER_LIMIT":      {PlatformCode: "PARAMETER_LIMIT", Action: actionDoNotRetry, NextAction: "adjust_parameters"},
	"METHOD_MISMATCH":      {PlatformCode: "METHOD_MISMATCH", Action: actionDoNotRetry, NextAction: "change_method_profile"},
	"VALIDATION_FAILED":    {PlatformCode: "VALIDATION_FAILED", Action: actionDoNotRetry, NextAction: "teacher_review"},
	"REVISION_CONFLICT":    {PlatformCode: "REVISION_CONFLICT", Action: actionDoNotRetry, NextAction: "reload_revision"},
	"POLICY_REVOKED":       {PlatformCode: "POLICY_REVOKED", Action: actionDoNotRetry, NextAction: "contact_admin"},
	"BUDGET_EXCEEDED":      {PlatformCode: "BUDGET_EXCEEDED", Action: actionDoNotRetry, NextAction: "raise_budget"},
	"GENERATION_PAUSED":    {PlatformCode: "GENERATION_PAUSED", Action: actionDoNotRetry, NextAction: "wait_for_resume"},
	"RATE_LIMITED":         {PlatformCode: "RATE_LIMITED", Action: actionRetryLater, PaidRetry: true, NextAction: "retry_later"},
	"PROVIDER_UNAVAILABLE": {PlatformCode: "PROVIDER_UNAVAILABLE", Action: actionRetryLater, PaidRetry: true, NextAction: "retry_later"},
	"RENDER_FAILED":        {PlatformCode: "RENDER_FAILED", Action: actionRetryNew, PaidRetry: true, NextAction: "retry_render"},
	"INTERNAL":             {PlatformCode: "INTERNAL", Action: actionRetryNew, PaidRetry: true, NextAction: "retry_later"},
	"TIMEOUT_UNDETERMINED": {PlatformCode: "TIMEOUT_UNDETERMINED", Action: actionRetryNew, PaidRetry: true, NextAction: "restart_new_key"},
	"OUTCOME_UNKNOWN":      {PlatformCode: "OUTCOME_UNKNOWN", Action: actionReconcile, PaidRetry: false, NextAction: "reconcile"},
	"RECEIPT_INVALID":      {PlatformCode: "RECEIPT_INVALID", Action: actionContact, PaidRetry: false, NextAction: "contact_admin"},
}

// httpStatusCode is used when the vendor body has no code of its own.
var httpStatusCode = map[int]string{
	400: "VALIDATION_FAILED",
	401: "POLICY_REVOKED",
	403: "POLICY_REVOKED",
	408: "TIMEOUT_UNDETERMINED",
	409: "REVISION_CONFLICT",
	429: "RATE_LIMITED",
	500: "INTERNAL",
	502: "PROVIDER_UNAVAILABLE",
	503: "PROVIDER_UNAVAILABLE",
	504: "TIMEOUT_UNDETERMINED",
}

// MapVendorOutcome translates a vendor or transport error into a platform
// retry decision. A vendor Retryable=true flag is ignored when the outcome is
// not definite: that path is always reconcile with PaidRetry=false.
func MapVendorOutcome(o VendorOutcome) RetryDecision {
	if o.Sent && !o.Definite {
		return RetryDecision{
			PlatformCode: "OUTCOME_UNKNOWN",
			Action:       actionReconcile,
			PaidRetry:    false,
			NextAction:   "reconcile",
			Reason:       "request may have been accepted; reconcile, never blindly retry a paid call",
		}
	}
	code := strings.TrimSpace(strings.ToUpper(o.Code))
	if code == "" {
		code = httpStatusCode[o.HTTPStatus]
	}
	if !o.Sent && (code == "" || code == "PROVIDER_UNAVAILABLE" || code == "INTERNAL") {
		return RetryDecision{
			PlatformCode: "PROVIDER_UNAVAILABLE",
			Action:       actionRetryLater,
			PaidRetry:    true,
			NextAction:   "retry_later",
			Reason:       "request did not leave this process; a retry is not a second paid call",
		}
	}
	if d, ok := definiteCodeAction[code]; ok {
		d.Reason = "definite vendor outcome"
		if d.Action == actionReconcile {
			d.PaidRetry = false
			d.Reason = "outcome unknown; reconcile, never blindly retry a paid call"
		}
		return d
	}
	return RetryDecision{
		PlatformCode: "INTERNAL",
		Action:       actionContact,
		PaidRetry:    false,
		NextAction:   "contact_admin",
		Reason:       "unmapped vendor code; do not blindly retry",
	}
}

// MapTransportError maps a platform transport error (including ErrServiceSent)
// onto the same decision table.
func MapTransportError(err error, sent bool) RetryDecision {
	if err == nil {
		return RetryDecision{PlatformCode: "", Action: "", PaidRetry: false, Reason: "no error"}
	}
	if errors.Is(err, ErrServiceSent) || sent {
		return MapVendorOutcome(VendorOutcome{Provider: "math", Code: "OUTCOME_UNKNOWN", Sent: true, Definite: false, Message: err.Error()})
	}
	return MapVendorOutcome(VendorOutcome{Provider: "math", Code: "PROVIDER_UNAVAILABLE", Sent: false, Definite: true, Message: err.Error()})
}
