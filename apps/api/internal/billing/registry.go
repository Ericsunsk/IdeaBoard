package billing

import (
	"errors"
	"sort"
	"strings"
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
	SKU             SKU                `json:"sku"`
	Namespace       string             `json:"namespace"`
	ProductKind     string             `json:"product_kind"`
	DisplayName     string             `json:"display_name"`
	Description     string             `json:"description"`
	SortOrder       int                `json:"sort_order"`
	PricingVariants []OutputResolution `json:"-"`
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
		displayName, err := NormalizeDisplayName(definition.DisplayName)
		if err != nil {
			return nil, err
		}
		description, err := NormalizeDescription(definition.Description)
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
		pricingVariants, err := normalizePricingVariants(definition.PricingVariants)
		if err != nil {
			return nil, err
		}
		definition.PricingVariants = pricingVariants
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
	for index := range items {
		items[index].PricingVariants = append([]OutputResolution(nil), r.items[index].PricingVariants...)
	}
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
	item.PricingVariants = append([]OutputResolution(nil), item.PricingVariants...)
	return item, exists
}

func (r *BillableItemRegistry) Contains(rawSKU string) bool {
	_, exists := r.Find(rawSKU)
	return exists
}

func ValidatePricingVariants(definition BillableItemDefinition, prices map[OutputResolution]int64) error {
	variants := definition.ResolvedPricingVariants()
	if len(variants) == 0 || len(prices) == 0 {
		return ErrInvalidCatalogEntry
	}
	allowed := make(map[OutputResolution]struct{}, len(variants))
	for _, variant := range variants {
		allowed[variant] = struct{}{}
	}
	if _, required := prices[variants[0]]; !required {
		return ErrInvalidCatalogEntry
	}
	for variant := range prices {
		if _, ok := allowed[variant]; !ok {
			return ErrInvalidCatalogEntry
		}
	}
	return nil
}

func ValidatePricingVariant(definition BillableItemDefinition, variant OutputResolution) error {
	if _, err := NormalizeOutputResolution(string(variant)); err != nil {
		return err
	}
	for _, allowed := range definition.ResolvedPricingVariants() {
		if variant == allowed {
			return nil
		}
	}
	return ErrInvalidOutputResolution
}
