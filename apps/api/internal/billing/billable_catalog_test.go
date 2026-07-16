package billing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBillableItemRegistryRejectsDuplicateSKUs(t *testing.T) {
	_, err := NewBillableItemRegistry([]BillableItemDefinition{
		{SKU: "ai-image:partial-edit", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "局部重绘"},
		{SKU: "AI-IMAGE:PARTIAL-EDIT", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "重复项"},
	})

	if !errors.Is(err, ErrDuplicateBillableItem) {
		t.Fatalf("expected duplicate item error, got %v", err)
	}
}

func TestBillableItemRegistryReturnsDeterministicCopies(t *testing.T) {
	registry, err := NewBillableItemRegistry([]BillableItemDefinition{
		{SKU: "ai-image:remove-bg", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "抠图", SortOrder: 20},
		{SKU: "ai-image:partial-edit", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "局部重绘", SortOrder: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	first := registry.Items()
	first[0].DisplayName = "mutated"
	second := registry.Items()

	if got := string(second[0].SKU); got != "ai-image:partial-edit" {
		t.Fatalf("expected registry order by sort order, got %q", got)
	}
	if got := second[0].DisplayName; got != "局部重绘" {
		t.Fatalf("registry leaked caller mutation: %q", got)
	}
}

func TestBillableItemRegistryEnforcesProductKindLength(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		productKind string
		wantError   bool
	}{
		{name: "maximum length", productKind: strings.Repeat("a", 64)},
		{name: "maximum Unicode length", productKind: strings.Repeat("类", 64)},
		{name: "over maximum length", productKind: strings.Repeat("a", 65), wantError: true},
		{name: "over maximum Unicode length", productKind: strings.Repeat("类", 65), wantError: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewBillableItemRegistry([]BillableItemDefinition{{
				SKU: "test:item", Namespace: "test", ProductKind: testCase.productKind, DisplayName: "Item",
			}})
			if testCase.wantError && !errors.Is(err, ErrInvalidCatalogEntry) {
				t.Fatalf("expected invalid catalog entry, got %v", err)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf("expected product kind to be accepted, got %v", err)
			}
		})
	}
}

func TestBillableCatalogMergesRegistryAndPersistedFacts(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	activePrice := PriceInfo{PriceVersionID: 11, Version: 2, UnitCredits: 25, Enabled: true, UpdatedAt: now}
	disabledPrice := PriceInfo{PriceVersionID: 12, Version: 1, UnitCredits: 10, Enabled: true, UpdatedAt: now}
	disabledPriceState := PriceInfo{PriceVersionID: 14, Version: 1, UnitCredits: 12, Enabled: false, UpdatedAt: now}
	archivedPrice := PriceInfo{PriceVersionID: 13, Version: 3, UnitCredits: 50, Enabled: true, UpdatedAt: now}
	registry, err := NewBillableItemRegistry([]BillableItemDefinition{
		{SKU: "ai-image:partial-edit", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "局部重绘", Description: "只修改选区", SortOrder: 10},
		{SKU: "ai-image:remove-bg", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "抠图", SortOrder: 20},
		{SKU: "ai-image:hd-upscale", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "HD", SortOrder: 30},
		{SKU: "ai-image:image-extend", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "扩图", SortOrder: 40},
	})
	facts := catalogFactReaderStub{items: []CatalogItem{
		{SKU: "ai-image:partial-edit", Enabled: true, CurrentPrice: &activePrice, UpdatedAt: now},
		{SKU: "ai-image:remove-bg", Enabled: false, CurrentPrice: &disabledPrice, UpdatedAt: now},
		{SKU: "ai-image:image-extend", Enabled: true, CurrentPrice: &disabledPriceState, UpdatedAt: now},
		{SKU: "legacy:removed", DisplayName: "旧功能", Description: "已从 Registry 移除", Enabled: true, CurrentPrice: &archivedPrice, UpdatedAt: now},
	}}
	service := NewBillableItemCatalog(registry, facts)

	items, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 {
		t.Fatalf("expected five catalog cards, got %d", len(items))
	}
	assertCatalogCard(t, items[0], "ai-image:partial-edit", BillableItemStatusActive, "局部重绘")
	assertCatalogCard(t, items[1], "ai-image:remove-bg", BillableItemStatusDisabled, "抠图")
	assertCatalogCard(t, items[2], "ai-image:hd-upscale", BillableItemStatusUnpriced, "HD")
	assertCatalogCard(t, items[3], "ai-image:image-extend", BillableItemStatusDisabled, "扩图")
	assertCatalogCard(t, items[4], "legacy:removed", BillableItemStatusArchived, "旧功能")
}

func assertCatalogCard(t *testing.T, item BillableItemCatalogCard, sku string, status BillableItemStatus, displayName string) {
	t.Helper()
	if string(item.SKU) != sku || item.Status != status || item.DisplayName != displayName {
		t.Fatalf("unexpected catalog card: %+v", item)
	}
}

type catalogFactReaderStub struct {
	items []CatalogItem
	err   error
}

func (s catalogFactReaderStub) ListCatalogFacts(context.Context) ([]CatalogItem, error) {
	return s.items, s.err
}
