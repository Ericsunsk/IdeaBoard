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
	Status       BillableItemStatus `json:"status"`
	CurrentPrice *PriceInfo         `json:"current_price,omitempty"`
	UpdatedAt    *time.Time         `json:"updated_at,omitempty"`
}

type CatalogFactReader interface {
	ListCatalogFacts(context.Context) ([]CatalogItem, error)
}

type CatalogPricePublisher interface {
	PublishPrice(context.Context, PublishPriceInput) (PriceInfo, error)
}

type CatalogStore interface {
	CatalogFactReader
	CatalogPricePublisher
	SetAvailability(context.Context, string, bool, int64) (AvailabilityInfo, error)
	ListPrices(context.Context, ListPricesInput) (PricePage, error)
	DeleteArchived(context.Context, string, int64) (ArchivedItemDeleteResult, error)
	Quote(context.Context, string, int64) (Quote, error)
	QuoteInTx(context.Context, pgx.Tx, string, int64) (Quote, error)
}

type BillableItemCatalog struct {
	registry *BillableItemRegistry
	store    CatalogStore
}

func NewBillableItemCatalog(registry *BillableItemRegistry, store CatalogStore) *BillableItemCatalog {
	return &BillableItemCatalog{registry: registry, store: store}
}

func (c *BillableItemCatalog) PublishPrice(ctx context.Context, rawSKU string, unitCredits, actorID int64) (PriceInfo, error) {
	if c == nil || c.registry == nil || c.store == nil {
		return PriceInfo{}, ErrServiceUnavailable
	}
	definition, exists := c.registry.Find(rawSKU)
	if !exists {
		return PriceInfo{}, ErrSKUNotFound
	}
	if unitCredits < 0 || unitCredits > MaxQuoteCredits || actorID <= 0 {
		return PriceInfo{}, ErrInvalidCatalogEntry
	}
	return c.store.PublishPrice(ctx, PublishPriceInput{
		Definition: definition, UnitCredits: unitCredits, ActorID: actorID,
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

func (c *BillableItemCatalog) ListPriceHistory(ctx context.Context, rawSKU string, beforeVersion int64, limit int) (PricePage, error) {
	if c == nil || c.registry == nil || c.store == nil {
		return PricePage{}, ErrServiceUnavailable
	}
	page, err := c.store.ListPrices(ctx, ListPricesInput{SKU: rawSKU, BeforeVersion: beforeVersion, Limit: limit})
	if errors.Is(err, ErrSKUNotFound) {
		if _, exists := c.registry.Find(rawSKU); exists {
			return PricePage{Prices: []PriceInfo{}}, nil
		}
	}
	return page, err
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

func (c *BillableItemCatalog) Quote(ctx context.Context, rawSKU string, quantity int64) (Quote, error) {
	if err := c.requireRegistered(rawSKU); err != nil {
		return Quote{}, err
	}
	quote, err := c.store.Quote(ctx, rawSKU, quantity)
	return quote, normalizeCatalogQuoteError(err)
}

func (c *BillableItemCatalog) QuoteInTx(ctx context.Context, tx pgx.Tx, rawSKU string, quantity int64) (Quote, error) {
	if err := c.requireRegistered(rawSKU); err != nil {
		return Quote{}, err
	}
	quote, err := c.store.QuoteInTx(ctx, tx, rawSKU, quantity)
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
		}
		if fact, exists := factsBySKU[definition.SKU]; exists {
			card.CurrentPrice = clonePriceInfo(fact.CurrentPrice)
			updatedAt := fact.UpdatedAt
			card.UpdatedAt = &updatedAt
			if fact.CurrentPrice != nil {
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
			Status:       BillableItemStatusArchived,
			CurrentPrice: clonePriceInfo(fact.CurrentPrice),
			UpdatedAt:    &updatedAt,
		})
	}
	return cards, nil
}

func clonePriceInfo(price *PriceInfo) *PriceInfo {
	if price == nil {
		return nil
	}
	cloned := *price
	return &cloned
}
