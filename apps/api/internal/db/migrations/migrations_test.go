package migrations

import (
	"context"
	"database/sql"
	"errors"
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

func TestSplitSQLStatementsIgnoresSemicolonsOutsideSQLCode(t *testing.T) {
	raw := `
-- Task facts retain their immutable media asset identity; only the stored URL is retired.
ALTER TABLE media_assets ADD COLUMN reclaim_after TIMESTAMPTZ(3);

INSERT INTO sample_items (label) VALUES ('keep;this');

/* Reclamation is reference-driven; task facts remain immutable. */
CREATE INDEX idx_media_assets_reclamation_due ON media_assets (reclaim_after);
`

	statements := splitSQLStatements(raw)
	if len(statements) != 3 {
		t.Fatalf("statements len=%d, want 3: %#v", len(statements), statements)
	}
	if got := firstSQLLine(statements[0]); got != "ALTER TABLE media_assets ADD COLUMN reclaim_after TIMESTAMPTZ(3)" {
		t.Fatalf("first statement starts with %q", got)
	}
	if !strings.Contains(statements[1], "'keep;this'") {
		t.Fatalf("string literal was split or altered: %q", statements[1])
	}
	if got := firstSQLLine(statements[2]); got != "/* Reclamation is reference-driven; task facts remain immutable. */" {
		t.Fatalf("third statement starts with %q", got)
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

func TestAdminBillingReasonMigrationDeclaresReversibleContract(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260712020000_admin_billing_reasons.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := extractUpSQL(string(raw))
	down := extractDownSQLForTest(string(raw))
	for _, expected := range []string{
		"CREATE TABLE billing_admin_actions",
		"ADD COLUMN reason VARCHAR(1000) NOT NULL",
		"create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT)",
		"start_credit_reconciliation(BIGINT, BIGINT, TEXT)",
		"DROP FUNCTION create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT)",
		"DROP FUNCTION start_credit_reconciliation(BIGINT, BIGINT)",
	} {
		if !strings.Contains(up, expected) {
			t.Fatalf("up migration missing %q", expected)
		}
	}
	for _, expected := range []string{
		"DROP TABLE IF EXISTS billing_admin_actions",
		"DROP COLUMN IF EXISTS reason",
		"CREATE OR REPLACE FUNCTION create_billing_sku(",
		"CREATE OR REPLACE FUNCTION start_credit_reconciliation(p_requested_by BIGINT, p_stale_hold_seconds BIGINT)",
	} {
		if !strings.Contains(down, expected) {
			t.Fatalf("down migration missing %q", expected)
		}
	}
	if err := validateMigrationStatements("20260712020000_admin_billing_reasons.sql down", parseSQLStatements(down)); err != nil {
		t.Fatalf("down migration failed validation: %v", err)
	}
}

func TestFeatureLevelPricingMigrationDeclaresSchemaRollbackContract(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260716040000_feature_level_pricing.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := extractUpSQL(string(raw))
	down := extractDownSQLForTest(string(raw))
	for _, expected := range []string{
		"DROP TABLE billing_price_version_states",
		"CREATE FUNCTION publish_billing_price(",
		"'old', CASE",
		"'new', jsonb_build_object(",
		"'outcome', 'published'",
		"REVOKE ALL ON FUNCTION publish_billing_price",
	} {
		if !strings.Contains(up, expected) {
			t.Fatalf("up migration missing %q", expected)
		}
	}
	for _, expected := range []string{
		"CREATE TABLE billing_price_version_states",
		"CREATE FUNCTION create_billing_price_version(",
		"CREATE FUNCTION set_billing_price_version_enabled(",
		"REVOKE ALL ON FUNCTION create_billing_price_version",
		"REVOKE ALL ON FUNCTION set_billing_price_version_enabled",
	} {
		if !strings.Contains(down, expected) {
			t.Fatalf("down migration missing %q", expected)
		}
	}
	if err := validateMigrationStatements("20260716040000_feature_level_pricing.sql down", parseSQLStatements(down)); err != nil {
		t.Fatalf("down migration failed validation: %v", err)
	}
}

func TestBillingCatalogSurfaceCleanupMigrationDropsLegacyCommands(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260716070000_billing_catalog_surface_cleanup.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := extractUpSQL(string(raw))
	down := extractDownSQLForTest(string(raw))
	upStatements := parseSQLStatements(up)
	wantUpStatements := []string{
		"DROP FUNCTION IF EXISTS create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT)",
		"DROP FUNCTION IF EXISTS update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT)",
	}
	if len(upStatements) != len(wantUpStatements) {
		t.Fatalf("up migration statements=%d, want %d", len(upStatements), len(wantUpStatements))
	}
	for i, want := range wantUpStatements {
		if got := firstSQLLine(upStatements[i].SQL); got != want {
			t.Fatalf("up migration statement %d starts with %q, want %q", i, got, want)
		}
	}
	for _, expected := range []string{
		"DROP FUNCTION IF EXISTS create_billing_sku",
		"DROP FUNCTION IF EXISTS update_billing_sku",
	} {
		if !strings.Contains(up, expected) {
			t.Fatalf("up migration missing %q", expected)
		}
	}
	for _, expected := range []string{
		"CREATE FUNCTION create_billing_sku(",
		"CREATE FUNCTION update_billing_sku(",
		"REVOKE ALL ON FUNCTION create_billing_sku",
		"REVOKE ALL ON FUNCTION update_billing_sku",
	} {
		if !strings.Contains(down, expected) {
			t.Fatalf("down migration missing %q", expected)
		}
	}
	if err := validateMigrationStatements("20260716070000_billing_catalog_surface_cleanup.sql down", parseSQLStatements(down)); err != nil {
		t.Fatalf("down migration failed validation: %v", err)
	}
}

func TestFeatureLevelPricingMigrationPublishesAndRollsBack(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	assertTableExists(t, ctx, conn, "billing_price_version_states", false)
	assertRegprocedureExists(t, ctx, conn, "publish_billing_price(text,text,text,bigint,bigint)", true)
	assertRegprocedureExists(t, ctx, conn, "create_billing_price_version(text,bigint,boolean,bigint,text)", false)
	assertRegprocedureExists(t, ctx, conn, "set_billing_price_version_enabled(text,bigint,boolean,bigint,text)", false)
	assertFunctionSearchPath(t, ctx, conn, "publish_billing_price(text,text,text,bigint,bigint)")

	var firstPriceID, firstVersion, firstCredits int64
	if err := conn.QueryRowContext(ctx, `
SELECT price_version_id, version, unit_credits
FROM publish_billing_price('ai-image:partial-edit', '局部重绘', '只修改选区', 0, 7)
`).Scan(&firstPriceID, &firstVersion, &firstCredits); err != nil {
		t.Fatalf("publish initial free price: %v", err)
	}
	if firstPriceID <= 0 || firstVersion != 1 || firstCredits != 0 {
		t.Fatalf("initial price=(%d,%d,%d)", firstPriceID, firstVersion, firstCredits)
	}
	var enabled bool
	if err := conn.QueryRowContext(ctx, `SELECT enabled FROM billing_skus WHERE sku = 'ai-image:partial-edit'`).Scan(&enabled); err != nil || !enabled {
		t.Fatalf("materialized SKU enabled=%v err=%v", enabled, err)
	}
	if _, err := conn.ExecContext(ctx, `
SELECT * FROM set_billing_item_availability('ai-image:partial-edit', FALSE, 8)
`); err != nil {
		t.Fatalf("disable materialized SKU: %v", err)
	}

	var secondPriceID, secondVersion int64
	if err := conn.QueryRowContext(ctx, `
SELECT price_version_id, version
FROM publish_billing_price('ai-image:partial-edit', '局部重绘', '只修改选区', 3, 9)
`).Scan(&secondPriceID, &secondVersion); err != nil {
		t.Fatalf("publish disabled SKU price: %v", err)
	}
	if secondPriceID == firstPriceID || secondVersion != 2 {
		t.Fatalf("second price=(%d,%d), first id=%d", secondPriceID, secondVersion, firstPriceID)
	}
	if err := conn.QueryRowContext(ctx, `SELECT enabled FROM billing_skus WHERE sku = 'ai-image:partial-edit'`).Scan(&enabled); err != nil || enabled {
		t.Fatalf("repriced disabled SKU enabled=%v err=%v", enabled, err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE billing_price_versions SET unit_credits = 4 WHERE price_version_id = $1`, secondPriceID); err == nil {
		t.Fatal("immutable Price Version accepted an update")
	}

	var metadata string
	if err := conn.QueryRowContext(ctx, `
SELECT metadata::text FROM billing_admin_actions
WHERE sku = 'ai-image:partial-edit' AND action_type = 'create_price'
ORDER BY id DESC LIMIT 1
`).Scan(&metadata); err != nil {
		t.Fatalf("read publish audit: %v", err)
	}
	for _, fragment := range []string{`"old"`, `"new"`, `"outcome": "published"`} {
		if !strings.Contains(metadata, fragment) {
			t.Fatalf("publish audit metadata %q missing %q", metadata, fragment)
		}
	}

	raw, err := migrationFS.ReadFile("20260716040000_feature_level_pricing.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range parseSQLStatements(extractDownSQLForTest(string(raw))) {
		if _, err := tx.ExecContext(ctx, statement.SQL); err != nil {
			_ = tx.Rollback()
			t.Fatalf("execute down statement %q: %v", firstSQLLine(meaningfulSQL(statement.SQL)), err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	assertTableExists(t, ctx, conn, "billing_price_version_states", true)
	assertRegprocedureExists(t, ctx, conn, "publish_billing_price(text,text,text,bigint,bigint)", false)
	assertRegprocedureExists(t, ctx, conn, "create_billing_price_version(text,bigint,boolean,bigint,text)", true)
	assertRegprocedureExists(t, ctx, conn, "set_billing_price_version_enabled(text,bigint,boolean,bigint,text)", true)
	assertFunctionSearchPath(t, ctx, conn, "create_billing_price_version(text,bigint,boolean,bigint,text)")
	assertFunctionSearchPath(t, ctx, conn, "set_billing_price_version_enabled(text,bigint,boolean,bigint,text)")
	var stateCount int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM billing_price_version_states WHERE enabled`).Scan(&stateCount); err != nil || stateCount != 2 {
		t.Fatalf("rollback enabled states=%d err=%v", stateCount, err)
	}
}

func TestRuntimeOperationalSettingsMigrationDeclaresReversibleContract(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260713020000_runtime_operational_settings.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := extractUpSQL(string(raw))
	down := extractDownSQLForTest(string(raw))
	for _, expected := range []string{
		"CREATE TABLE system_settings_admin_actions",
		"system_settings_admin_actions_append_only",
		"river_worker_concurrency",
		"api_outbound_allow_hosts",
		"nats_retention_days",
	} {
		if !strings.Contains(up, expected) {
			t.Fatalf("up migration missing %q", expected)
		}
	}
	for _, expected := range []string{
		"DROP TABLE IF EXISTS system_settings_admin_actions",
		"DELETE FROM system_configs",
	} {
		if !strings.Contains(down, expected) {
			t.Fatalf("down migration missing %q", expected)
		}
	}
	if err := validateMigrationStatements("20260713020000_runtime_operational_settings.sql down", parseSQLStatements(down)); err != nil {
		t.Fatalf("down migration failed validation: %v", err)
	}
}

func TestOutboundPolicySettingsMigrationRemovesRetiredRuntimeKeys(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260730010000_outbound_policy_settings.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := extractUpSQL(string(raw))
	down := extractDownSQLForTest(string(raw))
	for _, retiredKey := range []string{
		"river_worker_concurrency",
		"river_task_timeout_seconds",
		"river_no_capacity_retry_seconds",
		"river_max_retry",
		"nats_retention_days",
	} {
		if !strings.Contains(up, retiredKey) {
			t.Fatalf("up migration missing retired key %q", retiredKey)
		}
		if !strings.Contains(down, retiredKey) {
			t.Fatalf("down migration missing retired key %q", retiredKey)
		}
	}
	if strings.Contains(up, "'api_outbound_allow_hosts'") {
		t.Fatal("up migration must preserve the outbound policy config")
	}
	if !strings.Contains(up, "update_outbound_policy") {
		t.Fatal("up migration must allow outbound policy audit records")
	}
	if !strings.Contains(up, "action_type IN") {
		t.Fatal("up migration must preserve historical audit action types")
	}
	if strings.Contains(up, "UPDATE system_settings_admin_actions") ||
		strings.Contains(down, "UPDATE system_settings_admin_actions") {
		t.Fatal("migration must not rewrite append-only audit records")
	}
}

func TestOutboundPolicySettingsMigrationAppliesCleanContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var retiredCount int
	if err := conn.QueryRowContext(ctx, `
SELECT count(*)
FROM system_configs
WHERE config_key IN (
  'river_worker_concurrency',
  'river_task_timeout_seconds',
  'river_no_capacity_retry_seconds',
  'river_max_retry',
  'nats_retention_days'
)`).Scan(&retiredCount); err != nil {
		t.Fatal(err)
	}
	if retiredCount != 0 {
		t.Fatalf("retired runtime config count = %d, want 0", retiredCount)
	}

	var outboundPolicy string
	if err := conn.QueryRowContext(ctx, `
SELECT value
FROM system_configs
WHERE config_key = 'api_outbound_allow_hosts'`).Scan(&outboundPolicy); err != nil {
		t.Fatal(err)
	}
	if outboundPolicy != "[]" {
		t.Fatalf("outbound policy = %q, want []", outboundPolicy)
	}

	assertColumnExists(t, ctx, conn, "system_settings_admin_actions", "reason", false)
	if _, err := conn.ExecContext(ctx, `
INSERT INTO system_settings_admin_actions (action_type, actor_id, metadata)
VALUES ('update_outbound_policy', 1, '{"allow_hosts": []}'::jsonb)`); err != nil {
		t.Fatalf("insert outbound policy audit: %v", err)
	}
}

func TestOutboundPolicyAuditReasonCleanupMigrationContract(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260730020000_outbound_policy_audit_reason_cleanup.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := extractUpSQL(string(raw))
	down := extractDownSQLForTest(string(raw))
	if !strings.Contains(up, "DROP COLUMN reason") {
		t.Fatal("up migration must remove the obsolete audit reason")
	}
	if !strings.Contains(down, "ADD COLUMN reason") {
		t.Fatal("down migration must restore the previous audit schema")
	}
	if err := validateMigrationStatements("outbound policy audit cleanup up", parseSQLStatements(up)); err != nil {
		t.Fatal(err)
	}
	if err := validateMigrationStatements("outbound policy audit cleanup down", parseSQLStatements(down)); err != nil {
		t.Fatal(err)
	}
}

func TestBaselineMigrationExcludesVersionedRewardsSchema(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260609000000_baseline.sql")
	if err != nil {
		t.Fatalf("read baseline migration failed: %v", err)
	}
	baseline := extractUpSQL(string(raw))
	for _, tableName := range []string{
		"reward_campaigns",
		"reward_codes",
		"reward_redemptions",
		"reward_admin_actions",
	} {
		if strings.Contains(baseline, "CREATE TABLE "+tableName) {
			t.Fatalf("baseline migration must not create versioned Rewards table %q", tableName)
		}
	}
}

func TestSchemaSnapshotIncludesCanonicalRewardCodeContract(t *testing.T) {
	schemaRaw, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	schema := strings.TrimSpace(string(schemaRaw))
	for _, expected := range []string{
		"CREATE TABLE reward_codes",
		"CREATE TABLE reward_code_digest_aliases",
		"CREATE TABLE reward_redemptions",
		"CREATE TABLE reward_admin_actions",
		"CREATE TRIGGER reward_admin_actions_append_only",
		"CREATE TRIGGER reward_codes_identity_immutable",
		"CREATE TRIGGER set_reward_codes_updated_at",
	} {
		if !strings.Contains(schema, expected) {
			t.Fatalf("schema snapshot missing canonical RewardCode contract: %s", expected)
		}
	}
	for _, forbidden := range []string{"reward_campaigns", "reward_code_batches", "reward_code_definitions"} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("schema snapshot retains legacy Rewards surface: %s", forbidden)
		}
	}
}

func TestSchemaSnapshotIncludesCreditWalletRestrictionContract(t *testing.T) {
	schemaRaw, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	migrationRaw, err := migrationFS.ReadFile("20260717010000_credit_wallet_restrictions.sql")
	if err != nil {
		t.Fatalf("read Wallet Restriction migration failed: %v", err)
	}
	schema := string(schemaRaw)
	migrationUp := extractUpSQL(string(migrationRaw))
	for _, expected := range []string{
		"CREATE TABLE credit_wallet_restrictions",
		"CREATE UNIQUE INDEX uq_credit_wallet_restrictions_active_source",
		"CREATE FUNCTION create_credit_wallet_restriction",
		"CREATE FUNCTION release_credit_wallet_restriction",
		"CREATE FUNCTION execute_credit_command_v2",
		"released_at >= created_at",
		"created_by_actor_type IN ('admin', 'system', 'service')",
		"p_command_type IN ('grant', 'deduction', 'hold')",
		"ERRCODE = 'IB011'",
	} {
		if !strings.Contains(schema, expected) {
			t.Fatalf("schema snapshot missing Wallet Restriction contract: %s", expected)
		}
		if !strings.Contains(migrationUp, expected) {
			t.Fatalf("Wallet Restriction migration missing contract: %s", expected)
		}
	}
	schemaRestrictionFunctions := schema[strings.Index(schema, "CREATE FUNCTION create_credit_wallet_restriction"):strings.Index(schema, "CREATE FUNCTION execute_credit_command_v2")]
	migrationRestrictionFunctions := migrationUp[strings.Index(migrationUp, "CREATE FUNCTION create_credit_wallet_restriction"):strings.Index(migrationUp, "CREATE FUNCTION execute_credit_command_v2")]
	if strings.Contains(schema, "created_by_actor_type IN ('admin', 'system', 'service', 'user')") ||
		strings.Contains(schemaRestrictionFunctions, "p_actor_type NOT IN ('admin', 'system', 'service', 'user')") ||
		strings.Contains(migrationRestrictionFunctions, "p_actor_type NOT IN ('admin', 'system', 'service', 'user')") {
		t.Fatal("Wallet Restriction management must not accept user actors")
	}
}

func TestReconciliationLedgerIntegrityRestrictionMigrationContract(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260717020000_reconciliation_ledger_integrity_restrictions.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := extractUpSQL(string(raw))
	down := extractDownSQLForTest(string(raw))
	schemaRaw, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(schemaRaw)
	for _, expected := range []string{
		"DROP FUNCTION execute_credit_reconciliation(BIGINT)",
		"DROP TABLE credit_reconciliation_alerts",
		"create_credit_wallet_restriction(",
		"release_credit_wallet_restriction(",
		"'ledger_integrity'",
		"'reconciliation'",
		"'finding'",
		"ORDER BY restrictions.wallet_user_id, findings.id",
		"ORDER BY findings.wallet_user_id, findings.id",
	} {
		if !strings.Contains(up, expected) {
			t.Fatalf("Reconciliation Restriction migration missing %q", expected)
		}
	}
	if strings.Count(up, "credit_reconciliation_alerts") != 1 {
		t.Fatal("Reconciliation Restriction migration must only reference alerts while dropping the obsolete table")
	}
	if strings.Contains(up, "UPDATE credit_wallets") || strings.Contains(up, "SET status = 'frozen'") {
		t.Fatal("Reconciliation must not mutate Wallet lifecycle status")
	}
	if strings.Contains(up, "execute_credit_reconciliation_before_wallet_restrictions") {
		t.Fatal("Reconciliation Restriction migration must not retain an archived executable producer")
	}
	for _, expected := range []string{
		"CREATE TABLE credit_reconciliation_runs",
		"CREATE TABLE credit_reconciliation_findings",
		"CREATE TABLE credit_reconciliation_run_findings",
		"CREATE FUNCTION start_credit_reconciliation(",
		"CREATE FUNCTION fail_credit_reconciliation(",
		"CREATE FUNCTION execute_credit_reconciliation(",
		"create_credit_wallet_restriction(",
		"release_credit_wallet_restriction(",
		"'ledger_integrity'",
		"'reconciliation'",
		"'finding'",
	} {
		if !strings.Contains(schema, expected) {
			t.Fatalf("schema snapshot missing Reconciliation Restriction contract: %s", expected)
		}
	}
	if strings.Contains(schema, "credit_reconciliation_alerts") {
		t.Fatal("schema snapshot must not retain obsolete Reconciliation alerts")
	}
	schemaExecute := schema[strings.Index(schema, "CREATE FUNCTION execute_credit_reconciliation("):strings.Index(schema, "CREATE OR REPLACE FUNCTION prevent_credit_ledger_mutation()")]
	if strings.Contains(schemaExecute, "UPDATE credit_wallets") || strings.Contains(schemaExecute, "SET status = 'frozen'") {
		t.Fatal("schema snapshot Reconciliation must not mutate Wallet lifecycle status")
	}
	for _, expected := range []string{
		"CREATE TABLE credit_reconciliation_alerts",
		"CREATE OR REPLACE FUNCTION execute_credit_reconciliation(",
		"UPDATE credit_reconciliation_alerts",
		"UPDATE credit_wallets",
		"SECURITY DEFINER",
	} {
		if !strings.Contains(down, expected) {
			t.Fatalf("Reconciliation Restriction down migration missing %q", expected)
		}
	}
	if err := validateMigrationStatements("20260717020000_reconciliation_ledger_integrity_restrictions.sql down", parseSQLStatements(down)); err != nil {
		t.Fatalf("Reconciliation Restriction down migration failed validation: %v", err)
	}
}

func TestReconciliationLedgerIntegrityRestrictionMigrationRollsBackAndReapplies(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	const (
		file    = "20260717020000_reconciliation_ledger_integrity_restrictions.sql"
		version = 20260717020000
	)
	readDefinition := func() string {
		t.Helper()
		var definition string
		if err := conn.QueryRowContext(ctx, `
SELECT pg_get_functiondef(to_regprocedure($1))
`, "execute_credit_reconciliation(bigint)").Scan(&definition); err != nil {
			t.Fatalf("read Reconciliation function definition: %v", err)
		}
		return definition
	}
	assertCurrent := func() {
		t.Helper()
		assertTableExists(t, ctx, conn, "credit_reconciliation_alerts", false)
		assertRegprocedureExists(t, ctx, conn, "execute_credit_reconciliation_before_wallet_restrictions(bigint)", false)
		definition := readDefinition()
		for _, forbidden := range []string{"credit_reconciliation_alerts", "UPDATE credit_wallets", "SET status = 'frozen'"} {
			if strings.Contains(definition, forbidden) {
				t.Fatalf("current Reconciliation function retains %q", forbidden)
			}
		}
	}
	assertCurrent()

	raw, err := migrationFS.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range parseSQLStatements(extractDownSQLForTest(string(raw))) {
		if _, err := tx.ExecContext(ctx, statement.SQL); err != nil {
			_ = tx.Rollback()
			t.Fatalf("execute down statement %q: %v", firstSQLLine(meaningfulSQL(statement.SQL)), err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	assertTableExists(t, ctx, conn, "credit_reconciliation_alerts", true)
	assertRegprocedureExists(t, ctx, conn, "execute_credit_reconciliation_before_wallet_restrictions(bigint)", false)
	rolledBackDefinition := readDefinition()
	for _, expected := range []string{"credit_reconciliation_alerts", "UPDATE credit_wallets", "SET status = 'frozen'"} {
		if !strings.Contains(rolledBackDefinition, expected) {
			t.Fatalf("rolled-back Reconciliation function missing %q", expected)
		}
	}

	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("reapply migration: %v", err)
	}
	assertCurrent()
}

func TestReconciliationAutomationMigrationContract(t *testing.T) {
	raw, err := migrationFS.ReadFile("20260717030000_reconciliation_automation.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := extractUpSQL(string(raw))
	schemaRaw, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(schemaRaw)
	for _, expected := range []string{
		"ADD COLUMN trigger_type VARCHAR(16)",
		"ADD COLUMN scheduled_for TIMESTAMPTZ(3)",
		"ck_credit_reconciliation_run_audit",
		"ck_credit_reconciliation_run_stale_hold_fixed",
		"uq_credit_reconciliation_scheduled_for",
		"RETURNS TABLE (run_id BIGINT, enqueue_required BOOLEAN)",
		"p_trigger_type IS NULL",
		"p_only_if_overdue",
		"completed_at >= clock_timestamp() - INTERVAL '24 hours'",
		"RETURN QUERY SELECT NULL::BIGINT, FALSE",
		"v_run_status = 'failed'",
		"SET status = 'queued'",
		"RETURN QUERY SELECT v_run_id, FALSE",
		"RETURN QUERY SELECT v_run_id, TRUE",
	} {
		if !strings.Contains(up, expected) {
			t.Fatalf("Reconciliation automation migration missing %q", expected)
		}
	}
	for _, forbidden := range []string{
		"DROP COLUMN stale_hold_after",
		"CREATE FUNCTION execute_credit_reconciliation",
		"p_stale_hold_seconds",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("Reconciliation automation migration retains forbidden contract %q", forbidden)
		}
	}
	for _, expected := range []string{
		"trigger_type VARCHAR(16) NOT NULL",
		"scheduled_for TIMESTAMPTZ(3)",
		"ck_credit_reconciliation_run_audit",
		"ck_credit_reconciliation_run_stale_hold_fixed",
		"CREATE UNIQUE INDEX uq_credit_reconciliation_scheduled_for",
		"RETURNS TABLE (run_id BIGINT, enqueue_required BOOLEAN)",
	} {
		if !strings.Contains(schema, expected) {
			t.Fatalf("schema snapshot missing Reconciliation automation contract %q", expected)
		}
	}
}

func TestReconciliationAutomationMigrationRollsBackAndReapplies(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	const (
		file    = "20260717030000_reconciliation_automation.sql"
		version = 20260717030000
	)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(text,bigint,text,timestamp with time zone,boolean)", true)
	assertColumnExists(t, ctx, conn, "credit_reconciliation_runs", "trigger_type", true)
	assertColumnExists(t, ctx, conn, "credit_reconciliation_runs", "scheduled_for", true)

	rollbackTxMigrationForTest(t, ctx, conn, file, version)

	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(text,bigint,text,timestamp with time zone,boolean)", false)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(bigint,bigint,text)", true)
	assertColumnExists(t, ctx, conn, "credit_reconciliation_runs", "trigger_type", false)
	assertColumnExists(t, ctx, conn, "credit_reconciliation_runs", "scheduled_for", false)

	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("reapply Reconciliation automation migration: %v", err)
	}
	assertMigrationApplied(t, ctx, conn, version, true)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(text,bigint,text,timestamp with time zone,boolean)", true)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(bigint,bigint,text)", false)
	assertColumnExists(t, ctx, conn, "credit_reconciliation_runs", "trigger_type", true)
	assertColumnExists(t, ctx, conn, "credit_reconciliation_runs", "scheduled_for", true)
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
		"workflow_features",
		"feature_prompt_revisions",
		"model_defaults",
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
	functions := strings.Count(up, "CREATE OR REPLACE FUNCTION") + strings.Count(up, "CREATE FUNCTION")
	if functions != 9 {
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

func TestSchemaUsesCurrentFeaturePromptContract(t *testing.T) {
	raw, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	schema := string(raw)
	if !strings.Contains(schema, "CREATE TABLE feature_prompts") {
		t.Fatal("schema must create feature_prompts")
	}
	for _, forbidden := range []string{"workflow_features", "feature_prompt_revisions", "model_defaults"} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("schema must not retain legacy Prompt contract: %s", forbidden)
		}
	}
}

func TestUsageIndexesHaveSingleMigrationOwner(t *testing.T) {
	tests := []struct {
		indexName string
		owner     string
	}{
		{
			indexName: "idx_credit_transactions_capture_user_created",
			owner:     "20260713040000_credit_capture_usage_index.sql",
		},
		{
			indexName: "idx_task_billing_snapshots_user_created",
			owner:     "20260713050000_task_billing_usage_index.sql",
		},
	}
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	schema, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.indexName, func(t *testing.T) {
			var owners []string
			for _, file := range files {
				raw, err := migrationFS.ReadFile(file)
				if err != nil {
					t.Fatalf("read migration %s: %v", file, err)
				}
				if strings.Contains(string(raw), tt.indexName) {
					owners = append(owners, file)
				}
			}
			if len(owners) != 1 || owners[0] != tt.owner {
				t.Fatalf("index %s owners=%v, want [%s]", tt.indexName, owners, tt.owner)
			}
			if strings.Contains(string(schema), tt.indexName) {
				t.Fatalf("schema.sql must not duplicate migration-owned index %s", tt.indexName)
			}
		})
	}
}

func TestMigrationLocksAreScopedPerSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	firstConn := openIsolatedMigrationConn(t, ctx)
	secondConn := openIsolatedMigrationConn(t, ctx)

	firstRelease, err := acquireMigrationLock(ctx, firstConn)
	if err != nil {
		t.Fatalf("acquire first migration lock: %v", err)
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), migrationLockReleaseTimeout)
		defer cancel()
		if err := firstRelease(releaseCtx); err != nil {
			t.Fatalf("release first migration lock: %v", err)
		}
	}()

	secondLockCtx, cancelSecondLock := context.WithTimeout(ctx, time.Second)
	defer cancelSecondLock()
	secondRelease, err := acquireMigrationLock(secondLockCtx, secondConn)
	if err != nil {
		t.Fatalf("different schema migration lock should not block: %v", err)
	}
	releaseCtx, cancelRelease := context.WithTimeout(context.Background(), migrationLockReleaseTimeout)
	defer cancelRelease()
	if err := secondRelease(releaseCtx); err != nil {
		t.Fatalf("release second migration lock: %v", err)
	}
}

func TestMigrationLocksSerializeSameSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	db, quotedSchema := openIsolatedMigrationDB(t, ctx)
	db.SetMaxOpenConns(2)
	firstConn := openMigrationConnInSchema(t, ctx, db, quotedSchema)
	secondConn := openMigrationConnInSchema(t, ctx, db, quotedSchema)

	firstRelease, err := acquireMigrationLock(ctx, firstConn)
	if err != nil {
		t.Fatalf("acquire first migration lock: %v", err)
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), migrationLockReleaseTimeout)
		defer cancel()
		if err := firstRelease(releaseCtx); err != nil {
			t.Fatalf("release first migration lock: %v", err)
		}
	}()

	secondLockCtx, cancelSecondLock := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancelSecondLock()
	if _, err := acquireMigrationLock(secondLockCtx, secondConn); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("same schema migration lock error = %v, want context deadline exceeded", err)
	}
}

func TestDeriveMigrationLockKeyIsStableAndScoped(t *testing.T) {
	key := deriveMigrationLockKey("ideaboard", "public")
	if key != deriveMigrationLockKey("ideaboard", "public") {
		t.Fatal("same database and schema must derive the same migration lock key")
	}
	if key == deriveMigrationLockKey("ideaboard", "tenant") {
		t.Fatal("different schemas must derive different migration lock keys")
	}
	if key == deriveMigrationLockKey("ideaboard_test", "public") {
		t.Fatal("different databases must derive different migration lock keys")
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
	assertColumnDataType(t, ctx, conn, "task_billing_snapshots", "display_name", "character varying")
	assertColumnNullable(t, ctx, conn, "task_billing_snapshots", "display_name", false)
	for _, tableName := range []string{
		"reward_codes",
		"reward_code_digest_aliases",
		"reward_redemptions",
		"reward_admin_actions",
	} {
		assertTableExists(t, ctx, conn, tableName, true)
	}
	for _, tableName := range []string{"reward_campaigns", "reward_code_batches", "reward_code_definitions", "reward_code_redemptions", "reward_code_admin_actions"} {
		assertTableExists(t, ctx, conn, tableName, false)
	}
	assertColumnDataType(t, ctx, conn, "reward_codes", "code_digest", "bytea")
	assertColumnDataType(t, ctx, conn, "reward_codes", "expires_at", "timestamp with time zone")
	assertColumnDataType(t, ctx, conn, "reward_code_digest_aliases", "code_digest", "bytea")
	assertColumnDataType(t, ctx, conn, "reward_redemptions", "reward_code_display_name", "character varying")
	assertColumnDataType(t, ctx, conn, "reward_redemptions", "masked_code_hint", "character varying")
	assertColumnNullable(t, ctx, conn, "reward_redemptions", "reward_code_display_name", false)
	assertColumnNullable(t, ctx, conn, "reward_redemptions", "masked_code_hint", false)
	assertIndexValid(t, ctx, conn, "idx_tasks_terminal_created_at")
	assertIndexValid(t, ctx, conn, "idx_tasks_pending_updated_at")
	assertIndexValid(t, ctx, conn, "idx_task_attempts_running_created_at")
	assertIndexValid(t, ctx, conn, "idx_task_attempts_terminal_completed_at")
	assertIndexValid(t, ctx, conn, "idx_task_stat_events_model_latency")
	assertIndexValid(t, ctx, conn, "idx_task_stat_events_feature_api_date")
	assertIndexValid(t, ctx, conn, "idx_credit_transactions_capture_user_created")
	assertIndexValid(t, ctx, conn, "idx_task_billing_snapshots_user_created")
	assertIndexValid(t, ctx, conn, "idx_reward_codes_list")
	assertIndexValid(t, ctx, conn, "idx_reward_code_digest_aliases_code")
	assertIndexValid(t, ctx, conn, "idx_reward_redemptions_user")
	assertIndexValid(t, ctx, conn, "idx_reward_redemptions_code")
	assertIndexValid(t, ctx, conn, "idx_reward_admin_actions_code")
	assertStorageProfileSeed(t, ctx, conn)
	assertTriggerExists(t, ctx, conn, "api_accounts", "prevent_api_account_provider_endpoint_change")
	assertTriggerExists(t, ctx, conn, "users", "set_users_updated_at")
	assertTriggerExists(t, ctx, conn, "billing_admin_actions", "billing_admin_actions_append_only")
	assertTriggerExists(t, ctx, conn, "system_settings_admin_actions", "system_settings_admin_actions_append_only")
	assertTriggerExists(t, ctx, conn, "reward_admin_actions", "reward_admin_actions_append_only")
	assertTriggerExists(t, ctx, conn, "reward_codes", "set_reward_codes_updated_at")
	assertTriggerExists(t, ctx, conn, "reward_codes", "reward_codes_identity_immutable")
	assertRegprocedureExists(t, ctx, conn, "prevent_reward_admin_action_mutation()", true)
	assertRegprocedureExists(t, ctx, conn, "guard_reward_code_identity()", true)
	assertRegprocedureExists(t, ctx, conn, "reserve_reward_code_redemption_v1(bigint)", true)
	assertRegprocedureExists(t, ctx, conn, "reserve_reward_code_redemption_v1(bigint,timestamp with time zone)", false)
	for _, signature := range []string{
		"update_reward_code_definition_v1(bigint,text,bigint,bigint,timestamp with time zone,bigint)",
		"set_reward_code_enabled_v1(bigint,boolean,bigint)",
		"rotate_reward_code_v1(bigint,text,bytea,text,text,bytea,bigint)",
		"delete_reward_code_v1(bigint,bigint)",
		"reserve_reward_code_redemption_v1(bigint)",
	} {
		assertFunctionSearchPath(t, ctx, conn, signature)
	}
	assertFunctionSearchPath(t, ctx, conn, "apply_credit_posting_balances()")
	assertColumnDataType(t, ctx, conn, "credit_reconciliation_runs", "reason", "character varying")
	assertColumnDataType(t, ctx, conn, "credit_reconciliation_runs", "trigger_type", "character varying")
	assertColumnDataType(t, ctx, conn, "credit_reconciliation_runs", "scheduled_for", "timestamp with time zone")
	assertTableExists(t, ctx, conn, "credit_reconciliation_alerts", false)
	assertRegprocedureExists(t, ctx, conn, "execute_credit_reconciliation(bigint)", true)
	assertRegprocedureExists(t, ctx, conn, "execute_credit_reconciliation_before_wallet_restrictions(bigint)", false)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(text,bigint,text,timestamp with time zone,boolean)", true)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(bigint,bigint,text)", false)
	assertConstraintExists(t, ctx, conn, "credit_reconciliation_runs", "ck_credit_reconciliation_run_audit")
	assertConstraintExists(t, ctx, conn, "credit_reconciliation_runs", "ck_credit_reconciliation_run_stale_hold_fixed")
	assertIndexValid(t, ctx, conn, "uq_credit_reconciliation_scheduled_for")
	var reconciliationDefinition string
	if err := conn.QueryRowContext(ctx, `
SELECT pg_get_functiondef('execute_credit_reconciliation(bigint)'::regprocedure)
`).Scan(&reconciliationDefinition); err != nil {
		t.Fatalf("read active Reconciliation function definition: %v", err)
	}
	for _, forbidden := range []string{"credit_reconciliation_alerts", "UPDATE credit_wallets", "SET status = 'frozen'"} {
		if strings.Contains(reconciliationDefinition, forbidden) {
			t.Fatalf("active Reconciliation function retains forbidden producer behavior %q", forbidden)
		}
	}
	assertConstraintExists(t, ctx, conn, "tasks", "fk_tasks_account_provider_endpoint")
	assertConstraintExists(t, ctx, conn, "media_assets", "fk_media_assets_storage_profile_id")
	assertConstraintExists(t, ctx, conn, "boards", "uq_boards_board_id")
	assertConstraintExists(t, ctx, conn, "boards", "fk_boards_created_by")
	assertConstraintExists(t, ctx, conn, "board_members", "fk_board_members_board")
	assertConstraintExists(t, ctx, conn, "board_share_tokens", "fk_board_share_tokens_board")
	assertConstraintExists(t, ctx, conn, "task_billing_snapshots", "task_billing_snapshot_display_name_check")
	assertConstraintExists(t, ctx, conn, "reward_codes", "uq_reward_codes_creator_idempotency")
	assertConstraintExists(t, ctx, conn, "reward_codes", "fk_reward_codes_created_by")
	assertConstraintExists(t, ctx, conn, "reward_code_digest_aliases", "pk_reward_code_digest_aliases")
	assertConstraintExists(t, ctx, conn, "reward_code_digest_aliases", "fk_reward_code_digest_aliases_code")
	assertConstraintExists(t, ctx, conn, "reward_redemptions", "uq_reward_redemptions_user_code")
	assertConstraintExists(t, ctx, conn, "reward_redemptions", "fk_reward_redemptions_credit_transaction")
	assertConstraintExists(t, ctx, conn, "reward_redemptions", "ck_reward_redemptions_display_name")
	assertConstraintExists(t, ctx, conn, "reward_redemptions", "ck_reward_redemptions_masked_code_hint")
	assertConstraintExists(t, ctx, conn, "reward_admin_actions", "fk_reward_admin_actions_code")
}

func TestRewardCodeCoreMigrationIsCanonicalFreshSchema(t *testing.T) {
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		for _, forbidden := range []string{
			"rewards_public_campaign",
			"rewards_campaign_lifecycle",
			"rewards_single_use_batches",
			"rewards_redemption_code_index",
			"rewards_single_code_revoke",
			"rewards_query_indexes",
			"reward_code_core",
			"reward_code_management",
		} {
			if strings.Contains(file, forbidden) {
				t.Fatalf("embedded migrations retain removed Rewards contract %q", file)
			}
		}
	}

	raw, err := migrationFS.ReadFile("20260722020000_reward_code_contract.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := extractUpSQL(string(raw))
	down := extractDownSQLForTest(string(raw))
	if err := validateMigrationStatements("RewardCode core down", parseSQLStatements(down)); err != nil {
		t.Fatalf("canonical RewardCode down contract is invalid: %v", err)
	}
	for _, expected := range []string{
		"CREATE TABLE reward_codes",
		"CREATE TABLE reward_code_digest_aliases",
		"CREATE TABLE reward_redemptions",
		"CREATE TABLE reward_admin_actions",
		"CREATE FUNCTION update_reward_code_definition_v1",
		"CREATE FUNCTION reserve_reward_code_redemption_v1",
	} {
		if !strings.Contains(up, expected) {
			t.Fatalf("canonical RewardCode migration missing %q", expected)
		}
	}
	for _, forbidden := range []string{
		"reward_campaigns",
		"reward_code_batches",
		"campaign_id",
		"\n  code_id BIGINT",
		"single_use",
		"internal_note",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("canonical RewardCode migration retains removed contract %q", forbidden)
		}
	}
}

func TestRewardCodeCoreMigrationRejectsExistingLedgerFacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)

	if _, err := conn.ExecContext(ctx, `
CREATE TABLE schema_migrations (
  version BIGINT NOT NULL PRIMARY KEY,
  applied_at TIMESTAMPTZ(3) NOT NULL DEFAULT now()
)`); err != nil {
		t.Fatal(err)
	}

	const target = "20260722020000_reward_code_contract.sql"
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, file := range files {
		if file == target {
			break
		}
		raw, err := migrationFS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		version, err := migrationVersion(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := applyMigration(ctx, conn, file, parseSQLStatements(extractUpSQL(string(raw))), version); err != nil {
			t.Fatalf("apply prerequisite migration %s: %v", file, err)
		}
	}

	var userID int64
	if err := conn.QueryRowContext(ctx, `
INSERT INTO users (username, storage_uid)
VALUES ('reward-core-migration-user', 'RCM001')
RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert Rewards migration user: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO credit_wallets (user_id) VALUES ($1)`, userID); err != nil {
		t.Fatalf("insert Rewards migration Wallet: %v", err)
	}
	var transactionID string
	if err := conn.QueryRowContext(ctx, `
SELECT transaction_id
FROM execute_credit_command_v2(
  'grant', $1, 1, '', 'reward-core-migration-key',
  'system', 'migration-test', 'rewards', 'migration safety test',
  'reward_redemption', 'existing-fact'
)`, userID).Scan(&transactionID); err != nil {
		t.Fatalf("insert Rewards Ledger fact: %v", err)
	}

	raw, err := migrationFS.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	version, err := migrationVersion(target)
	if err != nil {
		t.Fatal(err)
	}
	err = applyMigration(ctx, conn, target, parseSQLStatements(extractUpSQL(string(raw))), version)
	if err == nil {
		t.Fatal("canonical RewardCode migration accepted an existing Rewards Ledger fact")
	}
	for _, fragment := range []string{"Rewards Ledger facts detected", "Fresh DB"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("migration error=%q, want %q", err, fragment)
		}
	}

	assertMigrationApplied(t, ctx, conn, version, false)
	assertTableExists(t, ctx, conn, "reward_codes", false)
	var ledgerFacts int
	if err := conn.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM credit_transactions WHERE transaction_id = $1`,
		transactionID,
	).Scan(&ledgerFacts); err != nil {
		t.Fatal(err)
	}
	if ledgerFacts != 1 {
		t.Fatalf("failed migration mutated immutable Ledger fact count=%d, want 1", ledgerFacts)
	}
}

func TestRewardCodeCoreMigrationDownRejectsDomainFacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var actorID int64
	if err := conn.QueryRowContext(ctx, `
INSERT INTO users (username, storage_uid)
VALUES ('reward-core-down-user', 'RCD001')
RETURNING id`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	var rewardCodeID int64
	if err := conn.QueryRowContext(ctx, `
INSERT INTO reward_codes (
  display_name, key_version, code_digest, code_hint, credit_amount,
  max_redemptions, created_by, idempotency_key, request_fingerprint
) VALUES (
  'Down guard Reward', 'v1', decode(repeat('05', 32), 'hex'), '2040', 10,
  0, $1, 'down-guard', decode(repeat('06', 32), 'hex')
)
RETURNING id`, actorID).Scan(&rewardCodeID); err != nil {
		t.Fatal(err)
	}

	const file = "20260722020000_reward_code_contract.sql"
	raw, err := migrationFS.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	statements := parseSQLStatements(extractDownSQLForTest(string(raw)))
	if len(statements) == 0 {
		_ = tx.Rollback()
		t.Fatal("canonical RewardCode migration has no down contract")
	}
	_, err = tx.ExecContext(ctx, statements[0].SQL)
	_ = tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "Fresh DB") {
		t.Fatalf("RewardCode down guard error=%v, want Fresh DB failure", err)
	}

	var remaining int
	if err := conn.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM reward_codes WHERE id = $1`,
		rewardCodeID,
	).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("failed down migration mutated RewardCode count=%d, want 1", remaining)
	}
	assertMigrationApplied(t, ctx, conn, 20260722020000, true)
}

func TestTaskBillingSnapshotDisplayNameMigrationBackfillsExistingRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if _, err := conn.ExecContext(ctx, `
CREATE TABLE schema_migrations (
  version BIGINT NOT NULL PRIMARY KEY,
  applied_at TIMESTAMPTZ(3) NOT NULL DEFAULT now()
)`); err != nil {
		t.Fatal(err)
	}

	const target = "20260714010000_task_billing_snapshot_display_name.sql"
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, file := range files {
		if file == target {
			break
		}
		raw, err := migrationFS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		version, err := migrationVersion(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := applyMigration(ctx, conn, file, parseSQLStatements(extractUpSQL(string(raw))), version); err != nil {
			t.Fatalf("apply prerequisite migration %s: %v", file, err)
		}
	}

	if _, err := conn.ExecContext(ctx, `
INSERT INTO billing_skus (sku, display_name, description, enabled, created_by, updated_by)
VALUES ('test:snapshot-backfill', 'Backfilled Product', '', TRUE, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	var priceVersionID int64
	if err := conn.QueryRowContext(ctx, `
INSERT INTO billing_price_versions (sku, version, unit_credits, created_by)
VALUES ('test:snapshot-backfill', 1, 0, 1)
RETURNING price_version_id`).Scan(&priceVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO billing_price_version_states (price_version_id, enabled, updated_by)
VALUES ($1, TRUE, 1)`, priceVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `
UPDATE billing_skus SET current_price_version_id = $1 WHERE sku = 'test:snapshot-backfill'`, priceVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO task_billing_snapshots (
  task_id, user_id, feature_slug, sku, price_version_id, price_version,
  unit_credits, quantity, total_credits, quoted_at
) VALUES (
  'snapshot-backfill-task', 1, 'pattern-gen', 'test:snapshot-backfill', $1, 1,
  0, 1, 0, clock_timestamp()
)`, priceVersionID); err != nil {
		t.Fatal(err)
	}

	raw, err := migrationFS.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	version, err := migrationVersion(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyMigration(ctx, conn, target, parseSQLStatements(extractUpSQL(string(raw))), version); err != nil {
		t.Fatalf("apply display name migration: %v", err)
	}

	var displayName string
	if err := conn.QueryRowContext(ctx, `
SELECT display_name FROM task_billing_snapshots WHERE task_id = 'snapshot-backfill-task'`).Scan(&displayName); err != nil {
		t.Fatal(err)
	}
	if displayName != "Backfilled Product" {
		t.Fatalf("backfilled display name=%q, want Backfilled Product", displayName)
	}
	assertColumnNullable(t, ctx, conn, "task_billing_snapshots", "display_name", false)
	if _, err := conn.ExecContext(ctx, `
UPDATE task_billing_snapshots SET display_name = 'Tampered' WHERE task_id = 'snapshot-backfill-task'`); err == nil {
		t.Fatal("snapshot display name mutation unexpectedly succeeded after migration")
	}
}

func TestUsageIndexMigrationsRollBackAndReapplyIndependently(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	tests := []struct {
		name                  string
		file                  string
		version               int64
		indexName             string
		dependencyVersion     int64
		dependencyTable       string
		independentUsageIndex string
	}{
		{
			name:                  "credit capture",
			file:                  "20260713040000_credit_capture_usage_index.sql",
			version:               20260713040000,
			indexName:             "idx_credit_transactions_capture_user_created",
			dependencyVersion:     baselineMigrationVersion,
			dependencyTable:       "credit_transactions",
			independentUsageIndex: "idx_task_billing_snapshots_user_created",
		},
		{
			name:                  "task billing",
			file:                  "20260713050000_task_billing_usage_index.sql",
			version:               20260713050000,
			indexName:             "idx_task_billing_snapshots_user_created",
			dependencyVersion:     20260711030000,
			dependencyTable:       "task_billing_snapshots",
			independentUsageIndex: "idx_credit_transactions_capture_user_created",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertIndexValid(t, ctx, conn, tt.indexName)
			rollbackNoTxMigrationForTest(t, ctx, conn, tt.file, tt.version)
			assertIndexExists(t, ctx, conn, tt.indexName, false)
			assertTableExists(t, ctx, conn, tt.dependencyTable, true)
			assertMigrationApplied(t, ctx, conn, tt.dependencyVersion, true)
			assertIndexValid(t, ctx, conn, tt.independentUsageIndex)

			if err := upWithConn(ctx, conn); err != nil {
				t.Fatalf("reapply migration %s: %v", tt.file, err)
			}
			assertIndexValid(t, ctx, conn, tt.indexName)
			assertMigrationApplied(t, ctx, conn, tt.version, true)
		})
	}
}

func TestRuntimeOperationalSettingsMigrationRollsBackAndReapplies(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	assertTableExists(t, ctx, conn, "system_settings_admin_actions", true)

	raw, err := migrationFS.ReadFile("20260713020000_runtime_operational_settings.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range parseSQLStatements(extractDownSQLForTest(string(raw))) {
		if _, err := tx.ExecContext(ctx, statement.SQL); err != nil {
			_ = tx.Rollback()
			t.Fatalf("execute down statement %q: %v", firstSQLLine(meaningfulSQL(statement.SQL)), err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 20260713020000`); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertTableExists(t, ctx, conn, "system_settings_admin_actions", false)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("reapply migration: %v", err)
	}
	assertTableExists(t, ctx, conn, "system_settings_admin_actions", true)
}

func TestAdminBillingReasonMigrationRollsBackAndReapplies(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	rollbackTxMigrationForTest(
		t,
		ctx,
		conn,
		"20260717030000_reconciliation_automation.sql",
		20260717030000,
	)

	assertRegprocedureExists(t, ctx, conn, "create_billing_sku(text,text,text,boolean,bigint,text)", false)
	assertRegprocedureExists(t, ctx, conn, "create_billing_sku(text,text,text,boolean,bigint)", false)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(bigint,bigint,text)", true)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(bigint,bigint)", false)

	cleanupRaw, err := migrationFS.ReadFile("20260716070000_billing_catalog_surface_cleanup.sql")
	if err != nil {
		t.Fatal(err)
	}
	cleanupTx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range parseSQLStatements(extractDownSQLForTest(string(cleanupRaw))) {
		if _, err := cleanupTx.ExecContext(ctx, statement.SQL); err != nil {
			_ = cleanupTx.Rollback()
			t.Fatalf("execute cleanup down statement %q: %v", firstSQLLine(meaningfulSQL(statement.SQL)), err)
		}
	}
	if _, err := cleanupTx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 20260716070000`); err != nil {
		_ = cleanupTx.Rollback()
		t.Fatal(err)
	}
	if err := cleanupTx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertRegprocedureExists(t, ctx, conn, "create_billing_sku(text,text,text,boolean,bigint,text)", true)

	raw, err := migrationFS.ReadFile("20260712020000_admin_billing_reasons.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range parseSQLStatements(extractDownSQLForTest(string(raw))) {
		if _, err := tx.ExecContext(ctx, statement.SQL); err != nil {
			_ = tx.Rollback()
			t.Fatalf("execute down statement %q: %v", firstSQLLine(meaningfulSQL(statement.SQL)), err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 20260712020000`); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	assertRegprocedureExists(t, ctx, conn, "create_billing_sku(text,text,text,boolean,bigint)", true)
	assertRegprocedureExists(t, ctx, conn, "create_billing_sku(text,text,text,boolean,bigint,text)", false)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(bigint,bigint)", true)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(bigint,bigint,text)", false)
	assertColumnExists(t, ctx, conn, "credit_reconciliation_runs", "reason", false)
	assertTableExists(t, ctx, conn, "billing_admin_actions", false)

	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("reapply migration after down: %v", err)
	}
	assertRegprocedureExists(t, ctx, conn, "create_billing_sku(text,text,text,boolean,bigint,text)", false)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(bigint,bigint,text)", false)
	assertRegprocedureExists(t, ctx, conn, "start_credit_reconciliation(text,bigint,text,timestamp with time zone,boolean)", true)
	assertColumnExists(t, ctx, conn, "credit_reconciliation_runs", "reason", true)
	assertTableExists(t, ctx, conn, "billing_admin_actions", true)
}

func TestMigrationsRejectRemovedRewardsVersion(t *testing.T) {
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
	if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (20260721010000)`); err != nil {
		t.Fatalf("insert removed Rewards migration version failed: %v", err)
	}

	err := upWithConn(ctx, conn)
	if err == nil {
		t.Fatalf("upWithConn succeeded with a removed Rewards migration version")
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

func openMigrationConnInSchema(t *testing.T, ctx context.Context, db *sql.DB, quotedSchema string) *sql.Conn {
	t.Helper()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open migration connection failed: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
	if _, err := conn.ExecContext(ctx, `SET search_path TO `+quotedSchema+`, public`); err != nil {
		t.Fatalf("set migration connection search_path failed: %v", err)
	}
	return conn
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

func assertColumnNullable(t *testing.T, ctx context.Context, conn *sql.Conn, tableName, columnName string, want bool) {
	t.Helper()
	var isNullable string
	if err := conn.QueryRowContext(ctx, `
SELECT is_nullable
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name = $1
  AND column_name = $2`, tableName, columnName).Scan(&isNullable); err != nil {
		t.Fatalf("read column nullability %s.%s failed: %v", tableName, columnName, err)
	}
	got := isNullable == "YES"
	if got != want {
		t.Fatalf("%s.%s nullable=%v, want %v", tableName, columnName, got, want)
	}
}

func assertColumnExists(t *testing.T, ctx context.Context, conn *sql.Conn, tableName, columnName string, want bool) {
	t.Helper()
	var exists bool
	if err := conn.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.columns
  WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2
)`, tableName, columnName).Scan(&exists); err != nil {
		t.Fatalf("read column existence %s.%s failed: %v", tableName, columnName, err)
	}
	if exists != want {
		t.Fatalf("column %s.%s exists=%v, want %v", tableName, columnName, exists, want)
	}
}

func assertTableExists(t *testing.T, ctx context.Context, conn *sql.Conn, tableName string, want bool) {
	t.Helper()
	var exists bool
	if err := conn.QueryRowContext(ctx, `
SELECT to_regclass(current_schema() || '.' || $1) IS NOT NULL`, tableName).Scan(&exists); err != nil {
		t.Fatalf("read table existence %s failed: %v", tableName, err)
	}
	if exists != want {
		t.Fatalf("table %s exists=%v, want %v", tableName, exists, want)
	}
}

func assertRegprocedureExists(t *testing.T, ctx context.Context, conn *sql.Conn, signature string, want bool) {
	t.Helper()
	var exists bool
	if err := conn.QueryRowContext(ctx, `
SELECT to_regprocedure(format('%I.%s', current_schema(), $1::text)) IS NOT NULL
`, signature).Scan(&exists); err != nil {
		t.Fatalf("read function existence %s failed: %v", signature, err)
	}
	if exists != want {
		t.Fatalf("function %s exists=%v, want %v", signature, exists, want)
	}
}

func assertFunctionSearchPath(t *testing.T, ctx context.Context, conn *sql.Conn, signature string) {
	t.Helper()
	var schemaName string
	var configuredSearchPath string
	if err := conn.QueryRowContext(ctx, `
SELECT current_schema(), COALESCE(array_to_string(proconfig, ','), '')
FROM pg_proc
WHERE oid = to_regprocedure(format('%I.%s', current_schema(), $1::text))
`, signature).Scan(&schemaName, &configuredSearchPath); err != nil {
		t.Fatalf("read function %s search_path failed: %v", signature, err)
	}
	want := "search_path=" + schemaName + ", pg_catalog, pg_temp"
	if configuredSearchPath != want {
		t.Fatalf("function %s search_path=%q, want %q", signature, configuredSearchPath, want)
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

func assertIndexExists(t *testing.T, ctx context.Context, conn *sql.Conn, indexName string, want bool) {
	t.Helper()
	var exists bool
	if err := conn.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = current_schema()
    AND c.relkind = 'i'
    AND c.relname = $1
)`, indexName).Scan(&exists); err != nil {
		t.Fatalf("read index existence %s failed: %v", indexName, err)
	}
	if exists != want {
		t.Fatalf("index %s exists=%v, want %v", indexName, exists, want)
	}
}

func assertMigrationApplied(t *testing.T, ctx context.Context, conn *sql.Conn, version int64, want bool) {
	t.Helper()
	applied, err := migrationApplied(ctx, conn, version)
	if err != nil {
		t.Fatalf("read migration %d state: %v", version, err)
	}
	if applied != want {
		t.Fatalf("migration %d applied=%v, want %v", version, applied, want)
	}
}

func rollbackNoTxMigrationForTest(
	t *testing.T,
	ctx context.Context,
	conn *sql.Conn,
	file string,
	version int64,
) {
	t.Helper()
	raw, err := migrationFS.ReadFile(file)
	if err != nil {
		t.Fatalf("read migration %s: %v", file, err)
	}
	statements := parseSQLStatements(extractDownSQLForTest(string(raw)))
	if len(statements) == 0 {
		t.Fatalf("migration %s has no down statements", file)
	}
	for _, statement := range statements {
		if !statement.NoTx {
			t.Fatalf("migration %s down statement must be no-tx: %s", file, firstSQLLine(statement.SQL))
		}
		if err := applyNoTxStatement(ctx, conn, file, statement); err != nil {
			t.Fatalf("apply migration %s down statement: %v", file, err)
		}
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version); err != nil {
		t.Fatalf("remove migration %s marker: %v", file, err)
	}
}

func rollbackTxMigrationForTest(
	t *testing.T,
	ctx context.Context,
	conn *sql.Conn,
	file string,
	version int64,
) {
	t.Helper()
	raw, err := migrationFS.ReadFile(file)
	if err != nil {
		t.Fatalf("read migration %s: %v", file, err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin migration %s rollback: %v", file, err)
	}
	for _, statement := range parseSQLStatements(extractDownSQLForTest(string(raw))) {
		if _, err := tx.ExecContext(ctx, statement.SQL); err != nil {
			_ = tx.Rollback()
			t.Fatalf("execute migration %s down statement %q: %v", file, firstSQLLine(meaningfulSQL(statement.SQL)), err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version); err != nil {
		_ = tx.Rollback()
		t.Fatalf("remove migration %s marker: %v", file, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit migration %s rollback: %v", file, err)
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

func extractDownSQLForTest(raw string) string {
	_, down, ok := strings.Cut(raw, "-- migrate:down")
	if !ok {
		return ""
	}
	return strings.TrimSpace(down)
}
