package application

import (
	"strings"
	"testing"

	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func TestValidateUpdateInput(t *testing.T) {
	token := "test-gateway-token"
	baseURL, accountID, gatewayID, gotToken, rotate, err := validateUpdateInput(UpdateInput{
		Token:     &token,
		AccountID: " ABCDEF0123456789ABCDEF0123456789 ",
		GatewayID: "Prod-Gateway ",
	}, 7)
	if err != nil {
		t.Fatalf("validateUpdateInput() error = %v", err)
	}
	if accountID != "abcdef0123456789abcdef0123456789" || gatewayID != "prod-gateway" {
		t.Fatalf("normalized = %q %q", accountID, gatewayID)
	}
	if baseURL != "https://gateway.ai.cloudflare.com/v1/abcdef0123456789abcdef0123456789/prod-gateway" {
		t.Fatalf("baseURL = %q", baseURL)
	}
	if !rotate || gotToken != token {
		t.Fatal("token rotation not detected")
	}

	// Non-Cloudflare hosts fail closed: there is no custom-egress path.
	for _, customURL := range []string{
		"https://my-gateway.example.com/",
		"https://gateway.myorg.internal",
		"http://127.0.0.1:8080/v1/a/g",
	} {
		if _, _, _, _, _, err := validateUpdateInput(UpdateInput{
			BaseURL: customURL,
			Token:   &token,
		}, 7); err == nil {
			t.Fatalf("validateUpdateInput(%q) accepted, want sole-egress rejection", customURL)
		}
	}

	// Test with standard Cloudflare BaseURL auto-extracting Account and Gateway
	cfBaseURL, cfAccountID, cfGatewayID, _, _, err := validateUpdateInput(UpdateInput{
		BaseURL: "https://gateway.ai.cloudflare.com/v1/abcdef0123456789abcdef0123456789/my-gateway",
		Token:   &token,
	}, 7)
	if err != nil {
		t.Fatalf("validateUpdateInput(cfBaseURL) error = %v", err)
	}
	if cfBaseURL != "https://gateway.ai.cloudflare.com/v1/abcdef0123456789abcdef0123456789/my-gateway" {
		t.Fatalf("cfBaseURL = %q", cfBaseURL)
	}
	if cfAccountID != "abcdef0123456789abcdef0123456789" || cfGatewayID != "my-gateway" {
		t.Fatalf("extracted account/gateway = %q / %q", cfAccountID, cfGatewayID)
	}

	keepBaseURL, keepAccount, keepGateway, _, keepRotate, err := validateUpdateInput(UpdateInput{
		AccountID: "abcdef0123456789abcdef0123456789",
		GatewayID: "default",
	}, 7)
	if err != nil || keepRotate || keepAccount == "" || keepGateway == "" || keepBaseURL == "" {
		t.Fatalf("nil token should keep secret: %v %v", keepRotate, err)
	}
	for _, tc := range []struct {
		name  string
		input UpdateInput
		actor int64
	}{
		{"missing actor", UpdateInput{AccountID: "abcdef0123456789abcdef0123456789", GatewayID: "g"}, 0},
		{"short account", UpdateInput{AccountID: "abc", GatewayID: "g"}, 7},
		{"account with illegal chars", UpdateInput{AccountID: "abcdef0123456789abcdef0123456789!", GatewayID: "g"}, 7},
		{"bad gateway id", UpdateInput{AccountID: "abcdef0123456789abcdef0123456789", GatewayID: "bad id!"}, 7},
		{"empty token", UpdateInput{Token: strPtr("  "), AccountID: "abcdef0123456789abcdef0123456789", GatewayID: "g"}, 7},
		{"bad base url scheme", UpdateInput{BaseURL: "ftp://example.com", Token: strPtr("tok")}, 7},
		{"bad base url empty host", UpdateInput{BaseURL: "http://", Token: strPtr("tok")}, 7},
		{"base url userinfo", UpdateInput{BaseURL: "https://user:pass@example.com", Token: strPtr("tok")}, 7},
		{"base url query", UpdateInput{BaseURL: "https://example.com/path?token=secret", Token: strPtr("tok")}, 7},
		{"explicit invalid account with custom url", UpdateInput{BaseURL: "https://example.com", AccountID: "not-valid", Token: strPtr("tok")}, 7},
	} {
		if _, _, _, _, _, err := validateUpdateInput(tc.input, tc.actor); err == nil {
			t.Fatalf("%s accepted, want error", tc.name)
		}
	}
}

func strPtr(s string) *string { return &s }

func TestGatewayBaseURL(t *testing.T) {
	got := gatewayBaseURL("acc123", "prod")
	if got != "https://gateway.ai.cloudflare.com/v1/acc123/prod" {
		t.Fatalf("baseURL = %q", got)
	}
}

func newGatewayTestRepository(t *testing.T) *ConfigRepository {
	t.Helper()
	db := testdb.Open(t)
	return NewConfigRepository(settings.NewSystemConfigRepository(db))
}

func TestConfigRepositoryRoundTrip(t *testing.T) {
	secretstest.EnableEncryption(t)
	repo := newGatewayTestRepository(t)
	ctx := t.Context()

	if _, err := repo.Resolve(ctx); err != ErrAIGatewayNotConfigured {
		t.Fatalf("Resolve() on empty = %v, want ErrAIGatewayNotConfigured", err)
	}
	token := "live-gateway-token-value"
	if err := repo.Update(ctx, UpdateInput{
		Token:     &token,
		AccountID: "abcdef0123456789abcdef0123456789",
		GatewayID: "prod",
	}, 42); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	summary, err := repo.GetSummary(ctx)
	if err != nil {
		t.Fatalf("GetSummary() error = %v", err)
	}
	if !summary.TokenConfigured || summary.AccountID == "" || summary.GatewayID != "prod" || summary.UpdatedAt == nil || !summary.IsFirstParty {
		t.Fatalf("summary = %+v", summary)
	}
	resolved, err := repo.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Token != token || !strings.HasPrefix(resolved.BaseURL, "https://gateway.ai.cloudflare.com/v1/") {
		t.Fatalf("resolved leaks or misbuilds: %+v", resolved.BaseURL)
	}

	// Custom BaseURLs fail closed on update: sole egress pins Cloudflare.
	if err := repo.Update(ctx, UpdateInput{
		BaseURL: "https://gateway.myorg.internal",
	}, 42); err == nil {
		t.Fatal("Update() with custom BaseURL succeeded, want sole-egress rejection")
	}
	keptCustom, err := repo.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve() after rejected custom BaseURL error = %v", err)
	}
	if keptCustom.BaseURL == "https://gateway.myorg.internal" {
		t.Fatalf("custom BaseURL persisted despite rejection: %+v", keptCustom)
	}

	if err := repo.Update(ctx, UpdateInput{
		AccountID: "abcdef0123456789abcdef0123456789",
		GatewayID: "prod-v2",
	}, 42); err != nil {
		t.Fatalf("Update() without token error = %v", err)
	}
	kept, err := repo.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve() after token-less update error = %v", err)
	}
	if kept.Token != token || kept.GatewayID != "prod-v2" {
		t.Fatalf("token not preserved across update: %+v", kept)
	}
	if kept.BaseURL != "https://gateway.ai.cloudflare.com/v1/abcdef0123456789abcdef0123456789/prod-v2" {
		t.Fatalf("identity-only update did not rebuild CF BaseURL: %+v", kept.BaseURL)
	}
}

func TestSaveCatalogValidation(t *testing.T) {
	secretstest.EnableEncryption(t)
	repo := newGatewayTestRepository(t)
	ctx := t.Context()

	valid := []CatalogItem{{
		ID: "test/model-1", Name: "Model 1", Provider: "Test",
		Mode: "chat", Description: "d", ContextOrResolution: "c", Hosting: "h", TermsURL: "t", DocsURL: "d",
	}}
	if err := repo.SaveCatalog(ctx, valid, 1); err != nil {
		t.Fatalf("SaveCatalog(valid) error = %v", err)
	}

	dup := append(append([]CatalogItem{}, valid...), CatalogItem{
		ID: "TEST/model-1", Name: "Dup", Provider: "Test",
	})
	if err := repo.SaveCatalog(ctx, dup, 1); err == nil {
		t.Fatal("SaveCatalog(duplicate ID) error = nil, want error")
	}

	badJSON := append([]CatalogItem{}, valid[0])
	badJSON[0].CustomParams = "{not-json"
	if err := repo.SaveCatalog(ctx, badJSON, 1); err == nil {
		t.Fatal("SaveCatalog(invalid custom_params) error = nil, want error")
	}

	nonObject := append([]CatalogItem{}, valid[0])
	nonObject[0].CustomParams = "[1,2]"
	if err := repo.SaveCatalog(ctx, nonObject, 1); err == nil {
		t.Fatal("SaveCatalog(non-object custom_params) error = nil, want error")
	}

	if err := repo.SaveCatalog(ctx, valid, 0); err == nil {
		t.Fatal("SaveCatalog(zero actor) error = nil, want error")
	}
}

func TestListCatalogSurfacesCorruption(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
	ctx := t.Context()

	if _, err := repo.ListCatalog(ctx); err != nil {
		t.Fatalf("ListCatalog(unconfigured) error = %v, want empty default", err)
	}
	settingsRepo := settings.NewSystemConfigRepository(db)
	if err := settingsRepo.Set(ctx, catalogConfigKey, "{corrupt"); err != nil {
		t.Fatalf("seed corrupt catalog: %v", err)
	}
	if _, err := repo.ListCatalog(ctx); err == nil {
		t.Fatal("ListCatalog(corrupt) error = nil, want integrity error")
	}
}
