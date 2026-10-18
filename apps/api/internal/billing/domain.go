package billing

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"ideaboard/api/internal/generation/routing"
)

const (
	MaxSKULength    = 128
	MaxQuoteCredits = int64(9_000_000_000_000_000)
)

type OutputResolution string

const (
	OutputResolution1K OutputResolution = OutputResolution(routing.Resolution1K)
	OutputResolution2K OutputResolution = OutputResolution(routing.Resolution2K)
	OutputResolution4K OutputResolution = OutputResolution(routing.Resolution4K)
)

// canonicalPricingVariants is the registry fallback when a definition omits
// its variant list: exactly 1k, 2k, and 4k. It names catalog values, never a
// "default" resolution.
var canonicalPricingVariants = [...]OutputResolution{
	OutputResolution1K,
	OutputResolution2K,
	OutputResolution4K,
}

func NormalizeOutputResolution(raw string) (OutputResolution, error) {
	// Canonical owner is routing.NormalizeResolution; this is a thin
	// type adapter so billing callers never fork the 1k/2k/4k set.
	// Blank/"default" is rejected here; omitted requests default to 1k
	// once in workflow.compileOutputResolution only.
	res, err := routing.NormalizeResolution(raw)
	if err != nil {
		return "", ErrInvalidOutputResolution
	}
	return OutputResolution(res), nil
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
		raw = canonicalPricingVariants[:]
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

// ResolvedPricingVariants returns the definition's normalized pricing
// variants, defaulting when empty. Registry construction normalizes the
// field; this re-applies the same rule for inputs arriving off-registry.
func (definition BillableItemDefinition) ResolvedPricingVariants() []OutputResolution {
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

func NormalizeDisplayName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 120 {
		return "", ErrInvalidCatalogEntry
	}
	return value, nil
}

func NormalizeDescription(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 1000 {
		return "", ErrInvalidCatalogEntry
	}
	return value, nil
}
