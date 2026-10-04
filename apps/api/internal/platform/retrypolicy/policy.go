package retrypolicy

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ParseRetryAfter parses the RFC Retry-After forms used by upstream HTTP APIs.
// Invalid, negative, and past values return zero; overflowing values clamp to
// the largest representable duration.
func ParseRetryAfter(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil && seconds >= 0 && !math.IsInf(seconds, 0) {
		maxDuration := float64(time.Duration(1<<63 - 1))
		if seconds >= maxDuration/float64(time.Second) {
			return time.Duration(1<<63 - 1)
		}
		return time.Duration(seconds * float64(time.Second))
	}
	if when, err := http.ParseTime(raw); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

// RetryableHTTPStatus identifies transient upstream statuses.
func RetryableHTTPStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500 && status <= 599
}
