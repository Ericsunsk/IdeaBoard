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
			"blend_ratio": 30,
		},
		func(string) (string, error) {
			return "SYS ratio={{blend_ratio}}", nil
		},
	)

	if got != "SYS ratio=30 user prompt" {
		t.Fatalf("got %q, want template result + base prompt", got)
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

func TestApplyFeatureSystemPromptTemplateWithoutUserPromptDoesNotAppendBasePrompt(t *testing.T) {
	got := ApplyFeatureSystemPrompt(
		"user prompt",
		"",
		"hd-upscale",
		map[string]interface{}{
			"upscale_mode": "enhancement",
		},
		func(string) (string, error) {
			return "SYS mode={{upscale_mode}}", nil
		},
	)

	if got != "SYS mode=enhancement" {
		t.Fatalf("got %q, want template result without base prompt fallback", got)
	}
}
