package providerinfra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	runtimeports "ideaboard/api/internal/runtime/ports"

	"ideaboard/api/internal/platform/safehttp"
	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
)

const (
	// defaultGatewayMaxResponseBytes caps one gateway payload (image
	// responses can carry base64 bodies; callers needing more set Config).
	defaultGatewayMaxResponseBytes = 64 << 20
	// defaultGatewayOutboundTarget names gateway egress in outbound
	// logs/metrics.
	defaultGatewayOutboundTarget = "ai-gateway"
	// cloudflareGatewayHost is the sole AI egress host. Any other BaseURL
	// host is rejected fail-closed at startup; there is no custom-egress
	// path. Request-time policy must additionally authorize the host via
	// the outbound allowlist.
	cloudflareGatewayHost = "gateway.ai.cloudflare.com"
)

// isCloudflareGatewayHost reports whether host is the pinned sole egress
// (case-insensitive, trailing-dot tolerant). Subdomains and lookalikes do
// not match: only the exact host passes.
func isCloudflareGatewayHost(host string) bool {
	return strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(host), "."), cloudflareGatewayHost)
}

// isLoopbackHost reports whether host is a loopback name for test-only
// clients (httptest servers bind 127.0.0.1 by default).
func isLoopbackHost(host string) bool {
	switch strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), ".")) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// GatewayClientConfig wires the sole AI egress. The token is a Cloudflare
// gateway token (least privilege); provider keys stay in Cloudflare (BYOK).
// There is no custom-egress knob: every AI inference call goes through the
// Cloudflare AI Gateway host.
type GatewayClientConfig struct {
	// BaseURL is the gateway root, e.g.
	// https://gateway.ai.cloudflare.com/v1/{account}/{gateway}.
	BaseURL string
	Token   string
	// Timeout bounds one call when the request sets none.
	Timeout time.Duration
	// MaxResponseBytes caps one response body; zero keeps the default.
	MaxResponseBytes int64
	// Outbound overrides the standard outbound client. Gateway sends remain
	// single-shot with a timeout cap; tests inject isolated instances here.
	Outbound *safehttp.Client
	// Target names this egress in outbound logs/metrics; empty keeps the
	// gateway default ("ai-gateway", vision passes "vision-search").
	Target string
	// Metrics receives per-call RED observations by feature; nil disables
	// observation. The outbound client keeps its own target-dimensioned
	// counters regardless.
	Metrics *redmetrics.Set
	// Resilience carries the process-shared per-model guards (concurrency
	// cap + circuit breaker). Nil builds a fresh isolated guard set with
	// conservative defaults (unit paths); production wiring injects the
	// shared instance so windows span executions.
	Resilience *GatewayResilience
}

// GatewayClient sends every AI request through Cloudflare AI Gateway.
// Transport rides the standard outbound client with a timeout cap and one
// send per call; egress stays pinned to the configured
// gateway host via the safehttp SSRF allowlist underneath.
type GatewayClient struct {
	baseURL    string
	token      string
	timeout    time.Duration
	maxBytes   int64
	outbound   *safehttp.Client
	target     string
	metrics    *redmetrics.Set
	resilience *GatewayResilience
}

var _ runtimeports.GatewayClient = (*GatewayClient)(nil)

// NewGatewayClient validates the egress configuration fail-fast: a missing
// base URL or token, a non-http(s) URL, and any non-Cloudflare host are all
// startup errors, never silent fallbacks.
func NewGatewayClient(cfg GatewayClientConfig) (*GatewayClient, error) {
	return newGatewayClient(cfg, false)
}

// NewGatewayClientForTest builds a GatewayClient against loopback test
// servers (httptest), bypassing the Cloudflare host pin. Production must
// use NewGatewayClient. Only loopback hosts (localhost, 127.0.0.1, ::1)
// pass; any other non-CF host is still rejected fail-closed, so a test
// helper can never become a production custom-egress path.
func NewGatewayClientForTest(cfg GatewayClientConfig) (*GatewayClient, error) {
	return newGatewayClient(cfg, true)
}

func newGatewayClient(cfg GatewayClientConfig, allowLoopbackForTest bool) (*GatewayClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("gateway base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return nil, fmt.Errorf("gateway base URL must be an http(s) URL with a host")
	}
	if !isCloudflareGatewayHost(parsed.Hostname()) {
		if !(allowLoopbackForTest && isLoopbackHost(parsed.Hostname())) {
			return nil, fmt.Errorf("gateway base URL host %q is not the Cloudflare AI Gateway", parsed.Hostname())
		}
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("gateway token is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = runtimeports.GatewayDefaultTimeout
	}
	maxBytes := cfg.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = defaultGatewayMaxResponseBytes
	}
	outbound := cfg.Outbound
	if outbound == nil {
		outbound = safehttp.NewClient(safehttp.Config{
			Timeout:     timeout,
			MaxTimeout:  runtimeports.GatewayDefaultTimeout,
			MaxAttempts: 1,
		})
	}
	target := strings.TrimSpace(cfg.Target)
	if target == "" {
		target = defaultGatewayOutboundTarget
	}
	resilience := cfg.Resilience
	if resilience == nil {
		resilience = NewGatewayResilience(DefaultGatewayResilienceConfig(), cfg.Metrics)
	}
	return &GatewayClient{
		baseURL:    baseURL,
		token:      strings.TrimSpace(cfg.Token),
		timeout:    timeout,
		maxBytes:   maxBytes,
		outbound:   outbound,
		target:     target,
		metrics:    cfg.Metrics,
		resilience: resilience,
	}, nil
}

// Do sends one gateway request: headers encode auth, task metadata,
// payload-logging opt-out, cache bypass, idempotency key, and gateway retry override.
// Non-2xx responses surface as UpstreamHTTPError so existing failure
// classifiers keep working; transport failures stay wrapped for outcome
// analysis. Per-model guards reject fail-closed with a retryable 429 before
// any network I/O (never counted as an upstream outcome).
func (c *GatewayClient) Do(ctx context.Context, req runtimeports.GatewayRequest) (result runtimeports.GatewayResponse, resultErr error) {
	if c == nil {
		return runtimeports.GatewayResponse{}, fmt.Errorf("gateway client is not initialized")
	}
	if err := req.Validate(); err != nil {
		return runtimeports.GatewayResponse{}, err
	}
	httpReq, err := c.buildRequest(ctx, req)
	if err != nil {
		return runtimeports.GatewayResponse{}, err
	}
	finish, err := c.resilience.checkAndAcquire(req.Model)
	if err != nil {
		c.metrics.ObserveGatewayRequest(req.Feature, err)
		var reject *gatewayResilienceReject
		if errors.As(err, &reject) {
			return runtimeports.GatewayResponse{}, fmt.Errorf("gateway request rejected: %w",
				runtimeports.NewUpstreamHTTPError(http.StatusTooManyRequests, reject.Error(), 0))
		}
		return runtimeports.GatewayResponse{}, fmt.Errorf("gateway request rejected: %w", err)
	}
	started := time.Now()
	defer func() { finish(time.Since(started), resultErr != nil) }()
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = c.timeout
	}
	// Keep the application-to-gateway request single-shot. Cloudflare owns
	// provider retries; River owns whole-task retries (ADR-0010). Repeating a
	// POST after an ambiguous 503/transport failure could duplicate a billable
	// provider call. req.MaxAttempts only controls Cloudflare's internal tries.
	// The gateway URL is operator-controlled. Do not add its host here:
	// request-time policy must explicitly authorize every configured egress.
	resp, err := c.outbound.Do(ctx, httpReq, safehttp.RequestOptions{
		Timeout:     timeout,
		MaxAttempts: 1,
		AllowHosts:  safehttp.AllowHosts(ctx),
		Target:      c.target,
	})
	if err != nil {
		c.metrics.ObserveGatewayRequest(req.Feature, err)
		// Preserve HTTP status semantics from the outbound client so probe,
		// provider and worker failure classifiers keep working.
		var statusErr *safehttp.HTTPStatusError
		if errors.As(err, &statusErr) {
			return runtimeports.GatewayResponse{}, fmt.Errorf("gateway request failed: %w",
				runtimeports.NewUpstreamHTTPError(statusErr.StatusCode, statusErr.Body, statusErr.RetryAfter))
		}
		return runtimeports.GatewayResponse{}, fmt.Errorf("gateway transport failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := readCappedResponseBody(resp.Body, c.maxBytes)
	if err != nil {
		c.metrics.ObserveGatewayRequest(req.Feature, err)
		return runtimeports.GatewayResponse{}, err
	}
	gatewayResp := runtimeports.GatewayResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       body,
		Receipt:    parseGatewayReceipt(resp),
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		non2xx := fmt.Errorf("gateway request failed: %w",
			runtimeports.NewUpstreamHTTPErrorFromResponse(resp, runtimeports.TruncateErrorBody(string(body))))
		c.metrics.ObserveGatewayRequest(req.Feature, non2xx)
		return gatewayResp, non2xx
	}
	c.metrics.ObserveGatewayRequest(req.Feature, nil)
	return gatewayResp, nil
}

func (c *GatewayClient) buildRequest(ctx context.Context, req runtimeports.GatewayRequest) (*http.Request, error) {
	target := c.baseURL + req.Endpoint.RequestPath()
	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	var body io.Reader
	if req.HTTPMethod() != http.MethodGet {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.HTTPMethod(), target, body)
	if err != nil {
		return nil, fmt.Errorf("gateway request build failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set(runtimeports.GatewayHeaderAuthorization, "Bearer "+c.token)
	// Payloads never persist in gateway logs; metadata (counts, model,
	// status, cost, duration) still does.
	httpReq.Header.Set(runtimeports.GatewayHeaderCollectLogPayload, "false")
	if metadata, err := gatewayRequestMetadata(req); err != nil {
		return nil, err
	} else if metadata != "" {
		httpReq.Header.Set(runtimeports.GatewayHeaderMetadata, metadata)
	}
	// Cache bypass is a transport invariant, independent of request callers.
	httpReq.Header.Set(runtimeports.GatewayHeaderSkipCache, "true")
	if req.MaxAttempts > 0 {
		httpReq.Header.Set(runtimeports.GatewayHeaderMaxAttempts, strconv.Itoa(req.MaxAttempts))
	}
	// Best-effort upstream idempotency: the execution token lets upstreams
	// that understand it dedup retried sends; others ignore the header.
	// Empty keeps the previous behavior (no header). Local settlement never
	// depends on it.
	if token := strings.TrimSpace(req.ExecutionToken); token != "" {
		httpReq.Header.Set(runtimeports.GatewayHeaderIdempotencyKey, token)
	}
	return httpReq, nil
}

// gatewayRequestMetadata encodes the searchable log tags. At most two of
// the five gateway entries are used; empty identifiers are omitted.
func gatewayRequestMetadata(req runtimeports.GatewayRequest) (string, error) {
	metadata := make(map[string]string, 2)
	if id := strings.TrimSpace(req.TaskID); id != "" {
		metadata[runtimeports.GatewayMetadataTaskID] = id
	}
	if feature := strings.TrimSpace(req.Feature); feature != "" {
		metadata[runtimeports.GatewayMetadataFeature] = feature
	}
	if len(metadata) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("gateway metadata encode failed: %w", err)
	}
	return string(encoded), nil
}

func readCappedResponseBody(body io.Reader, maxBytes int64) ([]byte, error) {
	if body == nil {
		return nil, fmt.Errorf("gateway response has no body")
	}
	if maxBytes <= 0 {
		maxBytes = defaultGatewayMaxResponseBytes
	}
	payload, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("gateway response read failed: %w", err)
	}
	if int64(len(payload)) > maxBytes {
		return nil, fmt.Errorf("gateway response exceeds %d bytes", maxBytes)
	}
	return payload, nil
}

// parseGatewayReceipt reads the documented acknowledgement headers.
// CacheHit is true only on an explicit HIT status.
func parseGatewayReceipt(resp *http.Response) runtimeports.GatewayReceipt {
	receipt := runtimeports.GatewayReceipt{}
	if resp == nil {
		return receipt
	}
	header := resp.Header
	receipt.StatusCode = resp.StatusCode
	receipt.Model = header.Get(runtimeports.GatewayHeaderModel)
	receipt.Provider = header.Get(runtimeports.GatewayHeaderProvider)
	receipt.Step = header.Get(runtimeports.GatewayHeaderStep)
	receipt.CacheStatus = header.Get(runtimeports.GatewayHeaderCacheStatus)
	receipt.LogID = header.Get(runtimeports.GatewayHeaderLogID)
	receipt.EventID = header.Get(runtimeports.GatewayHeaderEventID)
	receipt.CacheHit = strings.EqualFold(strings.TrimSpace(receipt.CacheStatus), "HIT")
	return receipt
}
