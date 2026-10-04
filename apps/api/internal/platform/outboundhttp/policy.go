package outboundhttp

import (
	"net/http"
	"time"

	"ideaboard/api/internal/platform/retrypolicy"
)

// parseRetryAfter parses a Retry-After header value: delta-seconds or an
// HTTP date. It mirrors runtime ports' parser locally because platform
// packages must not import business contexts. Unparseable or past values
// read as zero (no wait); callers fall back to backoff.
func parseRetryAfter(raw string) time.Duration {
	return retrypolicy.ParseRetryAfter(raw)
}

// retryableStatus reports whether an HTTP status is worth one more attempt:
// 408/429 (client should wait and retry) and 5xx (server-side transient).
// Every other status — including 4xx auth/quota/input failures — is final.
func retryableStatus(status int) bool {
	return retrypolicy.RetryableHTTPStatus(status)
}

// retryReason classifies one retry for logs and metrics. It stays a small
// closed set so metric cardinality never grows with upstream bodies.
func retryReason(status int, timedOut, transport bool) string {
	switch {
	case timedOut:
		return "timeout"
	case transport:
		return "transport_error"
	case status == http.StatusTooManyRequests:
		return "http_429"
	case status == http.StatusRequestTimeout:
		return "http_408"
	case status >= http.StatusInternalServerError:
		return "http_5xx"
	default:
		return "unknown"
	}
}
