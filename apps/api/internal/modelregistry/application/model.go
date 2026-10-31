package application

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	modelregistryports "ideaboard/api/internal/modelregistry/ports"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/workflow"
)

// RoutePreview is a read-only diagnosis of where a model identifier would
// route: provider namespace plus the default image-pipeline path. Pipeline
// operations with dedicated routes (cutout/upscale) resolve their own paths
// at execution time.
type RoutePreview struct {
	ProviderSlug    string `json:"provider_slug"`
	IsFirstParty    bool   `json:"is_first_party"`
	TargetPath      string `json:"target_path"`
	PathSource      string `json:"path_source"`
	PathSourceLabel string `json:"path_source_label"`
}

// PreviewGatewayRoute resolves routing facts without touching configuration.
// Preview targets the generic image pipeline, whose path is produced entirely
// by workflow.ResolveRoute (dialects.Resolve delegates to it for Generation
// and preview only reads namespace/path/source): preview output and the
// executed route cannot diverge, and this business package stays free of
// runtime imports per the architecture gate.
func PreviewGatewayRoute(modelID, customPath, customParams string) RoutePreview {
	provider, model := workflow.SplitModelIdentifier(modelID, "")
	// Preview targets the generic image pipeline (ModelImage adapter); pipeline
	// operations with dedicated routes resolve their own paths at execution time.
	resolved := workflow.ResolveRoute(provider, model, customPath, customParams)
	return RoutePreview{
		ProviderSlug:    resolved.Namespace,
		IsFirstParty:    workflow.IsFirstPartyGatewayProvider(provider),
		TargetPath:      resolved.Path,
		PathSource:      string(resolved.Source),
		PathSourceLabel: resolved.Source.Label(),
	}
}

// ConnectionProbeResult is a stateless reachability diagnosis: any HTTP
// status received from the target counts as reachable (even 4xx/5xx, which
// still prove routing, DNS, and TLS work); only transport-level failures
// (DNS, refused, timeout) count as unreachable.
type ConnectionProbeResult struct {
	Reachable  bool   `json:"reachable"`
	StatusCode int    `json:"status_code"`
	LatencyMs  int64  `json:"latency_ms"`
	Message    string `json:"message"`
}

// gatewayProbeOutbound is the shared outbound client for connection diagnosis.
var gatewayProbeOutbound = safehttp.NewClient(safehttp.Config{})

// gatewayProbeTarget names connection diagnosis in outbound logs/metrics.
const gatewayProbeTarget = "ai-gateway-probe"

// ProbeGatewayConnection probes gateway reachability with a bounded
// timeout. The provided token is used only for this probe and never persisted.
// Purpose split (deliberate): this is transport diagnosis, not availability.
// Any HTTP status counts as reachable (even 401/403 — they still prove
// routing, DNS and TLS); only transport failures are unreachable.
//
// Target selection: provider+path both empty probes the gateway root URL
// directly (any HTTP status still proves DNS/TLS/routing; CF exposes no
// unified health endpoint, so the root URL is the gateway-liveness probe).
// Exactly one of provider/path empty is rejected as ambiguous (no guessing).
func ProbeGatewayConnection(ctx context.Context, baseURL, token, provider, model, path string) ConnectionProbeResult {
	const probeTimeout = 8 * time.Second

	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return ConnectionProbeResult{Message: errorrender.MsgProbeGatewayConnection}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ConnectionProbeResult{Message: errorrender.MsgProbeGatewayConnection2}
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	path = strings.Trim(strings.TrimSpace(path), "/")
	onlyOneRoutePart := (provider == "") != (path == "")
	if onlyOneRoutePart {
		return ConnectionProbeResult{Message: errorrender.MsgProbeGatewayConnection3}
	}
	if strings.ContainsAny(provider+path, "?&#\r\n") {
		return ConnectionProbeResult{Message: errorrender.MsgProbeGatewayConnection4}
	}
	probeURL := trimmed
	if provider != "" {
		probeURL = trimmed + "/" + workflow.GatewayProviderPath(provider) + "/" + path
	}

	start := time.Now()

	doRequest := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(token) != "" {
			req.Header.Set(modelregistryports.GatewayHeaderAuthorization, "Bearer "+strings.TrimSpace(token))
		}
		req.Header.Set(modelregistryports.GatewayHeaderMaxAttempts, "1")
		req.Header.Set(modelregistryports.GatewayHeaderSkipCache, "true")
		if model != "" {
			req.Header.Set(modelregistryports.GatewayHeaderModel, model)
		}
		resp, err := gatewayProbeOutbound.Do(ctx, req, safehttp.RequestOptions{
			Timeout:     probeTimeout,
			MaxAttempts: 1,
			AllowHosts:  safehttp.AllowHosts(ctx),
			Target:      gatewayProbeTarget,
		})
		if err != nil {
			var statusErr *safehttp.HTTPStatusError
			if errors.As(err, &statusErr) {
				return gatewayProbeStatusResponse(req, statusErr), nil
			}
			return nil, err
		}
		return resp, nil
	}

	resp, err := doRequest()
	if err != nil {
		return ConnectionProbeResult{Message: errorrender.MsgProbeGatewayConnection5 + err.Error()}
	}
	defer resp.Body.Close()
	latency := time.Since(start).Milliseconds()
	return ConnectionProbeResult{
		Reachable:  true,
		StatusCode: resp.StatusCode,
		LatencyMs:  latency,
		Message:    fmt.Sprintf(errorrender.MsgProbeGatewayConnection6, latency, resp.StatusCode),
	}
}

// gatewayProbeStatusResponse converts an exhausted retryable probe status
// back to a response shape: diagnosis treats any HTTP status as reachable,
// so a single-shot 429/5xx still proves routing, DNS and TLS.
func gatewayProbeStatusResponse(req *http.Request, statusErr *safehttp.HTTPStatusError) *http.Response {
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
