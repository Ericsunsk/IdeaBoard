package billingregistry

import (
	"strings"
	"testing"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/generation/routing"
	"ideaboard/api/internal/workflow"
)

// TestServiceResolutionCatalogAgreement pins the #856 single-source contract:
// Service slugs come from workflow.AIImageFeatureSlugs, resolution values
// are exactly 1k/2k/4k with no "default" or blank variant, and the workflow,
// billing, and routing normalizers agree on every catalog value. Canvas,
// Admin Prompt, Pricing, and Model Pool consumers must read this catalog
// instead of maintaining local copies.
func TestServiceResolutionCatalogAgreement(t *testing.T) {
	items := WorkflowItems()
	if len(items) != len(workflow.AIImageFeatureSlugs()) {
		t.Fatalf("workflow items=%d, workflow features=%d", len(items), len(workflow.AIImageFeatureSlugs()))
	}
	for _, item := range items {
		service, found := strings.CutPrefix(string(item.SKU), "ai-image:")
		if !found || service == "" {
			t.Fatalf("sku %q is outside the ai-image service namespace", item.SKU)
		}
		slug, ok := workflow.ParseAIImageFeatureSlug(service)
		if !ok {
			t.Fatalf("sku %q names a service outside workflow.AIImageFeatureSlugs", item.SKU)
		}
		if string(item.SKU) != "ai-image:"+slug {
			t.Fatalf("sku %q does not track service %q", item.SKU, slug)
		}
		if len(item.PricingVariants) == 0 {
			t.Fatalf("sku %q lists no pricing variants", item.SKU)
		}
		for _, variant := range item.PricingVariants {
			if variant == "" || string(variant) == "default" {
				t.Fatalf("sku %q carries unowned variant %q", item.SKU, variant)
			}
			if _, err := billing.NormalizeOutputResolution(string(variant)); err != nil {
				t.Fatalf("sku %q variant %q rejected by billing: %v", item.SKU, variant, err)
			}
			if _, err := routing.NormalizeResolution(string(variant)); err != nil {
				t.Fatalf("sku %q variant %q rejected by routing: %v", item.SKU, variant, err)
			}
			if strict, ok := workflow.NormalizeOutputResolution(string(variant)); !ok || strict != string(variant) {
				t.Fatalf("sku %q variant %q rejected by workflow: (%q, %v)", item.SKU, variant, strict, ok)
			}
		}
	}
	if _, ok := workflow.NormalizeOutputResolution("default"); ok {
		t.Fatal(`workflow accepts "default" as a resolution`)
	}
	if _, err := billing.NormalizeOutputResolution("default"); err == nil {
		t.Fatal(`billing accepts "default" as a resolution`)
	}
	if _, err := routing.NormalizeResolution("default"); err == nil {
		t.Fatal(`routing accepts "default" as a resolution`)
	}
	if _, ok := workflow.NormalizeOutputResolution(""); ok {
		t.Fatal("workflow normalizes an omitted resolution inside the shared path")
	}
}
