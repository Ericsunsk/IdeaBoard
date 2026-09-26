package events

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/platform/secrets"
	"ideaboard/api/internal/testsupport/secretstest"
)

// The storage envelope must follow the Stripe-credential precedent:
// ciphertext at rest, whsec_ only as the encrypted-then-parsed transport
// form, and fail-closed reads for anything else.

func TestEncodeStoredWebhookSecretRequiresKey(t *testing.T) {
	secrets.EncryptionKey = nil
	if _, _, _, err := encodeStoredWebhookSecret([]byte(strings.Repeat("k", 32))); err == nil {
		t.Fatal("encode without a master key must fail closed")
	}
}

func TestWebhookSecretEnvelopeRoundTrip(t *testing.T) {
	secretstest.EnableEncryption(t)
	raw := []byte(strings.Repeat("r", 32))

	payload, kind, keyVersion, err := encodeStoredWebhookSecret(raw)
	if err != nil {
		t.Fatalf("encodeStoredWebhookSecret() error = %v", err)
	}
	if kind != secrets.MaterialKindEncrypted {
		t.Fatalf("material kind = %q, want %q", kind, secrets.MaterialKindEncrypted)
	}
	if keyVersion != secrets.KeyVersion() || len(keyVersion) != 16 {
		t.Fatalf("key version = %q, want the 16-hex active key fingerprint", keyVersion)
	}
	if strings.HasPrefix(strings.TrimSpace(payload), webhookSecretPrefix) {
		t.Fatal("stored payload must be ciphertext, not whsec_ plaintext")
	}
	if _, ok := ParseWhsecSecret([]byte(payload)); ok {
		t.Fatal("stored ciphertext must not parse as a whsec_ secret")
	}

	decoded, err := decodeStoredWebhookSecret(payload, kind, keyVersion)
	if err != nil {
		t.Fatalf("decodeStoredWebhookSecret() error = %v", err)
	}
	if string(decoded) != string(raw) {
		t.Fatal("decoded key must equal the stored raw key")
	}

	// The decoded key signs deliveries verifiable under the whsec_ transport form.
	body := []byte(`{"id":"evt_1"}`)
	now := time.Now().UTC()
	headers := SignWebhook(decoded, body, "evt_1", now)
	if err := VerifyWebhookSignature([]byte(FormatWhsecSecret(raw)), body, headers, now); err != nil {
		t.Fatalf("sign/verify across the envelope failed: %v", err)
	}
}

func TestDecodeStoredWebhookSecretLegacyPlaintext(t *testing.T) {
	secretstest.EnableEncryption(t)
	raw := []byte("legacy-secret-1234567890")
	transport := FormatWhsecSecret(raw)

	decoded, err := decodeStoredWebhookSecret(transport, webhookSecretMaterialPlaintext, "")
	if err != nil {
		t.Fatalf("legacy plaintext row must decode: %v", err)
	}
	if string(decoded) != string(raw) {
		t.Fatal("legacy decode must equal the whsec_-decoded key")
	}
}

func TestDecodeStoredWebhookSecretRejects(t *testing.T) {
	secretstest.EnableEncryption(t)
	raw := []byte(strings.Repeat("r", 32))
	payload, kind, _, err := encodeStoredWebhookSecret(raw)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct{ payload, kind string }{
		"unknown kind":        {payload, "stored"},
		"empty kind":          {payload, ""},
		"empty payload":       {"", kind},
		"tampered ciphertext": {payload[:len(payload)-4] + "AAAA", kind},
		"plaintext non-whsec": {"legacy-raw-secret-1234567890", webhookSecretMaterialPlaintext},
		"plaintext malformed": {"whsec_not-valid-base64!!!", webhookSecretMaterialPlaintext},
	}
	for name, tc := range cases {
		if _, err := decodeStoredWebhookSecret(tc.payload, tc.kind, secrets.KeyVersion()); err == nil {
			t.Fatalf("%s must fail closed", name)
		}
	}
}

func TestDecodeStoredWebhookSecretWrongKeyFailsClosed(t *testing.T) {
	secretstest.EnableEncryption(t)
	raw := []byte(strings.Repeat("r", 32))
	payload, kind, _, err := encodeStoredWebhookSecret(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Rotate the process key: rows under the retired key must not decode.
	secrets.EncryptionKey = []byte(strings.Repeat("2", 32))
	_, err = decodeStoredWebhookSecret(payload, kind, "wrong-version")
	if err == nil {
		t.Fatal("decode under a rotated master key must fail closed")
	}
	if msg := err.Error(); strings.Contains(msg, base64.StdEncoding.EncodeToString(raw)) {
		t.Fatal("decode errors must never carry secret material")
	}
}

func TestWebhookSecretKeyVersionStableAndDistinct(t *testing.T) {
	secretstest.EnableEncryption(t)
	first := secrets.KeyVersion()
	second := secrets.KeyVersion()
	if first == "" || first != second || len(first) != 16 {
		t.Fatalf("key version must be a stable 16-hex fingerprint, got %q", first)
	}
	secrets.EncryptionKey = []byte(strings.Repeat("9", 32))
	if got := secrets.KeyVersion(); got == first {
		t.Fatal("a rotated master key must produce a distinct version")
	}
	if got := secrets.KeyVersion(); len(got) != 16 {
		t.Fatalf("rotated key version = %q, want 16 hex chars", got)
	}
}

func TestMaskedSubscriptionViewsCarryNoSecret(t *testing.T) {
	raw := []byte("view-secret-1234567890")
	transport := FormatWhsecSecret(raw)
	sub := &WebhookSubscription{
		ID: "sub_view00000001", WorkspaceID: "ws_1", URL: "https://hooks.example.com/x",
		EventTypes: []string{EventV1TaskCompleted}, Secret: raw,
		Status: WebhookStatusActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		Delivered: map[string]struct{}{},
	}
	view := maskSubscription(sub)
	if !view.HasSecret {
		t.Fatal("masked view must report has_secret")
	}
	serialized, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{transport, string(raw), base64.StdEncoding.EncodeToString(raw)} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("masked view leaks secret material: %s", serialized)
		}
	}
}
