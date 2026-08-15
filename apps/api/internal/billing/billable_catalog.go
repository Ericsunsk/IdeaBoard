package billing

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var ErrDuplicateBillableItem = errors.New("duplicate billable item SKU")

type BillableItemStatus string

const (
	BillableItemStatusUnpriced BillableItemStatus = "unpriced"
	BillableItemStatusActive   BillableItemStatus = "active"
	BillableItemStatusDisabled BillableItemStatus = "disabled"
	BillableItemStatusArchived BillableItemStatus = "archived"
)

// BillableItemDefinition is the product-owned registration contract consumed
// by Billing. Provider models are intentionally absent from this identity.
type BillableItemDefinition struct {
	SKU         SKU    `json:"sku"`
	Namespace   string `json:"namespace"`
	ProductKind string `json:"product_kind"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

type BillableItemRegistry struct {
	items []BillableItemDefinition
	bySKU map[SKU]BillableItemDefinition
}

func NewBillableItemRegistry(definitions []BillableItemDefinition) (*BillableItemRegistry, error) {
	items := make([]BillableItemDefinition, 0, len(definitions))
	seen := make(map[SKU]struct{}, len(definitions))
	for _, definition := range definitions {
		sku, err := CanonicalizeSKU(string(definition.SKU))
		if err != nil || definition.SortOrder < 0 {
			return nil, ErrInvalidCatalogEntry
		}
		namespace, _, _ := strings.Cut(string(sku), ":")
		productKind := strings.TrimSpace(definition.ProductKind)
		if strings.TrimSpace(definition.Namespace) != namespace || productKind == "" || utf8.RuneCountInString(productKind) > 64 {
			return nil, ErrInvalidCatalogEntry
		}
		displayName, err := normalizeDisplayName(definition.DisplayName)
		if err != nil {
			return nil, err
		}
		description, err := normalizeDescription(definition.Description)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[sku]; exists {
			return nil, ErrDuplicateBillableItem
		}
		seen[sku] = struct{}{}
		definition.SKU = sku
		definition.Namespace = namespace
		definition.ProductKind = productKind
		definition.DisplayName = displayName
		definition.Description = description
		items = append(items, definition)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		return items[i].SKU < items[j].SKU
	})
	bySKU := make(map[SKU]BillableItemDefinition, len(items))
	for _, item := range items {
		bySKU[item.SKU] = item
	}
	return &BillableItemRegistry{items: items, bySKU: bySKU}, nil
}

func (r *BillableItemRegistry) Items() []BillableItemDefinition {
	if r == nil {
		return []BillableItemDefinition{}
	}
	items := make([]BillableItemDefinition, len(r.items))
	copy(items, r.items)
	return items
}

func (r *BillableItemRegistry) Find(rawSKU string) (BillableItemDefinition, bool) {
	if r == nil {
		return BillableItemDefinition{}, false
	}
	sku, err := CanonicalizeSKU(rawSKU)
	if err != nil {
		return BillableItemDefinition{}, false
	}
	item, exists := r.bySKU[sku]
	return item, exists
}

func (r *BillableItemRegistry) Contains(rawSKU string) bool {
	_, exists := r.Find(rawSKU)
	return exists
}

type BillableItemCatalogCard struct {
	BillableItemDefinition
	Status        BillableItemStatus             `json:"status"`
	CurrentPrices map[OutputResolution]PriceInfo `json:"current_prices"`
	UpdatedAt     *time.Time                     `json:"updated_at,omitempty"`
}

type CatalogFactReader interface {
	ListCatalogFacts(context.Context) ([]CatalogItem, error)
}

type CatalogPricePublisher interface {
	PublishPrices(context.Context, PublishPricesInput) (map[OutputResolution]PriceInfo, error)
}

type CatalogStore interface {
	CatalogFactReader
	CatalogPricePublisher
	SetAvailability(context.Context, string, bool, int64) (AvailabilityInfo, error)
	DeleteArchived(context.Context, string, int64) (ArchivedItemDeleteResult, error)
	QuoteAtResolution(context.Context, string, OutputResolution, int64) (Quote, error)
	QuoteAtResolutionInTx(context.Context, pgx.Tx, string, OutputResolution, int64) (Quote, error)
}

type BillableItemCatalog struct {
	registry *BillableItemRegistry
	store    CatalogStore
}

func NewBillableItemCatalog(registry *BillableItemRegistry, store CatalogStore) *BillableItemCatalog {
	return &BillableItemCatalog{registry: registry, store: store}
}

func (c *BillableItemCatalog) PublishPrices(ctx context.Context, rawSKU string, prices map[OutputResolution]int64, actorID int64) (map[OutputResolution]PriceInfo, error) {
	if c == nil || c.registry == nil || c.store == nil {
		return nil, ErrServiceUnavailable
	}
	definition, exists := c.registry.Find(rawSKU)
	if !exists {
		return nil, ErrSKUNotFound
	}
	if actorID <= 0 {
		return nil, ErrInvalidCatalogEntry
	}
	return c.store.PublishPrices(ctx, PublishPricesInput{
		Definition: definition, Prices: prices, ActorID: actorID,
	})
}

func (c *BillableItemCatalog) SetAvailability(ctx context.Context, rawSKU string, enabled bool, actorID int64) (AvailabilityInfo, error) {
	if c == nil || c.registry == nil || c.store == nil {
		return AvailabilityInfo{}, ErrServiceUnavailable
	}
	if _, exists := c.registry.Find(rawSKU); !exists {
		return AvailabilityInfo{}, ErrSKUNotFound
	}
	return c.store.SetAvailability(ctx, rawSKU, enabled, actorID)
}

func (c *BillableItemCatalog) DeleteArchived(ctx context.Context, rawSKU string, actorID int64) (ArchivedItemDeleteResult, error) {
	if c == nil || c.registry == nil || c.store == nil {
		return ArchivedItemDeleteResult{}, ErrServiceUnavailable
	}
	if actorID <= 0 {
		return ArchivedItemDeleteResult{}, ErrInvalidCatalogEntry
	}
	if _, err := CanonicalizeSKU(rawSKU); err != nil {
		return ArchivedItemDeleteResult{}, err
	}
	if _, exists := c.registry.Find(rawSKU); exists {
		return ArchivedItemDeleteResult{}, ErrItemNotArchived
	}
	return c.store.DeleteArchived(ctx, rawSKU, actorID)
}

func (c *BillableItemCatalog) QuoteAtResolution(ctx context.Context, rawSKU string, resolution OutputResolution, quantity int64) (Quote, error) {
	if err := c.requireRegistered(rawSKU); err != nil {
		return Quote{}, err
	}
	quote, err := c.store.QuoteAtResolution(ctx, rawSKU, resolution, quantity)
	return quote, normalizeCatalogQuoteError(err)
}

func (c *BillableItemCatalog) QuoteAtResolutionInTx(ctx context.Context, tx pgx.Tx, rawSKU string, resolution OutputResolution, quantity int64) (Quote, error) {
	if err := c.requireRegistered(rawSKU); err != nil {
		return Quote{}, err
	}
	quote, err := c.store.QuoteAtResolutionInTx(ctx, tx, rawSKU, resolution, quantity)
	return quote, normalizeCatalogQuoteError(err)
}

func (c *BillableItemCatalog) requireRegistered(rawSKU string) error {
	if c == nil || c.registry == nil || c.store == nil {
		return ErrServiceUnavailable
	}
	if _, err := CanonicalizeSKU(rawSKU); err != nil {
		return err
	}
	if _, exists := c.registry.Find(rawSKU); !exists {
		return ErrPricingUnavailable
	}
	return nil
}

func normalizeCatalogQuoteError(err error) error {
	if errors.Is(err, ErrSKUNotFound) {
		return ErrPricingUnavailable
	}
	return err
}

func (c *BillableItemCatalog) List(ctx context.Context) ([]BillableItemCatalogCard, error) {
	if c == nil || c.registry == nil || c.store == nil {
		return nil, ErrServiceUnavailable
	}
	facts, err := c.store.ListCatalogFacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list billable item catalog facts: %w", err)
	}
	factsBySKU := make(map[SKU]CatalogItem, len(facts))
	for _, fact := range facts {
		factsBySKU[fact.SKU] = fact
	}

	definitions := c.registry.Items()
	cards := make([]BillableItemCatalogCard, 0, len(definitions)+len(facts))
	for _, definition := range definitions {
		card := BillableItemCatalogCard{
			BillableItemDefinition: definition,
			Status:                 BillableItemStatusUnpriced,
			CurrentPrices:          map[OutputResolution]PriceInfo{},
		}
		if fact, exists := factsBySKU[definition.SKU]; exists {
			card.CurrentPrices = clonePriceMap(fact.CurrentPrices)
			updatedAt := fact.UpdatedAt
			card.UpdatedAt = &updatedAt
			if _, hasOneK := fact.CurrentPrices[OutputResolution1K]; hasOneK {
				if fact.Enabled {
					card.Status = BillableItemStatusActive
				} else {
					card.Status = BillableItemStatusDisabled
				}
			}
			delete(factsBySKU, definition.SKU)
		}
		cards = append(cards, card)
	}

	archived := make([]CatalogItem, 0, len(factsBySKU))
	for _, fact := range factsBySKU {
		archived = append(archived, fact)
	}
	sort.Slice(archived, func(i, j int) bool { return archived[i].SKU < archived[j].SKU })
	for _, fact := range archived {
		namespace, _, _ := strings.Cut(string(fact.SKU), ":")
		updatedAt := fact.UpdatedAt
		cards = append(cards, BillableItemCatalogCard{
			BillableItemDefinition: BillableItemDefinition{
				SKU: fact.SKU, Namespace: namespace, ProductKind: "legacy",
				DisplayName: fact.DisplayName, Description: fact.Description,
			},
			Status:        BillableItemStatusArchived,
			CurrentPrices: clonePriceMap(fact.CurrentPrices),
			UpdatedAt:     &updatedAt,
		})
	}
	return cards, nil
}

func clonePriceMap(prices map[OutputResolution]PriceInfo) map[OutputResolution]PriceInfo {
	cloned := make(map[OutputResolution]PriceInfo, len(prices))
	for resolution, price := range prices {
		cloned[resolution] = price
	}
	return cloned
}
