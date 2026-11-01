package providerinfra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	runtimeports "ideaboard/api/internal/runtime/ports"

	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/platform/workerwire"
	redmetrics "ideaboard/api/internal/runtime/infrastructure/metrics"
)

const (
	// defaultWorkerMaxResponseBytes caps one Worker payload (success and
	// failure envelopes are small JSON; callers needing more set Config).
	defaultWorkerMaxResponseBytes = 1 << 20
	// defaultWorkerOutboundTarget names Worker egress in outbound logs/metrics.
	defaultWorkerOutboundTarget = "edge-worker"
)

// WorkerClientConfig wires the ai-proxy Worker egress. Both fields are
// required: a missing base URL or secret fails construction closed (and
// wiring fails tasks deterministically before any send).
type WorkerClientConfig struct {
	// BaseURL is the Worker service root, e.g. https://worker.internal.
	BaseURL string
	// Secret is the WORKER_INTERNAL_SECRET bearer credential. Empty fails closed.
	Secret string
	// Timeout bounds one call when the request sets none.
	Timeout time.Duration
	// MaxResponseBytes caps one response body; zero keeps the default.
	MaxResponseBytes int64
	// Outbound overrides the standard outbound client. Worker sends remain
	// single-shot with a timeout cap; tests inject isolated instances here.
	Outbound *safehttp.Client
	// Target names this egress in outbound logs/metrics; empty keeps the
	// Worker default.
	Target string
	// Metrics receives per-call RED observations by feature; nil disables
	// observation.
	Metrics *redmetrics.Set
	// Resilience carries the process-shared per-model guards. Nil builds a
	// fresh isolated guard set (unit paths); production wiring injects the
	// shared instance so windows span executions.
	Resilience WorkerFuse
}

// WorkerClient sends image generation requests to the ai-proxy Worker
// as CanonicalImageRequest. The Worker owns shaping and provider routing.
type WorkerClient struct {
	endpoint   safehttp.ConfiguredEndpoint
	secret     string
	timeout    time.Duration
	maxBytes   int64
	outbound   *safehttp.Client
	target     string
	metrics    *redmetrics.Set
	resilience WorkerFuse
}

var _ runtimeports.WorkerClient = (*WorkerClient)(nil)

// NewWorkerClient validates the Worker egress configuration fail-fast: a
// missing base URL or secret and a non-http(s) URL are startup errors.
func NewWorkerClient(cfg WorkerClientConfig) (*WorkerClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("worker base URL is required (WORKER_BASE_URL)")
	}
	// The Worker base URL is operator configuration (WORKER_BASE_URL), not
	// user input: validate once here and pin every send to this endpoint
	// through the configured-egress path (loopback included, no allowlist).
	// The endpoint keeps the normalized base; no separate copy is stored.
	endpoint, err := safehttp.ParseConfiguredEndpoint(baseURL)
	if err != nil {
		return nil, fmt.Errorf("worker base URL must be an http(s) URL with a host")
	}
	if strings.TrimSpace(cfg.Secret) == "" {
		return nil, fmt.Errorf("worker secret is required (WORKER_INTERNAL_SECRET)")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = runtimeports.WorkerDefaultTimeout
	}
	maxBytes := cfg.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = defaultWorkerMaxResponseBytes
	}
	outbound := cfg.Outbound
	if outbound == nil {
		outbound = safehttp.NewClient(safehttp.Config{
			Timeout:     timeout,
			MaxTimeout:  runtimeports.WorkerDefaultTimeout,
			MaxAttempts: 1,
		})
	}
	target := strings.TrimSpace(cfg.Target)
	if target == "" {
		target = defaultWorkerOutboundTarget
	}
	resilience := cfg.Resilience
	if resilience == nil {
		resilience = NewWorkerResilience(DefaultWorkerResilienceConfig(), cfg.Metrics)
	}
	return &WorkerClient{
		endpoint:   endpoint,
		secret:     strings.TrimSpace(cfg.Secret),
		timeout:    timeout,
		maxBytes:   maxBytes,
		outbound:   outbound,
		target:     target,
		metrics:    cfg.Metrics,
		resilience: resilience,
	}, nil
}

// Do sends one generation request: single-shot POST with the Worker secret
// and the CanonicalImageRequest body. Non-2xx responses surface as
// WorkerError (edge code + upstream status) so the executor maps failure
// categories without string matching; transport failures stay wrapped.
func (c *WorkerClient) Do(ctx context.Context, req runtimeports.WorkerRequest) (result runtimeports.WorkerResponse, resultErr error) {
	if c == nil {
		return runtimeports.WorkerResponse{}, fmt.Errorf("worker client is not initialized")
	}
	if err := req.Validate(); err != nil {
		return runtimeports.WorkerResponse{}, err
	}
	modelRef := strings.TrimSpace(req.Provider) + "/" + strings.TrimSpace(req.Model)
	finish, err := c.resilience.checkAndAcquire(modelRef)
	if err != nil {
		c.metrics.ObserveWorkerRequest(req.Feature, err)
		var reject *workerResilienceReject
		if errors.As(err, &reject) {
			return runtimeports.WorkerResponse{}, fmt.Errorf("worker request rejected: %w",
				runtimeports.NewUpstreamHTTPError(http.StatusTooManyRequests, reject.Error(), 0))
		}
		return runtimeports.WorkerResponse{}, fmt.Errorf("worker request rejected: %w", err)
	}
	started := time.Now()
	defer func() { finish(time.Since(started), resultErr != nil) }()
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = c.timeout
	}
	wire := workerRequestToWire(req)
	body, err := json.Marshal(wire)
	if err != nil {
		return runtimeports.WorkerResponse{}, fmt.Errorf("worker request encode failed: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint.BaseURL()+workerwire.GeneratePath, bytes.NewReader(body))
	if err != nil {
		return runtimeports.WorkerResponse{}, fmt.Errorf("worker request build failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.secret)
	resp, err := c.outbound.Do(ctx, httpReq, safehttp.RequestOptions{
		Timeout:     timeout,
		MaxAttempts: 1,
		Configured:  c.endpoint,
		Target:      c.target,
	})
	if err != nil {
		c.metrics.ObserveWorkerRequest(req.Feature, err)
		var statusErr *safehttp.HTTPStatusError
		if errors.As(err, &statusErr) {
			// Transport-level status errors usually still carry the Worker
			// envelope: parse it first so the edge code survives, keeping
			// the Retry-After header when the envelope has none.
			workerErr := workerErrorFromBody(strings.TrimSpace(req.TaskID), statusErr.StatusCode, []byte(statusErr.Body))
			var parsed *runtimeports.WorkerError
			if errors.As(workerErr, &parsed) && statusErr.RetryAfter > 0 && parsed.RetryAfter == 0 {
				parsed.RetryAfter = statusErr.RetryAfter
			}
			return runtimeports.WorkerResponse{StatusCode: statusErr.StatusCode}, workerErr
		}
		return runtimeports.WorkerResponse{}, fmt.Errorf("worker transport failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := readWorkerResponseBody(resp.Body, c.maxBytes)
	if err != nil {
		c.metrics.ObserveWorkerRequest(req.Feature, err)
		return runtimeports.WorkerResponse{StatusCode: resp.StatusCode}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		workerErr := workerErrorFromBody(req.TaskID, resp.StatusCode, respBody)
		c.metrics.ObserveWorkerRequest(req.Feature, workerErr)
		return runtimeports.WorkerResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone()}, workerErr
	}
	success, err := decodeWorkerSuccess(respBody, strings.TrimSpace(req.TaskID))
	if err != nil {
		parsed := runtimeports.NewWorkerError(
			runtimeports.WorkerErrorAdapterInternal, fmt.Sprintf("worker response parse failed: %s", err),
			strings.TrimSpace(req.TaskID), resp.StatusCode, 0, 0)
		c.metrics.ObserveWorkerRequest(req.Feature, parsed)
		return runtimeports.WorkerResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone()}, parsed
	}
	c.metrics.ObserveWorkerRequest(req.Feature, nil)
	return workerSuccessToResponse(resp, success), nil
}

func workerRequestToWire(req runtimeports.WorkerRequest) workerwire.GenerateRequest {
	wire := workerwire.GenerateRequest{
		ImageRequest: workerwire.ImageRequest{
			TaskID:    strings.TrimSpace(req.TaskID),
			Provider:  strings.ToLower(strings.TrimSpace(req.Provider)),
			Model:     strings.TrimSpace(req.Model),
			Prompt:    req.Prompt,
			MaskIndex: req.MaskIndex,
		},
		UploadURL: strings.TrimSpace(req.UploadURL),
	}
	if req.Width > 0 {
		wire.Width = &req.Width
	}
	if req.Height > 0 {
		wire.Height = &req.Height
	}
	seed := req.Seed
	wire.Seed = &seed
	if len(req.Images) > 0 {
		wire.Images = append([]string(nil), req.Images...)
	}
	if len(req.ProviderOptions) > 0 {
		wire.ProviderOptions = req.ProviderOptions
	}
	return wire
}

// workerErrorFromStatus maps a transport-level HTTP status (no parseable
// Worker envelope) to a WorkerError with the closest edge code.
func workerErrorFromStatus(taskID string, status int, body string, retryAfter time.Duration) error {
	code := runtimeports.WorkerErrorUpstream
	switch status {
	case http.StatusBadRequest:
		code = runtimeports.WorkerErrorBadRequest
	case http.StatusUnauthorized, http.StatusForbidden:
		code = runtimeports.WorkerErrorUnauthorized
	case http.StatusTooManyRequests:
		code = runtimeports.WorkerErrorUpstream
	}
	return runtimeports.NewWorkerError(code, runtimeports.TruncateErrorBody(body), taskID, status, 0, retryAfter)
}

// workerErrorFromBody parses a non-2xx Worker envelope into a WorkerError.
// Unparseable bodies fall back to status-derived codes.
func workerErrorFromBody(taskID string, status int, body []byte) error {
	var failure workerwire.Failure
	if err := json.Unmarshal(body, &failure); err == nil && strings.TrimSpace(failure.Error.Code) != "" {
		upstream := 0
		if failure.Error.UpstreamStatus != nil {
			upstream = *failure.Error.UpstreamStatus
		}
		return runtimeports.NewWorkerError(
			strings.TrimSpace(failure.Error.Code),
			strings.TrimSpace(failure.Error.Message),
			taskID, status, upstream, 0,
		)
	}
	return workerErrorFromStatus(taskID, status, string(body), 0)
}

func decodeWorkerSuccess(body []byte, taskID string) (workerwire.Success, error) {
	var success workerwire.Success
	if err := json.Unmarshal(body, &success); err != nil {
		return workerwire.Success{}, err
	}
	if strings.TrimSpace(success.Status) != "completed" {
		return workerwire.Success{}, fmt.Errorf("worker status = %q, want completed", success.Status)
	}
	if strings.TrimSpace(success.TaskID) != "" && strings.TrimSpace(success.TaskID) != taskID {
		return workerwire.Success{}, fmt.Errorf("worker task_id mismatch")
	}
	if strings.TrimSpace(success.ImageURL) == "" {
		return workerwire.Success{}, fmt.Errorf("worker returned no image URL")
	}
	return success, nil
}

func workerSuccessToResponse(resp *http.Response, success workerwire.Success) runtimeports.WorkerResponse {
	receipt := runtimeports.WorkerReceipt{
		Model:          strings.TrimSpace(success.Receipt.Model),
		Provider:       strings.TrimSpace(success.Receipt.Provider),
		StatusCode:     resp.StatusCode,
		UpstreamStatus: success.Receipt.UpstreamStatus,
	}
	if receipt.Model == "" {
		receipt.Model = strings.TrimSpace(success.Model)
	}
	if receipt.Provider == "" {
		receipt.Provider = strings.TrimSpace(success.Provider)
	}
	if success.Receipt.LogID != nil {
		receipt.LogID = strings.TrimSpace(*success.Receipt.LogID)
	}
	if success.Receipt.CacheStatus != nil {
		receipt.CacheHit = strings.EqualFold(strings.TrimSpace(*success.Receipt.CacheStatus), "HIT")
	}
	out := runtimeports.WorkerResponse{
		StatusCode:  resp.StatusCode,
		Header:      resp.Header.Clone(),
		ImageURL:    strings.TrimSpace(success.ImageURL),
		ContentType: strings.TrimSpace(success.ContentType),
		Receipt:     receipt,
		RawPreview:  fmt.Sprintf("worker image model=%s", strings.TrimSpace(success.Model)),
	}
	if success.Seed != nil {
		out.Seed, out.HasSeed = *success.Seed, true
	}
	return out
}

func readWorkerResponseBody(body io.Reader, maxBytes int64) ([]byte, error) {
	if body == nil {
		return nil, fmt.Errorf("worker response has no body")
	}
	if maxBytes <= 0 {
		maxBytes = defaultWorkerMaxResponseBytes
	}
	payload, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("worker response read failed: %w", err)
	}
	if int64(len(payload)) > maxBytes {
		return nil, fmt.Errorf("worker response exceeds %d bytes", maxBytes)
	}
	return payload, nil
}
