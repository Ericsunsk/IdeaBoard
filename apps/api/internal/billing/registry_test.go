package billing

import (
	"errors"
	"strings"
	"testing"
)

func TestBillableItemRegistryRejectsDuplicateSKUs(t *testing.T) {
	_, err := NewBillableItemRegistry([]BillableItemDefinition{
		{SKU: "ai-image:inpaint", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "局部重绘"},
		{SKU: "AI-IMAGE:INPAINT", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "重复项"},
	})

	if !errors.Is(err, ErrDuplicateBillableItem) {
		t.Fatalf("expected duplicate item error, got %v", err)
	}
}

func TestBillableItemRegistryReturnsDeterministicCopies(t *testing.T) {
	registry, err := NewBillableItemRegistry([]BillableItemDefinition{
		{SKU: "ai-image:cutout", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "抠图", SortOrder: 20, PricingVariants: []OutputResolution{OutputResolution1K}},
		{SKU: "ai-image:inpaint", Namespace: "ai-image", ProductKind: "workflow-feature", DisplayName: "局部重绘", SortOrder: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	first := registry.Items()
	first[0].DisplayName = "mutated"
	first[0].PricingVariants[0] = OutputResolution1K
	second := registry.Items()

	if got := string(second[0].SKU); got != "ai-image:inpaint" {
		t.Fatalf("expected registry order by sort order, got %q", got)
	}
	if got := second[0].DisplayName; got != "局部重绘" {
		t.Fatalf("registry leaked caller mutation: %q", got)
	}
	removeBg, _ := registry.Find("ai-image:cutout")
	if removeBg.PricingVariants[0] != OutputResolution1K {
		t.Fatalf("registry leaked pricing variants mutation: %v", removeBg.PricingVariants)
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
