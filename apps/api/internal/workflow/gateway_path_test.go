package workflow

import (
	"testing"
)

func TestGatewayProviderPath(t *testing.T) {
	cases := map[string]string{
		"openrouter":   "openrouter",
		"fal":          "fal",
		"fal-ai":       "fal",
		"fal_ai":       "fal",
		"openai":       "openai",
		"Anthropic":    "anthropic",
		"gemini":       "google-ai-studio",
		"azureopenai":  "azure-openai",
		"perplexity":   "perplexity-ai",
		"bedrock":      "aws-bedrock",
		"vertex_ai":    "google-vertex-ai",
		"deepseek":     "deepseek",
		"google":       "custom-google",
		"myllm":        "custom-myllm",
		"custom-myllm": "custom-myllm",
		"   ":          "",
	}
	for input, want := range cases {
		if got := GatewayProviderPath(input); got != want {
			t.Fatalf("GatewayProviderPath(%q) = %q, want %q", input, got, want)
		}
	}
	if IsFirstPartyGatewayProvider("myllm") {
		t.Fatal("IsFirstPartyGatewayProvider(myllm) = true, want false")
	}
	if !IsFirstPartyGatewayProvider("OpenAI") {
		t.Fatal("IsFirstPartyGatewayProvider(OpenAI) = false, want true")
	}
}

func TestIsChatModel(t *testing.T) {
	trueCases := [][3]string{
		{"openai", "custom-model", `{"type":"chat"}`},
		{"custom", "llm", `{"protocol":"chat"}`},
		{"custom", "llm", `{"modalities":["text","image"]}`},
		{"openrouter", "anything", ""},
		{"anthropic", "claude-3-haiku", ""},
		{"openai", "gpt-4o", ""},
		{"google", "gemini-1.5-pro", ""},
		{"custom-provider", "qwen-vl-max", ""},
	}
	for _, tc := range trueCases {
		if !IsChatModel(tc[0], tc[1], tc[2]) {
			t.Fatalf("IsChatModel(%q, %q, %q) = false, want true", tc[0], tc[1], tc[2])
		}
	}
	falseCases := [][3]string{
		{"openrouter", "google/gemini", `{"type":"generation"}`},
		{"openai", "dall-e-3", ""},
		{"fal", "fal-ai/flux-pro", ""},
	}
	for _, tc := range falseCases {
		if IsChatModel(tc[0], tc[1], tc[2]) {
			t.Fatalf("IsChatModel(%q, %q, %q) = true, want false", tc[0], tc[1], tc[2])
		}
	}
}

func TestResolveRoute(t *testing.T) {
	cases := []struct {
		provider string
		model    string
		override string
		params   string
		wantPath string
		wantSrc  GatewayPathSource
		wantChat bool
		wantNS   string
	}{
		{"openrouter", "model", "", "", "api/v1/chat/completions", GatewayPathSourceOpenRouter, true, "openrouter"},
		{"anthropic", "claude-3-5-sonnet", "", "", "v1/messages", GatewayPathSourceAnthropic, true, "anthropic"},
		{"openai", "gpt-4o", "", "", "v1/chat/completions", GatewayPathSourceChat, true, "openai"},
		{"openai", "dall-e-3", "", "", "v1/images/generations", GatewayPathSourceGenerations, false, "openai"},
		// customParams 参与 chat 判定：执行与预览同一答案（旧分裂点）。
		{"openai", "dall-e-3", "", `{"type":"chat"}`, "v1/chat/completions", GatewayPathSourceChat, true, "openai"},
		{"custom-llm", "any-model", "", `{"protocol":"chat"}`, "v1/chat/completions", GatewayPathSourceChat, true, "custom-llm"},
		{"fal", "flux-dev", "", "", "flux-dev", GatewayPathSourceFal, false, "fal"},
		{"fal_ai", "fal-ai/flux-pro/v1.1", "", "", "fal-ai/flux-pro/v1.1", GatewayPathSourceFal, false, "fal"},
		{"openai", "dall-e-3", "custom/endpoint/v2", "", "custom/endpoint/v2", GatewayPathSourceOverride, false, "openai"},
	}
	for _, tc := range cases {
		got := ResolveRoute(tc.provider, tc.model, tc.override, tc.params)
		if got.Path != tc.wantPath || got.Source != tc.wantSrc || got.IsChat != tc.wantChat || got.Namespace != tc.wantNS {
			t.Fatalf("ResolveRoute(%q, %q, %q, %q) = %+v, want path %q src %q chat %v ns %q",
				tc.provider, tc.model, tc.override, tc.params, got, tc.wantPath, tc.wantSrc, tc.wantChat, tc.wantNS)
		}
	}
}

func TestSplitModelIdentifier(t *testing.T) {
	provider, model := SplitModelIdentifier("openai/gpt-image-1", "")
	if provider != "openai" || model != "gpt-image-1" {
		t.Fatalf("SplitModelIdentifier = %q/%q, want openai/gpt-image-1", provider, model)
	}
	provider, model = SplitModelIdentifier("seedream-v2", "ByteDance")
	if provider != "bytedance" || model != "seedream-v2" {
		t.Fatalf("SplitModelIdentifier fallback = %q/%q, want bytedance/seedream-v2", provider, model)
	}
}

func TestFormatPoolModel(t *testing.T) {
	if got := FormatPoolModel("OpenAI", "gpt-image-1"); got != "openai/gpt-image-1" {
		t.Fatalf("FormatPoolModel = %q, want openai/gpt-image-1", got)
	}
	// Nested-slash preservation is covered once by TestSplitPoolDisplay.
}

func TestSplitPoolDisplay(t *testing.T) {
	provider, model, err := SplitPoolDisplay("openai/gpt-image-1")
	if err != nil || provider != "openai" || model != "gpt-image-1" {
		t.Fatalf("SplitPoolDisplay = %q/%q, %v; want openai/gpt-image-1, nil", provider, model, err)
	}
	// Prefix/bare separation keeps nested vendor segments in the bare name.
	provider, model, err = SplitPoolDisplay("openrouter/google/gemini-3-flash")
	if err != nil || provider != "openrouter" || model != "google/gemini-3-flash" {
		t.Fatalf("SplitPoolDisplay nested = %q/%q, %v; want openrouter/google/gemini-3-flash, nil", provider, model, err)
	}
	// Bare models without a provider fail closed: execution must never
	// infer a provider from a prefix-less string. One representative per
	// reject branch (missing slash, empty segment, whitespace); the rest
	// share the same fail-closed path.
	for _, bare := range []string{"gpt-image-1", "openai/", "a b/c"} {
		if _, _, err := SplitPoolDisplay(bare); err == nil {
			t.Fatalf("SplitPoolDisplay(%q) = nil error, want fail-closed", bare)
		}
	}
}

func TestParseExecutionModelRef(t *testing.T) {
	provider, model, path, err := ParseExecutionModelRef("openai/dall-e-3")
	if err != nil || provider != "openai" || model != "dall-e-3" || path != "" {
		t.Fatalf("ParseExecutionModelRef = %q/%q@%q, %v; want openai/dall-e-3, nil", provider, model, path, err)
	}
	provider, model, path, err = ParseExecutionModelRef("myco/custom-model@v1/custom")
	if err != nil || provider != "myco" || model != "custom-model" || path != "v1/custom" {
		t.Fatalf("ParseExecutionModelRef override = %q/%q@%q, %v; want myco/custom-model@v1/custom, nil", provider, model, path, err)
	}
	// Unknown slugs stay open (custom-<slug> namespace downstream), and
	// bare models without a provider fail closed. Whitespace rejection is
	// covered once by TestSplitPoolDisplay.
	if _, _, _, err := ParseExecutionModelRef("dall-e-3"); err == nil {
		t.Fatal("ParseExecutionModelRef(bare) = nil error, want fail-closed")
	}
}
