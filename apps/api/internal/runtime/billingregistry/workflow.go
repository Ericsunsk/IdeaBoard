package billingregistry

import (
	"fmt"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/workflow"
)

const workflowProductKind = "workflow-feature"

type workflowPresentation struct {
	displayName string
	description string
}

var workflowPresentations = map[string]workflowPresentation{
	workflow.FeatureSlugPromptGeneration: {displayName: "Prompt to Image", description: "Generate images from text with optional reference images."},
	workflow.FeatureSlugInpaint:          {displayName: "Inpaint", description: "Repaint only the selected area of an image."},
	workflow.FeatureSlugRender:           {displayName: "Render", description: "Turn a sketch into a full image."},
	workflow.FeatureSlugVariations:       {displayName: "Variations", description: "Create same-style variations of an image."},
	workflow.FeatureSlugCutout:           {displayName: "Cutout", description: "Detect the subject and remove the background."},
	workflow.FeatureSlugStyleFusion:      {displayName: "Image Fusion", description: "Blend content and style from multiple reference images."},
	workflow.FeatureSlugHDUpscale:        {displayName: "Upscale", description: "Increase image resolution and detail."},
	workflow.FeatureSlugOutpaint:         {displayName: "Outpaint", description: "Extend the image beyond its edges."},
}

// WorkflowItems adapts Workflow's canonical executable features to the
// Billing-owned registration contract. Bootstrap composes this slice with
// future product contexts before constructing the immutable global Registry.
func WorkflowItems() []billing.BillableItemDefinition {
	slugs := workflow.AIImageFeatureSlugs()
	definitions := make([]billing.BillableItemDefinition, 0, len(slugs))
	for index, slug := range slugs {
		presentation, ok := workflowPresentations[slug]
		if !ok {
			presentation = workflowPresentation{displayName: slug}
		}
		resolutionValues := workflow.SupportedFeatureOutputResolutions(slug)
		pricingVariants := make([]billing.OutputResolution, 0, len(resolutionValues))
		for _, resolution := range resolutionValues {
			pricingVariants = append(pricingVariants, billing.OutputResolution(resolution))
		}
		definitions = append(definitions, billing.BillableItemDefinition{
			SKU:             billing.SKU(fmt.Sprintf("ai-image:%s", slug)),
			Namespace:       "ai-image",
			ProductKind:     workflowProductKind,
			DisplayName:     presentation.displayName,
			Description:     presentation.description,
			SortOrder:       (index + 1) * 10,
			PricingVariants: pricingVariants,
		})
	}
	return definitions
}

// VisionSearchItem is intentionally a separate SKU from image workflows:
// extraction is a metered AI operation, while the subsequent provider search
// remains free and has its own lifecycle/idempotency contract.
func VisionSearchItem() billing.BillableItemDefinition {
	return billing.BillableItemDefinition{
		SKU:             "ai-search:vision",
		Namespace:       "ai-search",
		ProductKind:     "search-feature",
		DisplayName:     "视觉搜索",
		Description:     "从参考图提取搜索意图。",
		SortOrder:       10,
		PricingVariants: []billing.OutputResolution{billing.OutputResolutionDefault},
	}
}
