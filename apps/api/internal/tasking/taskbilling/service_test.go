package taskbilling

import (
	"errors"
	"testing"

	"ideaboard/api/internal/workflow"
)

func TestAIImageWorkflowSKUContract(t *testing.T) {
	expected := map[string]string{
		workflow.FeatureSlugSketchToImage:  "ai-image:sketch-to-image.77e354c1741f5880a61d0383ea0fa39a419fb2353ef77092d2e13491ef0d5c18",
		workflow.FeatureSlugStyleVariation: "ai-image:style-variation.983fc1da69276686d51583c2bc7091b0f977c47da4d396daaac9e00321cc9898",
		workflow.FeatureSlugPartialEdit:    "ai-image:partial-edit.123ab0fba0b8a1df883012d03070a94885fd038434c325a802c822c819cdf495",
		workflow.FeatureSlugRemoveBg:       "ai-image:remove-bg.33d88374630408b27e09a1f4f801f2c107d50958a5b00544589021baf4ad87a6",
		workflow.FeatureSlugStyleFusion:    "ai-image:style-fusion.7b3785997201ac5b47f3d40e1f29b83087f5e072aed9060643f6c4866838eadd",
		workflow.FeatureSlugPatternGen:     "ai-image:pattern-gen.348fdd008de3164eb4a4992efcac81356355e27910dcaaa2472cb048241bd9b6",
		workflow.FeatureSlugHDUpscale:      "ai-image:hd-upscale.c39ccc2f77cadc35a8fd9d79e2cc2912f34bd21b0898ae3c16e2c0f30f8f874b",
		workflow.FeatureSlugImageExtend:    "ai-image:image-extend.f7cebe283e0ac68f9829326de28c54bddb2f9b885424530f5b59eae3d564e165",
	}

	for _, feature := range workflow.AIImageFeatureSlugs() {
		t.Run(feature, func(t *testing.T) {
			request, err := AIImageRequest(feature, " GPT-Image-1 ")
			if err != nil {
				t.Fatal(err)
			}
			sku, err := ResolveSKU(request)
			if err != nil {
				t.Fatal(err)
			}
			if string(sku) != expected[feature] {
				t.Fatalf("sku=%q, want %q", sku, expected[feature])
			}
			if !RequiresBilling(feature) {
				t.Fatalf("feature %q must require billing facts", feature)
			}
		})
	}
}

func TestAIImageWorkflowRequestRejectsNonWorkflowProducts(t *testing.T) {
	for _, feature := range []string{"", "crop", "color-adjust", "mask-draw", "unknown"} {
		t.Run(feature, func(t *testing.T) {
			_, err := AIImageRequest(feature, "gpt-image-1")
			if !errors.Is(err, ErrUnsupportedProduct) {
				t.Fatalf("err=%v, want ErrUnsupportedProduct", err)
			}
			if RequiresBilling(feature) {
				t.Fatalf("local/non-workflow feature %q must not require billing", feature)
			}
		})
	}
}

func TestAIImageWorkflowRequestRequiresCanonicalModel(t *testing.T) {
	_, err := AIImageRequest(workflow.FeatureSlugSketchToImage, "  ")
	if !errors.Is(err, ErrModelRequired) {
		t.Fatalf("err=%v, want ErrModelRequired", err)
	}
}
