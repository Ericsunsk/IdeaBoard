package prompts

import (
	"strings"
	"testing"
)

func TestApplyFeatureSystemPromptStyleVariationUsesConfiguredPromptOnly(t *testing.T) {
	got := ApplyFeatureSystemPrompt(
		"[style-variation] 延展强度：40%，前端默认文案",
		"[style-variation] 延展强度：40%，前端默认文案",
		"style-variation",
		map[string]interface{}{
			"variation_strength": 40,
		},
		func(string) (string, error) {
			return "BACKEND-ONLY strength={{strength}}", nil
		},
	)

	if got != "BACKEND-ONLY strength=40" {
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
		"multi-angle",
		"image-extend",
		"hd-upscale",
		"fabric-on-body",
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
					"model_url":      "https://img.example/model.png",
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
