// Package migration owns the database migration lifecycle for the Runtime.
package migration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	dbprivileges "ideaboard/api/internal/platform/database/privileges"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Runner applies Atlas, River and runtime privileges in that order. Composition
// supplies the queue migration adapter and the owning contexts' privilege policies.
type Runner struct {
	migrateRiver func(context.Context, *pgxpool.Pool) error
	policies     []dbprivileges.Policy
}

func New(migrateRiver func(context.Context, *pgxpool.Pool) error, policies ...dbprivileges.Policy) *Runner {
	return &Runner{migrateRiver: migrateRiver, policies: append([]dbprivileges.Policy(nil), policies...)}
}

// Migrate applies Atlas migrations with the schema-owner role, then performs
// the runtime-only River and privilege finalization step.
func (r *Runner) Migrate(ctx context.Context, cfg *config.Config, atlasBinary, migrationsDirectory string) error {
	if cfg == nil {
		return fmt.Errorf("migration config is nil")
	}
	if err := cfg.ValidateDatabase(); err != nil {
		return fmt.Errorf("invalid database configuration: %w", err)
	}
	migrationConfig, err := separatedMigrationDatabaseConfig(cfg.Database)
	if err != nil {
		return err
	}
	if strings.TrimSpace(atlasBinary) == "" {
		atlasBinary = config.DefaultAtlasBinary
	}
	if strings.TrimSpace(migrationsDirectory) == "" {
		migrationsDirectory = config.DefaultMigrationDirectory
	}
	if err := reconcileCleanBreakDatabase(ctx, migrationConfig, migrationsDirectory, cfg.Migration.CleanBreakAllowReset); err != nil {
		return fmt.Errorf("clean break reconciliation failed: %w", err)
	}
	cmd := exec.CommandContext(ctx, atlasBinary, "migrate", "apply", "--dir", migrationsDirectory, "--url", database.PostgresDSN(migrationConfig))
	var output bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &output)
	cmd.Stderr = io.MultiWriter(os.Stderr, &output)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("atlas schema migration failed: %w (output: %s)", err, strings.TrimSpace(output.String()))
	}
	if err := r.Finalize(ctx, cfg); err != nil {
		return fmt.Errorf("runtime migration finalization failed: %w", err)
	}
	return nil
}

// Finalize migrates River before configuring runtime privileges with the owner role.
func (r *Runner) Finalize(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("database config is nil")
	}
	if r.migrateRiver == nil {
		return fmt.Errorf("river migration is not configured")
	}
	migrationConfig, err := separatedMigrationDatabaseConfig(cfg.Database)
	if err != nil {
		return err
	}
	migrationDB, err := database.NewPostgres(ctx, migrationConfig)
	if err != nil {
		return fmt.Errorf("failed to connect migration database: %w", err)
	}
	defer migrationDB.Close()
	if err := r.migrateRiver(ctx, migrationDB); err != nil {
		return fmt.Errorf("failed to migrate River schema: %w", err)
	}
	privilegeTx, err := migrationDB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin runtime privilege transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = privilegeTx.Rollback(ctx)
		}
	}()
	if err := dbprivileges.Configure(ctx, privilegeTx, cfg.Database.User, r.policies...); err != nil {
		return err
	}
	if err := privilegeTx.Commit(ctx); err != nil {
		return fmt.Errorf("commit runtime privileges: %w", err)
	}
	committed = true
	return nil
}

func separatedMigrationDatabaseConfig(runtimeConfig config.DatabaseConfig) (config.DatabaseConfig, error) {
	migrationUser := strings.TrimSpace(runtimeConfig.MigrationUser)
	if migrationUser == "" || runtimeConfig.MigrationPassword == "" {
		return config.DatabaseConfig{}, fmt.Errorf("PG_MIGRATION_USER and PG_MIGRATION_PASSWORD are required for migrations")
	}
	if migrationUser == strings.TrimSpace(runtimeConfig.User) {
		return config.DatabaseConfig{}, fmt.Errorf("PG_MIGRATION_USER must differ from API_PG_USER so runtime cannot own schema objects")
	}
	migrationConfig := runtimeConfig
	migrationConfig.User = migrationUser
	migrationConfig.Password = runtimeConfig.MigrationPassword
	return migrationConfig, nil
}
