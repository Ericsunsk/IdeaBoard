package billingregistry

import (
	"slices"
	"strings"
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
		"ai-image:inpaint",
		"ai-image:render",
		"ai-image:variations",
		"ai-image:cutout",
		"ai-image:style-fusion",
		"ai-image:hd-upscale",
		"ai-image:outpaint",
	}
	if !slices.Equal(skus, want) {
		t.Fatalf("billing identities=%v, want=%v", skus, want)
	}
	for _, item := range items {
		slug := strings.TrimPrefix(string(item.SKU), "ai-image:")
		resolutionValues := workflow.SupportedFeatureOutputResolutions(slug)
		want := make([]billing.OutputResolution, 0, len(resolutionValues))
		for _, resolution := range resolutionValues {
			want = append(want, billing.OutputResolution(resolution))
		}
		if !slices.Equal(item.PricingVariants, want) {
			t.Fatalf("pricing variants for %s=%v, want %v", item.SKU, item.PricingVariants, want)
		}
		if (slug == workflow.FeatureSlugInpaint || slug == workflow.FeatureSlugOutpaint) &&
			!slices.Equal(item.PricingVariants, []billing.OutputResolution{billing.OutputResolutionDefault}) {
			t.Fatalf("fixed-price variants for %s=%v", item.SKU, item.PricingVariants)
		}
	}
}
