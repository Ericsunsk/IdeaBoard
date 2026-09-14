package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate media derivative id: %w", err)
	}
	return "derivative_" + hex.EncodeToString(value[:]), nil
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
