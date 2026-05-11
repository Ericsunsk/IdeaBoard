package worker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"api/internal/accounts"
)

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
		ModelSet: "gpt-image-1",
	}
	params := map[string]interface{}{"feature": "pattern-gen"}

	_, err := c.callOpenAI(context.Background(), account, "prompt", 1024, 1024, time.Second, params, "gpt-image-1")
	if err == nil {
		t.Fatalf("expected callOpenAI error")
	}

	var upstreamErr *upstreamHTTPError
	if !errors.As(err, &upstreamErr) {
		t.Fatalf("expected upstreamHTTPError, got %T (%v)", err, err)
	}
	if upstreamErr.statusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unexpected status code: got=%d want=%d", upstreamErr.statusCode, http.StatusUnprocessableEntity)
	}
}

func TestCallStabilityAINon200ReturnsTypedUpstreamHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
	}))
	defer server.Close()

	c := &ProviderExecutor{}
	account := &accounts.ApiAccount{
		ProxyURL: server.URL,
		ApiKey:   "test-key",
		ModelSet: "sdxl",
	}

	_, err := c.callStabilityAI(context.Background(), account, "prompt", 1024, 1024, time.Second, "sdxl")
	if err == nil {
		t.Fatalf("expected callStabilityAI error")
	}

	var upstreamErr *upstreamHTTPError
	if !errors.As(err, &upstreamErr) {
		t.Fatalf("expected upstreamHTTPError, got %T (%v)", err, err)
	}
	if upstreamErr.statusCode != http.StatusTooManyRequests {
		t.Fatalf("unexpected status code: got=%d want=%d", upstreamErr.statusCode, http.StatusTooManyRequests)
	}
}
