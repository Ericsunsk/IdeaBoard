package ports

import (
	"context"
	"ideaboard/api/internal/searchcatalog/domain"
	"net/http"
	"time"
)

// VisionGatewayClient is the single-AI-egress seam for vision search.
// Implementations send one OpenAI-compatible chat request through the
// central Cloudflare AI Gateway (openrouter route) and return the raw
// outcome; request shaping and response parsing stay in vision_provider.go.
// The interface lives here so this business context never imports the
// runtime gateway transport; composition injects the adapter.
type VisionGatewayClient interface {
	Do(ctx context.Context, req VisionGatewayRequest) (VisionGatewayResponse, error)
}

// VisionGatewayRequest carries one vision chat call. Model is the
// OpenRouter model ID from vision settings (provider/model), never
// defaulted in code; Body is the marshalled chat payload.
type VisionGatewayRequest struct {
	Model   string
	Body    []byte
	Timeout time.Duration
}

// VisionGatewayResponse is the raw gateway outcome for one vision call.
type VisionGatewayResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// merged from internal/searchcatalog/ports/vision.go (structural consolidation; symbols unchanged).
// VisionProviderConfig is the runtime configuration needed by the Vision use
// case. Vision rides the central AI gateway connection: only the model comes
// from vision settings (admin-configured, never defaulted in code). There is
// no endpoint or key here by design — those live in the ai-gateway settings.
type VisionProviderConfig struct {
	Model string
}

// VisionConfigReader resolves the vision model for one call.
type VisionConfigReader interface {
	VisionConfig(context.Context) (VisionProviderConfig, error)
}

// VisionProviderClient extracts understanding from images plus text.
type VisionProviderClient interface {
	Extract(context.Context, VisionProviderConfig, domain.VisionInput) (domain.VisionExtraction, error)
}
