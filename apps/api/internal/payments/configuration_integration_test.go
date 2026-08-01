package payments

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func TestConfigureStripePreparesThenAtomicallyPublishesSafeSummary(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	// The lifecycle lock and the atomic configuration write must share one
	// connection; otherwise a single-connection pool deadlocks itself.
	db.SetMaxOpenConns(1)
	gateway := &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: "acct_test", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_idea", WebhookEndpointID: "we_idea", WebhookSecret: "whsec_test",
	}}
	service := NewService(ServiceDependencies{Repository: NewRepository(db), ConfigurationGateway: gateway})

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
FROM payment_stripe_configurations WHERE singleton_id = 1`).Scan(&encryptedKey, &encryptedWebhookSecret, &materialKind); err != nil {
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

func TestStripeLifecycleTransactionRollsBackAndUnlocksAfterPanic(t *testing.T) {
	db := testdb.Open(t)
	db.SetMaxOpenConns(1)
	repo := NewRepository(db)
	panicked := false
	func() {
		defer func() { panicked = recover() != nil }()
		_ = repo.WithStripeConfigurationSharedLock(t.Context(), func(lockedContext context.Context) error {
			return repo.dbWithContext(lockedContext).withTx(func(_ *dbsqlc.Queries, tx *sql.Tx) error {
				if _, err := tx.ExecContext(lockedContext, `
INSERT INTO payment_admin_actions (action_type, actor_id, target_type, target_id, outcome, metadata)
VALUES ('configure_stripe', 42, 'stripe_configuration', 'current', 'failed', '{}'::jsonb)`); err != nil {
					t.Fatal(err)
				}
				panic("injected transaction panic")
			})
		})
	}()
	if !panicked {
		t.Fatal("transaction panic did not propagate")
	}
	var auditCount int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM payment_admin_actions`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 0 {
		t.Fatalf("panic committed %d payment admin actions", auditCount)
	}
	if err := repo.WithStripeConfigurationExclusiveLock(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("lifecycle lock remained held after panic: %v", err)
	}
}

func TestConfigureStripeFailurePreservesCurrentConfigurationAndAuditsSanitizedError(t *testing.T) {
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	gateway := &fakeStripeGateway{prepared: PreparedStripeConfiguration{
		AccountID: "acct_original", Mode: StripeModeTest, ChargesEnabled: true,
		ProductID: "prod_original", WebhookEndpointID: "we_original", WebhookSecret: "whsec_original",
	}}
	service := NewService(ServiceDependencies{Repository: NewRepository(db), ConfigurationGateway: gateway})
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

func TestStripeAPIKeyModePolicy(t *testing.T) {
	tests := []struct {
		name string
		key  string
		mode StripeMode
		ok   bool
	}{
		{name: "test secret", key: "sk_test_value", mode: StripeModeTest, ok: true},
		{name: "live restricted", key: "rk_live_value", mode: StripeModeLive, ok: true},
		{name: "live secret rejected", key: "sk_live_value", ok: false},
		{name: "test restricted rejected by product policy", key: "rk_test_value", ok: false},
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

func readConfigurationUpdatedAt(t *testing.T, db *sql.DB) time.Time {
	t.Helper()
	var value time.Time
	if err := db.QueryRowContext(t.Context(), `SELECT updated_at FROM payment_stripe_configurations WHERE singleton_id = 1`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
