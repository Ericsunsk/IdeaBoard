package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/workflow"
)

// WorkerPreview is the Admin dry-run diagnosis for one model identifier:
// pure local validation (snapshot-side model ref + custom params shape)
// plus, when Worker credentials are configured, a live dry-run against
// the Worker (no billing, no storage writes). Execution and preview share
// the model-ref validation (ParseExecutionModelRef): preview output and
// the executed target cannot diverge on malformed refs.
type WorkerPreview struct {
	Model      string `json:"model"`
	Provider   string `json:"provider"`
	Valid      bool   `json:"valid"`
	Reachable  bool   `json:"reachable"`
	StatusCode int    `json:"status_code"`
	LatencyMs  int64  `json:"latency_ms"`
	Message    string `json:"message"`
}

// ValidateWorkerModel validates one model identifier for Worker execution:
// structured provider/bare-model ref plus well-formed custom params JSON.
// It is the single ref-validation seam shared by preview and execution.
// The custom-params rule is canonical in workflow.ParseProviderOptions:
// malformed JSON fails closed, non-object JSON is ignored (never an error).
func ValidateWorkerModel(modelID, customParams string) (provider, model string, err error) {
	provider, model, err = workflow.ParseExecutionModelRef(modelID)
	if err != nil {
		return "", "", err
	}
	if _, err := workflow.ParseProviderOptions(customParams); err != nil {
		return "", "", err
	}
	return provider, model, nil
}

// PreviewWorkerRoute validates one model identifier and, when Worker
// credentials are present, dry-runs it against the Worker. A missing base
// URL or secret skips the live call (validation-only preview, reachable
// false): Admin still sees ref validity without network.
func PreviewWorkerRoute(ctx context.Context, baseURL, secret, modelID, customParams string) WorkerPreview {
	provider, model, err := ValidateWorkerModel(modelID, customParams)
	if err != nil {
		return WorkerPreview{Model: strings.TrimSpace(modelID), Valid: false, Message: err.Error()}
	}
	preview := WorkerPreview{
		Model:    workflow.FormatPoolModel(provider, model),
		Provider: provider,
		Valid:    true,
		Message:  "模型标识有效",
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(secret) == "" {
		preview.Message = "模型标识有效（未配置 Worker，未 dry-run）"
		return preview
	}
	// The preview dry-run carries the same body (incl. provider_options)
	// the probe below builds: one shared constructor, no hand-built maps.
	result := probeWorkerConnection(ctx, baseURL, secret, preview.Model, customParams)
	preview.Reachable = result.Reachable
	preview.StatusCode = result.StatusCode
	preview.LatencyMs = result.LatencyMs
	if result.Reachable {
		preview.Message = result.Message
	} else {
		preview.Message = "模型标识有效；" + result.Message
	}
	return preview
}

// workerDryRunRequest mirrors apps/ai-proxy/src/contracts.ts
// CanonicalImageRequest field-for-field (dry-run accepts the same shape;
// upload_url stays optional). Preview and Probe share newWorkerDryRunBody
// so the two call sites cannot hand-build divergent maps: if the TS
// contract gains a field, add it here (omitempty when TS marks it optional)
// and extend TestWorkerDryRunBodyMatchesContractsShape together.
type workerDryRunRequest struct {
	TaskID          string         `json:"task_id"`
	Provider        string         `json:"provider"`
	Model           string         `json:"model"`
	Prompt          string         `json:"prompt"`
	NegativePrompt  *string        `json:"negative_prompt,omitempty"`
	Width           *int           `json:"width,omitempty"`
	Height          *int           `json:"height,omitempty"`
	Steps           *int           `json:"steps,omitempty"`
	GuidanceScale   *float64       `json:"guidance_scale,omitempty"`
	Seed            *uint64        `json:"seed,omitempty"`
	Images          []string       `json:"images,omitempty"`
	ProviderOptions map[string]any `json:"provider_options,omitempty"`
	UploadURL       *string        `json:"upload_url,omitempty"`
	PublicURL       *string        `json:"public_url,omitempty"`
}

// newWorkerDryRunBody is the single dry-run request constructor shared by
// PreviewWorkerRoute and ProbeWorkerConnection. Custom params ride
// provider_options under the canonical workflow.ParseProviderOptions rule
// (non-object ignored); the probe diagnosis never sends images or URLs.
func newWorkerDryRunBody(provider, model, customParams string) ([]byte, error) {
	options, err := workflow.ParseProviderOptions(customParams)
	if err != nil {
		return nil, err
	}
	return json.Marshal(workerDryRunRequest{
		TaskID:          "probe",
		Provider:        provider,
		Model:           model,
		Prompt:          "probe",
		ProviderOptions: options,
	})
}

// ConnectionProbeResult is a stateless Worker dry-run diagnosis: reachable
// requires a dry-run 200 with a non-empty path_source (the Worker only
// echoes a source for a route it recognizes, so any value proves routing);
// any other status, an empty path_source, or a transport-level failure
// (DNS, refused, timeout) counts as unreachable. Go never enumerates
// sources: new providers are reachable without a Go change.
type ConnectionProbeResult struct {
	Reachable  bool   `json:"reachable"`
	StatusCode int    `json:"status_code"`
	LatencyMs  int64  `json:"latency_ms"`
	Message    string `json:"message"`
}

// workerProbeOutbound is the shared outbound client for Worker dry-run diagnosis.
var workerProbeOutbound = safehttp.NewClient(safehttp.Config{})

// workerProbeTarget names Worker dry-run diagnosis in outbound logs/metrics.
const workerProbeTarget = "edge-worker-probe"

// ProbeWorkerConnection dry-runs one model against the Worker with a bounded
// timeout. The provided secret is used for this probe only and never persisted.
// Purpose split (deliberate): this is transport diagnosis, not availability.
// Reachable requires a dry-run 200 with a non-empty path_source (any value
// proves the Worker recognized the route); other statuses, an empty
// path_source, and transport failures are unreachable. Custom params are unconfigured on this seam
// (TestConnection carries no custom_params field); Preview threads its own
// params through the shared internal probe below.
func ProbeWorkerConnection(ctx context.Context, baseURL, secret, modelID string) ConnectionProbeResult {
	return probeWorkerConnection(ctx, baseURL, secret, modelID, "")
}

// probeWorkerConnection is the shared dry-run probe: PreviewWorkerRoute
// passes its customParams so preview and probe send the identical body.
func probeWorkerConnection(ctx context.Context, baseURL, secret, modelID, customParams string) ConnectionProbeResult {
	const probeTimeout = 8 * time.Second

	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return ConnectionProbeResult{Message: errorrender.MsgProbeWorkerConnection}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ConnectionProbeResult{Message: errorrender.MsgProbeWorkerConnection2}
	}
	provider, model, err := ValidateWorkerModel(modelID, customParams)
	if err != nil {
		return ConnectionProbeResult{Message: errorrender.MsgPreviewWorkerRoute}
	}
	body, err := newWorkerDryRunBody(provider, model, customParams)
	if err != nil {
		return ConnectionProbeResult{Message: errorrender.MsgProbeWorkerConnection4 + err.Error()}
	}
	probeURL := trimmed + "/v1/dry-run"

	start := time.Now()

	doRequest := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, probeURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(secret) != "" {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(secret))
		}
		resp, err := workerProbeOutbound.Do(ctx, req, safehttp.RequestOptions{
			Timeout:     probeTimeout,
			MaxAttempts: 1,
			AllowHosts:  safehttp.AllowHosts(ctx),
			Target:      workerProbeTarget,
		})
		if err != nil {
			var statusErr *safehttp.HTTPStatusError
			if errors.As(err, &statusErr) {
				return workerProbeStatusResponse(req, statusErr), nil
			}
			return nil, err
		}
		return resp, nil
	}

	resp, err := doRequest()
	if err != nil {
		return ConnectionProbeResult{Message: errorrender.MsgProbeWorkerConnection5 + err.Error()}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	latency := time.Since(start).Milliseconds()
	detail := probeDryRunDetail(resp.StatusCode, respBody)
	if resp.StatusCode != http.StatusOK {
		return ConnectionProbeResult{
			Reachable:  false,
			StatusCode: resp.StatusCode,
			LatencyMs:  latency,
			Message:    errorrender.MsgProbeWorkerConnection5 + fmt.Sprintf("HTTP %d", resp.StatusCode) + detail,
		}
	}
	if source := dryRunPathSource(respBody); strings.TrimSpace(source) == "" {
		return ConnectionProbeResult{
			Reachable:  false,
			StatusCode: resp.StatusCode,
			LatencyMs:  latency,
			Message:    errorrender.MsgProbeWorkerConnection5 + "HTTP 200 path_source 缺失" + detail,
		}
	}
	return ConnectionProbeResult{
		Reachable:  true,
		StatusCode: resp.StatusCode,
		LatencyMs:  latency,
		Message:    fmt.Sprintf(errorrender.MsgProbeWorkerConnection6, latency, resp.StatusCode) + detail,
	}
}

// dryRunPathSource extracts the Worker dry-run path_source (target-scoped
// first, top-level fallback) for the reachability check: any non-empty
// value proves the Worker recognized the route.
func dryRunPathSource(body []byte) string {
	if len(bytes.TrimSpace(body)) == 0 {
		return ""
	}
	var dryRun struct {
		Target *struct {
			PathSource string `json:"path_source"`
		} `json:"target"`
		PathSource string `json:"path_source"`
	}
	if err := json.Unmarshal(body, &dryRun); err != nil {
		return ""
	}
	if dryRun.Target != nil {
		if source := strings.TrimSpace(dryRun.Target.PathSource); source != "" {
			return source
		}
	}
	return strings.TrimSpace(dryRun.PathSource)
}

// probeDryRunDetail extracts the Worker dry-run path_source (or the edge
// failure code) for the Admin message. Bodies stay truncated; secrets never
// appear (the Worker masks them server-side).
func probeDryRunDetail(status int, body []byte) string {
	if len(bytes.TrimSpace(body)) == 0 {
		return ""
	}
	var dryRun struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &dryRun); err != nil {
		return ""
	}
	if dryRun.Error != nil && strings.TrimSpace(dryRun.Error.Code) != "" {
		return " (" + strings.TrimSpace(dryRun.Error.Code) + ")"
	}
	if source := dryRunPathSource(body); source != "" {
		return " (" + source + ")"
	}
	return ""
}

// workerProbeStatusResponse converts an exhausted retryable probe status
// back to a response shape: diagnosis treats any HTTP status as reachable,
// so a single-shot 429/5xx still proves routing, DNS and TLS.
func workerProbeStatusResponse(req *http.Request, statusErr *safehttp.HTTPStatusError) *http.Response {
	preview := ""
	status := 0
	if statusErr != nil {
		preview = statusErr.Body
		status = statusErr.StatusCode
	}
	resp := &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(preview)),
		ContentLength: int64(len(preview)),
		Request:       req,
	}
	return resp
}
