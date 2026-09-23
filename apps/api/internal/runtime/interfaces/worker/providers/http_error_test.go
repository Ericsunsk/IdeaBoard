package providers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	runtimeports "ideaboard/api/internal/runtime/ports"
)

func TestUpstreamHTTPErrorParsesRetryAfter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.test", nil)
	resp := httptest.NewRecorder()
	resp.Header().Set("Retry-After", "1.5")
	response := resp.Result()
	response.StatusCode = http.StatusTooManyRequests
	response.Request = req

	err := runtimeports.NewUpstreamHTTPErrorFromResponse(response, "busy")
	var upstreamErr *runtimeports.UpstreamHTTPError
	if !errors.As(err, &upstreamErr) {
		t.Fatalf("error type = %T", err)
	}
	if upstreamErr.RetryAfter < time.Second || upstreamErr.RetryAfter > 2*time.Second {
		t.Fatalf("retry after = %s", upstreamErr.RetryAfter)
	}
}

func TestUpstreamHTTPErrorParsesRetryAfterHTTPDate(t *testing.T) {
	retryAt := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
	retryAfter := runtimeports.ParseRetryAfter(retryAt)
	if retryAfter < time.Second || retryAfter > 3*time.Second {
		t.Fatalf("retry after = %s", retryAfter)
	}
}

func TestUpstreamHTTPErrorIgnoresInvalidRetryAfter(t *testing.T) {
	if got := runtimeports.ParseRetryAfter("not-a-delay"); got != 0 {
		t.Fatalf("retry after = %s", got)
	}
}
