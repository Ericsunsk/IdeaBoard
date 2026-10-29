package application

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/platform/safehttp"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

var errTestControlPlane = errors.New("control plane unavailable")

type stubOutboundPolicy struct {
	hosts []string
	err   error
}

func (s stubOutboundPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	if s.err != nil {
		return settings.OutboundPolicySetting{}, s.err
	}
	return settings.OutboundPolicySetting{AllowHosts: s.hosts}, nil
}

var _ = safehttp.AllowHosts

func stubControlPlaneSlugs(t *testing.T, slugs []string, err error) {
	t.Helper()
	previous := controlPlaneSlugsFetcher
	controlPlaneSlugsFetcher = func(ctx context.Context, gateway ResolvedGateway) ([]string, error) {
		return slugs, err
	}
	t.Cleanup(func() { controlPlaneSlugsFetcher = previous })
	configuredSlugsCache.Lock()
	configuredSlugsCache.byToken = make(map[string]configuredSlugsEntry)
	configuredSlugsCache.Unlock()
}

const providersTestGatewayURL = "https://gateway.ai.cloudflare.com/v1/abcdef0123456789abcdef0123456789/providers-test"

func newProvidersTestRepo(t *testing.T, token string) *ConfigRepository {
	t.Helper()
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := context.Background()
	if err := repo.Update(ctx, UpdateInput{
		BaseURL: providersTestGatewayURL,
		Token:   &token,
	}, 42); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	return repo
}

func TestListConfiguredProvidersIncludesWorkersAIFirstParty(t *testing.T) {
	stubControlPlaneSlugs(t, []string{"openai"}, nil)
	repo := newProvidersTestRepo(t, "providers-workers-ai-token")
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
	stubControlPlaneSlugs(t, []string{"openai", "OpenAI", "custom-co"}, nil)
	repo := newProvidersTestRepo(t, "providers-dedupe-token")
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

func TestListConfiguredProvidersFailureFailsClosed(t *testing.T) {
	stubControlPlaneSlugs(t, nil, errTestControlPlane)
	repo := newProvidersTestRepo(t, "providers-fail-closed-token")
	if _, err := repo.ListConfiguredProviders(context.Background()); err == nil {
		t.Fatal("ListConfiguredProviders() = nil error, want control-plane failure")
	}
}

func TestResolveControlPlaneIdentityPrefersStoredIDs(t *testing.T) {
	accountID, gatewayID, err := resolveControlPlaneIdentity(ResolvedGateway{
		BaseURL:   "https://egress.internal.example.com/ai",
		AccountID: "abc123abc123abc123abc123abc123ab",
		GatewayID: "prod",
		Token:     "tok",
	})
	if err != nil {
		t.Fatalf("resolveControlPlaneIdentity() error = %v", err)
	}
	if accountID != "abc123abc123abc123abc123abc123ab" || gatewayID != "prod" {
		t.Errorf("identity = (%q, %q), want stored IDs", accountID, gatewayID)
	}

	accountID, gatewayID, err = resolveControlPlaneIdentity(ResolvedGateway{
		BaseURL: "https://gateway.ai.cloudflare.com/v1/acc123/gw456",
		Token:   "tok",
	})
	if err != nil {
		t.Fatalf("URL fallback error = %v", err)
	}
	if accountID != "acc123" || gatewayID != "gw456" {
		t.Errorf("identity = (%q, %q), want URL-parsed", accountID, gatewayID)
	}

	if _, _, err := resolveControlPlaneIdentity(ResolvedGateway{
		BaseURL: "https://egress.internal.example.com/ai",
		Token:   "tok",
	}); err == nil {
		t.Fatal("expected identity error, got nil")
	}
}

func TestExtractGatewayIdentity(t *testing.T) {
	tests := []struct {
		url      string
		wantAcc  string
		wantGate string
	}{
		{"https://gateway.ai.cloudflare.com/v1/acc123/gw456", "acc123", "gw456"},
		{"https://gateway.ai.cloudflare.com/v1/acc123/gw456/openai/models", "acc123", "gw456"},
		{"https://custom.gateway.com/v1/acc/gw", "", ""},
	}

	for _, tt := range tests {
		acc, gw := extractGatewayIdentity(tt.url)
		if acc != tt.wantAcc || gw != tt.wantGate {
			t.Errorf("extractGatewayIdentity(%q) = (%q, %q), want (%q, %q)", tt.url, acc, gw, tt.wantAcc, tt.wantGate)
		}
	}
}
