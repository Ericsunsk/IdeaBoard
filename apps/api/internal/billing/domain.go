package billing

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

const (
	MaxSKULength    = 128
	MaxQuoteCredits = int64(9_000_000_000_000_000)
)

type OutputResolution string

const (
	OutputResolutionDefault OutputResolution = "default"
	OutputResolution1K      OutputResolution = "1k"
	OutputResolution2K      OutputResolution = "2k"
	OutputResolution4K      OutputResolution = "4k"
)

var outputResolutions = [...]OutputResolution{
	OutputResolutionDefault,
	OutputResolution1K,
	OutputResolution2K,
	OutputResolution4K,
}

var defaultPricingVariants = [...]OutputResolution{
	OutputResolution1K,
	OutputResolution2K,
	OutputResolution4K,
}

func NormalizeOutputResolution(raw string) (OutputResolution, error) {
	resolution := OutputResolution(strings.ToLower(strings.TrimSpace(raw)))
	for _, supported := range outputResolutions {
		if resolution == supported {
			return resolution, nil
		}
	}
	return "", ErrInvalidOutputResolution
}

var (
	ErrInvalidSKU              = errors.New("invalid billing SKU")
	ErrInvalidQuantity         = errors.New("quantity must be a positive integer")
	ErrQuoteTotalExceeded      = errors.New("quote total exceeds allowed Credits")
	ErrInvalidCatalogEntry     = errors.New("invalid billing catalog entry")
	ErrInvalidOutputResolution = errors.New("invalid output resolution")
	ErrSKUNotFound             = errors.New("billing SKU not found")
	ErrPricingUnavailable      = errors.New("billing price is unavailable")
	ErrItemNotArchived         = errors.New("billing item is not archived")
	ErrArchivedItemInUse       = errors.New("archived billing item has durable references")
	ErrServiceUnavailable      = errors.New("billing service is unavailable")
)

// A SKU is globally unique as <namespace>:<local-key>. Both components use
// lowercase ASCII; namespace supports hyphens, and local keys support '.',
// '_' and '-' so future products can own names without changing Billing.
type SKU string

var skuPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$`)

func CanonicalizeSKU(raw string) (SKU, error) {
	canonical := strings.ToLower(strings.TrimSpace(raw))
	if len(canonical) > MaxSKULength || !skuPattern.MatchString(canonical) {
		return "", ErrInvalidSKU
	}
	return SKU(canonical), nil
}

type CatalogItem struct {
	SKU           SKU                            `json:"sku"`
	DisplayName   string                         `json:"display_name"`
	Description   string                         `json:"description"`
	Enabled       bool                           `json:"enabled"`
	CreatedAt     time.Time                      `json:"created_at"`
	UpdatedAt     time.Time                      `json:"updated_at"`
	CurrentPrices map[OutputResolution]PriceInfo `json:"current_prices,omitempty"`
}

type PriceInfo struct {
	PriceVersionID   int64            `json:"price_version_id"`
	Version          int64            `json:"version"`
	OutputResolution OutputResolution `json:"output_resolution"`
	UnitCredits      int64            `json:"unit_credits"`
	CreatedBy        int64            `json:"-"`
	CreatedAt        time.Time        `json:"created_at"`
}

type PublishPriceInput struct {
	Definition  BillableItemDefinition
	UnitCredits int64
	ActorID     int64
}

type PublishPricesInput struct {
	Definition BillableItemDefinition
	Prices     map[OutputResolution]int64
	ActorID    int64
}

func normalizePricingVariants(raw []OutputResolution) ([]OutputResolution, error) {
	if len(raw) == 0 {
		raw = defaultPricingVariants[:]
	}
	variants := make([]OutputResolution, 0, len(raw))
	seen := make(map[OutputResolution]struct{}, len(raw))
	for _, rawVariant := range raw {
		variant, err := NormalizeOutputResolution(string(rawVariant))
		if err != nil {
			return nil, ErrInvalidCatalogEntry
		}
		if _, exists := seen[variant]; exists {
			return nil, ErrInvalidCatalogEntry
		}
		seen[variant] = struct{}{}
		variants = append(variants, variant)
	}
	return variants, nil
}

func (definition BillableItemDefinition) pricingVariants() []OutputResolution {
	variants, err := normalizePricingVariants(definition.PricingVariants)
	if err != nil {
		return nil
	}
	return variants
}

type AvailabilityInfo struct {
	SKU       SKU                `json:"sku"`
	Status    BillableItemStatus `json:"status"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type ArchivedItemDeleteResult struct {
	SKU SKU `json:"sku"`
}

// PriceSnapshot is the immutable value object later Billing Commands persist.
// It contains no Provider Account or routing information.
type PriceSnapshot struct {
	SKU              SKU              `json:"sku"`
	DisplayName      string           `json:"display_name"`
	PriceVersionID   int64            `json:"price_version_id"`
	PriceVersion     int64            `json:"price_version"`
	OutputResolution OutputResolution `json:"output_resolution"`
	UnitCredits      int64            `json:"unit_credits"`
	QuotedAt         time.Time        `json:"quoted_at"`
}

func (snapshot PriceSnapshot) Total(quantity int64) (int64, error) {
	if quantity <= 0 {
		return 0, ErrInvalidQuantity
	}
	if snapshot.UnitCredits < 0 || snapshot.UnitCredits > MaxQuoteCredits {
		return 0, ErrPricingUnavailable
	}
	if snapshot.UnitCredits != 0 && quantity > MaxQuoteCredits/snapshot.UnitCredits {
		return 0, ErrQuoteTotalExceeded
	}
	return snapshot.UnitCredits * quantity, nil
}

type Quote struct {
	PriceSnapshot
	Quantity     int64 `json:"quantity"`
	TotalCredits int64 `json:"total_credits"`
}

func normalizeDisplayName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 120 {
		return "", ErrInvalidCatalogEntry
	}
	return value, nil
}

func normalizeDescription(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 1000 {
		return "", ErrInvalidCatalogEntry
	}
	return value, nil
}
