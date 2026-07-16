package billing

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
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
	return &BillableItemRegistry{items: items}, nil
}

func (r *BillableItemRegistry) Items() []BillableItemDefinition {
	if r == nil {
		return []BillableItemDefinition{}
	}
	items := make([]BillableItemDefinition, len(r.items))
	copy(items, r.items)
	return items
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

type BillableItemCatalog struct {
	registry *BillableItemRegistry
	facts    CatalogFactReader
}

func NewBillableItemCatalog(registry *BillableItemRegistry, facts CatalogFactReader) *BillableItemCatalog {
	return &BillableItemCatalog{registry: registry, facts: facts}
}

func (c *BillableItemCatalog) List(ctx context.Context) ([]BillableItemCatalogCard, error) {
	if c == nil || c.registry == nil || c.facts == nil {
		return nil, ErrServiceUnavailable
	}
	facts, err := c.facts.ListCatalogFacts(ctx)
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
				if fact.Enabled && fact.CurrentPrice.Enabled {
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
