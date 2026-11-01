package infrastructure

import (
	"context"
	"testing"

	"ideaboard/api/internal/modelregistry/domain"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func newProvidersTestRepo(t *testing.T, items []domain.CatalogItem) *ConfiguredProviders {
	t.Helper()
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	if err := repo.SaveCatalog(context.Background(), items, 42); err != nil {
		t.Fatalf("SaveCatalog() error = %v", err)
	}
	return NewConfiguredProviders(repo, DiscoveryConfig{})
}

func TestListConfiguredProvidersIncludesWorkersAIFirstParty(t *testing.T) {
	repo := newProvidersTestRepo(t, []domain.CatalogItem{
		{ID: "gpt-image-1", Name: "GPT", Provider: "openai"},
	})
	providers, err := repo.ListConfiguredProviders(context.Background())
	if err != nil {
		t.Fatalf("ListConfiguredProviders() error = %v", err)
	}
	bySlug := make(map[string]bool, len(providers))
	for _, p := range providers {
		bySlug[p.Slug] = true
	}
	if !bySlug["openai"] || !bySlug["workers-ai"] {
		t.Fatalf("providers = %+v, want openai + workers-ai constant", providers)
	}
}

func TestListConfiguredProvidersDedupesAndSynthesizes(t *testing.T) {
	repo := newProvidersTestRepo(t, []domain.CatalogItem{
		{ID: "m1", Name: "M1", Provider: "openai"},
		{ID: "m2", Name: "M2", Provider: "OpenAI"},
		{ID: "m3", Name: "M3", Provider: "custom-co"},
	})
	providers, err := repo.ListConfiguredProviders(context.Background())
	if err != nil {
		t.Fatalf("ListConfiguredProviders() error = %v", err)
	}
	seen := map[string]int{}
	for _, p := range providers {
		seen[p.Slug]++
		if p.DisplayName == "" || p.Slug == "" {
			t.Fatalf("provider = %+v, want slug + display name", p)
		}
	}
	if seen["openai"] != 1 {
		t.Fatalf("openai count = %d, want 1 (dedupe)", seen["openai"])
	}
	if seen["custom-co"] != 1 {
		t.Fatalf("custom-co missing in %+v (unknown slugs must synthesize, not vanish)", providers)
	}
	if seen["workers-ai"] != 1 {
		t.Fatalf("workers-ai missing in %+v", providers)
	}
}

func TestListConfiguredProvidersEmptyCatalogReturnsWorkersAIOnly(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	providers, err := NewConfiguredProviders(repo, DiscoveryConfig{}).ListConfiguredProviders(context.Background())
	if err != nil {
		t.Fatalf("ListConfiguredProviders() error = %v", err)
	}
	if len(providers) != 1 || providers[0].Slug != "workers-ai" {
		t.Fatalf("providers = %+v, want workers-ai only", providers)
	}
}
