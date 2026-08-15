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
	workflow.FeatureSlugPromptGeneration: {displayName: "Prompt 生图", description: "使用文字与可选参考图生成图片。"},
	workflow.FeatureSlugLocalInpaint:     {displayName: "局部重绘", description: "仅重绘图片选定区域。"},
	workflow.FeatureSlugSketchToImage:    {displayName: "草图成图", description: "将草图转换为完整图片。"},
	workflow.FeatureSlugStyleVariation:   {displayName: "风格延展", description: "基于图片生成同风格变体。"},
	workflow.FeatureSlugRemoveBg:         {displayName: "抠图", description: "识别主体并移除图片背景。"},
	workflow.FeatureSlugStyleFusion:      {displayName: "多图融合", description: "融合多张参考图的内容与风格。"},
	workflow.FeatureSlugHDUpscale:        {displayName: "HD", description: "提升图片分辨率与细节。"},
	workflow.FeatureSlugImageExtend:      {displayName: "扩图", description: "向指定方向扩展图片画面。"},
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
		definitions = append(definitions, billing.BillableItemDefinition{
			SKU:         billing.SKU(fmt.Sprintf("ai-image:%s", slug)),
			Namespace:   "ai-image",
			ProductKind: workflowProductKind,
			DisplayName: presentation.displayName,
			Description: presentation.description,
			SortOrder:   (index + 1) * 10,
		})
		if slug == workflow.FeatureSlugRemoveBg || slug == workflow.FeatureSlugHDUpscale {
			definitions[len(definitions)-1].PricingVariants = []billing.OutputResolution{billing.OutputResolutionDefault}
		}
	}
	return definitions
}
