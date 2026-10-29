package application

import (
	"encoding/json"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/errorrender"
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

	// Open egress: any http(s) host passes (audited, not allowlisted).
	gotBase, gotAccount, gotGateway, _, _, err := validateUpdateInput(UpdateInput{
		BaseURL:   "https://my-gateway.example.com/",
		AccountID: "abcdef0123456789abcdef0123456789",
		GatewayID: "my-gateway",
		Token:     &token,
	}, 7)
	if err != nil {
		t.Fatalf("validateUpdateInput(custom URL) rejected, want open-host pass: %v", err)
	}
	if gotBase == "" || gotAccount == "" || gotGateway == "" {
		t.Fatalf("validateUpdateInput(custom URL) = %q / %q / %q, want normalized", gotBase, gotAccount, gotGateway)
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
		{"explicit invalid account with custom url", UpdateInput{BaseURL: "https://example.com", AccountID: "not-valid", Token: strPtr("tok")}, 7},
		{"bare custom url without identity", UpdateInput{BaseURL: "https://gateway.example.com", Token: strPtr("tok")}, 7},
	} {
		if _, _, _, _, _, err := validateUpdateInput(tc.input, tc.actor); err == nil {
			t.Fatalf("%s accepted, want error", tc.name)
		}
	}

	// Userinfo, query, and fragment are rejected so secrets never
	// ride inside the stored URL.
	for _, tc := range []struct {
		name  string
		input UpdateInput
	}{
		{"arbitrary url userinfo", UpdateInput{BaseURL: "https://user:pass@gateway.example.com", Token: strPtr("tok")}},
		{"arbitrary url query", UpdateInput{BaseURL: "https://gateway.example.com/?token=secret", Token: strPtr("tok")}},
		{"arbitrary url fragment", UpdateInput{BaseURL: "https://gateway.example.com/#frag", Token: strPtr("tok")}},
	} {
		if _, _, _, _, _, err := validateUpdateInput(tc.input, 7); err == nil {
			t.Fatalf("%s accepted, want error", tc.name)
		} else if err.Error() != errorrender.MsgValidateUpdateInput4 {
			t.Fatalf("%s error = %q, want %q", tc.name, err.Error(), errorrender.MsgValidateUpdateInput4)
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
	db := testdb.Open(t)
	repo := NewConfigRepository(settings.NewSystemConfigRepository(db))
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

	customToken := "rotated-gateway-token-value"
	if err := repo.Update(ctx, UpdateInput{
		BaseURL:   "https://gateway.example.com",
		AccountID: "abcdef0123456789abcdef0123456789",
		GatewayID: "custom-one",
		Token:     &customToken,
	}, 42); err != nil {
		t.Fatalf("Update() with arbitrary BaseURL error = %v, want open-host pass", err)
	}
	keptCustom, err := repo.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve() after custom BaseURL error = %v", err)
	}
	if keptCustom.BaseURL != "https://gateway.example.com" {
		t.Fatalf("custom BaseURL not persisted: %+v", keptCustom)
	}
	var auditAction string
	var auditActor int64
	var auditMetadataRaw []byte
	if err := db.QueryRow(ctx, `
SELECT action_type, actor_id, metadata
FROM system_settings_admin_actions
WHERE action_type = 'update_ai_gateway'
ORDER BY id DESC
LIMIT 1`).Scan(&auditAction, &auditActor, &auditMetadataRaw); err != nil {
		t.Fatalf("read ai gateway audit: %v", err)
	}
	if auditAction != "update_ai_gateway" || auditActor != 42 {
		t.Fatalf("audit = type=%q actor=%d, want update_ai_gateway/42", auditAction, auditActor)
	}
	var auditMetadata map[string]any
	if err := json.Unmarshal(auditMetadataRaw, &auditMetadata); err != nil {
		t.Fatalf("decode ai gateway audit metadata: %v", err)
	}
	if auditMetadata["base_url_host"] != "gateway.example.com" {
		t.Fatalf("audit base_url_host = %v, want gateway.example.com", auditMetadata["base_url_host"])
	}
	if auditMetadata["prev_base_url_host"] != "gateway.ai.cloudflare.com" {
		t.Fatalf("audit prev_base_url_host = %v, want gateway.ai.cloudflare.com", auditMetadata["prev_base_url_host"])
	}
	if rotated, _ := auditMetadata["token_rotated"].(bool); !rotated {
		t.Fatalf("audit token_rotated = %v, want true", auditMetadata["token_rotated"])
	}
	if strings.Contains(string(auditMetadataRaw), customToken) {
		t.Fatal("audit metadata leaks token plaintext")
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
	if kept.Token != customToken || kept.GatewayID != "prod-v2" {
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
		ID: "model-1", Name: "Model 1", Provider: "Test",
		Mode: "chat", Description: "d", ContextOrResolution: "c", Hosting: "h", TermsURL: "t", DocsURL: "d",
	}}
	if err := repo.SaveCatalog(ctx, valid, 1); err != nil {
		t.Fatalf("SaveCatalog(valid) error = %v", err)
	}

	dup := append(append([]CatalogItem{}, valid...), CatalogItem{
		ID: "MODEL-1", Name: "Dup", Provider: "Test",
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
