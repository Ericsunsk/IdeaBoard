package outboundhttp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"ideaboard/api/internal/platform/safehttp"
)

func testClient(t *testing.T, cfg Config) (*Client, *Metrics) {
	t.Helper()
	metrics := NewMetrics(prometheus.NewRegistry())
	cfg.Metrics = metrics
	if cfg.Sleep == nil {
		// Keep unit tests fast without sleeping real backoff delays.
		cfg.Sleep = func(context.Context, time.Duration) error { return nil }
	}
	return NewClient(cfg), metrics
}

func postRequest(t *testing.T, server *httptest.Server) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"prompt":"a cat"}`))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

func counterValue(t *testing.T, metrics *Metrics, name string, labels ...string) float64 {
	t.Helper()
	var vec *prometheus.CounterVec
	switch name {
	case "outbound_requests_total":
		vec = metrics.requests
	case "outbound_retries_total":
		vec = metrics.retries
	case "outbound_retry_after_honored_total":
		vec = metrics.retryAfterHits
	default:
		t.Fatalf("unknown metric %q", name)
	}
	return testutil.ToFloat64(vec.WithLabelValues(labels...))
}

func TestDoSucceedsWithoutRetry(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		payload, _ := io.ReadAll(r.Body)
		if string(payload) != `{"prompt":"a cat"}` {
			t.Errorf("body = %q, want replayed payload", payload)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client, metrics := testClient(t, Config{})
	resp, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{AllowHosts: []string{"127.0.0.1", "localhost"}})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", hits.Load())
	}
	if got := counterValue(t, metrics, "outbound_requests_total", "127.0.0.1", outcomeSuccess); got != 1 {
		t.Fatalf("requests success = %v, want 1", got)
	}
}

func TestDoRetriesThenSucceeds(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, metrics := testClient(t, Config{MaxAttempts: 3})
	resp, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{AllowHosts: []string{"127.0.0.1", "localhost"}})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if hits.Load() != 3 {
		t.Fatalf("hits = %d, want 3", hits.Load())
	}
	if got := counterValue(t, metrics, "outbound_retries_total", "127.0.0.1", "http_5xx"); got != 2 {
		t.Fatalf("retries http_5xx = %v, want 2", got)
	}
}

func TestDoReplaysRequestUsingExistingGetBody(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		if string(payload) != `{"prompt":"a cat"}` {
			t.Errorf("attempt %d body = %q, want replayed payload", hits.Load()+1, payload)
		}
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req := postRequest(t, server)
	firstBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(strings.NewReader(string(firstBody)))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(firstBody))), nil
	}
	client, _ := testClient(t, Config{MaxAttempts: 2})
	resp, err := client.Do(context.Background(), req, RequestOptions{AllowHosts: []string{"127.0.0.1", "localhost"}})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if hits.Load() != 2 {
		t.Fatalf("hits = %d, want 2", hits.Load())
	}
}

func TestDoTreatsNilContextAsBackground(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, _ := testClient(t, Config{MaxAttempts: 1})
	resp, err := client.Do(nil, postRequest(t, server), RequestOptions{AllowHosts: []string{"127.0.0.1", "localhost"}})
	if err != nil {
		t.Fatalf("Do(nil context) error = %v", err)
	}
	resp.Body.Close()
}

func TestSnapshotRequestBodyClosesReaderAfterReadError(t *testing.T) {
	closed := atomic.Bool{}
	reader := &failingReadCloser{closed: &closed}
	req, err := http.NewRequest(http.MethodPost, "http://example.com", reader)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	_, err = snapshotRequestBody(context.Background(), req)
	if err == nil {
		t.Fatal("snapshotRequestBody succeeded with failing reader")
	}
	if !closed.Load() {
		t.Fatal("failing request body was not closed")
	}
}

type failingReadCloser struct {
	closed *atomic.Bool
}

func (r *failingReadCloser) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func (r *failingReadCloser) Close() error {
	r.closed.Store(true)
	return nil
}

func TestDoHonorsRetryAfter(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var waited []time.Duration
	client, metrics := testClient(t, Config{
		MaxAttempts:       2,
		MaxRetryAfterWait: 20 * time.Millisecond,
		Sleep: func(_ context.Context, d time.Duration) error {
			waited = append(waited, d)
			return nil
		},
	})
	resp, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{AllowHosts: []string{"127.0.0.1", "localhost"}})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if hits.Load() != 2 {
		t.Fatalf("hits = %d, want 2", hits.Load())
	}
	if len(waited) != 1 || waited[0] != 20*time.Millisecond {
		t.Fatalf("waited = %v, want [20ms] (Retry-After 120s capped)", waited)
	}
	if got := counterValue(t, metrics, "outbound_retry_after_honored_total", "127.0.0.1"); got != 1 {
		t.Fatalf("retry-after honored = %v, want 1", got)
	}
	if got := counterValue(t, metrics, "outbound_retries_total", "127.0.0.1", "http_429"); got != 1 {
		t.Fatalf("retries http_429 = %v, want 1", got)
	}
}

func TestDoNeverRetriesDeterministicFailures(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		var hits atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(status)
		}))
		client, metrics := testClient(t, Config{MaxAttempts: 3})
		resp, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{AllowHosts: []string{"127.0.0.1", "localhost"}})
		if err != nil {
			t.Fatalf("status %d: Do() error = %v", status, err)
		}
		resp.Body.Close()
		if hits.Load() != 1 {
			t.Fatalf("status %d: hits = %d, want 1", status, hits.Load())
		}
		if got := counterValue(t, metrics, "outbound_requests_total", "127.0.0.1", outcomeClientError); got != 1 {
			t.Fatalf("status %d: client-error outcome = %v, want 1", status, got)
		}
		server.Close()
	}
}

func TestDoBoundsAttemptsOnPersistentFailure(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client, metrics := testClient(t, Config{MaxAttempts: 3})
	_, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{AllowHosts: []string{"127.0.0.1", "localhost"}})
	if err == nil {
		t.Fatal("Do() succeeded on persistent 502, want error")
	}
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusBadGateway || statusErr.Attempts != 3 {
		t.Fatalf("err = %v, want HTTPStatusError 502 after 3 attempts", err)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits = %d, want bounded 3", hits.Load())
	}
	if got := counterValue(t, metrics, "outbound_requests_total", "127.0.0.1", outcomeServerError); got != 1 {
		t.Fatalf("server-error outcome = %v, want 1", got)
	}
}

func TestDoDoesNotCountFinalAttemptAsRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client, metrics := testClient(t, Config{MaxAttempts: 1})
	_, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{AllowHosts: []string{"127.0.0.1", "localhost"}})
	if err == nil {
		t.Fatal("Do() succeeded on persistent 502")
	}
	if got := counterValue(t, metrics, "outbound_retries_total", "127.0.0.1", "http_5xx"); got != 0 {
		t.Fatalf("final failed attempt counted as retry = %v, want 0", got)
	}
}

func TestDoDoesNotRetrySafeHTTPPolicyRejection(t *testing.T) {
	client, metrics := testClient(t, Config{MaxAttempts: 3})
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	_, err = client.Do(context.Background(), req, RequestOptions{Target: "policy-test"})
	if !errors.Is(err, safehttp.ErrPolicyViolation) {
		t.Fatalf("err = %v, want safehttp policy violation", err)
	}
	if got := counterValue(t, metrics, "outbound_retries_total", "policy-test", "transport_error"); got != 0 {
		t.Fatalf("policy rejection retries = %v, want 0", got)
	}
	if got := counterValue(t, metrics, "outbound_requests_total", "policy-test", outcomeClientError); got != 1 {
		t.Fatalf("policy rejection request metric = %v, want 1 client error", got)
	}
}

func TestDoTimeoutStaysWithinBudget(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	client, metrics := testClient(t, Config{MaxAttempts: 2})
	start := time.Now()
	// One attempt consumes the whole 60ms budget, so no second attempt may
	// start: retries buy earlier recovery, never a longer budget (ADR-0010).
	_, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{
		Timeout:    60 * time.Millisecond,
		AllowHosts: []string{"127.0.0.1", "localhost"},
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Do() succeeded against a hanging server, want timeout")
	}
	if !isTimeoutError(err, nil, nil) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want timeout", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1 (budget exhausted by the timed-out attempt)", hits.Load())
	}
	if elapsed > 5*time.Second {
		t.Fatalf("elapsed = %v, want the call bounded by its timeout", elapsed)
	}
	if got := counterValue(t, metrics, "outbound_requests_total", "127.0.0.1", outcomeTimeout); got != 1 {
		t.Fatalf("timeout outcome = %v, want 1", got)
	}
}

func TestDoRetriesFastTimeoutErrors(t *testing.T) {
	// A listener that accepts and immediately resets connections produces
	// fast transport failures, leaving budget for a bounded retry.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	defer listener.Close()

	client, metrics := testClient(t, Config{MaxAttempts: 3})
	req, err := http.NewRequest(http.MethodGet, "http://"+addr, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	_, err = client.Do(context.Background(), req, RequestOptions{
		Timeout:    5 * time.Second,
		AllowHosts: []string{"127.0.0.1", "localhost"},
	})
	if err == nil {
		t.Fatal("Do() succeeded against a resetting listener, want error")
	}
	if got := counterValue(t, metrics, "outbound_retries_total", "127.0.0.1", "transport_error"); got == 0 {
		t.Fatal("transport-error retries counter stayed 0, want observed retries")
	}
}

func TestDoClampsTimeoutToMax(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	client, _ := testClient(t, Config{MaxAttempts: 1, MaxTimeout: 40 * time.Millisecond})
	start := time.Now()
	_, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{
		Timeout:    30 * time.Second,
		AllowHosts: []string{"127.0.0.1", "localhost"},
	})
	if err == nil {
		t.Fatal("Do() succeeded, want clamped-timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("elapsed = %v, want the 40ms cap to bound the call", elapsed)
	}
}

func TestDoRespectsCallerCancellation(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, metrics := testClient(t, Config{MaxAttempts: 3})
	_, err := client.Do(ctx, postRequest(t, server), RequestOptions{AllowHosts: []string{"127.0.0.1", "localhost"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled passthrough", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("hits = %d, want 0 (no send after cancel)", hits.Load())
	}
	if got := counterValue(t, metrics, "outbound_requests_total", "127.0.0.1", outcomeCanceled); got != 1 {
		t.Fatalf("canceled outcome = %v, want 1", got)
	}
}

func TestDoSingleShotOverride(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, _ := testClient(t, Config{MaxAttempts: 3})
	_, err := client.Do(context.Background(), postRequest(t, server), RequestOptions{
		MaxAttempts: 1,
		AllowHosts:  []string{"127.0.0.1", "localhost"},
	})
	if err == nil {
		t.Fatal("Do() succeeded on 503, want error")
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want single-shot 1", hits.Load())
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("120"); got != 120*time.Second {
		t.Fatalf("delta seconds = %v, want 120s", got)
	}
	if got := parseRetryAfter(""); got != 0 {
		t.Fatalf("empty = %v, want 0", got)
	}
	if got := parseRetryAfter("not-a-date"); got != 0 {
		t.Fatalf("garbage = %v, want 0", got)
	}
	future := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got <= 0 || got > 90*time.Second {
		t.Fatalf("http date = %v, want ~90s", got)
	}
	if retryableStatus(http.StatusOK) || retryableStatus(http.StatusBadRequest) || retryableStatus(http.StatusForbidden) {
		t.Fatal("2xx/4xx must not be retryable")
	}
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable} {
		if !retryableStatus(status) {
			t.Fatalf("status %d must be retryable", status)
		}
	}
}
