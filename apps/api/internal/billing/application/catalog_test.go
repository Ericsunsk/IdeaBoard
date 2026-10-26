package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"ideaboard/api/internal/billing"
)

// Domain vocabulary aliases keep the moved tests byte-identical.
type (
	SKU                      = billing.SKU
	OutputResolution         = billing.OutputResolution
	Quote                    = billing.Quote
	PriceInfo                = billing.PriceInfo
	CatalogItem              = billing.CatalogItem
	PublishPricesInput       = billing.PublishPricesInput
	AvailabilityInfo         = billing.AvailabilityInfo
	ArchivedItemDeleteResult = billing.ArchivedItemDeleteResult
	BillableItemDefinition   = billing.BillableItemDefinition
	BillableItemStatus       = billing.BillableItemStatus
)

const (
	OutputResolution1K         = billing.OutputResolution1K
	BillableItemStatusActive   = billing.BillableItemStatusActive
	BillableItemStatusArchived = billing.BillableItemStatusArchived
	BillableItemStatusDisabled = billing.BillableItemStatusDisabled
	BillableItemStatusUnpriced = billing.BillableItemStatusUnpriced
)

// NewBillableItemRegistry builds the product registry under test.
var NewBillableItemRegistry = billing.NewBillableItemRegistry

// Error sentinels under test.
var (
	ErrInvalidCatalogEntry = billing.ErrInvalidCatalogEntry
	ErrItemNotArchived     = billing.ErrItemNotArchived
	ErrSKUNotFound         = billing.ErrSKUNotFound
)

func TestBillableCatalogMergesRegistryAndPersistedFacts(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	activePrice := PriceInfo{PriceVersionID: 11, Version: 2, OutputResolution: OutputResolution1K, UnitCredits: 25, CreatedAt: now}
	disabledPrice := PriceInfo{PriceVersionID: 12, Version: 1, OutputResolution: OutputResolution1K, UnitCredits: 10, CreatedAt: now}
	activePriceTwo := PriceInfo{PriceVersionID: 14, Version: 1, OutputResolution: OutputResolution1K, UnitCredits: 12, CreatedAt: now}
	archivedPrice := PriceInfo{PriceVersionID: 13, Version: 3, OutputResolution: OutputResolution1K, UnitCredits: 50, CreatedAt: now}
	registry, err := NewBillableItemRegistry([]BillableItemDefinition{
		{SKU: "ai-image:inpaint", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "局部重绘", Description: "只修改选区", SortOrder: 10},
		{SKU: "ai-image:cutout", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "抠图", SortOrder: 20, PricingVariants: []OutputResolution{OutputResolution1K}},
		{SKU: "ai-image:upscale", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "Upscale", SortOrder: 30},
		{SKU: "ai-image:outpaint", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "扩图", SortOrder: 40},
	})
	if err != nil {
		t.Fatal(err)
	}
	facts := catalogFactReaderStub{items: []CatalogItem{
		{SKU: "ai-image:inpaint", Enabled: true, CurrentPrices: map[OutputResolution]PriceInfo{OutputResolution1K: activePrice}, UpdatedAt: now},
		{SKU: "ai-image:cutout", Enabled: false, CurrentPrices: map[OutputResolution]PriceInfo{OutputResolution1K: disabledPrice}, UpdatedAt: now},
		{SKU: "ai-image:outpaint", Enabled: true, CurrentPrices: map[OutputResolution]PriceInfo{OutputResolution1K: activePriceTwo}, UpdatedAt: now},
		{SKU: "legacy:removed", DisplayName: "旧功能", Description: "已从 Registry 移除", Enabled: true, CurrentPrices: map[OutputResolution]PriceInfo{OutputResolution1K: archivedPrice}, UpdatedAt: now},
	}}
	service := NewBillableItemCatalog(registry, &facts)

	items, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 {
		t.Fatalf("expected five catalog cards, got %d", len(items))
	}
	assertCatalogCard(t, items[0], "ai-image:inpaint", BillableItemStatusActive, "局部重绘")
	assertCatalogCard(t, items[1], "ai-image:cutout", BillableItemStatusDisabled, "抠图")
	assertCatalogCard(t, items[2], "ai-image:upscale", BillableItemStatusUnpriced, "Upscale")
	assertCatalogCard(t, items[3], "ai-image:outpaint", BillableItemStatusActive, "扩图")
	assertCatalogCard(t, items[4], "legacy:removed", BillableItemStatusArchived, "旧功能")
}

func assertCatalogCard(t *testing.T, item BillableItemCatalogCard, sku string, status BillableItemStatus, displayName string) {
	t.Helper()
	if string(item.SKU) != sku || item.Status != status || item.DisplayName != displayName {
		t.Fatalf("unexpected catalog card: %+v", item)
	}
}

type catalogFactReaderStub struct {
	items        []CatalogItem
	err          error
	publishInput PublishPricesInput
	deleted      ArchivedItemDeleteResult
}

func (s *catalogFactReaderStub) DeleteArchived(_ context.Context, sku string, _ int64) (ArchivedItemDeleteResult, error) {
	s.deleted = ArchivedItemDeleteResult{SKU: SKU(sku)}
	return s.deleted, s.err
}

func (s *catalogFactReaderStub) ListCatalogFacts(context.Context) ([]CatalogItem, error) {
	return s.items, s.err
}

func (s *catalogFactReaderStub) PublishPrices(_ context.Context, input PublishPricesInput) (map[OutputResolution]PriceInfo, error) {
	s.publishInput = input
	return map[OutputResolution]PriceInfo{OutputResolution1K: {PriceVersionID: 1, Version: 1, OutputResolution: OutputResolution1K, UnitCredits: input.Prices[OutputResolution1K]}}, nil
}

func (s *catalogFactReaderStub) SetAvailability(context.Context, string, bool, int64) (AvailabilityInfo, error) {
	return AvailabilityInfo{}, s.err
}

func (s *catalogFactReaderStub) Quote(context.Context, pgx.Tx, string, OutputResolution, int64) (Quote, error) {
	return Quote{}, s.err
}

func TestBillableCatalogPublishesOnlyRegisteredItems(t *testing.T) {
	registry, err := NewBillableItemRegistry([]BillableItemDefinition{{
		SKU: "ai-image:inpaint", Namespace: "ai-image", ProductKind: "workflow-feature",
		DisplayName: "局部重绘", Description: "只修改选区",
	}})
	if err != nil {
		t.Fatal(err)
	}
	store := &catalogFactReaderStub{}
	catalog := NewBillableItemCatalog(registry, store)

	prices, err := catalog.PublishPrices(t.Context(), "AI-IMAGE:INPAINT", map[OutputResolution]int64{OutputResolution1K: 0}, 9)
	if err != nil {
		t.Fatal(err)
	}
	if prices[OutputResolution1K].UnitCredits != 0 || store.publishInput.Definition.SKU != "ai-image:inpaint" || store.publishInput.ActorID != 9 {
		t.Fatalf("publish input = %+v prices = %+v", store.publishInput, prices)
	}
	if _, err := catalog.PublishPrices(t.Context(), "ai-image:unknown", map[OutputResolution]int64{OutputResolution1K: 1}, 9); !errors.Is(err, ErrSKUNotFound) {
		t.Fatalf("unregistered publish error = %v", err)
	}
}

func TestBillableCatalogOnlyDeletesRegistryAbsentItems(t *testing.T) {
	registry, err := NewBillableItemRegistry([]BillableItemDefinition{{
		SKU: "ai-image:inpaint", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "局部重绘",
	}})
	if err != nil {
		t.Fatal(err)
	}
	store := &catalogFactReaderStub{}
	catalog := NewBillableItemCatalog(registry, store)

	if _, err := catalog.DeleteArchived(t.Context(), "ai-image:inpaint", 9); !errors.Is(err, ErrItemNotArchived) {
		t.Fatalf("registered delete error = %v", err)
	}
	result, err := catalog.DeleteArchived(t.Context(), "legacy:removed", 9)
	if err != nil || result.SKU != "legacy:removed" {
		t.Fatalf("archived delete result = %+v error = %v", result, err)
	}
}
