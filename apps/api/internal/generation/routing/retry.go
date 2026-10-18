package routing

import (
	"errors"
	"strings"

	"ideaboard/api/internal/platform/retrypolicy"
)

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

// timeoutMarkers records deadline-exceeded and timeout failures distinctly
// from network errors. Single home for the timeout family (P0-4): the
// worker and tasking must not keep their own copies.
var timeoutMarkers = []string{"timeout", "deadline exceeded"}

// networkMarkers records connection resets, refused connections,
// DNS/transport failures, and other transient I/O errors. Single home for
// the network family (P0-4).
var networkMarkers = []string{
	"network", "connection reset", "temporary", "econnreset", "econnrefused",
}

// contentSafetyMarkers forces terminal convergence for content-safety
// rejections regardless of status code. Matching is a fail-closed direction
// only (terminal, never fallback), so the list stays tight to canonical
// provider phrasing. Single home (P0-4).
var contentSafetyMarkers = []string{
	"content safety", "content-safety", "content_safety", "safety rejection",
}

func containsFailureMarker(haystack string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(haystack, marker) {
			return true
		}
	}
	return false
}

// httpStatusCoder extracts an upstream HTTP status through the structural
// seam (concrete upstream errors implement HTTPStatus() int), without
// importing provider SDKs, net, or HTTP clients.
type httpStatusCoder interface {
	HTTPStatus() int
}

// ClassifyFailure maps one provider-seam failure onto the AttemptFailure
// taxonomy: the single retry taxonomy (P0-4, ADR-0012 rule 6). Deterministic
// markers take precedence (a 500 carrying a business rejection is
// terminal); a failure with no marker at all fails closed to terminal and
// never spends the fallback budget.
func ClassifyFailure(httpStatus int, message string) AttemptFailure {
	var failure AttemptFailure
	failure.HTTPStatus = httpStatus
	lowered := strings.ToLower(message)
	switch httpStatus {
	case 400, 422:
		failure.InvalidParams = true
	case 401, 403:
		failure.AuthFailed = true
	case 402:
		failure.InsufficientCredits = true
	case 404:
		failure.NotFound = true
	}
	if containsFailureMarker(lowered, contentSafetyMarkers) {
		failure.ContentSafetyRejected = true
	}
	if failure.Deterministic() {
		return failure
	}
	if containsFailureMarker(lowered, timeoutMarkers) {
		failure.Timeout = true
	}
	if containsFailureMarker(lowered, networkMarkers) {
		failure.NetworkError = true
	}
	return failure
}

// ClassifyError maps one provider-seam error onto the AttemptFailure
// taxonomy: the upstream HTTP status arrives through the structural
// HTTPStatus() int seam, timeout/network/content-safety markers through the
// message families above. A nil error maps to the zero failure (which is
// terminal and never retryable). This is the only provider-error mapping;
// callers must not sniff timeout/network strings themselves.
func ClassifyError(err error) AttemptFailure {
	if err == nil {
		return AttemptFailure{}
	}
	status := 0
	var coder httpStatusCoder
	if errors.As(err, &coder) {
		status = coder.HTTPStatus()
	}
	return ClassifyFailure(status, err.Error())
}
