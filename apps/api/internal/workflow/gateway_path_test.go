package workflow

import (
	"testing"
)

func TestGatewayProviderPath(t *testing.T) {
	cases := map[string]string{
		"openrouter":   "openrouter",
		"fal":          "fal",
		"fal-ai":       "fal-ai",
		"fal_ai":       "fal_ai",
		"openai":       "openai",
		"Anthropic":    "anthropic",
		"gemini":       "gemini",
		"azureopenai":  "azureopenai",
		"perplexity":   "perplexity",
		"bedrock":      "bedrock",
		"vertex_ai":    "vertex_ai",
		"deepseek":     "deepseek",
		"google":       "google",
		"myllm":        "myllm",
		"custom-myllm": "custom-myllm",
		"   ":          "",
	}
	for input, want := range cases {
		if got := GatewayProviderPath(input); got != want {
			t.Fatalf("GatewayProviderPath(%q) = %q, want %q", input, got, want)
		}
	}
	if IsFirstPartyGatewayProvider("custom-myllm") {
		t.Fatal("IsFirstPartyGatewayProvider(custom-myllm) = true, want false")
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
		{"fal", "flux-dev", "", "", "fal-ai/flux-dev", GatewayPathSourceFal, false, "fal"},
		{"fal_ai", "fal-ai/flux-pro/v1.1", "", "", "fal-ai/flux-pro/v1.1", GatewayPathSourceFal, false, "fal_ai"},
		{"fal-ai", "fast-sdxl", "", "", "fal-ai/fast-sdxl", GatewayPathSourceFal, false, "fal-ai"},
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

func TestInferGatewayPathSourceTableDriven(t *testing.T) {
	// Model-routed provider (fal)
	path, src := InferGatewayPathSource("fal", "birefnet", "", false)
	if path != "fal-ai/birefnet" || src != GatewayPathSourceFal {
		t.Fatalf("InferGatewayPathSource(fal, birefnet) = %q, %q; want fal-ai/birefnet, %q", path, src, GatewayPathSourceFal)
	}

	// Model-routed provider with existing owner
	path, src = InferGatewayPathSource("fal", "fal-ai/birefnet", "", false)
	if path != "fal-ai/birefnet" || src != GatewayPathSourceFal {
		t.Fatalf("InferGatewayPathSource(fal, fal-ai/birefnet) = %q, %q; want fal-ai/birefnet, %q", path, src, GatewayPathSourceFal)
	}

	// Convention providers (openrouter, anthropic)
	path, src = InferGatewayPathSource("openrouter", "meta-llama/llama-3", "", true)
	if path != "api/v1/chat/completions" || src != GatewayPathSourceOpenRouter {
		t.Fatalf("InferGatewayPathSource(openrouter) = %q, %q; want api/v1/chat/completions, %q", path, src, GatewayPathSourceOpenRouter)
	}

	path, src = InferGatewayPathSource("anthropic", "claude-3", "", true)
	if path != "v1/messages" || src != GatewayPathSourceAnthropic {
		t.Fatalf("InferGatewayPathSource(anthropic) = %q, %q; want v1/messages, %q", path, src, GatewayPathSourceAnthropic)
	}

	// Model path with slash for non-fal provider
	path, src = InferGatewayPathSource("replicate", "blackforestlabs/flux-1-schnell", "", false)
	if path != "blackforestlabs/flux-1-schnell" || src != GatewayPathSourceOverride {
		t.Fatalf("InferGatewayPathSource(replicate) = %q, %q; want blackforestlabs/flux-1-schnell, %q", path, src, GatewayPathSourceOverride)
	}

	// Override wins over everything
	path, src = InferGatewayPathSource("fal", "birefnet", "custom/cutout", false)
	if path != "custom/cutout" || src != GatewayPathSourceOverride {
		t.Fatalf("InferGatewayPathSource(override) = %q, %q; want custom/cutout, %q", path, src, GatewayPathSourceOverride)
	}

	// Fallback to chat
	path, src = InferGatewayPathSource("custom-provider", "model", "", true)
	if path != "v1/chat/completions" || src != GatewayPathSourceChat {
		t.Fatalf("InferGatewayPathSource(fallback chat) = %q, %q; want v1/chat/completions, %q", path, src, GatewayPathSourceChat)
	}

	// Fallback to generations
	path, src = InferGatewayPathSource("custom-provider", "model", "", false)
	if path != "v1/images/generations" || src != GatewayPathSourceGenerations {
		t.Fatalf("InferGatewayPathSource(fallback generations) = %q, %q; want v1/images/generations, %q", path, src, GatewayPathSourceGenerations)
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
