package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSplitSQLStatementsPreservesStatementBlocks(t *testing.T) {
	raw := `
-- comment before statement
CREATE TABLE sample_items (id BIGINT PRIMARY KEY);

-- statement:begin
DO $$
BEGIN
  RAISE NOTICE 'keep semicolons inside dollar quoted blocks; even here';
END;
$$
-- statement:end

CREATE INDEX idx_sample_items_id ON sample_items (id);
`

	statements := splitSQLStatements(raw)
	if len(statements) != 3 {
		t.Fatalf("statements len=%d, want 3: %#v", len(statements), statements)
	}
	if !strings.Contains(statements[0], "CREATE TABLE sample_items (id BIGINT PRIMARY KEY)") {
		t.Fatalf("first statement=%q", statements[0])
	}
	if !strings.Contains(statements[1], "RAISE NOTICE 'keep semicolons inside dollar quoted blocks; even here';") {
		t.Fatalf("statement block was split or altered: %q", statements[1])
	}
	if !strings.Contains(statements[2], "CREATE INDEX idx_sample_items_id ON sample_items (id)") {
		t.Fatalf("third statement=%q", statements[2])
	}
}

func TestParseSQLStatementsMarksNoTxStatements(t *testing.T) {
	raw := `
CREATE TABLE sample_items (id BIGINT PRIMARY KEY);

-- statement:no-tx
-- statement:index-name=idx_sample_items_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sample_items_id ON sample_items (id);
`

	statements := parseSQLStatements(raw)
	if len(statements) != 2 {
		t.Fatalf("statements len=%d, want 2: %#v", len(statements), statements)
	}
	if statements[0].NoTx {
		t.Fatalf("first statement should be transactional: %#v", statements[0])
	}
	if !statements[1].NoTx {
		t.Fatalf("concurrent index statement should be no-tx: %#v", statements[1])
	}
	if statements[1].IndexName != "idx_sample_items_id" {
		t.Fatalf("concurrent index statement index name=%q", statements[1].IndexName)
	}
	if !strings.Contains(statements[1].SQL, "CREATE INDEX CONCURRENTLY IF NOT EXISTS") {
		t.Fatalf("unexpected no-tx statement SQL: %q", statements[1].SQL)
	}
}

func TestStatementAnnotationsApplyOnlyToNextStatement(t *testing.T) {
	raw := `
-- statement:no-tx
-- statement:index-name=idx_sample_items_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sample_items_id ON sample_items (id);

CREATE TABLE sample_items (id BIGINT PRIMARY KEY);
`

	statements := parseSQLStatements(raw)
	if len(statements) != 2 {
		t.Fatalf("statements len=%d, want 2: %#v", len(statements), statements)
	}
	if !statements[0].NoTx || statements[0].IndexName != "idx_sample_items_id" {
		t.Fatalf("first statement annotation mismatch: %#v", statements[0])
	}
	if statements[1].NoTx || statements[1].IndexName != "" {
		t.Fatalf("second statement should not inherit annotations: %#v", statements[1])
	}
}

func TestValidateMigrationStatementsRejectsUnsafeConcurrentIndex(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name: "concurrent statement without no-tx",
			raw: `
-- statement:index-name=idx_sample_items_id
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sample_items_id ON sample_items (id);
`,
			wantErr: "without -- statement:no-tx",
		},
		{
			name: "concurrent create without if not exists",
			raw: `
-- statement:no-tx
-- statement:index-name=idx_sample_items_id
CREATE INDEX CONCURRENTLY idx_sample_items_id ON sample_items (id);
`,
			wantErr: "must use IF NOT EXISTS",
		},
		{
			name: "concurrent create without index name annotation",
			raw: `
-- statement:no-tx
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sample_items_id ON sample_items (id);
`,
			wantErr: "must declare -- statement:index-name=<name>",
		},
		{
			name: "unsupported no-tx statement",
			raw: `
-- statement:no-tx
ALTER TABLE sample_items ADD COLUMN name TEXT;
`,
			wantErr: "unsupported no-tx statement",
		},
		{
			name: "concurrent drop without if exists",
			raw: `
-- statement:no-tx
DROP INDEX CONCURRENTLY idx_sample_items_id;
`,
			wantErr: "drop must use IF EXISTS",
		},
		{
			name: "invalid index name annotation",
			raw: `
-- statement:no-tx
-- statement:index-name=1bad_index
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sample_items_id ON sample_items (id);
`,
			wantErr: "invalid concurrent index name",
		},
		{
			name: "uppercase index name annotation",
			raw: `
-- statement:no-tx
-- statement:index-name=IdxSampleItems
CREATE INDEX CONCURRENTLY IF NOT EXISTS IdxSampleItems ON sample_items (id);
`,
			wantErr: "invalid concurrent index name",
		},
		{
			name: "declared index name mismatch",
			raw: `
-- statement:no-tx
-- statement:index-name=idx_declared
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_actual ON sample_items (id);
`,
			wantErr: "declares idx_declared but creates idx_actual",
		},
		{
			name: "mixed no-tx and transactional statements",
			raw: `
-- statement:no-tx
DROP INDEX CONCURRENTLY IF EXISTS idx_sample_items_id;

CREATE TABLE sample_items (id BIGINT PRIMARY KEY);
`,
			wantErr: "must not mix transactional statements",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMigrationStatements("test.sql", parseSQLStatements(tt.raw))
			if err == nil {
				t.Fatalf("validateMigrationStatements succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateMigrationStatements error=%q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestCreateIndexStatementNameParsesConcurrentIndex(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "plain",
			sql:  "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sample_items_id ON sample_items (id)",
			want: "idx_sample_items_id",
		},
		{
			name: "unique",
			sql:  "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_sample_items_id ON sample_items (id)",
			want: "uq_sample_items_id",
		},
		{
			name: "without if not exists",
			sql:  "CREATE INDEX CONCURRENTLY idx_sample_items_id ON sample_items (id)",
			want: "idx_sample_items_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := createIndexStatementName(tt.sql)
			if !ok {
				t.Fatalf("createIndexStatementName did not parse %q", tt.sql)
			}
			if got != tt.want {
				t.Fatalf("createIndexStatementName=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractUpSQLExcludesDownMigration(t *testing.T) {
	raw := `
-- migrate:up
CREATE TABLE up_only (id BIGINT PRIMARY KEY);

-- migrate:down
DROP TABLE up_only;
`

	up := extractUpSQL(raw)
	if strings.Contains(up, "DROP TABLE") {
		t.Fatalf("up migration includes down SQL: %q", up)
	}
	if !strings.Contains(up, "CREATE TABLE up_only") {
		t.Fatalf("up migration missing create table statement: %q", up)
	}
}

func TestEmbeddedMigrationsHaveUniqueParseableUpStatements(t *testing.T) {
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		t.Fatalf("glob migrations failed: %v", err)
	}
	sort.Strings(files)
	seenVersions := map[int64]string{}
	for _, file := range files {
		version, err := migrationVersion(file)
		if err != nil {
			t.Fatalf("migration %s has invalid version: %v", file, err)
		}
		if previous := seenVersions[version]; previous != "" {
			t.Fatalf("migration version %d is duplicated by %s and %s", version, previous, file)
		}
		seenVersions[version] = file

		raw, err := migrationFS.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s failed: %v", file, err)
		}
		statements := parseSQLStatements(extractUpSQL(string(raw)))
		if len(statements) == 0 {
			t.Fatalf("migration %s has no up statements", file)
		}
		if err := validateMigrationStatements(file, statements); err != nil {
			t.Fatalf("migration %s failed validation: %v", file, err)
		}
		for _, statement := range statements {
			if strings.Contains(statement.SQL, "-- migrate:down") {
				t.Fatalf("migration %s up statements include down migration: %q", file, statement.SQL)
			}
			if strings.Count(statement.SQL, "$$")%2 != 0 {
				t.Fatalf("migration %s has split dollar-quoted statement: %q", file, statement.SQL)
			}
			if strings.Contains(strings.ToUpper(statement.SQL), "CONCURRENTLY") && !statement.NoTx {
				t.Fatalf("migration %s has concurrent statement without %s: %q", file, statementNoTx, statement.SQL)
			}
		}
	}
}

func TestEmbeddedMigrationsKeepBaselineFirst(t *testing.T) {
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		t.Fatalf("glob migrations failed: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 || files[0] != "20260609000000_baseline.sql" {
		t.Fatalf("embedded migrations=%v, want baseline migration first", files)
	}
}

func TestBaselineMigrationMatchesSchemaSnapshot(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260609000000_baseline.sql")
	if err != nil {
		t.Fatalf("read baseline migration failed: %v", err)
	}
	schemaRaw, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}

	baselineSchema := normalizeBaselineSchema(extractUpSQL(string(raw)))
	schema := strings.TrimSpace(string(schemaRaw))
	if baselineSchema != schema {
		t.Fatalf("baseline migration drifted from schema snapshot")
	}
}

func TestBaselineBusinessTableGuardMatchesBaselineTables(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260609000000_baseline.sql")
	if err != nil {
		t.Fatalf("read baseline migration failed: %v", err)
	}

	baselineTables := extractCreatedTableNames(extractUpSQL(string(raw)))
	if len(baselineTables) != len(baselineBusinessTableNames) {
		t.Fatalf(
			"baseline table guard size drifted: guard=%v baseline=%v",
			sortedStringSet(baselineBusinessTableNames),
			sortedStringSet(baselineTables),
		)
	}
	for tableName := range baselineBusinessTableNames {
		if _, ok := baselineTables[tableName]; !ok {
			t.Fatalf("baseline table guard contains %q, but baseline SQL does not create it", tableName)
		}
	}
	for tableName := range baselineTables {
		if _, ok := baselineBusinessTableNames[tableName]; !ok {
			t.Fatalf("baseline SQL creates %q, but baseline table guard does not include it", tableName)
		}
	}
}

func TestBaselineMigrationHasCleanSchemaContract(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260609000000_baseline.sql")
	if err != nil {
		t.Fatalf("read baseline migration failed: %v", err)
	}

	up := extractUpSQL(string(raw))
	for _, expected := range []string{
		"CREATE TABLE users",
		"CREATE TABLE api_accounts",
		"CREATE TABLE api_account_models",
		"CREATE TABLE task_attempts",
		"CREATE TABLE task_cancellations",
		"CREATE TABLE media_assets",
		"CREATE TABLE storage_profiles",
		"CREATE TABLE feature_prompts",
		"CREATE TABLE boards",
		"CREATE TABLE board_share_tokens",
		"stat_date DATE NOT NULL",
		"account_id BIGINT",
		"assigned_provider_endpoint_id BIGINT",
		"state JSONB NOT NULL",
		"revision BIGINT NOT NULL DEFAULT 1",
		"state_schema_version INTEGER NOT NULL DEFAULT 1",
		"owner_user_id BIGINT NOT NULL",
		"created_by BIGINT NOT NULL",
		"last_saved_by BIGINT",
		"token_hash VARCHAR(128) NOT NULL",
		"token_prefix VARCHAR(16) NOT NULL",
		"CONSTRAINT uq_api_accounts_id_provider_endpoint_id UNIQUE (id, provider_endpoint_id)",
		"CONSTRAINT fk_tasks_account_provider_endpoint FOREIGN KEY (account_id, assigned_provider_endpoint_id) REFERENCES api_accounts(id, provider_endpoint_id)",
		"CONSTRAINT uq_boards_board_id UNIQUE (board_id)",
		"CONSTRAINT fk_boards_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE CASCADE",
		"CONSTRAINT fk_boards_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT",
		"CONSTRAINT fk_boards_last_saved_by FOREIGN KEY (last_saved_by) REFERENCES users(id) ON DELETE SET NULL",
		"CONSTRAINT fk_board_members_board FOREIGN KEY (board_id) REFERENCES boards(board_id) ON DELETE CASCADE",
		"CONSTRAINT fk_board_share_tokens_board FOREIGN KEY (board_id) REFERENCES boards(board_id) ON DELETE CASCADE",
		"CREATE INDEX idx_tasks_account_id ON tasks (account_id) WHERE account_id IS NOT NULL",
		"CREATE INDEX idx_tasks_terminal_created_at ON tasks (created_at) WHERE status IN ('completed', 'failed', 'cancelled')",
		"VALUES ('builtin-local', '本地存储', 'local', TRUE, TRUE, TRUE, 'active', '{}'::jsonb)",
	} {
		if !strings.Contains(up, expected) {
			t.Fatalf("baseline migration missing contract: %s", expected)
		}
	}

	for _, forbidden := range []string{
		"ALTER TABLE boards ADD COLUMN",
		"UPDATE boards SET",
		"CREATE TABLE user_boards",
		"ALTER COLUMN stat_date TYPE DATE",
		"ALTER COLUMN state TYPE JSONB",
		"TRUNCATE TABLE board_share_tokens",
		"RENAME COLUMN token TO token_hash",
		"ADD COLUMN token_prefix",
		"NOT VALID",
		"VALIDATE CONSTRAINT",
		"CONCURRENTLY",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("baseline migration should not keep upgrade-only path: %s", forbidden)
		}
	}
}

func TestBaselineMigrationWrapsFunctionBlocks(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260609000000_baseline.sql")
	if err != nil {
		t.Fatalf("read baseline migration failed: %v", err)
	}

	up := extractUpSQL(string(raw))
	if starts := strings.Count(up, "-- statement:begin"); starts != 9 {
		t.Fatalf("baseline function block starts=%d, want 9", starts)
	}
	if ends := strings.Count(up, "-- statement:end"); ends != 9 {
		t.Fatalf("baseline function block ends=%d, want 9", ends)
	}
	if functions := strings.Count(up, "CREATE OR REPLACE FUNCTION"); functions != 9 {
		t.Fatalf("baseline functions=%d, want 9", functions)
	}
	if statements := parseSQLStatements(up); len(statements) < 50 {
		t.Fatalf("baseline parse produced too few statements: %d", len(statements))
	}
}

func TestBaselineStorageSeedIsLastUpStatement(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260609000000_baseline.sql")
	if err != nil {
		t.Fatalf("read baseline migration failed: %v", err)
	}
	statements := parseSQLStatements(extractUpSQL(string(raw)))
	if len(statements) == 0 {
		t.Fatalf("baseline migration has no up statements")
	}
	lastStatement := meaningfulSQL(statements[len(statements)-1].SQL)
	if !strings.HasPrefix(lastStatement, "INSERT INTO storage_profiles ") {
		t.Fatalf("baseline storage seed must remain the last up statement, got %q", firstSQLLine(lastStatement))
	}
}

func TestSchemaDoesNotCreateUpdatedAtTriggerForAppendOnlyPromptRevisions(t *testing.T) {
	raw, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	schema := string(raw)
	if strings.Contains(schema, "set_feature_prompt_revisions_updated_at") {
		t.Fatalf("feature_prompt_revisions has no updated_at column; schema must not create an updated_at trigger")
	}
}

func TestMigrationsFreshDatabaseAppliesAllVersionsInIsolatedSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)

	release, err := acquireMigrationLock(ctx, conn)
	if err != nil {
		t.Fatalf("acquire migration lock failed: %v", err)
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), migrationLockReleaseTimeout)
		defer cancel()
		if err := release(releaseCtx); err != nil {
			t.Fatalf("release migration lock failed: %v", err)
		}
	}()

	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("first migration run failed: %v", err)
	}
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("second migration run should be idempotent: %v", err)
	}

	var applied int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("count schema migrations failed: %v", err)
	}
	if applied != embeddedMigrationCount(t) {
		t.Fatalf("applied migrations=%d, want %d", applied, embeddedMigrationCount(t))
	}

	assertColumnDataType(t, ctx, conn, "task_daily_stats", "stat_date", "date")
	assertColumnDataType(t, ctx, conn, "task_stat_events", "stat_date", "date")
	assertColumnDataType(t, ctx, conn, "task_daily_stat_backfills", "stat_date", "date")
	assertColumnDataType(t, ctx, conn, "boards", "state", "jsonb")
	assertIndexValid(t, ctx, conn, "idx_tasks_terminal_created_at")
	assertIndexValid(t, ctx, conn, "idx_tasks_pending_updated_at")
	assertIndexValid(t, ctx, conn, "idx_task_attempts_running_created_at")
	assertIndexValid(t, ctx, conn, "idx_task_attempts_terminal_completed_at")
	assertIndexValid(t, ctx, conn, "idx_task_stat_events_model_latency")
	assertIndexValid(t, ctx, conn, "idx_task_stat_events_feature_api_date")
	assertStorageProfileSeed(t, ctx, conn)
	assertTriggerExists(t, ctx, conn, "api_accounts", "prevent_api_account_provider_endpoint_change")
	assertTriggerExists(t, ctx, conn, "users", "set_users_updated_at")
	assertConstraintExists(t, ctx, conn, "tasks", "fk_tasks_account_provider_endpoint")
	assertConstraintExists(t, ctx, conn, "media_assets", "fk_media_assets_storage_profile_id")
	assertConstraintExists(t, ctx, conn, "boards", "uq_boards_board_id")
	assertConstraintExists(t, ctx, conn, "boards", "fk_boards_created_by")
	assertConstraintExists(t, ctx, conn, "board_members", "fk_board_members_board")
	assertConstraintExists(t, ctx, conn, "board_share_tokens", "fk_board_share_tokens_board")
}

func TestMigrationsRejectUnknownAppliedVersions(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)

	if _, err := conn.ExecContext(ctx, `
CREATE TABLE schema_migrations (
  version BIGINT NOT NULL PRIMARY KEY,
  applied_at TIMESTAMPTZ(3) NOT NULL DEFAULT now()
)`); err != nil {
		t.Fatalf("create schema_migrations failed: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (20260602000100)`); err != nil {
		t.Fatalf("insert legacy migration version failed: %v", err)
	}

	err := upWithConn(ctx, conn)
	if err == nil {
		t.Fatalf("upWithConn succeeded with unknown legacy migration version")
	}
	if !strings.Contains(err.Error(), "not present in this binary") {
		t.Fatalf("upWithConn error=%q, want unknown version guard", err.Error())
	}
}

func TestMigrationsRejectBaselineStampWithLegacyHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)

	if _, err := conn.ExecContext(ctx, `
CREATE TABLE schema_migrations (
  version BIGINT NOT NULL PRIMARY KEY,
  applied_at TIMESTAMPTZ(3) NOT NULL DEFAULT now()
)`); err != nil {
		t.Fatalf("create schema_migrations failed: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1), (20260602000100)`, baselineMigrationVersion); err != nil {
		t.Fatalf("insert mixed migration versions failed: %v", err)
	}

	err := upWithConn(ctx, conn)
	if err == nil {
		t.Fatalf("upWithConn succeeded with baseline marker plus legacy migration history")
	}
	if !strings.Contains(err.Error(), "not present in this binary") {
		t.Fatalf("upWithConn error=%q, want unknown version guard", err.Error())
	}
}

func TestMigrationsRejectUnstampedExistingAPISchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)

	if _, err := conn.ExecContext(ctx, `CREATE TABLE users (id BIGINT PRIMARY KEY)`); err != nil {
		t.Fatalf("create existing API table failed: %v", err)
	}

	err := upWithConn(ctx, conn)
	if err == nil {
		t.Fatalf("upWithConn succeeded with unstamped existing API schema")
	}
	if !strings.Contains(err.Error(), "current schema already contains API table") {
		t.Fatalf("upWithConn error=%q, want existing API schema guard", err.Error())
	}
}

func TestMigrationsAllowAuthTablesBeforeBaseline(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)

	for _, statement := range []string{
		`CREATE TABLE "user" (id TEXT PRIMARY KEY)`,
		`CREATE TABLE "session" (id TEXT PRIMARY KEY)`,
		`CREATE TABLE "account" (id TEXT PRIMARY KEY)`,
		`CREATE TABLE "verification" (id TEXT PRIMARY KEY)`,
	} {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("create auth table failed: %v", err)
		}
	}

	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("upWithConn should allow Better Auth tables before baseline: %v", err)
	}
}

func TestMigrationsRejectStampedIncompleteBaselineSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)

	if _, err := conn.ExecContext(ctx, `
CREATE TABLE schema_migrations (
  version BIGINT NOT NULL PRIMARY KEY,
  applied_at TIMESTAMPTZ(3) NOT NULL DEFAULT now()
)`); err != nil {
		t.Fatalf("create schema_migrations failed: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, baselineMigrationVersion); err != nil {
		t.Fatalf("insert baseline migration version failed: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE existing_app_table (id BIGINT PRIMARY KEY)`); err != nil {
		t.Fatalf("create existing app table failed: %v", err)
	}

	err := upWithConn(ctx, conn)
	if err == nil {
		t.Fatalf("upWithConn succeeded with stamped incomplete baseline schema")
	}
	if !strings.Contains(err.Error(), "baseline migration") || !strings.Contains(err.Error(), "marked applied") {
		t.Fatalf("upWithConn error=%q, want stamped baseline guard", err.Error())
	}
}

func TestMigrationsUpRunsWithSingleOpenConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	db, quotedSchema := openIsolatedMigrationDB(t, ctx)

	if err := Up(ctx, db); err != nil {
		t.Fatalf("migration Up failed with MaxOpenConns=1: %v", err)
	}
	if _, err := db.ExecContext(ctx, `SET search_path TO `+quotedSchema+`, public`); err != nil {
		t.Fatalf("restore isolated migration search_path failed: %v", err)
	}

	var applied int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("count schema migrations failed: %v", err)
	}
	if applied != embeddedMigrationCount(t) {
		t.Fatalf("applied migrations=%d, want %d", applied, embeddedMigrationCount(t))
	}
}

func openIsolatedMigrationConn(t *testing.T, ctx context.Context) *sql.Conn {
	t.Helper()
	rawDSN := strings.TrimSpace(os.Getenv("API_TEST_DATABASE_DSN"))
	if rawDSN == "" {
		t.Skip("set API_TEST_DATABASE_DSN to run migration replay integration tests")
	}

	db, err := sql.Open("pgx", rawDSN)
	if err != nil {
		t.Fatalf("open PostgreSQL test db failed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	schemaName := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
	quotedSchema := quoteIdentifier(schemaName)
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA `+quotedSchema); err != nil {
		t.Fatalf("create isolated migration schema failed: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `DROP SCHEMA IF EXISTS `+quotedSchema+` CASCADE`)
	})

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open isolated migration connection failed: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
	if _, err := conn.ExecContext(ctx, `SET search_path TO `+quotedSchema+`, public`); err != nil {
		t.Fatalf("set isolated migration search_path failed: %v", err)
	}
	return conn
}

func openIsolatedMigrationDB(t *testing.T, ctx context.Context) (*sql.DB, string) {
	t.Helper()
	rawDSN := strings.TrimSpace(os.Getenv("API_TEST_DATABASE_DSN"))
	if rawDSN == "" {
		t.Skip("set API_TEST_DATABASE_DSN to run migration replay integration tests")
	}

	db, err := sql.Open("pgx", rawDSN)
	if err != nil {
		t.Fatalf("open PostgreSQL test db failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})

	schemaName := fmt.Sprintf("migration_test_%d", time.Now().UnixNano())
	quotedSchema := quoteIdentifier(schemaName)
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA `+quotedSchema); err != nil {
		t.Fatalf("create isolated migration schema failed: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `DROP SCHEMA IF EXISTS `+quotedSchema+` CASCADE`)
	})

	if _, err := db.ExecContext(ctx, `SET search_path TO `+quotedSchema+`, public`); err != nil {
		t.Fatalf("set isolated migration search_path failed: %v", err)
	}
	return db, quotedSchema
}

func embeddedMigrationCount(t *testing.T) int {
	t.Helper()
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		t.Fatalf("glob migrations failed: %v", err)
	}
	return len(files)
}

func assertColumnDataType(t *testing.T, ctx context.Context, conn *sql.Conn, tableName, columnName, want string) {
	t.Helper()
	var got string
	if err := conn.QueryRowContext(ctx, `
SELECT data_type
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name = $1
  AND column_name = $2`, tableName, columnName).Scan(&got); err != nil {
		t.Fatalf("read column type %s.%s failed: %v", tableName, columnName, err)
	}
	if got != want {
		t.Fatalf("%s.%s data_type=%s, want %s", tableName, columnName, got, want)
	}
}

func assertIndexValid(t *testing.T, ctx context.Context, conn *sql.Conn, indexName string) {
	t.Helper()
	var valid bool
	if err := conn.QueryRowContext(ctx, `
SELECT i.indisvalid
FROM pg_class c
JOIN pg_index i ON i.indexrelid = c.oid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = current_schema()
  AND c.relname = $1`, indexName).Scan(&valid); err != nil {
		t.Fatalf("read index %s validity failed: %v", indexName, err)
	}
	if !valid {
		t.Fatalf("index %s is not valid", indexName)
	}
}

func assertStorageProfileSeed(t *testing.T, ctx context.Context, conn *sql.Conn) {
	t.Helper()
	var count int
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM storage_profiles
WHERE profile_id = 'builtin-local'
  AND provider = 'local'
  AND enabled
  AND is_current
  AND builtin
  AND status = 'active'`).Scan(&count); err != nil {
		t.Fatalf("read builtin storage profile seed failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("builtin-local storage profile seed count=%d, want 1", count)
	}
}

func assertTriggerExists(t *testing.T, ctx context.Context, conn *sql.Conn, tableName string, triggerName string) {
	t.Helper()
	var exists bool
	if err := conn.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM pg_trigger trigger
  JOIN pg_class table_class ON table_class.oid = trigger.tgrelid
  JOIN pg_namespace namespace ON namespace.oid = table_class.relnamespace
  WHERE namespace.nspname = current_schema()
    AND table_class.relname = $1
    AND trigger.tgname = $2
    AND NOT trigger.tgisinternal
)`, tableName, triggerName).Scan(&exists); err != nil {
		t.Fatalf("read trigger %s.%s failed: %v", tableName, triggerName, err)
	}
	if !exists {
		t.Fatalf("trigger %s.%s is missing", tableName, triggerName)
	}
}

func assertConstraintExists(t *testing.T, ctx context.Context, conn *sql.Conn, tableName string, constraintName string) {
	t.Helper()
	var exists bool
	if err := conn.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM pg_constraint constraint_row
  JOIN pg_class table_class ON table_class.oid = constraint_row.conrelid
  JOIN pg_namespace namespace ON namespace.oid = table_class.relnamespace
  WHERE namespace.nspname = current_schema()
    AND table_class.relname = $1
    AND constraint_row.conname = $2
)`, tableName, constraintName).Scan(&exists); err != nil {
		t.Fatalf("read constraint %s.%s failed: %v", tableName, constraintName, err)
	}
	if !exists {
		t.Fatalf("constraint %s.%s is missing", tableName, constraintName)
	}
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func extractCreatedTableNames(sql string) map[string]struct{} {
	tableNames := map[string]struct{}{}
	const createTablePrefix = "CREATE TABLE "
	const createTableIfNotExistsPrefix = "IF NOT EXISTS "
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(trimmed), createTablePrefix) {
			continue
		}
		tableName := strings.TrimSpace(trimmed[len(createTablePrefix):])
		if strings.HasPrefix(strings.ToUpper(tableName), createTableIfNotExistsPrefix) {
			tableName = strings.TrimSpace(tableName[len(createTableIfNotExistsPrefix):])
		}
		if end := strings.IndexAny(tableName, " ("); end >= 0 {
			tableName = tableName[:end]
		}
		tableName = strings.Trim(tableName, `"`)
		if tableName != "" {
			tableNames[tableName] = struct{}{}
		}
	}
	return tableNames
}

func sortedStringSet(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
}

func normalizeBaselineSchema(up string) string {
	if schema, _, ok := strings.Cut(up, "\nINSERT INTO storage_profiles "); ok {
		up = schema
	}
	lines := strings.Split(up, "\n")
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		switch strings.TrimSpace(line) {
		case "-- statement:begin", "-- statement:end":
			continue
		default:
			normalized = append(normalized, line)
		}
	}
	return strings.TrimSpace(strings.Join(normalized, "\n"))
}
