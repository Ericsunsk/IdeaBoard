package wiring

import (
	"context"
	"testing"

	aigatewayapplication "ideaboard/api/internal/aigateway/application"
)

type routeCatalogStub []aigatewayapplication.CatalogItem

func (s routeCatalogStub) ListCatalog(context.Context) ([]aigatewayapplication.CatalogItem, error) {
	return s, nil
}

func TestLookupRouteOverrideProviderModelIdentity(t *testing.T) {
	reader := aiGatewayModelRouteReader{repo: routeCatalogStub{
		{Provider: "openai", ID: "GPT-IMAGE-1", CustomPath: "openai/custom-generate", CustomParams: `{"quality":"high"}`},
		{Provider: "azureopenai", ID: "gpt-image-1", CustomPath: "azure/custom-generate", CustomParams: `{"quality":"medium"}`},
		{Provider: "openrouter", ID: "Google/Gemini-3-Flash", CustomPath: "openrouter/custom-generate", CustomParams: `{"temperature":0.5}`},
	}}
	cases := []struct {
		name, provider, model, wantPath, wantParams string
		wantFound                                   bool
	}{
		{name: "case-insensitive native model", provider: "openai", model: "gpt-image-1", wantPath: "openai/custom-generate", wantParams: `{"quality":"high"}`, wantFound: true},
		{name: "same-name provider isolation", provider: " AzureOpenAI ", model: " GPT-IMAGE-1 ", wantPath: "azure/custom-generate", wantParams: `{"quality":"medium"}`, wantFound: true},
		{name: "nested native model", provider: "OpenRouter", model: "google/gemini-3-flash", wantPath: "openrouter/custom-generate", wantParams: `{"temperature":0.5}`, wantFound: true},
		{name: "different provider", provider: "anthropic", model: "gpt-image-1"},
		{name: "empty native model", provider: "openai", model: " "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, params, found, err := reader.LookupRouteOverride(context.Background(), tc.provider, tc.model)
			if err != nil || path != tc.wantPath || params != tc.wantParams || found != tc.wantFound {
				t.Fatalf("LookupRouteOverride(%q, %q) = (%q, %q, %v, %v), want (%q, %q, %v, nil)", tc.provider, tc.model, path, params, found, err, tc.wantPath, tc.wantParams, tc.wantFound)
			}
		})
	}
}
