package routing

import "ideaboard/api/internal/platform/retrypolicy"

// AttemptFailure describes one failed execution attempt in
// provider-neutral terms (ADR-0012 rule 6). The worker (later wave) maps
// provider errors into this shape; routing classifies without importing
// provider SDKs, net, or HTTP clients.
//
// Retry taxonomy:
//
//	Retryable (may consume the single bounded model fallback):
//	  HTTP 429 / 408 / 5xx, timeouts, network errors.
//	Deterministic (terminal directly, NEVER model fallback):
//	  invalid parameters, insufficient credits, content-safety
//	  rejection, authentication/authorization failures, not-found,
//	  and successful responses with unusable payloads.
//
// Deterministic markers take precedence over transient ones: a 500
// carrying a content-safety rejection is terminal. Failures with no
// marker at all fail closed to terminal: unknown outcomes must not
// spend the fallback budget.
//
// Vision search stays outside this taxonomy (ADR-0012 rule 14). Vision
// failures must never be passed here.
type AttemptFailure struct {
	// HTTPStatus is the upstream HTTP status, or 0 when the failure
	// has no HTTP status (timeout before response, network error,
	// local finalize/storage failure, unusable payload).
	HTTPStatus int
	// Timeout marks deadline-exceeded and timeout failures.
	Timeout bool
	// NetworkError marks connection resets, refused connections,
	// DNS/transport failures, and other transient I/O errors.
	NetworkError bool
	// InvalidParams marks invalid-parameter rejections (incl. 400/422
	// business validation and unusable success payloads reported
	// here via UnusablePayload).
	InvalidParams bool
	// InsufficientCredits marks quota/credit rejections (incl. 402).
	InsufficientCredits bool
	// ContentSafetyRejected marks content-safety rejections.
	ContentSafetyRejected bool
	// AuthFailed marks authentication/authorization failures
	// (incl. 401/403).
	AuthFailed bool
	// NotFound marks not-found failures (incl. 404).
	NotFound bool
	// UnusablePayload marks successful responses whose payload cannot
	// be used (empty body, parse failure, missing image). The call
	// succeeded, so retrying another model for the same prompt is a
	// business decision, not an upstream transient: terminal.
	UnusablePayload bool
}

// Deterministic reports whether any business-error marker is set. A
// deterministic failure goes terminal directly and never triggers
// model fallback, regardless of status code or transient markers.
func (f AttemptFailure) Deterministic() bool {
	return f.InvalidParams || f.InsufficientCredits || f.ContentSafetyRejected ||
		f.AuthFailed || f.NotFound || f.UnusablePayload
}

// Retryable reports whether the failure may consume the single bounded
// model fallback to the snapshot's retry candidate.
func (f AttemptFailure) Retryable() bool {
	if f.Deterministic() {
		return false
	}
	if f.Timeout || f.NetworkError {
		return true
	}
	return RetryableHTTPStatus(f.HTTPStatus)
}

// RetryableHTTPStatus classifies an upstream HTTP status on its own:
// 429, 408, and 5xx are transient; every other status (including 0,
// meaning "no HTTP status") is not retryable by status alone.
func RetryableHTTPStatus(status int) bool {
	return retrypolicy.RetryableHTTPStatus(status)
}
