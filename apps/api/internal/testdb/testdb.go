package testdb

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"api/internal/db/migrations"
	"api/internal/platform/database"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(t testing.TB) *sql.DB {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv("API_TEST_DATABASE_DSN"))
	if raw == "" {
		t.Skip("set API_TEST_DATABASE_DSN to run PostgreSQL integration tests")
	}

	db, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatalf("open PostgreSQL test db failed: %v", err)
	}
	t.Cleanup(func() {
		cleanup(t, db)
		_ = db.Close()
	})
	if err := db.Ping(); err != nil {
		t.Fatalf("ping PostgreSQL test db failed: %v", err)
	}
	if err := migrations.Up(t.Context(), db); err != nil {
		t.Fatalf("migrate PostgreSQL test db failed: %v", err)
	}
	cleanup(t, db)
	return db
}

func cleanup(t testing.TB, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `
DROP TRIGGER IF EXISTS fail_mark_completed ON tasks;
DROP TRIGGER IF EXISTS fail_mark_failed ON tasks;
DROP FUNCTION IF EXISTS fail_mark_completed();
DROP FUNCTION IF EXISTS fail_mark_failed();
TRUNCATE TABLE
  board_share_tokens,
  board_members,
  user_boards,
  line_draft_modes,
  system_configs,
  channels,
  task_daily_stat_backfills,
  task_cancellations,
  task_stat_events,
  task_daily_stats,
  task_outputs,
  tasks,
  api_accounts,
  users_used_uids,
  users
RESTART IDENTITY CASCADE;
`)
	if err != nil && !database.IsMissingTableError(err) {
		t.Fatalf("cleanup PostgreSQL test db failed: %v", err)
	}
}
