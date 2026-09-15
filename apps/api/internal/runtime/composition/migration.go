package composition

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"

	"github.com/jackc/pgx/v5"
)

const defaultMigrationDirectory = "file://internal/db/migrations"

// Migrate applies Atlas migrations with the schema-owner role, then performs
// the runtime-only River and privilege finalization step.
func Migrate(ctx context.Context, cfg *config.Config, atlasBinary, migrationsDirectory string) error {
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
		atlasBinary = "atlas"
	}
	if strings.TrimSpace(migrationsDirectory) == "" {
		migrationsDirectory = defaultMigrationDirectory
	}
	if err := reconcileCleanBreakDatabase(ctx, migrationConfig, migrationsDirectory); err != nil {
		return fmt.Errorf("clean break reconciliation failed: %w", err)
	}
	cmd := exec.CommandContext(ctx, atlasBinary, "migrate", "apply", "--dir", migrationsDirectory, "--url", database.PostgresDSN(migrationConfig))
	var output bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &output)
	cmd.Stderr = io.MultiWriter(os.Stderr, &output)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("atlas schema migration failed: %w (output: %s)", err, strings.TrimSpace(output.String()))
	}
	if err := finalizeAppDatabases(ctx, cfg); err != nil {
		return fmt.Errorf("runtime migration finalization failed: %w", err)
	}
	return nil
}

func reconcileCleanBreakDatabase(ctx context.Context, migrationConfig config.DatabaseConfig, migrationsDirectory string) error {
	db, err := database.NewPostgres(ctx, migrationConfig)
	if err != nil {
		return fmt.Errorf("connect migration database: %w", err)
	}
	defer db.Close()

	var revisionsTable sql.NullString
	err = db.QueryRowContext(ctx, `
		SELECT COALESCE(
			to_regclass('atlas_schema_revisions.atlas_schema_revisions')::text,
			to_regclass('public.atlas_schema_revisions')::text
		)
	`).Scan(&revisionsTable)
	if err != nil {
		return fmt.Errorf("check revisions table: %w", err)
	}
	if !revisionsTable.Valid || revisionsTable.String == "" {
		return nil
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT version::text FROM %s ORDER BY version ASC", revisionsTable.String))
	if err != nil {
		return fmt.Errorf("query atlas schema revisions: %w", err)
	}
	defer rows.Close()

	var recordedVersions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err == nil && v != "" {
			recordedVersions = append(recordedVersions, v)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate atlas schema revisions: %w", err)
	}
	if len(recordedVersions) == 0 {
		return nil
	}

	dirPath := strings.TrimPrefix(migrationsDirectory, "file://")
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("read migrations directory %s: %w", dirPath, err)
	}

	availableVersions := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".sql") {
			parts := strings.SplitN(name, "_", 2)
			if len(parts) > 0 {
				availableVersions[parts[0]] = true
			}
		}
	}

	hasMissingRevisions := false
	for _, rec := range recordedVersions {
		if !availableVersions[rec] {
			hasMissingRevisions = true
			break
		}
	}

	if hasMissingRevisions {
		owner := strings.TrimSpace(migrationConfig.User)
		if owner == "" {
			owner = "ideaboard"
		}
		sanitizedOwner := pgx.Identifier{owner}.Sanitize()
		stmt := fmt.Sprintf(`
			DROP SCHEMA IF EXISTS atlas_schema_revisions CASCADE;
			DROP TABLE IF EXISTS public.atlas_schema_revisions CASCADE;
			DROP SCHEMA public CASCADE;
			CREATE SCHEMA public;
			ALTER SCHEMA public OWNER TO %s;
			GRANT ALL ON SCHEMA public TO %s;
			GRANT ALL ON SCHEMA public TO public;
		`, sanitizedOwner, sanitizedOwner)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("reset clean break schema: %w", err)
		}
	}

	return nil
}
