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
	"time"
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
  applied_at TIMESTAMPTZ(3) NOT NULL DEFAULT now()
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
		if err := applyMigration(ctx, db, file, statements, version); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, file string, statements []string, version int64) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", file, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply migration %s: %w", file, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return fmt.Errorf("record migration %s: %w", file, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", file, err)
	}
	committed = true
	return nil
}

func acquireMigrationLock(ctx context.Context, db *sql.DB) (func(context.Context) error, error) {
	const (
		lockKey           int64 = 7341278127769961450
		lockTimeout             = 30 * time.Second
		lockRetryInterval       = 250 * time.Millisecond
	)
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open migration lock connection: %w", err)
	}
	releaseConn := func() {
		_ = conn.Close()
	}

	deadline := time.Now().Add(lockTimeout)
	for {
		var locked bool
		if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&locked); err != nil {
			releaseConn()
			return nil, fmt.Errorf("acquire migration lock: %w", err)
		}
		if locked {
			break
		}
		if time.Now().After(deadline) {
			releaseConn()
			return nil, fmt.Errorf("acquire migration lock: lock %d is busy", lockKey)
		}
		sleep := lockRetryInterval
		if remaining := time.Until(deadline); remaining < sleep {
			sleep = remaining
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			releaseConn()
			return nil, fmt.Errorf("acquire migration lock: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return func(ctx context.Context) error {
		defer releaseConn()
		var released bool
		if err := conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1)`, lockKey).Scan(&released); err != nil {
			return fmt.Errorf("release migration lock: %w", err)
		}
		if !released {
			return fmt.Errorf("release migration lock: lock %d was not held", lockKey)
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
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = $1`, version).Scan(&count); err != nil {
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
	const (
		blockStart = "-- statement:begin"
		blockEnd   = "-- statement:end"
	)
	var statements []string
	var regular strings.Builder
	for len(raw) > 0 {
		start := strings.Index(raw, blockStart)
		if start < 0 {
			regular.WriteString(raw)
			break
		}
		regular.WriteString(raw[:start])
		statements = append(statements, splitSemicolonStatements(regular.String())...)
		regular.Reset()

		blockBodyStart := start + len(blockStart)
		rest := raw[blockBodyStart:]
		end := strings.Index(rest, blockEnd)
		if end < 0 {
			regular.WriteString(raw[start:])
			break
		}
		block := strings.TrimSpace(rest[:end])
		if meaningfulSQL(block) != "" {
			statements = append(statements, block)
		}
		raw = rest[end+len(blockEnd):]
	}
	statements = append(statements, splitSemicolonStatements(regular.String())...)
	return statements
}

func splitSemicolonStatements(raw string) []string {
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
