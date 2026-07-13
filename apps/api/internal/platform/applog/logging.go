package applog

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Configure installs the process-wide slog logger.
func Configure(appEnv string) {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	handler := newHandler(io.Writer(os.Stderr), appEnv, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
}

func newHandler(w io.Writer, appEnv string, opts *slog.HandlerOptions) slog.Handler {
	if isProductionEnv(appEnv) {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}

func isProductionEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production", "release":
		return true
	default:
		return false
	}
}
