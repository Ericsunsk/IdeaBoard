package workflow

import (
	"net/http"
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

func TestClassifyGatewayResponse(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   Reachability
	}{
		{http.StatusUnauthorized, "{}", ReachabilityUnavailable},
		{http.StatusForbidden, "{}", ReachabilityUnavailable},
		{http.StatusBadRequest, `{"code":2008}`, ReachabilityUnavailable},
		{http.StatusBadRequest, `{"code": 2008}`, ReachabilityUnavailable},
		{http.StatusBadRequest, `{"code":9106}`, ReachabilityReachable},
		{http.StatusBadRequest, `{"code": 7001}`, ReachabilityReachable},
		{http.StatusBadRequest, `{"code":7003}`, ReachabilityReachable},
		{http.StatusOK, "{}", ReachabilityReachable},
		{http.StatusBadRequest, `{"success":false}`, ReachabilityUnreachable},
		{http.StatusInternalServerError, "boom", ReachabilityUnreachable},
	}
	for _, tc := range cases {
		if got := ClassifyGatewayResponse(tc.status, tc.body); got != tc.want {
			t.Errorf("ClassifyGatewayResponse(%d, %q) = %v, want %v", tc.status, tc.body, got, tc.want)
		}
	}
}
