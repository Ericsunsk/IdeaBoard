package billingregistry

import (
	"testing"

	"ideaboard/api/internal/billing"
)

func TestLibraryMaterializeRegistryIdentity(t *testing.T) {
	registry, err := billing.NewBillableItemRegistry([]billing.BillableItemDefinition{LibraryMaterializeItem()})
	if err != nil {
		t.Fatalf("NewBillableItemRegistry(library:materialize): %v", err)
	}
	definition, exists := registry.Find(LibraryMaterializeSKU)
	if !exists {
		t.Fatalf("Find(%q) = false, want true", LibraryMaterializeSKU)
	}
	if definition.SKU != billing.SKU(LibraryMaterializeSKU) {
		t.Fatalf("SKU = %q, want %q", definition.SKU, LibraryMaterializeSKU)
	}
	if definition.Namespace != "library" || definition.ProductKind != "library-feature" {
		t.Fatalf("unexpected product identity: %+v", definition)
	}
	if len(definition.PricingVariants) != 1 || definition.PricingVariants[0] != billing.OutputResolutionDefault {
		t.Fatalf("pricing variants = %v, want [default]", definition.PricingVariants)
	}
	if err := billing.ValidatePricingVariant(definition, billing.OutputResolutionDefault); err != nil {
		t.Fatalf("ValidatePricingVariant(default): %v", err)
	}
}

func TestLibraryMaterializeComposesWithWorkflowAndVision(t *testing.T) {
	items := WorkflowItems()
	items = append(items, VisionSearchItem(), LibraryMaterializeItem())
	registry, err := billing.NewBillableItemRegistry(items)
	if err != nil {
		t.Fatalf("NewBillableItemRegistry(composed): %v", err)
	}
	if !registry.Contains(LibraryMaterializeSKU) {
		t.Fatalf("composed registry misses %q", LibraryMaterializeSKU)
	}
	if !registry.Contains("ai-search:vision") || !registry.Contains("ai-image:generation") {
		t.Fatalf("composed registry lost workflow/vision identities")
	}
}
