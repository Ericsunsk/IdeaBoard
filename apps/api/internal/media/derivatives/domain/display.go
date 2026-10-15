package domain

// Display describes same-origin image renditions, independently of asset identity.
type Display struct {
	Variants  []DisplayVariant `json:"variants"`
	ExpiresAt string           `json:"expires_at,omitempty"`
}

type DisplayVariant struct {
	Tier    string `json:"tier"`
	MaxSide int    `json:"max_side"`
	URL     string `json:"url"`
}

func DisplayVariants(urlForTier func(string) string) *Display {
	variants := make([]DisplayVariant, 0, 3)
	for _, tier := range []string{TierThumb, TierPreview, TierLargePreview} {
		variants = append(variants, DisplayVariant{Tier: tier, MaxSide: TierSpecs[tier].MaxSide, URL: urlForTier(tier)})
	}
	return &Display{Variants: variants}
}
