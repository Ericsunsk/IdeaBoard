package events

import (
	"fmt"
	"strings"

	"ideaboard/api/internal/platform/secrets"
)

// Outbound webhook signing-secret storage envelope (Security H1).
//
// The event_subscriptions.signing_secret column is dual-use during the
// encryption cutover:
//
//   - material_kind='encrypted': payload is AES-256-GCM ciphertext (via
//     APP_SECRETS_ENCRYPTION_KEY) over the canonical whsec_ transport
//     string. This is the only form new writes produce.
//   - material_kind='plaintext': pre-migration rows holding the reversible
//     whsec_ string directly. Accepted on read only so deliveries survive
//     the cutover; every successful plaintext read triggers a best-effort
//     re-encrypt-on-read that converges the row to 'encrypted'.
//
// signing_secret_key_version records secrets.KeyVersion() (a SHA-256
// fingerprint of the active master key, never key material) so a master
// key rotation is detectable: rows encrypted under a retired key fail
// closed instead of verifying against the wrong key.
const webhookSecretMaterialPlaintext = "plaintext"

// encodeStoredWebhookSecret encrypts raw key bytes for persistence.
// The ciphertext covers the canonical whsec_ transport string, so the
// decrypted value feeds the same ParseWhsecSecret path as legacy rows.
// Fails closed when the master key is not configured.
func encodeStoredWebhookSecret(raw []byte) (payload, materialKind, keyVersion string, err error) {
	if err := secrets.RequireEncryptionEnabled(); err != nil {
		return "", "", "", err
	}
	encrypted, err := secrets.EncryptString(FormatWhsecSecret(raw))
	if err != nil {
		return "", "", "", err
	}
	return encrypted, secrets.MaterialKindEncrypted, secrets.KeyVersion(), nil
}

// decodeStoredWebhookSecret resolves raw HMAC key bytes from a persisted
// (payload, materialKind) pair. Encrypted rows decrypt then parse the
// whsec_ form; legacy 'plaintext' rows parse directly. Anything else —
// unknown kinds, undecryptable payloads, malformed whsec_ — fails closed.
// The returned error never carries secret material.
func decodeStoredWebhookSecret(payload, materialKind, keyVersion string) ([]byte, error) {
	switch strings.TrimSpace(materialKind) {
	case secrets.MaterialKindEncrypted:
		if err := secrets.RequireEncryptionEnabled(); err != nil {
			return nil, fmt.Errorf("events: webhook signing secret unavailable: %w", err)
		}
		persistedVersion := strings.TrimSpace(keyVersion)
		activeVersion := secrets.KeyVersion()
		if persistedVersion == "" || activeVersion == "" || persistedVersion != activeVersion {
			return nil, fmt.Errorf("events: webhook signing secret key version mismatch")
		}
		transport, err := secrets.DecryptMaterialString(payload, materialKind)
		if err != nil {
			return nil, fmt.Errorf("events: webhook signing secret unavailable: %w", err)
		}
		raw, ok := ParseWhsecSecret([]byte(transport))
		if !ok {
			return nil, fmt.Errorf("events: persisted webhook secret is malformed")
		}
		return raw, nil
	case webhookSecretMaterialPlaintext:
		raw, ok := ParseWhsecSecret([]byte(payload))
		if !ok {
			return nil, fmt.Errorf("events: persisted webhook secret is malformed")
		}
		return raw, nil
	default:
		return nil, fmt.Errorf("events: unsupported webhook secret material kind %q", strings.TrimSpace(materialKind))
	}
}
