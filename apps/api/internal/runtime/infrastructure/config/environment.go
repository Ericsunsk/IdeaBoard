package configinfra

import (
	"fmt"
	"os"
	"strings"

	"ideaboard/api/internal/platform/applog"
	"ideaboard/api/internal/platform/config"
)

const RuntimeUploadDir = "uploads"

// ConfigureRuntimeLogging applies logging configuration based on environment.
func ConfigureRuntimeLogging(cfg *config.Config) {
	if cfg == nil {
		return
	}
	applog.Configure(cfg.App.Env)
}

// EnsureRuntimeUploadDir ensures the upload directory exists with restricted permissions.
func EnsureRuntimeUploadDir() error {
	if err := os.MkdirAll(RuntimeUploadDir, 0700); err != nil {
		return fmt.Errorf("failed to ensure runtime upload directory %q: %w", RuntimeUploadDir, err)
	}
	return nil
}

// ValidateAuthInternalKey verifies that production environments supply a non-placeholder auth key.
func ValidateAuthInternalKey(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if !IsProductionRuntime(cfg.App.Env) {
		return nil
	}
	internalKey := strings.TrimSpace(cfg.Auth.InternalAPIKey)
	if internalKey == "" {
		return fmt.Errorf("AUTH_INTERNAL_KEY must be set when APP_ENV=production")
	}
	if IsUnsafeProductionSecret(internalKey) {
		return fmt.Errorf("AUTH_INTERNAL_KEY must not use placeholder or development values in production")
	}
	// DESIGN.md requires a high-entropy secret. 32 chars is the floor for a
	// generated token (e.g. 256-bit hex is 64); shorter values are almost
	// certainly human-chosen and brute-forceable.
	if len(internalKey) < 32 {
		return fmt.Errorf("AUTH_INTERNAL_KEY must be at least 32 characters when APP_ENV=production")
	}
	return nil
}

// IsProductionRuntime checks if the environment string matches production flags.
func IsProductionRuntime(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production", "release":
		return true
	default:
		return false
	}
}

// IsUnsafeProductionSecret detects dangerous placeholder secrets.
func IsUnsafeProductionSecret(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return true
	}
	if strings.HasPrefix(normalized, "replace-with") {
		return true
	}
	if strings.Contains(normalized, "change-before-production") {
		return true
	}
	switch normalized {
	case "changeme", "change-me", "password", "secret":
		return true
	default:
		return false
	}
}
