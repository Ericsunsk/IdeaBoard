package bootstrap

import (
	"log/slog"
	"strings"

	"ideaboard/api/internal/platform/applog"
	"ideaboard/api/internal/platform/config"
)

func configureRuntimeLogging(cfg *config.Config) {
	if cfg == nil {
		return
	}
	if err := applog.Configure(cfg.App.LogFile, cfg.App.Env); err != nil {
		slog.Warn("failed to open log file", "path", cfg.App.LogFile, "err", err)
	}
}

func requireAuthInternalKeyForProduction(cfg *config.Config) {
	if cfg == nil {
		return
	}
	if !isProductionRuntime(cfg.App.Env) {
		return
	}
	internalKey := strings.TrimSpace(cfg.Auth.InternalAPIKey)
	if internalKey == "" {
		fatal("AUTH_INTERNAL_KEY must be set when APP_ENV=production")
	}
	if isUnsafeProductionSecret(internalKey) {
		fatal("AUTH_INTERNAL_KEY must not use placeholder or development values in production")
	}
}

func isProductionRuntime(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production", "release":
		return true
	default:
		return false
	}
}

func isUnsafeProductionSecret(value string) bool {
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
