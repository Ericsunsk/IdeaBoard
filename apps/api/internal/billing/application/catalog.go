package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"ideaboard/api/internal/billing"
)

// CatalogStore is the persistence seam for catalog orchestration. The tx
// scoped quote keeps the submission single-transaction boundary (see
// tasking/application/billing); callers pass their own pgx.Tx per house
// convention.
type CatalogStore interface {
	ListCatalogFacts(context.Context) ([]billing.CatalogItem, error)
	PublishPrices(context.Context, billing.PublishPricesInput) (map[billing.OutputResolution]billing.PriceInfo, error)
	SetAvailability(context.Context, string, bool, int64) (billing.AvailabilityInfo, error)
	DeleteArchived(context.Context, string, int64) (billing.ArchivedItemDeleteResult, error)
	QuoteAtResolution(context.Context, string, billing.OutputResolution, int64) (billing.Quote, error)
	QuoteAtResolutionInTx(context.Context, pgx.Tx, string, billing.OutputResolution, int64) (billing.Quote, error)
}

// BillableItemCatalogCard is the admin projection merging one registry
// definition with its persisted facts.
type BillableItemCatalogCard struct {
	billing.BillableItemDefinition
	Status          billing.BillableItemStatus                     `json:"status"`
	CurrentPrices   map[billing.OutputResolution]billing.PriceInfo `json:"current_prices"`
	PricingVariants []billing.OutputResolution                     `json:"pricing_variants"`
	UpdatedAt       *time.Time                                     `json:"updated_at,omitempty"`
}

// BillableItemCatalog gates every catalog command on the immutable product
// registry before touching the store.
type BillableItemCatalog struct {
	registry *billing.BillableItemRegistry
	store    CatalogStore
}

// NewBillableItemCatalog composes the product registry with its store.
func NewBillableItemCatalog(registry *billing.BillableItemRegistry, store CatalogStore) *BillableItemCatalog {
	return &BillableItemCatalog{registry: registry, store: store}
}

func (c *BillableItemCatalog) PublishPrices(ctx context.Context, rawSKU string, prices map[billing.OutputResolution]int64, actorID int64) (map[billing.OutputResolution]billing.PriceInfo, error) {
	if c == nil || c.registry == nil || c.store == nil {
		return nil, billing.ErrServiceUnavailable
	}
	definition, exists := c.registry.Find(rawSKU)
	if !exists {
		return nil, billing.ErrSKUNotFound
	}
	if actorID <= 0 {
		return nil, billing.ErrInvalidCatalogEntry
	}
	if err := billing.ValidatePricingVariants(definition, prices); err != nil {
		return nil, err
	}
	return c.store.PublishPrices(ctx, billing.PublishPricesInput{
		Definition: definition, Prices: prices, ActorID: actorID,
	})
}

func (c *BillableItemCatalog) SetAvailability(ctx context.Context, rawSKU string, enabled bool, actorID int64) (billing.AvailabilityInfo, error) {
	if c == nil || c.registry == nil || c.store == nil {
		return billing.AvailabilityInfo{}, billing.ErrServiceUnavailable
	}
	if _, exists := c.registry.Find(rawSKU); !exists {
		return billing.AvailabilityInfo{}, billing.ErrSKUNotFound
	}
	return c.store.SetAvailability(ctx, rawSKU, enabled, actorID)
}

func (c *BillableItemCatalog) DeleteArchived(ctx context.Context, rawSKU string, actorID int64) (billing.ArchivedItemDeleteResult, error) {
	if c == nil || c.registry == nil || c.store == nil {
		return billing.ArchivedItemDeleteResult{}, billing.ErrServiceUnavailable
	}
	if actorID <= 0 {
		return billing.ArchivedItemDeleteResult{}, billing.ErrInvalidCatalogEntry
	}
	if _, err := billing.CanonicalizeSKU(rawSKU); err != nil {
		return billing.ArchivedItemDeleteResult{}, err
	}
	if _, exists := c.registry.Find(rawSKU); exists {
		return billing.ArchivedItemDeleteResult{}, billing.ErrItemNotArchived
	}
	return c.store.DeleteArchived(ctx, rawSKU, actorID)
}

func (c *BillableItemCatalog) QuoteAtResolution(ctx context.Context, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	definition, err := c.registeredDefinition(rawSKU)
	if err != nil {
		return billing.Quote{}, err
	}
	if err := billing.ValidatePricingVariant(definition, resolution); err != nil {
		return billing.Quote{}, err
	}
	quote, err := c.store.QuoteAtResolution(ctx, rawSKU, resolution, quantity)
	return quote, normalizeCatalogQuoteError(err)
}

func (c *BillableItemCatalog) QuoteAtResolutionInTx(ctx context.Context, tx pgx.Tx, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	definition, err := c.registeredDefinition(rawSKU)
	if err != nil {
		return billing.Quote{}, err
	}
	if err := billing.ValidatePricingVariant(definition, resolution); err != nil {
		return billing.Quote{}, err
	}
	quote, err := c.store.QuoteAtResolutionInTx(ctx, tx, rawSKU, resolution, quantity)
	return quote, normalizeCatalogQuoteError(err)
}

func (c *BillableItemCatalog) registeredDefinition(rawSKU string) (billing.BillableItemDefinition, error) {
	if c == nil || c.registry == nil || c.store == nil {
		return billing.BillableItemDefinition{}, billing.ErrServiceUnavailable
	}
	if _, err := billing.CanonicalizeSKU(rawSKU); err != nil {
		return billing.BillableItemDefinition{}, err
	}
	definition, exists := c.registry.Find(rawSKU)
	if !exists {
		return billing.BillableItemDefinition{}, billing.ErrPricingUnavailable
	}
	return definition, nil
}

func normalizeCatalogQuoteError(err error) error {
	if errors.Is(err, billing.ErrSKUNotFound) {
		return billing.ErrPricingUnavailable
	}
	return err
}

func (c *BillableItemCatalog) List(ctx context.Context) ([]BillableItemCatalogCard, error) {
	if c == nil || c.registry == nil || c.store == nil {
		return nil, billing.ErrServiceUnavailable
	}
	facts, err := c.store.ListCatalogFacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list billable item catalog facts: %w", err)
	}
	factsBySKU := make(map[billing.SKU]billing.CatalogItem, len(facts))
	for _, fact := range facts {
		factsBySKU[fact.SKU] = fact
	}

	definitions := c.registry.Items()
	cards := make([]BillableItemCatalogCard, 0, len(definitions)+len(facts))
	for _, definition := range definitions {
		card := BillableItemCatalogCard{
			BillableItemDefinition: definition,
			Status:                 billing.BillableItemStatusUnpriced,
			CurrentPrices:          map[billing.OutputResolution]billing.PriceInfo{},
			PricingVariants:        append([]billing.OutputResolution(nil), definition.PricingVariants...),
		}
		if fact, exists := factsBySKU[definition.SKU]; exists {
			card.CurrentPrices = clonePriceMap(fact.CurrentPrices)
			updatedAt := fact.UpdatedAt
			card.UpdatedAt = &updatedAt
			if hasConfiguredPricingVariant(definition, fact.CurrentPrices) {
				if fact.Enabled {
					card.Status = billing.BillableItemStatusActive
				} else {
					card.Status = billing.BillableItemStatusDisabled
				}
			}
			delete(factsBySKU, definition.SKU)
		}
		cards = append(cards, card)
	}

	archived := make([]billing.CatalogItem, 0, len(factsBySKU))
	for _, fact := range factsBySKU {
		archived = append(archived, fact)
	}
	sort.Slice(archived, func(i, j int) bool { return archived[i].SKU < archived[j].SKU })
	for _, fact := range archived {
		namespace, _, _ := strings.Cut(string(fact.SKU), ":")
		updatedAt := fact.UpdatedAt
		cards = append(cards, BillableItemCatalogCard{
			BillableItemDefinition: billing.BillableItemDefinition{
				SKU: fact.SKU, Namespace: namespace, ProductKind: "legacy",
				DisplayName: fact.DisplayName, Description: fact.Description,
			},
			Status:          billing.BillableItemStatusArchived,
			CurrentPrices:   clonePriceMap(fact.CurrentPrices),
			PricingVariants: sortedPriceVariants(fact.CurrentPrices),
			UpdatedAt:       &updatedAt,
		})
	}
	return cards, nil
}

func hasConfiguredPricingVariant(definition billing.BillableItemDefinition, prices map[billing.OutputResolution]billing.PriceInfo) bool {
	for _, variant := range definition.PricingVariants {
		if _, ok := prices[variant]; ok {
			return true
		}
	}
	return false
}

func sortedPriceVariants(prices map[billing.OutputResolution]billing.PriceInfo) []billing.OutputResolution {
	variants := make([]billing.OutputResolution, 0, len(prices))
	for variant := range prices {
		variants = append(variants, variant)
	}
	sort.Slice(variants, func(i, j int) bool {
		return pricingVariantOrder(variants[i]) < pricingVariantOrder(variants[j])
	})
	return variants
}

func pricingVariantOrder(variant billing.OutputResolution) int {
	switch variant {
	case billing.OutputResolution1K:
		return 0
	case billing.OutputResolution2K:
		return 1
	case billing.OutputResolution4K:
		return 2
	default:
		return 4
	}
}

func clonePriceMap(prices map[billing.OutputResolution]billing.PriceInfo) map[billing.OutputResolution]billing.PriceInfo {
	cloned := make(map[billing.OutputResolution]billing.PriceInfo, len(prices))
	for resolution, price := range prices {
		cloned[resolution] = price
	}
	return cloned
}
