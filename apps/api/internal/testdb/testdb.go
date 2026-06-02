package testdb

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"api/internal/db/migrations"

	_ "github.com/go-sql-driver/mysql"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

func Open(t testing.TB) *sql.DB {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv("API_TEST_DATABASE_DSN"))
	if raw == "" {
		t.Skip("set API_TEST_DATABASE_DSN to run MySQL integration tests")
	}
	if cfg, err := mysqlDriver.ParseDSN(raw); err == nil {
		cfg.ParseTime = true
		cfg.MultiStatements = true
		if cfg.Loc == nil {
			cfg.Loc = time.Local
		}
		raw = cfg.FormatDSN()
	}

	db, err := sql.Open("mysql", raw)
	if err != nil {
		t.Fatalf("open mysql test db failed: %v", err)
	}
	t.Cleanup(func() {
		cleanup(t, db)
		_ = db.Close()
	})
	if err := db.Ping(); err != nil {
		t.Fatalf("ping mysql test db failed: %v", err)
	}
	if err := migrations.Up(t.Context(), db); err != nil {
		t.Fatalf("migrate mysql test db failed: %v", err)
	}
	cleanup(t, db)
	return db
}

func cleanup(t testing.TB, db *sql.DB) {
	t.Helper()
	for _, stmt := range []string{
		`DROP TRIGGER IF EXISTS fail_mark_completed`,
		`DROP TRIGGER IF EXISTS fail_mark_failed`,
		`SET FOREIGN_KEY_CHECKS = 0`,
		`TRUNCATE TABLE board_share_tokens`,
		`TRUNCATE TABLE board_members`,
		`TRUNCATE TABLE user_boards`,
		`TRUNCATE TABLE line_draft_modes`,
		`TRUNCATE TABLE system_configs`,
		`TRUNCATE TABLE channels`,
		`TRUNCATE TABLE task_daily_stat_backfills`,
		`TRUNCATE TABLE task_stat_events`,
		`TRUNCATE TABLE task_daily_stats`,
		`TRUNCATE TABLE task_outputs`,
		`TRUNCATE TABLE tasks`,
		`TRUNCATE TABLE api_accounts`,
		`TRUNCATE TABLE users_used_uids`,
		`TRUNCATE TABLE users`,
		`SET FOREIGN_KEY_CHECKS = 1`,
	} {
		if _, err := db.ExecContext(t.Context(), stmt); err != nil && !isMissingTableError(err) {
			t.Fatalf("cleanup mysql test db failed on %q: %v", stmt, err)
		}
	}
}

func isMissingTableError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1146
	}
	return strings.Contains(strings.ToLower(err.Error()), "doesn't exist")
}
