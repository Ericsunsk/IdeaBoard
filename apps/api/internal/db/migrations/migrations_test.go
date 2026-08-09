package migrations

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSplitSQLStatementsPreservesDollarQuotedBlocks(t *testing.T) {
	raw := `
CREATE TABLE sample_items (id BIGINT PRIMARY KEY);
-- statement:begin
DO $$
BEGIN
  RAISE NOTICE 'keep semicolons; inside dollar quoted blocks';
END;
$$
-- statement:end
CREATE INDEX idx_sample_items_id ON sample_items (id);
`
	statements := splitSQLStatements(raw)
	if len(statements) != 3 {
		t.Fatalf("statements len=%d, want 3", len(statements))
	}
	if !strings.Contains(statements[1], "RAISE NOTICE") {
		t.Fatalf("dollar quoted statement was split: %q", statements[1])
	}
}

func TestValidateMigrationStatementsRejectsUnsafeConcurrentIndex(t *testing.T) {
	for name, raw := range map[string]string{
		"missing no-tx":         "-- statement:index-name=idx_items_id\nCREATE INDEX CONCURRENTLY IF NOT EXISTS idx_items_id ON items (id);",
		"missing if-not-exists": "-- statement:no-tx\n-- statement:index-name=idx_items_id\nCREATE INDEX CONCURRENTLY idx_items_id ON items (id);",
		"mixed paths":           "-- statement:no-tx\nDROP INDEX CONCURRENTLY IF EXISTS idx_items_id;\nCREATE TABLE items (id BIGINT);",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateMigrationStatements("test.sql", parseSQLStatements(raw)); err == nil {
				t.Fatal("validation unexpectedly succeeded")
			}
		})
	}
}

func TestExtractUpSQLExcludesDownMigration(t *testing.T) {
	up := extractUpSQL("-- migrate:up\nCREATE TABLE up_only (id BIGINT);\n-- migrate:down\nDROP TABLE up_only;")
	if strings.Contains(up, "DROP TABLE") || !strings.Contains(up, "CREATE TABLE up_only") {
		t.Fatalf("unexpected up SQL: %q", up)
	}
}

func TestEmbeddedMigrationsStartWithCleanBaseline(t *testing.T) {
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 || files[0] != "20260810000000_baseline.sql" {
		t.Fatalf("embedded migrations=%v, want clean baseline first", files)
	}
	raw, err := migrationFS.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	statements := parseSQLStatements(extractUpSQL(string(raw)))
	if len(statements) < 100 {
		t.Fatalf("baseline contains only %d statements; expected the complete current schema", len(statements))
	}
	if err := validateMigrationStatements(files[0], statements); err != nil {
		t.Fatalf("baseline validation failed: %v", err)
	}
	for _, required := range []string{
		"CREATE TABLE users",
		"CREATE TABLE task_billing_snapshots",
		"CREATE TABLE reward_codes",
		"CREATE TABLE payment_orders",
		"CREATE FUNCTION execute_credit_command_v2",
	} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("baseline missing %s", required)
		}
	}
}

func TestMigrationChecksumIsStableAndCanonical(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260810000000_baseline.sql")
	if err != nil {
		t.Fatal(err)
	}
	checksum := migrationChecksum(raw)
	decoded, err := hex.DecodeString(checksum)
	if err != nil || len(decoded) != 32 {
		t.Fatalf("checksum=%q is not a SHA-256 digest", checksum)
	}
	if checksum != migrationChecksum(raw) {
		t.Fatal("checksum is not deterministic")
	}
}

func TestSchemaAssemblySourceIsDeterministic(t *testing.T) {
	raw, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || !strings.Contains(string(raw), "CREATE TABLE users") {
		t.Fatal("schema snapshot is empty or missing identity contract")
	}
}

func TestSchemaSnapshotCoversCanonicalBaselineObjects(t *testing.T) {
	baseline, err := migrationFS.ReadFile(baselineMigrationFile)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatal(err)
	}

	baselineTables, err := baselineBusinessTableNames()
	if err != nil {
		t.Fatal(err)
	}
	snapshotTables := objectNames(snapshot, createTableNamePattern)
	missingTables := missingObjectNames(baselineTables, snapshotTables)
	if len(missingTables) > 0 {
		t.Fatalf("schema snapshot is missing baseline table(s): %s", strings.Join(missingTables, ", "))
	}

	baselineFunctions := objectNames(baseline, regexp.MustCompile(`(?im)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+(?:[a-zA-Z_][a-zA-Z0-9_$]*\.)?([a-zA-Z_][a-zA-Z0-9_$]*)`))
	snapshotFunctions := objectNames(snapshot, regexp.MustCompile(`(?im)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+(?:[a-zA-Z_][a-zA-Z0-9_$]*\.)?([a-zA-Z_][a-zA-Z0-9_$]*)`))
	missingFunctions := missingObjectNames(baselineFunctions, snapshotFunctions)
	if len(missingFunctions) > 0 {
		t.Fatalf("schema snapshot is missing baseline function(s): %s", strings.Join(missingFunctions, ", "))
	}
}

func TestSchemaSnapshotCoversCanonicalBaselineIndexes(t *testing.T) {
	baseline, err := migrationFS.ReadFile(baselineMigrationFile)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	indexPattern := regexp.MustCompile(`(?im)^\s*CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-zA-Z_][a-zA-Z0-9_$]*)`)
	missing := missingObjectNames(objectNames(baseline, indexPattern), objectNames(snapshot, indexPattern))
	if len(missing) > 0 {
		t.Fatalf("schema snapshot is missing baseline index(es): %s", strings.Join(missing, ", "))
	}
}

func TestSchemaSnapshotAppliesToFreshSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	raw, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, string(raw)); err != nil {
		t.Fatalf("schema snapshot failed on a fresh schema: %v", err)
	}
	assertTableExists(t, ctx, conn, "users", true)
	assertTableExists(t, ctx, conn, "payment_orders", true)
}

func objectNames(raw []byte, pattern *regexp.Regexp) map[string]struct{} {
	matches := pattern.FindAllStringSubmatch(string(raw), -1)
	names := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		names[strings.ToLower(match[1])] = struct{}{}
	}
	return names
}

func missingObjectNames(required, actual map[string]struct{}) []string {
	missing := make([]string, 0)
	for name := range required {
		if _, ok := actual[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func TestMigrationLocksAreScopedPerSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	first := openIsolatedMigrationConn(t, ctx)
	second := openIsolatedMigrationConn(t, ctx)
	release, err := acquireMigrationLock(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	defer release(context.Background())
	otherRelease, err := acquireMigrationLock(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if err := otherRelease(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationLockKeyIsStableAndScoped(t *testing.T) {
	if deriveMigrationLockKey("db", "schema") != deriveMigrationLockKey("db", "schema") {
		t.Fatal("migration lock key is not stable")
	}
	if deriveMigrationLockKey("db", "schema") == deriveMigrationLockKey("db", "other") {
		t.Fatal("migration lock key ignores schema")
	}
}

func TestMigrationsFreshDatabaseIsIdempotentAndTamperEvident(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("first migration run failed: %v", err)
	}
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != embeddedMigrationCount(t) {
		t.Fatalf("migration history rows=%d, want %d", count, embeddedMigrationCount(t))
	}
	var version int64
	var filename, checksum string
	if err := conn.QueryRowContext(ctx, `SELECT version, filename, checksum FROM schema_migrations WHERE version = $1`, baselineMigrationVersion).Scan(&version, &filename, &checksum); err != nil {
		t.Fatal(err)
	}
	if version != baselineMigrationVersion || filename != "20260810000000_baseline.sql" || len(checksum) != 64 {
		t.Fatalf("unexpected migration history row: %d %q %q", version, filename, checksum)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE schema_migrations SET checksum = repeat('0', 64)`); err == nil {
		t.Fatal("migration history update unexpectedly succeeded")
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM schema_migrations`); err == nil {
		t.Fatal("migration history delete unexpectedly succeeded")
	}
}

func TestMigrationsRejectLegacyHistorySchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if _, err := conn.ExecContext(ctx, `CREATE TABLE schema_migrations (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	if err := upWithConn(ctx, conn); err == nil || !strings.Contains(err.Error(), "legacy or incomplete") {
		t.Fatalf("upWithConn error=%v, want legacy history failure", err)
	}
}

func TestMigrationsRejectIncompleteHistoryConstraints(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if _, err := conn.ExecContext(ctx, `
CREATE TABLE schema_migrations (
  version BIGINT NOT NULL,
  filename TEXT NOT NULL,
  checksum CHAR(64) NOT NULL,
  applied_at TIMESTAMPTZ(3) NOT NULL DEFAULT now()
)`); err != nil {
		t.Fatal(err)
	}
	if err := upWithConn(ctx, conn); err == nil || !strings.Contains(err.Error(), "missing constraint") {
		t.Fatalf("upWithConn error=%v, want incomplete history constraint failure", err)
	}
}

func TestVerifyMigrationHistoryRejectsUnknownVersionChecksumAndGap(t *testing.T) {
	tests := []struct {
		name     string
		rows     string
		want     string
		embedded []embeddedMigration
	}{
		{name: "unknown", rows: `INSERT INTO schema_migrations(version, filename, checksum) VALUES (9, 'unknown.sql', repeat('0', 64))`, want: "not present", embedded: []embeddedMigration{{Version: 1, File: "one.sql", Checksum: strings.Repeat("1", 64)}}},
		{name: "checksum", rows: `INSERT INTO schema_migrations(version, filename, checksum) VALUES (1, 'one.sql', repeat('0', 64))`, want: "mismatch", embedded: []embeddedMigration{{Version: 1, File: "one.sql", Checksum: strings.Repeat("1", 64)}}},
		{name: "gap", rows: `INSERT INTO schema_migrations(version, filename, checksum) VALUES (2, 'two.sql', repeat('2', 64))`, want: "skips an earlier", embedded: []embeddedMigration{{Version: 1, File: "one.sql", Checksum: strings.Repeat("1", 64)}, {Version: 2, File: "two.sql", Checksum: strings.Repeat("2", 64)}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			conn := openIsolatedMigrationConn(t, ctx)
			if err := ensureMigrationHistorySchema(ctx, conn); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.ExecContext(ctx, tt.rows); err != nil {
				t.Fatal(err)
			}
			if _, err := verifyMigrationHistory(ctx, conn, tt.embedded); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("verifyMigrationHistory error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestTransactionalMigrationFailureLeavesNoPartialStateAndCanRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := ensureMigrationHistorySchema(ctx, conn); err != nil {
		t.Fatal(err)
	}
	failed := []migrationStatement{{SQL: "CREATE TABLE retry_items (id BIGINT PRIMARY KEY)"}, {SQL: "INSERT INTO missing_table VALUES (1)"}}
	if err := applyTransactionalMigration(ctx, conn, "00000000000001_retry.sql", failed, 1, strings.Repeat("1", 64)); err == nil {
		t.Fatal("failed migration unexpectedly succeeded")
	}
	assertTableExists(t, ctx, conn, "retry_items", false)
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed migration recorded %d history rows", count)
	}
	succeeded := []migrationStatement{{SQL: "CREATE TABLE retry_items (id BIGINT PRIMARY KEY)"}}
	if err := applyTransactionalMigration(ctx, conn, "00000000000001_retry.sql", succeeded, 1, strings.Repeat("1", 64)); err != nil {
		t.Fatalf("retry migration failed: %v", err)
	}
	assertTableExists(t, ctx, conn, "retry_items", true)
}

func TestNoTxMigrationRecordsOnlyAfterConcurrentIndexSucceeds(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := ensureMigrationHistorySchema(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "CREATE TABLE no_tx_items (id BIGINT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	statements := parseSQLStatements("-- statement:no-tx\n-- statement:index-name=idx_no_tx_items_id\nCREATE INDEX CONCURRENTLY IF NOT EXISTS idx_no_tx_items_id ON no_tx_items (id);")
	if err := applyNoTxMigration(ctx, conn, "00000000000002_no_tx.sql", statements, 2, strings.Repeat("2", 64)); err != nil {
		t.Fatalf("no-tx migration failed: %v", err)
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = 2`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("no-tx migration history rows=%d, want 1", count)
	}
}

func openIsolatedMigrationConn(t *testing.T, ctx context.Context) *sql.Conn {
	t.Helper()
	rawDSN := strings.TrimSpace(os.Getenv("API_TEST_DATABASE_DSN"))
	if rawDSN == "" {
		t.Skip("set API_TEST_DATABASE_DSN to run PostgreSQL migration tests")
	}
	db, err := sql.Open("pgx", rawDSN)
	if err != nil {
		t.Fatalf("open PostgreSQL test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	schemaName := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
	quoted := quoteIdentifier(schemaName)
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA `+quoted); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `DROP SCHEMA IF EXISTS `+quoted+` CASCADE`)
	})
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open migration connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.ExecContext(ctx, `SET search_path TO `+quoted+`, public`); err != nil {
		t.Fatalf("set migration search_path: %v", err)
	}
	return conn
}

func assertTableExists(t *testing.T, ctx context.Context, conn *sql.Conn, tableName string, want bool) {
	t.Helper()
	var exists bool
	if err := conn.QueryRowContext(ctx, `SELECT to_regclass(current_schema() || '.' || $1) IS NOT NULL`, tableName).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists != want {
		t.Fatalf("table %s exists=%v, want %v", tableName, exists, want)
	}
}

func assertRegprocedureExists(t *testing.T, ctx context.Context, conn *sql.Conn, signature string, want bool) {
	t.Helper()
	var exists bool
	if err := conn.QueryRowContext(ctx, `SELECT to_regprocedure(format('%I.%s', current_schema(), $1::text)) IS NOT NULL`, signature).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists != want {
		t.Fatalf("function %s exists=%v, want %v", signature, exists, want)
	}
}

func assertFunctionSearchPath(t *testing.T, ctx context.Context, conn *sql.Conn, signature string) {
	t.Helper()
	var schemaName, configured string
	if err := conn.QueryRowContext(ctx, `SELECT current_schema(), COALESCE(array_to_string(proconfig, ','), '') FROM pg_proc WHERE oid = to_regprocedure(format('%I.%s', current_schema(), $1::text))`, signature).Scan(&schemaName, &configured); err != nil {
		t.Fatal(err)
	}
	if configured != "search_path="+schemaName+", pg_catalog, pg_temp" {
		t.Fatalf("function %s search_path=%q", signature, configured)
	}
}

func assertTriggerExists(t *testing.T, ctx context.Context, conn *sql.Conn, tableName, triggerName string) {
	t.Helper()
	var exists bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_trigger tr JOIN pg_class c ON c.oid = tr.tgrelid JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = current_schema() AND c.relname = $1 AND tr.tgname = $2 AND NOT tr.tgisinternal)`, tableName, triggerName).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("trigger %s.%s is missing", tableName, triggerName)
	}
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func embeddedMigrationCount(t *testing.T) int {
	t.Helper()
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	return len(files)
}

func assertStorageProfileSeed(t *testing.T, ctx context.Context, conn *sql.Conn) {
	t.Helper()
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM storage_profiles WHERE profile_id = 'builtin-local' AND provider = 'local' AND enabled AND is_current AND builtin AND status = 'active'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("builtin storage profile rows=%d, want 1", count)
	}
}
