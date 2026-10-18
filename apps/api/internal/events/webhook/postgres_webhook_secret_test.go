package webhook_test

// Encrypted-at-rest persistence proof for outbound webhook signing secrets.
// Requires API_TEST_DATABASE_DSN; testdb SKIPs gracefully otherwise.

import (
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/events/webhook"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
)

func mustMintSubscriptionID(t *testing.T) string {
	t.Helper()
	id, err := contract.MintPublicID("sub_", 12)
	if err != nil {
		t.Fatalf("mint subscription id: %v", err)
	}
	return id
}

// TestPostgresWebhookSecretEncryptedAtRest proves new writes land as
// ciphertext: the column holds no whsec_ material, the kind/version
// envelope is recorded, and loads resolve the original signing key.
func TestPostgresWebhookSecretEncryptedAtRest(t *testing.T) {
	secretstest.EnableEncryption(t)
	pool := testdb.Open(t)
	if _, err := pool.Exec(t.Context(), `
INSERT INTO workspaces (public_id, owner_user_id, name)
VALUES ('ws_1', 1, 'webhook secret encryption test')
ON CONFLICT (public_id) DO NOTHING`); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	persistence := webhook.NewPostgresWebhookPersistence(pool)
	raw := []byte(strings.Repeat("k", 32))
	sub := &webhook.WebhookSubscription{
		ID: mustMintSubscriptionID(t), WorkspaceID: "ws_1",
		URL: "https://hooks.example.com/x", EventTypes: []string{events.EventV1TaskCompleted},
		Secret: append([]byte(nil), raw...), Status: webhook.WebhookStatusActive,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := persistence.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}

	var stored, kind, version string
	if err := pool.QueryRow(t.Context(), `
SELECT signing_secret, signing_secret_material_kind, signing_secret_key_version
FROM event_subscriptions WHERE public_id = $1`, sub.ID).Scan(&stored, &kind, &version); err != nil {
		t.Fatalf("read stored secret row: %v", err)
	}
	if kind != secrets.MaterialKindEncrypted {
		t.Fatalf("material kind = %q, want encrypted", kind)
	}
	if version != secrets.KeyVersion() || len(version) != 16 {
		t.Fatalf("key version = %q, want the active key fingerprint", version)
	}
	if strings.HasPrefix(strings.TrimSpace(stored), "whsec_") {
		t.Fatal("stored signing_secret must be ciphertext, not whsec_ plaintext")
	}
	transport, err := secrets.DecryptMaterialString(stored, kind)
	if err != nil {
		t.Fatalf("stored payload must decrypt under the active key: %v", err)
	}
	decoded, ok := webhook.ParseWhsecSecret([]byte(transport))
	if !ok || string(decoded) != string(raw) {
		t.Fatal("stored ciphertext must cover the whsec_ transport of the raw key")
	}

	loaded, err := persistence.LoadSubscription(t.Context(), sub.ID)
	if err != nil {
		t.Fatalf("LoadSubscription() error = %v", err)
	}
	if loaded == nil || string(loaded.Secret) != string(raw) {
		t.Fatal("loaded secret must equal the stored raw key")
	}
	body := []byte(`{"id":"evt_1"}`)
	now := time.Now().UTC()
	headers := webhook.SignWebhook(loaded.Secret, body, "evt_1", now)
	if err := webhook.VerifyWebhookSignature([]byte(webhook.FormatWhsecSecret(raw)), body, headers, now); err != nil {
		t.Fatalf("sign/verify with the loaded key failed: %v", err)
	}
}

// TestPostgresWebhookSecretLegacyPlaintextInterop proves pre-migration
// whsec_ rows keep delivering and converge to ciphertext on read:
// the first load returns the raw key and re-encrypts the row, later
// loads stay stable.
func TestPostgresWebhookSecretLegacyPlaintextInterop(t *testing.T) {
	secretstest.EnableEncryption(t)
	pool := testdb.Open(t)
	if _, err := pool.Exec(t.Context(), `
INSERT INTO workspaces (public_id, owner_user_id, name)
VALUES ('ws_1', 1, 'webhook legacy secret test')
ON CONFLICT (public_id) DO NOTHING`); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	raw := []byte("legacy-secret-1234567890")
	transport := webhook.FormatWhsecSecret(raw)
	subID := mustMintSubscriptionID(t)
	if _, err := pool.Exec(t.Context(), `
INSERT INTO event_subscriptions (public_id, workspace_id, endpoint_url, signing_secret, signing_secret_material_kind, signing_secret_key_version, event_types, status)
SELECT $1, internal_id, 'https://hooks.example.com/legacy', $2, 'plaintext', '', $3, 'active'
FROM workspaces WHERE public_id = 'ws_1'`, subID, transport, []string{events.EventV1TaskCompleted}); err != nil {
		t.Fatalf("seed legacy plaintext row: %v", err)
	}

	persistence := webhook.NewPostgresWebhookPersistence(pool)
	loaded, err := persistence.LoadSubscription(t.Context(), subID)
	if err != nil {
		t.Fatalf("LoadSubscription() legacy row error = %v", err)
	}
	if loaded == nil || string(loaded.Secret) != string(raw) {
		t.Fatal("legacy row must load the whsec_-decoded key")
	}

	var stored, kind, version string
	if err := pool.QueryRow(t.Context(), `
SELECT signing_secret, signing_secret_material_kind, signing_secret_key_version
FROM event_subscriptions WHERE public_id = $1`, subID).Scan(&stored, &kind, &version); err != nil {
		t.Fatalf("read converged row: %v", err)
	}
	if kind != secrets.MaterialKindEncrypted || stored == transport {
		t.Fatalf("legacy row must converge to ciphertext, kind=%q changed=%v", kind, stored != transport)
	}
	if version != secrets.KeyVersion() {
		t.Fatalf("converged key version = %q, want %q", version, secrets.KeyVersion())
	}

	reloaded, err := persistence.LoadSubscription(t.Context(), subID)
	if err != nil {
		t.Fatalf("LoadSubscription() converged row error = %v", err)
	}
	if string(reloaded.Secret) != string(raw) {
		t.Fatal("converged row must keep resolving the same key")
	}
}

// TestPostgresWebhookSecretRotationFailsClosed proves rows encrypted under
// a retired master key do not decode: the error carries no secret material.
func TestPostgresWebhookSecretRotationFailsClosed(t *testing.T) {
	secretstest.EnableEncryption(t)
	pool := testdb.Open(t)
	if _, err := pool.Exec(t.Context(), `
INSERT INTO workspaces (public_id, owner_user_id, name)
VALUES ('ws_1', 1, 'webhook rotation test')
ON CONFLICT (public_id) DO NOTHING`); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	persistence := webhook.NewPostgresWebhookPersistence(pool)
	raw := []byte(strings.Repeat("k", 32))
	sub := &webhook.WebhookSubscription{
		ID: mustMintSubscriptionID(t), WorkspaceID: "ws_1",
		URL: "https://hooks.example.com/x", EventTypes: []string{events.EventV1TaskCompleted},
		Secret: append([]byte(nil), raw...), Status: webhook.WebhookStatusActive,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := persistence.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v", err)
	}

	secrets.EncryptionKey = []byte(strings.Repeat("2", 32))
	_, err := persistence.LoadSubscription(t.Context(), sub.ID)
	if err == nil {
		t.Fatal("load under a rotated master key must fail closed")
	}
	if strings.Contains(err.Error(), "whsec_") {
		t.Fatalf("rotation error must not carry secret material: %v", err)
	}
}

// TestPostgresWebhookSecretWriteFailsWithoutKey proves writes fail closed
// when the master key is not configured.
func TestPostgresWebhookSecretWriteFailsWithoutKey(t *testing.T) {
	pool := testdb.Open(t)
	secrets.EncryptionKey = nil
	persistence := webhook.NewPostgresWebhookPersistence(pool)
	sub := &webhook.WebhookSubscription{
		ID: mustMintSubscriptionID(t), WorkspaceID: "ws_1",
		URL: "https://hooks.example.com/x", EventTypes: []string{events.EventV1TaskCompleted},
		Secret: []byte(strings.Repeat("k", 32)), Status: webhook.WebhookStatusActive,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := persistence.CreateSubscription(t.Context(), sub); err == nil {
		t.Fatal("CreateSubscription() without a master key must fail closed")
	}
}
