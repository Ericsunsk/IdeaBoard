package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadEncryptionKeyRequiresBase64Encoded32Bytes(t *testing.T) {
	t.Cleanup(func() {
		EncryptionKey = nil
	})

	t.Setenv(EncryptionKeyEnv, "")
	if err := LoadEncryptionKey(); err == nil {
		t.Fatal("LoadEncryptionKey should require a configured key")
	}

	t.Setenv(EncryptionKeyEnv, "not-base64")
	if err := LoadEncryptionKey(); err == nil {
		t.Fatal("LoadEncryptionKey should reject invalid base64")
	}

	t.Setenv(EncryptionKeyEnv, base64.StdEncoding.EncodeToString([]byte("short")))
	if err := LoadEncryptionKey(); err == nil {
		t.Fatal("LoadEncryptionKey should reject non-32-byte keys")
	}

	t.Setenv(EncryptionKeyEnv, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err := LoadEncryptionKey(); err != nil {
		t.Fatalf("LoadEncryptionKey returned error for valid key: %v", err)
	}
}

func TestEncryptDecryptStringRoundTrip(t *testing.T) {
	t.Cleanup(func() {
		EncryptionKey = nil
	})
	t.Setenv(EncryptionKeyEnv, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err := LoadEncryptionKey(); err != nil {
		t.Fatalf("LoadEncryptionKey returned error: %v", err)
	}

	encrypted, err := EncryptString("runtime-secret")
	if err != nil {
		t.Fatalf("EncryptString returned error: %v", err)
	}
	if encrypted == "runtime-secret" {
		t.Fatal("encrypted payload should not equal plaintext")
	}

	plain, err := DecryptMaterialString(encrypted, MaterialKindEncrypted)
	if err != nil {
		t.Fatalf("DecryptMaterialString returned error: %v", err)
	}
	if plain != "runtime-secret" {
		t.Fatalf("plain = %q, want runtime-secret", plain)
	}
}

func TestDecryptMaterialStringRejectsUnsupportedKind(t *testing.T) {
	if _, err := DecryptMaterialString("payload", "stored"); err == nil {
		t.Fatal("DecryptMaterialString should reject unsupported material kind")
	}
}
