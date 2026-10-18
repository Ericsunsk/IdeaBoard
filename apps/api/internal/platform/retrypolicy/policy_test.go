package retrypolicy

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfterDeltaSeconds(t *testing.T) {
	if got := ParseRetryAfter("120"); got != 120*time.Second {
		t.Fatalf("ParseRetryAfter(120) = %v, want 2m0s", got)
	}
}

func TestParseRetryAfterRejectsInvalidAndNegative(t *testing.T) {
	for _, raw := range []string{"", "  ", "not-a-delay", "-5", "99999999999999999999999x"} {
		if got := ParseRetryAfter(raw); got != 0 {
			t.Fatalf("ParseRetryAfter(%q) = %v, want 0", raw, got)
		}
	}
}

func TestRetryableHTTPStatusCoversTransientOnly(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, 500, 502, 503} {
		if !RetryableHTTPStatus(status) {
			t.Fatalf("RetryableHTTPStatus(%d) = false, want true", status)
		}
	}
	for _, status := range []int{200, 400, 401, 403, 404, 422} {
		if RetryableHTTPStatus(status) {
			t.Fatalf("RetryableHTTPStatus(%d) = true, want false", status)
		}
	}
}
