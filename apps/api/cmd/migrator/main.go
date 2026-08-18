package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

const (
	atlasBinary   = "/app/atlas"
	serverBinary  = "/app/ideaboard-server"
	migrationsDir = "file:///app/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn, err := migrationDSN()
	if err != nil {
		fatal(err)
	}

	if err := run(ctx, atlasBinary, "migrate", "apply", "--dir", migrationsDir, "--url", dsn); err != nil {
		fatal(fmt.Errorf("atlas schema migration failed: %w", err))
	}
	if err := run(ctx, serverBinary, "finalize-migrate"); err != nil {
		fatal(fmt.Errorf("runtime migration finalization failed: %w", err))
	}
}

func migrationDSN() (string, error) {
	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	port := strings.TrimSpace(os.Getenv("DB_PORT"))
	user := strings.TrimSpace(os.Getenv("DB_MIGRATION_USER"))
	password := os.Getenv("DB_MIGRATION_PASSWORD")
	database := strings.TrimSpace(os.Getenv("DB_NAME"))
	sslMode := strings.TrimSpace(os.Getenv("DB_SSLMODE"))
	if host == "" || port == "" || user == "" || password == "" || database == "" {
		return "", fmt.Errorf("DB_HOST, DB_PORT, DB_MIGRATION_USER, DB_MIGRATION_PASSWORD, and DB_NAME are required")
	}
	if sslMode == "" {
		sslMode = "disable"
	}
	query := url.Values{}
	query.Set("sslmode", sslMode)
	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password),
		Host:     host + ":" + port,
		Path:     "/" + database,
		RawQuery: query.Encode(),
	}).String(), nil
}

func run(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
