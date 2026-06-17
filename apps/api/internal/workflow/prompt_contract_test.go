package workflow

import "testing"

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
