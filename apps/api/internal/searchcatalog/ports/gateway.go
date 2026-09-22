package ports

import (
	"context"
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
