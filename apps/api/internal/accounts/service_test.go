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

func TestNormalizeAccountModelInputsRejectsInvalidFeatureSlug(t *testing.T) {
	enabled := true
	_, err := normalizeAccountModelInputs([]AccountModelInput{{
		Name:         "gpt-image-1",
		Enabled:      &enabled,
		FeatureSlugs: []string{"style-fusion", "style fusion"},
	}})
	if err == nil {
		t.Fatal("expected invalid feature slug error")
	}
	if !strings.Contains(err.Error(), "feature_slug") {
		t.Fatalf("error=%q, want feature slug validation context", err.Error())
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
