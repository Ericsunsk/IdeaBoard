// Package secrets provides at-rest encryption for API keys and other secrets.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	EncryptionKeyEnv      = "APP_SECRETS_ENCRYPTION_KEY"
	MaterialKindEncrypted = "encrypted"
)

var (
	// EncryptionKey is loaded once from APP_SECRETS_ENCRYPTION_KEY at startup.
	// Runtime secret writes and reads require this key to be configured.
	EncryptionKey []byte
)

// LoadEncryptionKey reads the base64-encoded AES-256-GCM master key.
func LoadEncryptionKey() error {
	raw := strings.TrimSpace(os.Getenv(EncryptionKeyEnv))
	if raw == "" {
		EncryptionKey = nil
		return fmt.Errorf("%s is required", EncryptionKeyEnv)
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return fmt.Errorf("%s must be base64 encoded: %w", EncryptionKeyEnv, err)
	}
	if len(key) != 32 {
		return fmt.Errorf("%s must decode to exactly 32 bytes for AES-256-GCM", EncryptionKeyEnv)
	}
	EncryptionKey = key
	return nil
}

// IsEncryptionEnabled reports whether a master key has been configured.
func IsEncryptionEnabled() bool {
	return len(EncryptionKey) == 32
}

// KeyVersion identifies the active master key generation without exposing
// key material: the first 16 hex chars of SHA-256 over the key. Stored
// alongside each ciphertext so a rotation is detectable (a row whose
// version differs from the active key fails closed with an actionable
// error instead of silently falling back to another key).
func KeyVersion() string {
	if !IsEncryptionEnabled() {
		return ""
	}
	sum := sha256.Sum256(EncryptionKey)
	return hex.EncodeToString(sum[:])[:16]
}

func RequireEncryptionEnabled() error {
	if !IsEncryptionEnabled() {
		return errors.New("加密密钥未配置")
	}
	return nil
}

// Encrypt encrypts plaintext with AES-256-GCM using the configured master key.
// Returns the base64-encoded ciphertext (nonce + ciphertext).
func Encrypt(plaintext []byte) (string, error) {
	if err := RequireEncryptionEnabled(); err != nil {
		return "", err
	}
	block, err := aes.NewCipher(EncryptionKey)
	if err != nil {
		return "", fmt.Errorf("创建 AES cipher 失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建 GCM 失败: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成 nonce 失败: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func EncryptString(plaintext string) (string, error) {
	return Encrypt([]byte(plaintext))
}

// Decrypt decrypts a base64-encoded ciphertext produced by Encrypt.
func Decrypt(encoded string) ([]byte, error) {
	if err := RequireEncryptionEnabled(); err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("解码密文失败: %w", err)
	}
	block, err := aes.NewCipher(EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("创建 AES cipher 失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("密文数据不完整")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func DecryptString(encoded string) (string, error) {
	plaintext, err := Decrypt(encoded)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func DecryptMaterialString(payload, materialKind string) (string, error) {
	if strings.TrimSpace(materialKind) != MaterialKindEncrypted {
		return "", fmt.Errorf("unsupported secret material kind %q", materialKind)
	}
	if strings.TrimSpace(payload) == "" {
		return "", errors.New("secret payload is empty")
	}
	return DecryptString(payload)
}
