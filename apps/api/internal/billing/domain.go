package billing

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxSKULength         = 128
	MaxQuoteCredits      = int64(9_000_000_000_000_000)
	MaxAdminReasonLength = 1000
	DefaultPageLimit     = 50
	MaxPageLimit         = 100
)

var (
	ErrInvalidSKU          = errors.New("invalid billing SKU")
	ErrInvalidQuantity     = errors.New("quantity must be a positive integer")
	ErrQuoteTotalExceeded  = errors.New("quote total exceeds allowed Credits")
	ErrInvalidCatalogEntry = errors.New("invalid billing catalog entry")
	ErrSKUAlreadyExists    = errors.New("billing SKU already exists")
	ErrSKUNotFound         = errors.New("billing SKU not found")
	ErrPriceNotFound       = errors.New("billing price version not found")
	ErrPricingUnavailable  = errors.New("billing price is unavailable")
	ErrServiceUnavailable  = errors.New("billing service is unavailable")
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
	SKU                   SKU        `json:"sku"`
	DisplayName           string     `json:"display_name"`
	Description           string     `json:"description"`
	Enabled               bool       `json:"enabled"`
	CurrentPriceVersionID *int64     `json:"current_price_version_id,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	CurrentPrice          *PriceInfo `json:"current_price,omitempty"`
}

type PriceInfo struct {
	PriceVersionID int64     `json:"price_version_id"`
	Version        int64     `json:"version"`
	UnitCredits    int64     `json:"unit_credits"`
	Enabled        bool      `json:"enabled"`
	CreatedBy      int64     `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedBy      int64     `json:"updated_by"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CatalogPage struct {
	Items        []CatalogItem `json:"items"`
	NextAfterSKU *SKU          `json:"next_after_sku,omitempty"`
}

type CreateSKUInput struct {
	SKU         string
	DisplayName string
	Description string
	Enabled     bool
	ActorID     int64
	Reason      string
}

type UpdateSKUInput struct {
	SKU         string
	DisplayName *string
	Description *string
	Enabled     *bool
	ActorID     int64
	Reason      string
}

type CreatePriceInput struct {
	SKU         string
	UnitCredits int64
	Enabled     bool
	ActorID     int64
	Reason      string
}

type SetPriceEnabledInput struct {
	SKU     string
	Version int64
	Enabled bool
	ActorID int64
	Reason  string
}

type ListCatalogInput struct {
	AfterSKU string
	Limit    int
}

type ListPricesInput struct {
	SKU           string
	BeforeVersion int64
	Limit         int
}

type PricePage struct {
	Prices            []PriceInfo `json:"prices"`
	NextBeforeVersion *int64      `json:"next_before_version,omitempty"`
}

// PriceSnapshot is the immutable value object later Billing Commands persist.
// It contains no Provider Account or routing information.
type PriceSnapshot struct {
	SKU            SKU       `json:"sku"`
	PriceVersionID int64     `json:"price_version_id"`
	PriceVersion   int64     `json:"price_version"`
	UnitCredits    int64     `json:"unit_credits"`
	QuotedAt       time.Time `json:"quoted_at"`
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

func normalizeAdminReason(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > MaxAdminReasonLength {
		return "", ErrInvalidCatalogEntry
	}
	return value, nil
}

func normalizePageLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultPageLimit, nil
	}
	if limit < 1 || limit > MaxPageLimit {
		return 0, fmt.Errorf("%w: page limit", ErrInvalidCatalogEntry)
	}
	return limit, nil
}
