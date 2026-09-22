package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	runtimeports "ideaboard/api/internal/runtime/ports"

	"ideaboard/api/internal/platform/safehttp"
)

const (
	// defaultGatewayTimeout matches the frozen gateway total budget.
	defaultGatewayTimeout = 300 * time.Second
	// defaultGatewayMaxResponseBytes caps one gateway payload (image
	// responses can carry base64 bodies; callers needing more set Config).
	defaultGatewayMaxResponseBytes = 64 << 20
)

// GatewayClientConfig wires the sole AI egress. The token is a Cloudflare
// gateway token (least privilege); provider keys stay in Cloudflare (BYOK).
type GatewayClientConfig struct {
	// BaseURL is the gateway root, e.g.
	// https://gateway.ai.cloudflare.com/v1/{account}/{gateway}.
	BaseURL string
	Token   string
	// Timeout bounds one call when the request sets none.
	Timeout time.Duration
	// MaxResponseBytes caps one response body; zero keeps the default.
	MaxResponseBytes int64
}

// GatewayClient sends every AI request through Cloudflare AI Gateway.
// Egress stays pinned to the configured gateway host via safehttp.
type GatewayClient struct {
	baseURL  string
	token    string
	host     string
	timeout  time.Duration
	maxBytes int64
}

var _ runtimeports.GatewayClient = (*GatewayClient)(nil)

// NewGatewayClient validates the egress configuration fail-fast.
func NewGatewayClient(cfg GatewayClientConfig) (*GatewayClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("gateway base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return nil, fmt.Errorf("gateway base URL must be an http(s) URL with a host")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("gateway token is required")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultGatewayTimeout
	}
	maxBytes := cfg.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = defaultGatewayMaxResponseBytes
	}
	return &GatewayClient{
		baseURL:  baseURL,
		token:    strings.TrimSpace(cfg.Token),
		host:     parsed.Hostname(),
		timeout:  timeout,
		maxBytes: maxBytes,
	}, nil
}

// Do sends one gateway request: headers encode auth, task metadata,
// payload-logging opt-out, cache policy, and retry override. Non-2xx
// responses surface as UpstreamHTTPError so existing failure classifiers
// keep working; transport failures stay wrapped for outcome analysis.
func (c *GatewayClient) Do(ctx context.Context, req runtimeports.GatewayRequest) (runtimeports.GatewayResponse, error) {
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
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = c.timeout
	}
	resp, err := safehttp.DoRequestWithAllowHosts(ctx, httpReq, timeout, c.host)
	if err != nil {
		return runtimeports.GatewayResponse{}, fmt.Errorf("gateway transport failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := readCappedResponseBody(resp.Body, c.maxBytes)
	if err != nil {
		return runtimeports.GatewayResponse{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return runtimeports.GatewayResponse{}, fmt.Errorf("gateway request failed: %w",
			runtimeports.NewUpstreamHTTPErrorFromResponse(resp, runtimeports.TruncateErrorBody(string(body))))
	}
	return runtimeports.GatewayResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       body,
		Receipt:    parseGatewayReceipt(resp),
	}, nil
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
	applyGatewayCacheHeaders(httpReq, req.Cache)
	if req.MaxAttempts > 0 {
		httpReq.Header.Set(runtimeports.GatewayHeaderMaxAttempts, strconv.Itoa(req.MaxAttempts))
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

func applyGatewayCacheHeaders(httpReq *http.Request, policy runtimeports.GatewayCachePolicy) {
	switch policy.Mode {
	case runtimeports.GatewayCacheSkip:
		httpReq.Header.Set(runtimeports.GatewayHeaderSkipCache, "true")
	case runtimeports.GatewayCacheTTL:
		if key := strings.TrimSpace(policy.Key); key != "" {
			httpReq.Header.Set(runtimeports.GatewayHeaderCacheKey, key)
		}
		if policy.TTLSeconds > 0 {
			httpReq.Header.Set(runtimeports.GatewayHeaderCacheTTL, strconv.Itoa(policy.TTLSeconds))
		}
	default:
	}
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
