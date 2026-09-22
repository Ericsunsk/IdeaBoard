package ports

import (
	"context"

	"ideaboard/api/internal/searchcatalog/domain"
)

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
