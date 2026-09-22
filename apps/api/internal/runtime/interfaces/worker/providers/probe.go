package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runtimeports "ideaboard/api/internal/runtime/ports"
)

// ProbeVerdict is the observable outcome of one model probe. Probes run
// straight at the provider through the primary gateway (never through a
// dynamic route), single-shot, cache skipped: whatever responds IS the
// probed target, so fallback can never fake a healthy verdict.
type ProbeVerdict struct {
	// Reachable is true when any HTTP response arrived, regardless of
	// status. Transport failures are unreachable.
	Reachable  bool
	StatusCode int
	// BodyPreview carries truncated raw output for capability判定 upstream.
	BodyPreview string
	Receipt     runtimeports.GatewayReceipt
}

// ProbeModel checks one provider model path (e.g. "v1/models") with a GET
// request. Non-2xx responses are reachable-but-failing and surface as
// UpstreamHTTPError for the caller to classify; only transport failures
// report unreachable.
// ProbeModel checks one gateway route (e.g. "openrouter/api/v1/models")
// with a GET request. The provider slug is the opaque routing token from
// the model mapping; probes never parse response shapes.
func ProbeModel(
	ctx context.Context,
	client runtimeports.GatewayClient,
	provider, model, path string,
	timeout time.Duration,
) (ProbeVerdict, error) {
	if client == nil {
		return ProbeVerdict{}, fmt.Errorf("gateway client is not initialized")
	}
	if strings.TrimSpace(provider) == "" {
		return ProbeVerdict{}, fmt.Errorf("probe provider is required")
	}
	if strings.TrimSpace(path) == "" {
		return ProbeVerdict{}, fmt.Errorf("probe path is required")
	}
	resp, err := client.Do(ctx, runtimeports.GatewayRequest{
		Endpoint: runtimeports.GatewayEndpoint{
			Kind:     runtimeports.GatewayEndpointProviderNative,
			Provider: gatewayProviderPath(strings.TrimSpace(provider)),
			Path:     path,
		},
		Model:       strings.TrimSpace(model),
		Feature:     "probe",
		Method:      "GET",
		Cache:       runtimeports.GatewayCachePolicy{Mode: runtimeports.GatewayCacheSkip},
		MaxAttempts: 1,
		Timeout:     timeout,
	})
	if err != nil {
		var upstreamErr *runtimeports.UpstreamHTTPError
		if errors.As(err, &upstreamErr) {
			return ProbeVerdict{
				Reachable:   true,
				StatusCode:  upstreamErr.StatusCode,
				BodyPreview: upstreamErr.Body,
			}, err
		}
		return ProbeVerdict{}, fmt.Errorf("probe transport failed: %w", err)
	}
	return ProbeVerdict{
		Reachable:   true,
		StatusCode:  resp.StatusCode,
		BodyPreview: runtimeports.TruncateErrorBody(string(resp.Body)),
		Receipt:     resp.Receipt,
	}, nil
}
