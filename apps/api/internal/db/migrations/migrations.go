package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var migrationFS embed.FS

func Up(ctx context.Context, db *sql.DB) (err error) {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	release, err := acquireMigrationLock(ctx, db)
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := release(context.Background()); err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()

	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version BIGINT NOT NULL PRIMARY KEY,
  applied_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
)`); err != nil {
		return err
	}

	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, file := range files {
		version, err := migrationVersion(file)
		if err != nil {
			return err
		}
		applied, err := migrationApplied(ctx, db, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		raw, err := migrationFS.ReadFile(file)
		if err != nil {
			return err
		}
		statements := splitSQLStatements(extractUpSQL(string(raw)))
		if len(statements) == 0 {
			return fmt.Errorf("migration %s has no statements", file)
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply migration %s: %w", file, err)
			}
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			return fmt.Errorf("record migration %s: %w", file, err)
		}
	}
	return nil
}

func acquireMigrationLock(ctx context.Context, db *sql.DB) (func(context.Context) error, error) {
	const lockName = "ideaboard_schema_migrations"
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open migration lock connection: %w", err)
	}
	releaseConn := func() {
		_ = conn.Close()
	}

	var locked sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 30)`, lockName).Scan(&locked); err != nil {
		releaseConn()
		return nil, fmt.Errorf("acquire migration lock: %w", err)
	}
	if !locked.Valid || locked.Int64 != 1 {
		releaseConn()
		return nil, fmt.Errorf("acquire migration lock: lock %q is busy", lockName)
	}
	return func(ctx context.Context) error {
		defer releaseConn()
		var released sql.NullInt64
		if err := conn.QueryRowContext(ctx, `SELECT RELEASE_LOCK(?)`, lockName).Scan(&released); err != nil {
			return fmt.Errorf("release migration lock: %w", err)
		}
		if !released.Valid || released.Int64 != 1 {
			return fmt.Errorf("release migration lock: lock %q was not held", lockName)
		}
		return nil
	}, nil
}

func migrationVersion(file string) (int64, error) {
	name := path.Base(file)
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration %s must start with numeric version", file)
	}
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse migration version %s: %w", file, err)
	}
	return version, nil
}

func migrationApplied(ctx context.Context, db *sql.DB, version int64) (bool, error) {
	var count int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func extractUpSQL(raw string) string {
	raw = strings.Replace(raw, "-- migrate:up", "", 1)
	if before, _, ok := strings.Cut(raw, "-- migrate:down"); ok {
		return before
	}
	return raw
}

func splitSQLStatements(raw string) []string {
	parts := strings.Split(raw, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		if meaningfulSQL(stmt) == "" {
			continue
		}
		statements = append(statements, stmt)
	}
	return statements
}

func meaningfulSQL(stmt string) string {
	lines := strings.Split(stmt, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "\n")
}
