package workflow

import (
	"reflect"
	"testing"

	"ideaboard/api/internal/tasking/tasks"
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

func TestBuildProviderPromptParamsDerivesProviderFieldsFromNestedContract(t *testing.T) {
	params := map[string]interface{}{
		"feature": FeatureSlugHDUpscale,
		ParamKeyHD: map[string]interface{}{
			"clarity_strength": 75,
		},
	}

	providerParams := BuildProviderPromptParams(params)

	if _, exists := providerParams["clarity_strength"]; exists {
		t.Fatalf("did not expect HD clarity strength in provider params: %#v", providerParams)
	}
	if _, exists := params["output_quality"]; exists {
		t.Fatalf("provider param build should not mutate original params: %#v", params)
	}
}

func TestValidatePatternGenSubmitContract(t *testing.T) {
	if PatternGenIOContract.Feature != FeatureSlugPatternGen {
		t.Fatalf("pattern-gen contract feature=%q", PatternGenIOContract.Feature)
	}
	if PatternGenIOContract.Output.Kind != "image-results" || PatternGenIOContract.Output.MinCount != 1 {
		t.Fatalf("pattern-gen output contract=%+v", PatternGenIOContract.Output)
	}

	if err := ValidatePatternGenSubmitContract("repeatable textile pattern", map[string]interface{}{
		"feature":       FeatureSlugPatternGen,
		"ratio":         "1:1",
		"provider_hint": "keep",
		"output_count":  2,
	}); err != nil {
		t.Fatalf("ValidatePatternGenSubmitContract returned error: %v", err)
	}

	err := ValidatePatternGenSubmitContract("repeatable textile pattern", map[string]interface{}{
		"feature":    FeatureSlugPatternGen,
		"image_urls": []string{"https://img.example/source.png"},
	})
	if err == nil {
		t.Fatalf("expected pattern-gen image input validation error")
	}
}

func TestValidatePatternGenTaskContractAllowsProviderHintsAndControls(t *testing.T) {
	err := ValidatePatternGenTaskContract("repeatable textile pattern", &tasks.WorkflowTaskParams{
		SchemaVersion: tasks.CurrentParamsSchemaVersion,
		Feature:       FeatureSlugPatternGen,
		Controls: &tasks.GenerationControls{
			Ratio:       "16:9",
			OutputCount: 2,
		},
		Extra: map[string]interface{}{
			"provider_hint": "keep",
		},
	})
	if err != nil {
		t.Fatalf("ValidatePatternGenTaskContract returned error: %v", err)
	}
}

func TestBuildProviderTaskParamsDerivesProviderMapFromCanonicalContract(t *testing.T) {
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

	providerParams := BuildProviderTaskParams(params)

	if got := providerParams["feature"]; got != FeatureSlugStyleFusion {
		t.Fatalf("feature = %#v, want style-fusion", got)
	}
	if !reflect.DeepEqual(providerParams["image_urls"], []string{"https://img.example/a.png", "https://img.example/b.png"}) {
		t.Fatalf("image_urls = %#v", providerParams["image_urls"])
	}
	if !reflect.DeepEqual(providerParams["reference_image_urls"], []string{"https://img.example/ref.png"}) {
		t.Fatalf("reference_image_urls = %#v", providerParams["reference_image_urls"])
	}
	if got := providerParams["mask_image_url"]; got != "https://img.example/mask.png" {
		t.Fatalf("mask_image_url = %#v", got)
	}
	if got := providerParams["custom_width"]; got != float64(640) {
		t.Fatalf("custom_width = %#v", got)
	}
	if got := providerParams["fusion_input_weights"]; !reflect.DeepEqual(got, []float64{0.7, 0.3}) {
		t.Fatalf("fusion_input_weights = %#v", got)
	}
	if got := providerParams["provider_hint"]; got != "keep" {
		t.Fatalf("provider_hint = %#v, want keep", got)
	}
	if _, exists := providerParams["variation_strength"]; exists {
		t.Fatalf("provider-only key from Extra leaked into provider params: %#v", providerParams)
	}
}

func TestBuildProviderTaskParamsEncodesCanonicalFeatureContracts(t *testing.T) {
	tests := []struct {
		name       string
		params     *tasks.WorkflowTaskParams
		want       map[string]interface{}
		wantAbsent []string
	}{
		{
			name: "style variation",
			params: &tasks.WorkflowTaskParams{
				Feature:   FeatureSlugStyleVariation,
				Variation: &tasks.StyleVariationParams{Strength: 42},
				Extra: map[string]interface{}{
					"provider_hint":      "keep",
					"variation_strength": 99,
					"output_quality":     "high",
					"upscale_factor":     4,
				},
			},
			want: map[string]interface{}{
				"feature":            FeatureSlugStyleVariation,
				"variation":          map[string]interface{}{"strength": float64(42)},
				"variation_strength": float64(42),
				"provider_hint":      "keep",
			},
			wantAbsent: []string{"output_quality", "upscale_factor"},
		},
		{
			name: "image extend",
			params: &tasks.WorkflowTaskParams{
				Feature: FeatureSlugImageExtend,
				Extend: &tasks.ImageExtendParams{
					TopPercent:    1,
					RightPercent:  2,
					BottomPercent: 3,
					LeftPercent:   4,
				},
			},
			want: map[string]interface{}{
				"feature":       FeatureSlugImageExtend,
				"extend":        map[string]interface{}{"top_percent": float64(1), "right_percent": float64(2), "bottom_percent": float64(3), "left_percent": float64(4)},
				"extend_top":    float64(1),
				"extend_right":  float64(2),
				"extend_bottom": float64(3),
				"extend_left":   float64(4),
			},
		},
		{
			name: "hd upscale",
			params: &tasks.WorkflowTaskParams{
				Feature: FeatureSlugHDUpscale,
				HD:      &tasks.HDUpscaleParams{ClarityStrength: 75},
			},
			want: map[string]interface{}{
				"feature": FeatureSlugHDUpscale,
				"hd":      map[string]interface{}{"clarity_strength": float64(75)},
			},
			wantAbsent: []string{"clarity_strength", "output_quality"},
		},
		{
			name: "style fusion",
			params: &tasks.WorkflowTaskParams{
				Feature: FeatureSlugStyleFusion,
				Fusion: &tasks.StyleFusionParams{
					Mode:         "weighted_reference",
					InputWeights: []float64{0.8, 0.2},
				},
			},
			want: map[string]interface{}{
				"feature":              FeatureSlugStyleFusion,
				"fusion":               map[string]interface{}{"mode": "weighted_reference", "input_weights": []float64{0.8, 0.2}},
				"fusion_input_weights": []float64{0.8, 0.2},
			},
		},
		{
			name: "prompt workflow",
			params: &tasks.WorkflowTaskParams{
				Feature: FeatureSlugPatternGen,
				Extra:   map[string]interface{}{"provider_hint": "keep"},
			},
			want: map[string]interface{}{
				"feature":       FeatureSlugPatternGen,
				"provider_hint": "keep",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerParams := BuildProviderTaskParams(tt.params)
			for key, want := range tt.want {
				if got := providerParams[key]; !reflect.DeepEqual(got, want) {
					t.Fatalf("%s = %#v, want %#v; provider params=%#v", key, got, want, providerParams)
				}
			}
			for _, key := range tt.wantAbsent {
				if _, exists := providerParams[key]; exists {
					t.Fatalf("did not expect %s in provider params: %#v", key, providerParams)
				}
			}
		})
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
