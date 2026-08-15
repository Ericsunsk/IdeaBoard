package billingregistry

import (
	"slices"
	"testing"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/workflow"
)

func TestWorkflowRegistryUsesFeatureLevelBillingIdentities(t *testing.T) {
	registry, err := billing.NewBillableItemRegistry(WorkflowItems())
	if err != nil {
		t.Fatal(err)
	}
	items := registry.Items()
	if len(items) != len(workflow.AIImageFeatureSlugs()) {
		t.Fatalf("items=%d, workflow features=%d", len(items), len(workflow.AIImageFeatureSlugs()))
	}

	skus := make([]string, 0, len(items))
	for _, item := range items {
		skus = append(skus, string(item.SKU))
		if item.Namespace != "ai-image" || item.ProductKind != "workflow-feature" {
			t.Fatalf("unexpected product identity: %+v", item)
		}
	}
	want := []string{
		"ai-image:prompt-generation",
		"ai-image:local-inpaint",
		"ai-image:sketch-to-image",
		"ai-image:style-variation",
		"ai-image:remove-bg",
		"ai-image:style-fusion",
		"ai-image:hd-upscale",
		"ai-image:image-extend",
	}
	if !slices.Equal(skus, want) {
		t.Fatalf("billing identities=%v, want=%v", skus, want)
	}
	for _, item := range items {
		if string(item.SKU) == "ai-image:remove-bg" {
			if !slices.Equal(item.PricingVariants, []billing.OutputResolution{billing.OutputResolutionDefault}) {
				t.Fatalf("remove-bg pricing variants=%v", item.PricingVariants)
			}
		} else if !slices.Equal(item.PricingVariants, []billing.OutputResolution{billing.OutputResolution1K, billing.OutputResolution2K, billing.OutputResolution4K}) {
			t.Fatalf("default pricing variants for %s=%v", item.SKU, item.PricingVariants)
		}
	}
}
