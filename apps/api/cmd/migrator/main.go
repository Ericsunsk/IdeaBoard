package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/composition/wiring"
)

const (
	atlasBinary   = "/app/atlas"
	migrationsDir = "file:///app/migrations"
)

// This binary intentionally duplicates the migrate path instead of invoking
// the main server image: it ships in a minimal init-job image with only the
// atlas binary and migration files. Keep it flag-free; all migration policy
// lives in wiring.Migrate.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadAndValidate()
	if err != nil {
		fatal(err)
	}
	if err := wiring.Migrate(ctx, cfg, atlasBinary, migrationsDir); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
