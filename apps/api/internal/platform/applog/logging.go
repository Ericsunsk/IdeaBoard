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
	var inner slog.Handler
	if isProductionEnv(appEnv) {
		inner = slog.NewJSONHandler(w, opts)
	} else {
		inner = slog.NewTextHandler(w, opts)
	}
	// PII redaction covers every environment: development logs are the most
	// likely to be pasted into issues and chat.
	return NewRedactingHandler(inner)
}

func isProductionEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production", "release":
		return true
	default:
		return false
	}
}
