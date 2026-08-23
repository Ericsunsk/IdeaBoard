package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
)

const defaultMigrationDirectory = "file://internal/db/migrations"

// Migrate applies Atlas migrations with the schema-owner role, then performs
// the runtime-only River and privilege finalization step.
func Migrate(ctx context.Context, cfg *config.Config, atlasBinary, migrationsDirectory string) error {
	if cfg == nil {
		return fmt.Errorf("migration config is nil")
	}
	migrationConfig, err := separatedMigrationDatabaseConfig(cfg.Database)
	if err != nil {
		return err
	}
	if strings.TrimSpace(atlasBinary) == "" {
		atlasBinary = "atlas"
	}
	if strings.TrimSpace(migrationsDirectory) == "" {
		migrationsDirectory = defaultMigrationDirectory
	}
	cmd := exec.CommandContext(ctx, atlasBinary, "migrate", "apply", "--dir", migrationsDirectory, "--url", database.PostgresDSN(migrationConfig))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("atlas schema migration failed: %w", err)
	}
	if err := finalizeAppDatabases(ctx, cfg); err != nil {
		return fmt.Errorf("runtime migration finalization failed: %w", err)
	}
	return nil
}
