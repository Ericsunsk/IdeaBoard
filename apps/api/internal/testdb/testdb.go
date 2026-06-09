package testdb

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/db/migrations"
	"ideaboard/api/internal/platform/database"

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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, `
DROP TRIGGER IF EXISTS fail_mark_completed ON tasks;
DROP TRIGGER IF EXISTS fail_mark_failed ON tasks;
DROP FUNCTION IF EXISTS fail_mark_completed();
DROP FUNCTION IF EXISTS fail_mark_failed();
TRUNCATE TABLE
  storage_profile_secrets,
  storage_profiles,
  feature_prompt_revisions,
  feature_prompts,
  media_refs,
  media_assets,
  task_attempts,
  api_account_secrets,
  api_account_models,
  board_share_tokens,
  board_members,
  user_boards,
  line_draft_modes,
  system_configs,
  channels,
  task_daily_stat_backfills,
  task_cancellations,
  task_execution_metrics,
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
	seedDefaults(t, db)
}

func seedDefaults(t testing.TB, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, `
INSERT INTO storage_profiles (profile_id, name, provider, enabled, is_current, builtin, status, config)
VALUES ('builtin-local', '本地存储', 'local', TRUE, TRUE, TRUE, 'active', '{}'::jsonb)
ON CONFLICT (profile_id) DO NOTHING;
`)
	if err != nil && !database.IsMissingTableError(err) {
		t.Fatalf("seed PostgreSQL test defaults failed: %v", err)
	}
	seedTaskFixtureUsers(t, db)
}

func seedTaskFixtureUsers(t testing.TB, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, `
WITH seed_users (id, username, storage_uid) AS (
  VALUES
    (1, 'test-fixture-user-1', 'T00001'),
    (3, 'test-fixture-user-3', 'T00003'),
    (7, 'test-fixture-user-7', 'T00007'),
    (8, 'test-fixture-user-8', 'T00008'),
    (10, 'test-fixture-user-10', 'T00010'),
    (11, 'test-fixture-user-11', 'T00011'),
    (12, 'test-fixture-user-12', 'T00012'),
    (42, 'test-fixture-user-42', 'T00042'),
    (88, 'test-fixture-user-88', 'T00088'),
    (90, 'test-fixture-user-90', 'T00090'),
    (99, 'test-fixture-user-99', 'T00099'),
    (101, 'test-fixture-user-101', 'T00101')
)
INSERT INTO users (id, username, storage_uid, role, status)
SELECT id, username, storage_uid, 'user', 1
FROM seed_users
ON CONFLICT (id) DO NOTHING;

WITH seed_uids (storage_uid) AS (
  VALUES
    ('T00001'),
    ('T00003'),
    ('T00007'),
    ('T00008'),
    ('T00010'),
    ('T00011'),
    ('T00012'),
    ('T00042'),
    ('T00088'),
    ('T00090'),
    ('T00099'),
    ('T00101')
)
INSERT INTO users_used_uids (storage_uid)
SELECT storage_uid
FROM seed_uids
ON CONFLICT (storage_uid) DO NOTHING;

SELECT setval(pg_get_serial_sequence('users', 'id'), (SELECT COALESCE(MAX(id), 1) FROM users), true);
`)
	if err != nil && !database.IsMissingTableError(err) {
		t.Fatalf("seed PostgreSQL test fixture users failed: %v", err)
	}
}
