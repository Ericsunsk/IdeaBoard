package accounts

import (
	"strings"
	"testing"

	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/workflow"
)

func TestParseWorkflowFeatureSlug(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "canonical slug is valid", raw: "style-fusion", want: "style-fusion", ok: true},
		{name: "remove-bg slug is valid", raw: "remove-bg", want: "remove-bg", ok: true},
		{name: "underscore slug is invalid", raw: "style_fusion", want: "", ok: false},
		{name: "unsupported slug is invalid", raw: "unknown-workflow", want: "", ok: false},
		{name: "invalid chars are rejected", raw: "style fusion", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := workflow.ParseWorkflowFeatureSlug(tt.raw)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("raw=%q got=(%q,%v), want=(%q,%v)", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestApplyFeatureSystemPromptUsesCanonicalKeys(t *testing.T) {
	params := map[string]interface{}{
		"hd": map[string]interface{}{
			"clarity_strength": 75,
		},
	}

	got := prompts.ApplyFeatureSystemPromptWithConfig(
		"base prompt",
		"user prompt",
		"hd-upscale",
		params,
		prompts.FeaturePromptRuntimeConfig{
			Prompt:                "SYS={{user_prompt}}|{{clarity_strength}}",
			UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy("hd-upscale"),
		},
	)

	want := "SYS=user prompt|75"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFeatureSystemPromptDoesNotContainHiddenConstraintSnippet(t *testing.T) {
	got := prompts.ApplyFeatureSystemPromptWithConfig(
		"user prompt",
		"user prompt",
		"image-extend",
		map[string]interface{}{
			"extend": map[string]interface{}{
				"top_percent":    30,
				"right_percent":  0,
				"bottom_percent": 0,
				"left_percent":   0,
			},
		},
		prompts.FeaturePromptRuntimeConfig{
			Prompt:                "SYS=canvas extend only",
			UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy("image-extend"),
		},
	)

	if got != "SYS=canvas extend only" {
		t.Fatalf("got %q, want only configured system prompt", got)
	}
	if strings.Contains(got, "【执行语义】") {
		t.Fatalf("expected no hidden constraint text, got %q", got)
	}
}
