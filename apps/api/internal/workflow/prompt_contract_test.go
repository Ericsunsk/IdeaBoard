package workflow

import (
	"reflect"
	"testing"

	"ideaboard/api/internal/tasks"
)

func TestNormalizeStructuredPromptContractParamsMovesFlatVariationParams(t *testing.T) {
	params := map[string]interface{}{
		"variation": map[string]interface{}{
			"strength": 35,
		},
	}

	if err := NormalizeStructuredPromptContractParams(params, FeatureSlugStyleVariation); err != nil {
		t.Fatalf("NormalizeStructuredPromptContractParams returned error: %v", err)
	}

	contract, ok := params[ParamKeyVariation].(map[string]interface{})
	if !ok {
		t.Fatalf("variation = %#v, want object", params[ParamKeyVariation])
	}
	if contract["strength"] != float64(35) {
		t.Fatalf("strength = %#v, want 35", contract["strength"])
	}
}

func TestNormalizeStructuredPromptContractParamsNormalizesExtendContract(t *testing.T) {
	params := map[string]interface{}{
		"extend": map[string]interface{}{
			"top_percent":    0,
			"right_percent":  25,
			"bottom_percent": 0,
			"left_percent":   10,
		},
	}

	if err := NormalizeStructuredPromptContractParams(params, FeatureSlugImageExtend); err != nil {
		t.Fatalf("NormalizeStructuredPromptContractParams returned error: %v", err)
	}

	extend, ok := params[ParamKeyExtend].(map[string]interface{})
	if !ok {
		t.Fatalf("extend = %#v, want object", params[ParamKeyExtend])
	}
	if extend["right_percent"] != float64(25) {
		t.Fatalf("right_percent = %#v, want 25", extend["right_percent"])
	}
}

func TestFlattenProviderTaskParamsDerivesProviderFieldsFromNestedContract(t *testing.T) {
	params := map[string]interface{}{
		"feature": FeatureSlugHDUpscale,
		ParamKeyHD: map[string]interface{}{
			"clarity_strength": 75,
		},
	}

	flat := FlattenProviderTaskParams(params)

	if _, exists := flat["clarity_strength"]; exists {
		t.Fatalf("did not expect HD clarity strength to be flattened for providers: %#v", flat)
	}
	if _, exists := params["output_quality"]; exists {
		t.Fatalf("flatten should not mutate original params: %#v", params)
	}
}

func TestFlattenWorkflowTaskParamsDerivesProviderMapFromCanonicalContract(t *testing.T) {
	useMask := true
	params := &tasks.WorkflowTaskParams{
		Feature: FeatureSlugStyleFusion,
		Model:   "gpt-image-1",
		Media: &tasks.MediaInputs{
			Inputs: []tasks.MediaRef{
				{URL: "https://img.example/a.png", Role: "input", Weight: 0.7},
				{URL: "https://img.example/b.png", Role: "input", Weight: 0.3},
			},
			References: []tasks.MediaRef{{URL: "https://img.example/ref.png", Role: "reference"}},
			Mask:       &tasks.MediaRef{URL: "https://img.example/mask.png", Role: "mask"},
			UseMask:    &useMask,
		},
		Controls: &tasks.GenerationControls{
			CustomWidth:  640,
			CustomHeight: 480,
			Ratio:        "4:3",
			OutputCount:  2,
		},
		Fusion: &tasks.StyleFusionParams{
			Mode:         "weighted_reference",
			InputWeights: []float64{0.7, 0.3},
		},
		Extra: map[string]interface{}{
			"provider_hint":      "keep",
			"variation_strength": 99,
		},
	}

	flat := FlattenWorkflowTaskParams(params)

	if got := flat["feature"]; got != FeatureSlugStyleFusion {
		t.Fatalf("feature = %#v, want style-fusion", got)
	}
	if !reflect.DeepEqual(flat["image_urls"], []string{"https://img.example/a.png", "https://img.example/b.png"}) {
		t.Fatalf("image_urls = %#v", flat["image_urls"])
	}
	if !reflect.DeepEqual(flat["reference_image_urls"], []string{"https://img.example/ref.png"}) {
		t.Fatalf("reference_image_urls = %#v", flat["reference_image_urls"])
	}
	if got := flat["mask_image_url"]; got != "https://img.example/mask.png" {
		t.Fatalf("mask_image_url = %#v", got)
	}
	if got := flat["custom_width"]; got != float64(640) {
		t.Fatalf("custom_width = %#v", got)
	}
	if got := flat["fusion_input_weights"]; !reflect.DeepEqual(got, []float64{0.7, 0.3}) {
		t.Fatalf("fusion_input_weights = %#v", got)
	}
	if got := flat["provider_hint"]; got != "keep" {
		t.Fatalf("provider_hint = %#v, want keep", got)
	}
	if _, exists := flat["variation_strength"]; exists {
		t.Fatalf("reserved old flat key leaked from Extra: %#v", flat)
	}
}

func TestStyleFusionPromptContractUsesPublishedWeightsVariable(t *testing.T) {
	params := map[string]interface{}{
		ParamKeyFusion: map[string]interface{}{
			"mode":          "weighted_reference",
			"input_weights": []interface{}{0.6, 0.4},
		},
	}

	contract, ok := ReadStyleFusionPromptContract(params)
	if !ok {
		t.Fatal("ReadStyleFusionPromptContract returned false")
	}
	vars := contract.PromptVariables()
	if _, exists := vars["{{fusion_mode}}"]; exists {
		t.Fatalf("did not expect unpublished fusion_mode variable: %#v", vars)
	}
	weights, ok := vars["{{fusion_input_weights}}"].([]float64)
	if !ok {
		t.Fatalf("fusion_input_weights = %#v, want []float64", vars["{{fusion_input_weights}}"])
	}
	if len(weights) != 2 || weights[0] != 0.6 || weights[1] != 0.4 {
		t.Fatalf("weights = %#v, want [0.6 0.4]", weights)
	}
}
