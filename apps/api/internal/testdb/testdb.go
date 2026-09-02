package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

var (
	schemaCounter      uint64
	loadSchemaSnapshot = sync.OnceValues(func() (string, error) {
		path, err := schemaSnapshotPath()
		if err != nil {
			return "", err
		}
		return readSchemaSnapshot(path)
	})
)

func Open(t testing.TB) *sql.DB {
	t.Helper()

	env := createIsolatedEnv(t)
	db := openIsolatedSQLDB(t, env)
	migrateAndSeed(t, db, env)
	return db
}

func OpenWithPool(t testing.TB) (*sql.DB, *pgxpool.Pool) {
	t.Helper()

	env := createIsolatedEnv(t)
	db := openIsolatedSQLDB(t, env)
	migrateAndSeed(t, db, env)
	pool := openIsolatedPool(t, env)
	return db, pool
}

type isolatedEnv struct {
	rawDSN     string
	schemaName string
}

func createIsolatedEnv(t testing.TB) isolatedEnv {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv("API_TEST_DATABASE_DSN"))
	if raw == "" {
		if isRequiredDatabase() {
			t.Fatal("API_TEST_DATABASE_DSN is required for PostgreSQL integration tests")
		}
		t.Skip("set API_TEST_DATABASE_DSN to run PostgreSQL integration tests")
	}

	adminDB, err := sql.Open("pgx", raw)
	if err != nil {
		t.Fatalf("open PostgreSQL test db failed: %v", err)
	}

	schemaName := nextSchemaName()
	quotedSchema := pgx.Identifier{schemaName}.Sanitize()
	schemaCreated := false
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if schemaCreated {
			// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- quotedSchema is pg identifier sanitized above
			_, _ = adminDB.ExecContext(ctx, `DROP SCHEMA IF EXISTS `+quotedSchema+` CASCADE`)
		}
		_ = adminDB.Close()
	})

	if err := adminDB.Ping(); err != nil {
		t.Fatalf("ping PostgreSQL test db failed: %v", err)
	}
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- quotedSchema is pg identifier sanitized above
	if _, err := adminDB.ExecContext(t.Context(), `CREATE SCHEMA `+quotedSchema); err != nil {
		t.Fatalf("create PostgreSQL test schema failed: %v", err)
	}
	schemaCreated = true

	return isolatedEnv{
		rawDSN:     raw,
		schemaName: schemaName,
	}
}

func isRequiredDatabase() bool {
	required, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("API_TEST_DATABASE_REQUIRED")))
	return err == nil && required
}

func openIsolatedSQLDB(t testing.TB, env isolatedEnv) *sql.DB {
	t.Helper()

	cfg, err := pgx.ParseConfig(env.rawDSN)
	if err != nil {
		t.Fatalf("parse PostgreSQL test dsn failed: %v", err)
	}
	cfg.RuntimeParams["search_path"] = searchPathForSchema(env.schemaName)

	db := stdlib.OpenDB(*cfg)
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := db.Ping(); err != nil {
		t.Fatalf("ping isolated PostgreSQL test db failed: %v", err)
	}
	return db
}

func openIsolatedPool(t testing.TB, env isolatedEnv) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(env.rawDSN)
	if err != nil {
		t.Fatalf("parse PostgreSQL test pool dsn failed: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = searchPathForSchema(env.schemaName)
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("open isolated PostgreSQL test pool failed: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(t.Context()); err != nil {
		t.Fatalf("ping isolated PostgreSQL test pool failed: %v", err)
	}
	return pool
}

func migrateAndSeed(t testing.TB, db *sql.DB, env isolatedEnv) {
	t.Helper()

	if err := applySchemaSnapshot(t.Context(), env.rawDSN, env.schemaName); err != nil {
		t.Fatalf("initialize PostgreSQL test schema from snapshot failed: %v", err)
	}
	seedDefaults(t, db)
}

func applySchemaSnapshot(ctx context.Context, rawDSN, schemaName string) error {
	snapshot, err := loadSchemaSnapshot()
	if err != nil {
		return err
	}

	cfg, err := pgx.ParseConfig(rawDSN)
	if err != nil {
		return fmt.Errorf("parse test database dsn: %w", err)
	}
	cfg.RuntimeParams["search_path"] = searchPathForSchema(schemaName)
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open test database snapshot connection: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, snapshot); err != nil {
		return fmt.Errorf("apply canonical schema snapshot: %w", err)
	}
	return nil
}

func readSchemaSnapshot(path string) (string, error) {
	snapshot, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read canonical schema snapshot %q: %w", path, err)
	}
	snapshotSQL := string(snapshot)
	if strings.TrimSpace(snapshotSQL) == "" {
		return "", fmt.Errorf("canonical schema snapshot %q is empty", path)
	}
	return snapshotSQL, nil
}

func schemaSnapshotPath() (string, error) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve canonical schema snapshot path")
	}
	return filepath.Join(filepath.Dir(sourcePath), "../db/schema/schema.sql"), nil
}

func nextSchemaName() string {
	counter := atomic.AddUint64(&schemaCounter, 1)
	return "ideaboard_test_" +
		strconv.FormatInt(int64(os.Getpid()), 36) + "_" +
		strconv.FormatInt(time.Now().UnixNano(), 36) + "_" +
		strconv.FormatUint(counter, 36)
}

func searchPathForSchema(schemaName string) string {
	return pgx.Identifier{schemaName}.Sanitize() + ",public"
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
	if err != nil {
		t.Fatalf("seed PostgreSQL test defaults failed: %v", err)
	}
	seedTaskFixtureUsers(t, db)
}

func seedTaskFixtureUsers(t testing.TB, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, `
WITH seed_users (id, display_name, storage_uid) AS (
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
    (91, 'test-fixture-user-91', 'T00091'),
    (92, 'test-fixture-user-92', 'T00092'),
    (93, 'test-fixture-user-93', 'T00093'),
    (94, 'test-fixture-user-94', 'T00094'),
    (95, 'test-fixture-user-95', 'T00095'),
    (99, 'test-fixture-user-99', 'T00099'),
    (101, 'test-fixture-user-101', 'T00101')
)
INSERT INTO users (id, display_name, storage_uid, role, status)
SELECT id, display_name, storage_uid, 'user', 1
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
    ('T00091'),
    ('T00092'),
    ('T00093'),
    ('T00094'),
    ('T00095'),
    ('T00099'),
    ('T00101')
)
INSERT INTO users_used_uids (storage_uid)
SELECT storage_uid
FROM seed_uids
ON CONFLICT (storage_uid) DO NOTHING;

SELECT setval(pg_get_serial_sequence('users', 'id'), (SELECT COALESCE(MAX(id), 1) FROM users), true);
`)
	if err != nil {
		t.Fatalf("seed PostgreSQL test fixture users failed: %v", err)
	}
}
