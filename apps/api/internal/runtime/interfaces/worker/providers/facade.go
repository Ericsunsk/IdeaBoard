package providers

import (
	"strings"

	"ideaboard/api/internal/runtime/interfaces/worker/providers/domain"
	"ideaboard/api/internal/runtime/interfaces/worker/providers/infrastructure"
	"ideaboard/api/internal/workflow"
)

// ========================================================================
// Infrastructure Facade: AI Gateway Client
// ========================================================================

// GatewayClientConfig wires the sole AI egress via Cloudflare AI Gateway.
type GatewayClientConfig = infrastructure.GatewayClientConfig

// GatewayClient sends every AI request through Cloudflare AI Gateway.
type GatewayClient = infrastructure.GatewayClient

// NewGatewayClient validates the egress configuration fail-fast.
func NewGatewayClient(cfg GatewayClientConfig) (*GatewayClient, error) {
	return infrastructure.NewGatewayClient(cfg)
}

// ========================================================================
// Domain Facade: Cache Keys & Dialects
// ========================================================================

// DefaultTextCacheTTLSeconds delegates to domain.DefaultTextCacheTTLSeconds.
const DefaultTextCacheTTLSeconds = domain.DefaultTextCacheTTLSeconds

// BuildTextCacheKey delegates to domain.BuildTextCacheKey.
func BuildTextCacheKey(feature, model string, canonical []byte) (string, error) {
	return domain.BuildTextCacheKey(feature, model, canonical)
}

func gatewayProviderPath(provider string) string {
	return workflow.GatewayProviderPath(provider)
}

// ParseImageResponses extracts image payloads from any gateway response
// body using the domain extraction engine.
func ParseImageResponses(_ string, body []byte) ([]string, error) {
	return domain.ExtractImageResponses(body)
}

// ========================================================================
// Domain Facade: Gateway Translator Middleware
// ========================================================================

// GatewayTranslator provides request translation and response extraction.
type GatewayTranslator = domain.GatewayTranslator

// NewGatewayTranslator creates a new GatewayTranslator instance.
func NewGatewayTranslator() *GatewayTranslator {
	return domain.NewGatewayTranslator()
}

// ExtractGatewayImageResponses extracts image URLs or base64 payloads from
// any vendor response body.
func ExtractGatewayImageResponses(body []byte) ([]string, error) {
	return domain.ExtractImageResponses(body)
}

// BuildGatewayRequestBody constructs an adaptive payload for the target model.
func BuildGatewayRequestBody(
	c *Executor,
	providerReq providerRequest,
	provider, model string,
	operation workflow.RuntimeOperation,
	customParamsJSON string,
) ([]byte, error) {
	refs := append(append([]string{}, providerReq.ImageRefs...), providerReq.MaskRefs...)
	if strings.TrimSpace(providerReq.CanvasRef) != "" {
		refs = append(refs, providerReq.CanvasRef)
	}

	var resolvedPublicURLs []string
	for _, ref := range refs {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		pubURL, err := resolveProviderPublicImageURL(c, ref, "输入图像")
		if err != nil {
			return nil, err
		}
		resolvedPublicURLs = append(resolvedPublicURLs, pubURL)
	}

	return domain.BuildRequestBody(domain.RequestParams{
		Prompt:           providerReq.Prompt,
		ResolvedImages:   resolvedPublicURLs,
		Provider:         provider,
		Model:            model,
		Operation:        operation,
		CustomParamsJSON: customParamsJSON,
	})
}
