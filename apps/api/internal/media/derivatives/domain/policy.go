package domain

import (
	"ideaboard/api/internal/platform/contract"
)

type TierPolicy struct {
	Tier   string
	Format string
}

var DefaultTierPolicies = []TierPolicy{
	{Tier: TierThumb, Format: FormatWebP},
	{Tier: TierPreview, Format: FormatWebP},
	{Tier: TierProcessingInput, Format: FormatWebP},
}

var OnDemandTierPolicies = map[string]TierPolicy{
	TierLargePreview:    {Tier: TierLargePreview, Format: FormatWebP},
	TierProcessingInput: {Tier: TierProcessingInput, Format: FormatWebP},
}

func NewDerivativeID() (string, error) {
	return contract.MintPublicID("derivative_", 12)
}

func DerivativeMimeType(format string) string {
	switch format {
	case FormatPNG:
		return "image/png"
	case FormatWebP:
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
