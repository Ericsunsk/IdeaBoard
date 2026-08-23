package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/runtime/bootstrap"
)

const (
	atlasBinary   = "/app/atlas"
	migrationsDir = "file:///app/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Migrate(ctx, config.Load(), atlasBinary, migrationsDir); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
