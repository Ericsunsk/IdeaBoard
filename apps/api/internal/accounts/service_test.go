package accounts

import (
	"strings"
	"testing"

	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/workflow"
)

func TestTemplateValueDescriptions(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "viewpoint uses display label",
			got:  prompts.DescribeViewpointTemplateValue("top_45"),
			want: "45° 侧视",
		},
		{
			name: "lighting uses readable label",
			got:  prompts.DescribeViewpointLightingTemplateValue("hard_light"),
			want: "质感硬光",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

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
		{name: "unsupported slug is invalid", raw: "line-draft", want: "", ok: false},
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
		"upscale_mode": "enhancement",
	}

	got := prompts.ApplyFeatureSystemPrompt(
		"base prompt",
		"user prompt",
		"hd-upscale",
		params,
		func(slug string) (string, error) {
			return "SYS={{user_prompt}}|{{upscale_mode}}", nil
		},
	)

	want := "SYS=user prompt|enhancement"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFeatureSystemPromptDoesNotContainHiddenConstraintSnippet(t *testing.T) {
	got := prompts.ApplyFeatureSystemPrompt(
		"user prompt",
		"user prompt",
		"image-extend",
		map[string]interface{}{
			"extend_top": 30,
		},
		func(slug string) (string, error) {
			return "SYS=canvas extend only", nil
		},
	)

	if got != "SYS=canvas extend only user prompt" {
		t.Fatalf("got %q, want only configured system prompt + base prompt", got)
	}
	if strings.Contains(got, "【执行语义】") {
		t.Fatalf("expected no hidden constraint text, got %q", got)
	}
}
