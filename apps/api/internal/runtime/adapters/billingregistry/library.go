package billingregistry

import (
	"ideaboard/api/internal/billing"
)

// LibraryMaterializeSKU is the metered identity quoted for workspace
// library materialization (lasset_ -> managed media asset link jobs,
// both the manual POST /materializations surface and the task-completion
// auto-materialize trigger). It shares the billing-owned registration
// contract with the workflow and vision-search items above: Bootstrap
// composes this slice entry into the immutable global Registry before
// constructing it, so the BillableItemCatalog gates quotes on this
// definition and unpriced SKUs fail closed as pricing-unavailable.
const LibraryMaterializeSKU = "library:materialize"

// LibraryMaterializeItem adapts the Library product to the
// Billing-owned registration contract. Single default pricing variant:
// one materialization job costs one unit price regardless of output
// resolution (resolution tiers are a workflow-image concern).
func LibraryMaterializeItem() billing.BillableItemDefinition {
	return billing.BillableItemDefinition{
		SKU:             billing.SKU(LibraryMaterializeSKU),
		Namespace:       "library",
		ProductKind:     "library-feature",
		DisplayName:     "Library Materialize",
		Description:     "Save a workspace library reference as a managed media asset.",
		SortOrder:       15,
		PricingVariants: []billing.OutputResolution{billing.OutputResolutionDefault},
	}
}
