package tests

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
		{name: "canonical slug is valid", raw: "fusion", want: "fusion", ok: true},
		{name: "cutout slug is valid", raw: "cutout", want: "cutout", ok: true},
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
		"upscale": map[string]interface{}{
			"scale": 2.0,
		},
	}

	got := prompts.ApplyFeatureSystemPromptWithConfig(
		"base prompt",
		"user prompt",
		"upscale",
		params,
		prompts.FeaturePromptRuntimeConfig{
			Prompt:                "SYS={{user_prompt}}",
			UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy("upscale"),
		},
	)

	want := "SYS=user prompt"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFeatureSystemPromptDoesNotContainHiddenConstraintSnippet(t *testing.T) {
	got := prompts.ApplyFeatureSystemPromptWithConfig(
		"user prompt",
		"user prompt",
		"outpaint",
		map[string]interface{}{
			"outpaint": map[string]interface{}{
				"target_width": 1024, "target_height": 1280,
				"image_offset_x": 0, "image_offset_y": 256,
			},
		},
		prompts.FeaturePromptRuntimeConfig{
			Prompt:                "SYS=canvas extend only",
			UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy("outpaint"),
		},
	)

	if got != "SYS=canvas extend only" {
		t.Fatalf("got %q, want only configured system prompt", got)
	}
	if strings.Contains(got, "【执行语义】") {
		t.Fatalf("expected no hidden constraint text, got %q", got)
	}
}

func TestResolveRunningStatus(t *testing.T) {
	tests := []struct {
		name      string
		status    int16
		failCount int
		want      string
	}{
		{name: "disabled account is stopped", status: 0, failCount: 0, want: RunningStatusStopped},
		{name: "high fail count is faulted", status: 1, failCount: 5, want: RunningStatusFaulted},
		{name: "active normal account is running", status: 1, failCount: 0, want: RunningStatusRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveRunningStatus(tt.status, tt.failCount)
			if got != tt.want {
				t.Fatalf("status=%d failCount=%d got=%q want=%q", tt.status, tt.failCount, got, tt.want)
			}
		})
	}
}
