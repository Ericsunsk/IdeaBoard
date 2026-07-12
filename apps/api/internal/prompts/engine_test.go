package prompts

import (
	"strings"
	"testing"
)

func ApplyFeatureSystemPrompt(basePrompt, userPrompt, featureSlug string, params map[string]interface{}, loadPrompt func(string) (string, error)) string {
	if featureSlug == "" || loadPrompt == nil {
		return basePrompt
	}
	prompt, err := loadPrompt(featureSlug)
	if err != nil || prompt == "" {
		return basePrompt
	}
	return ApplyFeatureSystemPromptWithConfig(basePrompt, userPrompt, featureSlug, params, FeaturePromptRuntimeConfig{
		Prompt:                prompt,
		UserPromptMergePolicy: DefaultFeaturePromptMergePolicy(featureSlug),
	})
}

func TestApplyFeatureSystemPromptStyleVariationUsesConfiguredPromptOnly(t *testing.T) {
	got := ApplyFeatureSystemPrompt(
		"[style-variation] 延展强度：40%，前端默认文案",
		"[style-variation] 延展强度：40%，前端默认文案",
		"style-variation",
		map[string]interface{}{
			"variation": map[string]interface{}{
				"strength": 40,
			},
		},
		func(string) (string, error) {
			return "BACKEND-ONLY variation_strength={{variation_strength}}", nil
		},
	)

	if got != "BACKEND-ONLY variation_strength=40" {
		t.Fatalf("got %q, want backend-only configured prompt", got)
	}
}

func TestApplyFeatureSystemPromptNonStyleVariationStillAppendsBasePrompt(t *testing.T) {
	got := ApplyFeatureSystemPrompt(
		"user prompt",
		"user prompt",
		"pattern-gen",
		nil,
		func(string) (string, error) {
			return "SYS", nil
		},
	)

	if got != "SYS user prompt" {
		t.Fatalf("got %q, want appended base prompt", got)
	}
}

func TestApplyFeatureSystemPromptDoesNotAppendHiddenConstraints(t *testing.T) {
	features := []string{
		"style-fusion",
		"image-extend",
		"hd-upscale",
	}

	for _, feature := range features {
		t.Run(feature, func(t *testing.T) {
			got := ApplyFeatureSystemPrompt(
				"user prompt",
				"user prompt",
				feature,
				map[string]interface{}{
					"mask_image_url": "https://mask.example/a.png",
					"use_mask":       true,
				},
				func(string) (string, error) {
					return "SYS-CONFIG", nil
				},
			)

			if got != "SYS-CONFIG user prompt" {
				t.Fatalf("got %q, want only configured system prompt + base prompt", got)
			}
			if strings.Contains(got, "【执行语义】") {
				t.Fatalf("expected no hidden constraint injection, got %q", got)
			}
		})
	}
}

func TestApplyFeatureSystemPromptTemplateWithoutUserPromptStillKeepsBasePrompt(t *testing.T) {
	got := ApplyFeatureSystemPrompt(
		"user prompt",
		"user prompt",
		"style-fusion",
		map[string]interface{}{
			"fusion": map[string]interface{}{
				"input_weights": []interface{}{0.6, 0.4},
			},
		},
		func(string) (string, error) {
			return "SYS weights={{fusion_input_weights}}", nil
		},
	)

	if got != "SYS weights=Image 1: 60%, Image 2: 40% user prompt" {
		t.Fatalf("got %q, want template result + base prompt", got)
	}
}

func TestApplyFeatureSystemPromptInvalidFusionWeightsLeavePlaceholderUnresolved(t *testing.T) {
	got := ApplyFeatureSystemPrompt(
		"",
		"",
		"style-fusion",
		map[string]interface{}{
			"fusion": map[string]interface{}{
				"input_weights": []interface{}{0.5, "invalid", 0.25},
			},
		},
		func(string) (string, error) {
			return "SYS {{fusion_input_weights}}", nil
		},
	)

	if got != "SYS {{fusion_input_weights}}" {
		t.Fatalf("got %q, want unresolved fusion weights placeholder", got)
	}
}

func TestApplyFeatureSystemPromptDoesNotExposeAssetOrDeprecatedVariables(t *testing.T) {
	got := ApplyFeatureSystemPrompt(
		"",
		"",
		"hd-upscale",
		map[string]interface{}{
			"category":           "dress",
			"pattern_mode":       "tile",
			"original_image_url": "https://img.example/original.png",
			"mask_image_url":     "https://img.example/mask.png",
			"use_mask":           true,
			"hd": map[string]interface{}{
				"clarity_strength": 75,
			},
		},
		func(string) (string, error) {
			return "SYS {{category}} {{pattern_mode}} {{original_image_url}} {{mask_image_url}} {{use_mask}} {{clarity_strength}}", nil
		},
	)

	want := "SYS {{category}} {{pattern_mode}} {{original_image_url}} {{mask_image_url}} {{use_mask}} 75"
	if got != want {
		t.Fatalf("got %q, want unsupported placeholders to remain unchanged", got)
	}
	if strings.Contains(got, "dress") ||
		strings.Contains(got, "tile") ||
		strings.Contains(got, "https://img.example/original.png") ||
		strings.Contains(got, "https://img.example/mask.png") ||
		strings.Contains(got, "true") {
		t.Fatalf("got %q, want no runtime media or provider-only values exposed", got)
	}
}

func TestApplyFeatureSystemPromptTemplateWithUserPromptDoesNotAppendBasePromptTwice(t *testing.T) {
	got := ApplyFeatureSystemPrompt(
		"user prompt",
		"user prompt",
		"style-fusion",
		nil,
		func(string) (string, error) {
			return "SYS {{user_prompt}}", nil
		},
	)

	if got != "SYS user prompt" {
		t.Fatalf("got %q, want user prompt injected once", got)
	}
}

func TestApplyFeatureSystemPromptDoesNotReplacePlaceholdersInsideUserPrompt(t *testing.T) {
	got := ApplyFeatureSystemPrompt(
		"",
		"literal {{variation_strength}}",
		"style-variation",
		map[string]interface{}{
			"variation": map[string]interface{}{
				"strength": 40,
			},
		},
		func(string) (string, error) {
			return "SYS variation_strength={{variation_strength}} prompt={{user_prompt}}", nil
		},
	)

	if got != "SYS variation_strength=40 prompt=literal {{variation_strength}}" {
		t.Fatalf("got %q, want user prompt placeholder content left literal", got)
	}
}

func TestApplyFeatureSystemPromptHDUpscaleDoesNotExposeUseMask(t *testing.T) {
	got := ApplyFeatureSystemPrompt(
		"user prompt",
		"",
		"hd-upscale",
		map[string]interface{}{
			"image_urls": []interface{}{"https://img.example/original.png"},
			"hd": map[string]interface{}{
				"clarity_strength": 75,
			},
			"use_mask": true,
		},
		func(string) (string, error) {
			return "SYS use_mask={{use_mask}} clarity={{clarity_strength}}", nil
		},
	)

	if got != "SYS use_mask={{use_mask}} clarity=75" {
		t.Fatalf("got %q, want unsupported use_mask placeholder to remain unchanged", got)
	}
}
