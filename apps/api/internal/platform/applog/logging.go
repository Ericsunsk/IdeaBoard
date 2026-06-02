package applog

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Configure installs the process-wide slog logger.
func Configure(logFilePath, appEnv string) error {
	writer := io.Writer(os.Stderr)
	var openErr error

	logPath := strings.TrimSpace(logFilePath)
	if logPath == "" {
		logPath = "server.log"
	}
	if !strings.EqualFold(logPath, "none") {
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			openErr = err
		} else {
			writer = io.MultiWriter(os.Stderr, logFile)
		}
	}

	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	handler := newHandler(writer, appEnv, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return openErr
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
