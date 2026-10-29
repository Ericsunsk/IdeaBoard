package providerinfra

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	aigatewayports "ideaboard/api/internal/aigateway/ports"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/settings"
	runtimeports "ideaboard/api/internal/runtime/ports"
	"ideaboard/api/internal/testsupport/testenv"

	"github.com/prometheus/client_golang/prometheus"
)

type gatewayTestOutboundPolicy struct{}

func (gatewayTestOutboundPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: []string{"localhost", "127.0.0.1", "::1"}}, nil
}

func TestMain(m *testing.M) {
	restore := safehttp.ConfigurePolicyProvider(gatewayTestOutboundPolicy{})
	code := m.Run()
	restore()
	os.Exit(code)
}

func TestNewGatewayClientRejectsBadConfig(t *testing.T) {
	for _, cfg := range []GatewayClientConfig{
		{},
		{BaseURL: "https://gateway.ai.cloudflare.com/v1/a/g"},
		{Token: "token"},
		{BaseURL: "ftp://gateway.ai.cloudflare.com/v1/a/g", Token: "token"},
		{BaseURL: "https:///v1/a/g", Token: "token"},
	} {
		if _, err := NewGatewayClient(cfg); err == nil {
			t.Fatalf("NewGatewayClient(%+v) succeeded, want error", cfg)
		}
	}
	// Open egress: any http(s) host passes (audited, not allowlisted).
	for _, cfg := range []GatewayClientConfig{
		{BaseURL: "https://gateway.ai.cloudflare.com/v1/a/g", Token: "token"},
		{BaseURL: "https://gateway.example.com/v1/a/g", Token: "token"},
	} {
		if _, err := NewGatewayClient(cfg); err != nil {
			t.Fatalf("NewGatewayClient(%+v) error = %v, want open-host pass", cfg, err)
		}
	}
	if _, err := NewGatewayClient(GatewayClientConfig{
		BaseURL: "http://127.0.0.1:8080/v1/a/g",
		Token:   "token",
	}); err != nil {
		t.Fatalf("NewGatewayClient(loopback) error = %v, want nil", err)
	}
}

func TestGatewayRequestValidation(t *testing.T) {
	valid := runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/chat/completions",
		},
		Body: []byte(`{"model":"openai/gpt-4o-mini"}`),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	emptyProvider := valid
	emptyProvider.Endpoint.Provider = "  "
	if err := emptyProvider.Validate(); err == nil {
		t.Fatal("empty provider accepted")
	}
	emptyPath := valid
	emptyPath.Endpoint.Path = ""
	if err := emptyPath.Validate(); err == nil {
		t.Fatal("empty path accepted")
	}
	native := valid
	native.Endpoint = runtimeports.GatewayEndpoint{
		Provider: "openai",
		Path:     "v1/images/generations",
	}
	if err := native.Validate(); err != nil {
		t.Fatalf("native request rejected: %v", err)
	}
	for _, mutate := range []func(*runtimeports.GatewayRequest){
		func(r *runtimeports.GatewayRequest) { r.Body = nil },
		func(r *runtimeports.GatewayRequest) { r.MaxAttempts = 6 },
		func(r *runtimeports.GatewayRequest) { r.MaxAttempts = -1 },
	} {
		req := valid
		mutate(&req)
		if err := req.Validate(); err == nil {
			t.Fatalf("invalid request accepted: %+v", req)
		}
	}
	empty := valid
	empty.Endpoint = runtimeports.GatewayEndpoint{}
	if err := empty.Validate(); err == nil {
		t.Fatal("empty endpoint accepted")
	}
}

func TestGatewayClientUsesRequestTimeAllowlistWithoutAppendingTarget(t *testing.T) {
	var capture gatewayCapture
	server := newGatewayCaptureServer(t, &capture, http.StatusOK, nil, `{}`)
	defer server.Close()
	client := newGatewayTestClient(t, server)
	policy := &mutableGatewayTestPolicy{hosts: []string{"127.0.0.1"}}
	restore := safehttp.ConfigurePolicyProvider(policy)
	defer restore()

	if _, err := client.Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/chat/completions",
		},
		Body: []byte(`{}`),
	}); err != nil {
		t.Fatalf("authorized gateway request failed: %v", err)
	}
	policy.hosts = []string{"gateway.example.invalid"}
	if _, err := client.Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/chat/completions",
		},
		Body: []byte(`{}`),
	}); err == nil {
		t.Fatal("gateway request succeeded after request-time allowlist revoked")
	}
}

type mutableGatewayTestPolicy struct {
	hosts []string
}

func (p *mutableGatewayTestPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: append([]string(nil), p.hosts...)}, nil
}

type gatewayCapture struct {
	method      string
	path        string
	header      http.Header
	body        []byte
	contentType string
}

func newGatewayCaptureServer(t *testing.T, capture *gatewayCapture, status int, responseHeaders map[string]string, responseBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.method = r.Method
		capture.path = r.URL.Path
		capture.header = r.Header.Clone()
		capture.contentType = r.Header.Get("Content-Type")
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		capture.body = payload
		for key, value := range responseHeaders {
			w.Header().Set(key, value)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(responseBody))
	}))
}

func newGatewayTestClient(t *testing.T, server *httptest.Server) *GatewayClient {
	t.Helper()
	// Direct construction: the httptest server is loopback by design.
	return &GatewayClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(server.URL), "/"),
		token:    "test-token",
		timeout:  runtimeports.GatewayDefaultTimeout,
		maxBytes: defaultGatewayMaxResponseBytes,
		outbound: safehttp.NewClient(safehttp.Config{
			Timeout:     runtimeports.GatewayDefaultTimeout,
			MaxTimeout:  runtimeports.GatewayDefaultTimeout,
			MaxAttempts: 1,
		}),
		target:     defaultGatewayOutboundTarget,
		resilience: NewGatewayResilience(DefaultGatewayResilienceConfig(), nil),
	}
}

// newGatewayFastRetryClient keeps retry semantics but skips real waiting so
// exhaustion tests stay fast.
func newGatewayFastRetryClient(t *testing.T, server *httptest.Server) *GatewayClient {
	t.Helper()
	return &GatewayClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(server.URL), "/"),
		token:    "test-token",
		timeout:  runtimeports.GatewayDefaultTimeout,
		maxBytes: defaultGatewayMaxResponseBytes,
		outbound: safehttp.NewClient(safehttp.Config{
			MaxAttempts:       3,
			MaxRetryAfterWait: time.Second,
			Sleep:             func(context.Context, time.Duration) error { return nil },
			Metrics:           safehttp.NewMetrics(prometheus.NewRegistry()),
		}),
		target:     defaultGatewayOutboundTarget,
		resilience: NewGatewayResilience(DefaultGatewayResilienceConfig(), nil),
	}
}

func assertGatewayBaseHeaders(t *testing.T, capture *gatewayCapture, taskID, feature string) {
	t.Helper()
	if got := capture.header.Get(aigatewayports.GatewayHeaderAuthorization); got != "Bearer test-token" {
		t.Fatalf("auth header = %q, want Bearer test-token", got)
	}
	if got := capture.header.Get(aigatewayports.GatewayHeaderCollectLogPayload); got != "false" {
		t.Fatalf("collect-log-payload = %q, want false", got)
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(capture.header.Get(aigatewayports.GatewayHeaderMetadata)), &metadata); err != nil {
		t.Fatalf("metadata header is not JSON: %v", err)
	}
	if metadata[runtimeports.GatewayMetadataTaskID] != taskID || metadata[runtimeports.GatewayMetadataFeature] != feature {
		t.Fatalf("metadata = %v, want task/feature", metadata)
	}
	if capture.contentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", capture.contentType)
	}
}

func TestGatewayClientForwardsExecutionTokenAsIdempotencyKey(t *testing.T) {
	var capture gatewayCapture
	server := newGatewayCaptureServer(t, &capture, http.StatusOK, nil, `{}`)
	defer server.Close()

	_, err := newGatewayTestClient(t, server).Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/chat/completions",
		},
		Body:           []byte(`{}`),
		ExecutionToken: "exec-123",
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if got := capture.header.Get(aigatewayports.GatewayHeaderIdempotencyKey); got != "exec-123" {
		t.Fatalf("idempotency key = %q, want exec-123", got)
	}

	_, err = newGatewayTestClient(t, server).Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/chat/completions",
		},
		Body: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if got := capture.header.Get(aigatewayports.GatewayHeaderIdempotencyKey); got != "" {
		t.Fatalf("idempotency key = %q, want empty when no execution token", got)
	}
}

func TestGatewayClientImageRequestSkipsCache(t *testing.T) {
	var capture gatewayCapture
	server := newGatewayCaptureServer(t, &capture, http.StatusOK,
		map[string]string{aigatewayports.GatewayHeaderCacheStatus: "MISS"}, `{"ok":true}`)
	defer server.Close()

	resp, err := newGatewayTestClient(t, server).Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/images/generations",
		},
		Model:   "openai/gpt-image-1",
		TaskID:  "task-1",
		Feature: "render",
		Body:    []byte(`{"prompt":"a cat"}`),
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if capture.method != http.MethodPost || capture.path != "/openai/v1/images/generations" {
		t.Fatalf("request = %s %s, want POST /openai/v1/images/generations", capture.method, capture.path)
	}
	assertGatewayBaseHeaders(t, &capture, "task-1", "render")
	if got := capture.header.Get(aigatewayports.GatewayHeaderSkipCache); got != "true" {
		t.Fatalf("skip-cache = %q, want true", got)
	}
	if resp.Receipt.CacheHit {
		t.Fatal("MISS response reported as cache hit")
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Fatalf("body = %q", resp.Body)
	}
}

func TestGatewayClientProbeIsSingleShot(t *testing.T) {
	var capture gatewayCapture
	server := newGatewayCaptureServer(t, &capture, http.StatusOK, nil, `{}`)
	defer server.Close()

	_, err := newGatewayTestClient(t, server).Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/models",
		},
		TaskID:      "probe-1",
		Method:      http.MethodGet,
		MaxAttempts: 1,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if got := capture.header.Get(aigatewayports.GatewayHeaderMaxAttempts); got != "1" {
		t.Fatalf("max-attempts = %q, want 1", got)
	}
	if capture.method != http.MethodGet || capture.path != "/openai/v1/models" {
		t.Fatalf("request = %s %s, want GET /openai/v1/models", capture.method, capture.path)
	}
	if got := capture.header.Get(aigatewayports.GatewayHeaderSkipCache); got != "true" {
		t.Fatalf("skip-cache = %q, want true", got)
	}
}

func TestGatewayClientSurfacesUpstreamErrors(t *testing.T) {
	var capture gatewayCapture
	server := newGatewayCaptureServer(t, &capture, http.StatusTooManyRequests, nil, `{"error":"slow down"}`)
	defer server.Close()

	_, err := newGatewayTestClient(t, server).Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/chat/completions",
		},
		Body: []byte(`{}`),
	})
	if err == nil {
		t.Fatal("Do() succeeded on 429, want UpstreamHTTPError")
	}
	var upstreamErr *runtimeports.UpstreamHTTPError
	if !errors.As(err, &upstreamErr) || upstreamErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("err = %v, want UpstreamHTTPError 429", err)
	}
	if status, ok := runtimeports.ProviderHTTPStatus(err); !ok || status != http.StatusTooManyRequests {
		t.Fatalf("ProviderHTTPStatus() = %d, %v, want 429, true", status, ok)
	}
}

func TestGatewayClientDoesNotRetryGatewayRequestsAfter503(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"upstream outcome is ambiguous"}`))
	}))
	defer server.Close()

	_, err := newGatewayTestClient(t, server).Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/images/generations",
		},
		Model: "openai/gpt-image-1",
		Body:  []byte(`{"prompt":"a cat"}`),
	})
	var upstreamErr *runtimeports.UpstreamHTTPError
	if !errors.As(err, &upstreamErr) || upstreamErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Do() error = %v, want UpstreamHTTPError 503", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want single gateway send", attempts)
	}
}

func TestGatewayClientPreservesRetryAfterForWorkerRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"slow down"}`))
	}))
	defer server.Close()

	_, err := newGatewayFastRetryClient(t, server).Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/chat/completions",
		},
		Body: []byte(`{}`),
	})
	if err == nil {
		t.Fatal("Do() succeeded on persistent 429, want UpstreamHTTPError")
	}
	var upstreamErr *runtimeports.UpstreamHTTPError
	if !errors.As(err, &upstreamErr) || upstreamErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("err = %v, want UpstreamHTTPError 429", err)
	}
	if upstreamErr.RetryAfter != 7*time.Second {
		t.Fatalf("RetryAfter = %v, want 7s (preserved for classifiers)", upstreamErr.RetryAfter)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want single gateway send", attempts)
	}
}

func TestGatewayClientProbeStaysSingleShot(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := newGatewayTestClient(t, server).Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/models",
		},
		Body:        []byte(`{}`),
		MaxAttempts: 1,
		Timeout:     5 * time.Second,
	})
	if err == nil {
		t.Fatal("Do() succeeded on 503, want error")
	}
	var upstreamErr *runtimeports.UpstreamHTTPError
	if !errors.As(err, &upstreamErr) || upstreamErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("err = %v, want UpstreamHTTPError 503", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want single-shot 1 for probes", attempts)
	}
}

func TestGatewayClientCapsResponseBody(t *testing.T) {
	var capture gatewayCapture
	server := newGatewayCaptureServer(t, &capture, http.StatusOK, nil, strings.Repeat("x", 64))
	defer server.Close()

	client := &GatewayClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(server.URL), "/"),
		token:    "test-token",
		timeout:  runtimeports.GatewayDefaultTimeout,
		maxBytes: 16,
		outbound: safehttp.NewClient(safehttp.Config{
			Timeout:     runtimeports.GatewayDefaultTimeout,
			MaxTimeout:  runtimeports.GatewayDefaultTimeout,
			MaxAttempts: 1,
		}),
		target:     defaultGatewayOutboundTarget,
		resilience: NewGatewayResilience(DefaultGatewayResilienceConfig(), nil),
	}
	_, err := client.Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/chat/completions",
		},
		Body: []byte(`{}`),
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v, want body cap error", err)
	}
}

func TestGatewayLiveSmoke(t *testing.T) {
	baseURL := testenv.Get("CF_AI_GATEWAY_SMOKE_BASE_URL")
	token := testenv.Get("CF_AI_GATEWAY_SMOKE_TOKEN")
	if baseURL == "" || token == "" {
		t.Skipf("set CF_AI_GATEWAY_SMOKE_BASE_URL and CF_AI_GATEWAY_SMOKE_TOKEN to run the live gateway smoke test")
	}
	client, err := NewGatewayClient(GatewayClientConfig{BaseURL: baseURL, Token: token, Timeout: 60 * time.Second})
	if err != nil {
		t.Fatalf("NewGatewayClient() error = %v", err)
	}
	resp, err := client.Do(context.Background(), runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Provider: "openai",
			Path:     "v1/chat/completions",
		},
		TaskID:  "smoke",
		Feature: "smoke",
		Body:    []byte(`{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"ping"}],"max_tokens":8}`),
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK || len(resp.Body) == 0 {
		t.Fatalf("status = %d, body len = %d", resp.StatusCode, len(resp.Body))
	}
}
