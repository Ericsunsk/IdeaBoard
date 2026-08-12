package payments

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func TestConfigureStripePreparesThenAtomicallyPublishesSafeSummary(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	db.SetMaxOpenConns(1)
	gateway := &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: "acct_test", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_idea", WebhookEndpointID: "we_idea", WebhookSecret: "whsec_test",
	}}
	repo := NewRepository(db)
	service := newCompleteTestCheckoutWorkflow(t, repo, NewPaymentRuntime(nil, nil), gateway, testCheckoutGateway{}, testWalletReader{}, testShutdownEnqueuer{})

	summary, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_safe_fixture", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	})
	if err != nil {
		t.Fatalf("ConfigureStripe() error = %v", err)
	}
	if !summary.Configured || !summary.APIKeyConfigured || !summary.WebhookSecretConfigured {
		t.Fatalf("summary secret readiness = %+v", summary)
	}
	if summary.AccountID != "acct_test" || summary.Mode != StripeModeTest || !summary.ChargesEnabled {
		t.Fatalf("summary account = %+v", summary)
	}
	if !summary.ProductReady || !summary.WebhookReady || !summary.PurchasesEnabled {
		t.Fatalf("summary readiness = %+v", summary)
	}
	if strings.Contains(summary.AccountID, "sk_test") {
		t.Fatalf("summary leaked API key: %+v", summary)
	}

	var encryptedKey, encryptedWebhookSecret, materialKind string
	if err := db.QueryRowContext(t.Context(), `
SELECT api_key_ciphertext, webhook_secret_ciphertext, secret_material_kind
	FROM payment_stripe_credentials credentials
	JOIN payment_stripe_configurations configuration ON configuration.active_credential_id = credentials.id
	WHERE configuration.singleton_id = 1`).Scan(&encryptedKey, &encryptedWebhookSecret, &materialKind); err != nil {
		t.Fatalf("read stored configuration: %v", err)
	}
	if encryptedKey == "sk_test_safe_fixture" || encryptedWebhookSecret == "whsec_test" || materialKind != "encrypted" {
		t.Fatalf("secrets were not encrypted")
	}

	var actorID int64
	var outcome string
	if err := db.QueryRowContext(t.Context(), `
SELECT actor_id, outcome FROM payment_admin_actions ORDER BY id DESC LIMIT 1`).Scan(&actorID, &outcome); err != nil {
		t.Fatalf("read success audit: %v", err)
	}
	if actorID != 42 || outcome != "succeeded" {
		t.Fatalf("success audit = actor=%d outcome=%q", actorID, outcome)
	}

	loaded, err := service.GetStripeConfiguration(t.Context())
	if err != nil || loaded != summary {
		t.Fatalf("GetStripeConfiguration() = %+v, %v; want %+v", loaded, err, summary)
	}
}

func TestConfigureStripeFailurePreservesCurrentConfigurationAndAuditsSanitizedError(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: "acct_original", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_original", WebhookEndpointID: "we_original", WebhookSecret: "whsec_original",
	}}
	repo := NewRepository(db)
	service := newCompleteTestCheckoutWorkflow(t, repo, NewPaymentRuntime(nil, nil), gateway, testCheckoutGateway{}, testWalletReader{}, testShutdownEnqueuer{})
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_original", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatalf("seed configuration: %v", err)
	}

	gateway.err = NewGatewayError(GatewayErrorPermissionDenied, "permission denied", errors.New("raw provider body sk_test_new"))
	_, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 99, APIKey: "rk_live_new", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: false, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	})
	if !errors.Is(err, ErrStripePermissionDenied) {
		t.Fatalf("ConfigureStripe() error = %v, want ErrStripePermissionDenied", err)
	}

	loaded, loadErr := service.GetStripeConfiguration(t.Context())
	if loadErr != nil || loaded.AccountID != "acct_original" || !loaded.PurchasesEnabled {
		t.Fatalf("configuration changed after failure: %+v, %v", loaded, loadErr)
	}

	var outcome, errorCode, errorSummary string
	if err := db.QueryRowContext(t.Context(), `
SELECT outcome, error_code, error_summary
FROM payment_admin_actions ORDER BY id DESC LIMIT 1`).Scan(&outcome, &errorCode, &errorSummary); err != nil {
		t.Fatalf("read failure audit: %v", err)
	}
	if outcome != "failed" || errorCode != string(GatewayErrorPermissionDenied) {
		t.Fatalf("failure audit = outcome=%q code=%q", outcome, errorCode)
	}
	if strings.Contains(errorSummary, "sk_test_new") || strings.Contains(errorSummary, "raw provider") {
		t.Fatalf("failure audit leaked provider detail: %q", errorSummary)
	}
}

func TestConfigureStripeReusesOnlyAnUnchangedCredentialGeneration(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: "acct_stable", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_stable", WebhookEndpointID: "we_stable", WebhookSecret: "whsec_stable",
	}}
	repo := NewRepository(db)
	service := newCompleteTestCheckoutWorkflow(t, repo, NewPaymentRuntime(nil, nil), gateway, testCheckoutGateway{}, testWalletReader{}, testShutdownEnqueuer{})
	command := ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_stable", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}
	if _, err := service.ConfigureStripe(t.Context(), command); err != nil {
		t.Fatalf("initial ConfigureStripe() error = %v", err)
	}
	initial, err := repo.GetStripeConfiguration(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfigureStripe(t.Context(), command); err != nil {
		t.Fatalf("repeated ConfigureStripe() error = %v", err)
	}
	repeated, err := repo.GetStripeConfiguration(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if repeated.CredentialID != initial.CredentialID {
		t.Fatalf("unchanged credential rotated from %q to %q", initial.CredentialID, repeated.CredentialID)
	}

	command.APIKey = "sk_test_rotated"
	if _, err := service.ConfigureStripe(t.Context(), command); err != nil {
		t.Fatalf("rotated ConfigureStripe() error = %v", err)
	}
	rotated, err := repo.GetStripeConfiguration(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if rotated.CredentialID == initial.CredentialID {
		t.Fatal("changed API Key reused the previous credential generation")
	}
	var credentialCount int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM payment_stripe_credentials`).Scan(&credentialCount); err != nil {
		t.Fatal(err)
	}
	if credentialCount != 2 {
		t.Fatalf("credential generations = %d, want 2", credentialCount)
	}
}

func TestReplaceStripeConfigurationRejectsAStaleRevision(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: "acct_revision", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_revision", WebhookEndpointID: "we_revision", WebhookSecret: "whsec_revision",
	}}
	repo := NewRepository(db)
	service := newCompleteTestCheckoutWorkflow(t, repo, NewPaymentRuntime(nil, nil), gateway, testCheckoutGateway{}, testWalletReader{}, testShutdownEnqueuer{})
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_revision", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatal(err)
	}
	current, err := repo.GetStripeConfiguration(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	updated := current
	updated.UpdatedBy = 99
	if _, _, err := repo.ReplaceStripeConfiguration(t.Context(), current.Revision, updated, ConfigurationAudit{ActorID: 99, Outcome: "succeeded"}); err != nil {
		t.Fatalf("first ReplaceStripeConfiguration() error = %v", err)
	}
	updated.UpdatedBy = 42
	if _, _, err := repo.ReplaceStripeConfiguration(t.Context(), current.Revision, updated, ConfigurationAudit{ActorID: 42, Outcome: "succeeded"}); !errors.Is(err, ErrStripeConfigurationChanged) {
		t.Fatalf("stale ReplaceStripeConfiguration() error = %v, want ErrStripeConfigurationChanged", err)
	}
}

func TestDisablingPurchasesShutsDownThePreviousCredentialAccount(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: "acct_previous", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_previous", WebhookEndpointID: "we_previous", WebhookSecret: "whsec_previous",
	}}
	repo := NewRepository(db)
	service := newCompleteTestCheckoutWorkflow(t, repo, NewPaymentRuntime(nil, nil), gateway, testCheckoutGateway{}, testWalletReader{}, testShutdownEnqueuer{})
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_previous", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: true, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatal(err)
	}
	previous, err := repo.GetStripeConfiguration(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	gateway.prepared = PreparedStripeConfiguration{
		AccountID: "acct_next", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_next", WebhookEndpointID: "we_next", WebhookSecret: "whsec_next",
	}
	if _, err := service.ConfigureStripe(t.Context(), ConfigureStripeCommand{
		ActorID: 42, APIKey: "sk_test_next", ProductTaxCode: "txcd_10105001",
		PurchasesEnabled: false, WebhookURL: "https://app.example.com/api/webhooks/stripe",
	}); err != nil {
		t.Fatal(err)
	}
	shutdowns, err := repo.ListActivePaymentCheckoutShutdowns(t.Context())
	if err != nil || len(shutdowns) != 1 {
		t.Fatalf("Checkout shutdowns = %+v, %v", shutdowns, err)
	}
	if shutdowns[0].StripeCredentialID != previous.CredentialID || shutdowns[0].StripeAccountID != previous.AccountID {
		t.Fatalf("Checkout shutdown identity = %+v, want credential=%q account=%q", shutdowns[0], previous.CredentialID, previous.AccountID)
	}
}

func TestStripeAPIKeyModePolicy(t *testing.T) {
	tests := []struct {
		name string
		key  string
		mode StripeMode
		ok   bool
	}{
		{name: "test secret", key: "sk_test_value", mode: StripeModeTest, ok: true},
		{name: "test restricted", key: "rk_test_value", mode: StripeModeTest, ok: true},
		{name: "live secret", key: "sk_live_value", mode: StripeModeLive, ok: true},
		{name: "live restricted", key: "rk_live_value", mode: StripeModeLive, ok: true},
		{name: "publishable rejected", key: "pk_live_value", ok: false},
		{name: "malformed", key: "secret", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, err := ClassifyStripeAPIKey(tt.key)
			if tt.ok && (err != nil || mode != tt.mode) {
				t.Fatalf("ClassifyStripeAPIKey() = %q, %v", mode, err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("ClassifyStripeAPIKey() accepted %q", tt.key)
			}
		})
	}
}

type fakeStripeGateway struct {
	prepared PreparedStripeConfiguration
	err      error
	calls    []PrepareStripeConfiguration
}

func (f *fakeStripeGateway) PrepareConfiguration(_ context.Context, input PrepareStripeConfiguration) (PreparedStripeConfiguration, error) {
	f.calls = append(f.calls, input)
	if f.err != nil {
		return PreparedStripeConfiguration{}, f.err
	}
	return f.prepared, nil
}
