package providers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	accounts "ideaboard/api/internal/accountpool/accounts/domain"
)

func TestUpstreamHTTPErrorParsesRetryAfter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.test", nil)
	resp := httptest.NewRecorder()
	resp.Header().Set("Retry-After", "1.5")
	response := resp.Result()
	response.StatusCode = http.StatusTooManyRequests
	response.Request = req

	err := newUpstreamHTTPErrorFromResponse(response, "busy")
	var upstreamErr *upstreamHTTPError
	if !errors.As(err, &upstreamErr) {
		t.Fatalf("error type = %T", err)
	}
	if upstreamErr.RetryAfter < time.Second || upstreamErr.RetryAfter > 2*time.Second {
		t.Fatalf("retry after = %s", upstreamErr.RetryAfter)
	}
}

func TestUpstreamHTTPErrorParsesRetryAfterHTTPDate(t *testing.T) {
	retryAt := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
	retryAfter := parseRetryAfter(retryAt)
	if retryAfter < time.Second || retryAfter > 3*time.Second {
		t.Fatalf("retry after = %s", retryAfter)
	}
}

func TestUpstreamHTTPErrorIgnoresInvalidRetryAfter(t *testing.T) {
	if got := parseRetryAfter("not-a-delay"); got != 0 {
		t.Fatalf("retry after = %s", got)
	}
}

func TestCallOpenAINon200ReturnsTypedUpstreamHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"invalid_image_request"}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		Models:   []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
	}
	params := map[string]interface{}{"feature": "generation"}

	_, err := c.callOpenAIForTest(context.Background(), account, "prompt", 1024, 1024, time.Second, params, "gpt-image-1")
	if err == nil {
		t.Fatalf("expected callOpenAI error")
	}

	var upstreamErr *upstreamHTTPError
	if !errors.As(err, &upstreamErr) {
		t.Fatalf("expected upstreamHTTPError, got %T (%v)", err, err)
	}
	if upstreamErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unexpected status code: got=%d want=%d", upstreamErr.StatusCode, http.StatusUnprocessableEntity)
	}
}
