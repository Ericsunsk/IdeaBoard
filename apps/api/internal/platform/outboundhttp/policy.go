package outboundhttp

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// parseRetryAfter parses a Retry-After header value: delta-seconds or an
// HTTP date. It mirrors runtime ports' parser locally because platform
// packages must not import business contexts. Unparseable or past values
// read as zero (no wait); callers fall back to backoff.
func parseRetryAfter(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds * float64(time.Second))
	}
	if when, err := http.ParseTime(raw); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

// retryableStatus reports whether an HTTP status is worth one more attempt:
// 408/429 (client should wait and retry) and 5xx (server-side transient).
// Every other status — including 4xx auth/quota/input failures — is final.
func retryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	default:
		return status >= http.StatusInternalServerError
	}
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
