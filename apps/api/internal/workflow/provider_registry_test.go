package workflow

import (
	"testing"
)

func TestFormatProviderDisplayName(t *testing.T) {
	cases := map[string]string{
		"fal-ai":           "Fal AI",
		"silicon-flow":     "Silicon Flow",
		"google-ai-studio": "Google AI Studio",
		"my-custom-llm":    "My Custom LLM",
		"workers-ai":       "Workers AI",
		"openai":           "Openai",
		"deepseek":         "Deepseek",
		"openrouter":       "Openrouter",
		"anthropic":        "Anthropic",
		"custom_provider":  "Custom Provider",
		"   ":              "",
		"":                 "",
	}

	for slug, want := range cases {
		got := FormatProviderDisplayName(slug)
		if got != want {
			t.Errorf("FormatProviderDisplayName(%q) = %q, want %q", slug, got, want)
		}
	}
}

func TestResolveProvider(t *testing.T) {
	entry, known := ResolveProvider("fal-ai")
	if !known {
		t.Fatal("expected known=true for fal-ai")
	}
	if entry.Slug != "fal-ai" {
		t.Errorf("entry.Slug = %q, want %q", entry.Slug, "fal-ai")
	}
	if entry.DisplayName != "Fal AI" {
		t.Errorf("entry.DisplayName = %q, want %q", entry.DisplayName, "Fal AI")
	}
	if entry.GatewayNamespace != "fal-ai" {
		t.Errorf("entry.GatewayNamespace = %q, want %q", entry.GatewayNamespace, "fal-ai")
	}

	// Empty slug returns known=false
	emptyEntry, known := ResolveProvider("   ")
	if known {
		t.Fatal("expected known=false for empty string")
	}
	if emptyEntry.Slug != "" {
		t.Errorf("emptyEntry.Slug = %q, want empty", emptyEntry.Slug)
	}
}
