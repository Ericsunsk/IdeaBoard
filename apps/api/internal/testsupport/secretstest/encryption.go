package secretstest

import (
	"encoding/base64"
	"strings"
	"testing"

	"ideaboard/api/internal/platform/secrets"
)

func EnableEncryption(t testing.TB) {
	t.Helper()
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("1", 32)))
	t.Setenv(secrets.EncryptionKeyEnv, key)
	if err := secrets.LoadEncryptionKey(); err != nil {
		t.Fatalf("load test encryption key failed: %v", err)
	}
	t.Cleanup(func() {
		secrets.EncryptionKey = nil
	})
}
