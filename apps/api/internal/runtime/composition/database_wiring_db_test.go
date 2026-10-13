package composition

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
	dbprivileges "ideaboard/api/internal/platform/database/privileges"
	workerriver "ideaboard/api/internal/runtime/interfaces/worker/river"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/testenv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type startupDatabaseFixture struct {
	ownerDB *pgxpool.Pool
	config  *config.Config
	roleSQL string
}

func TestOpenAppDatabasesStartsWithConfiguredLimitedRole(t *testing.T) {
	fixture := newStartupDatabaseFixture(t)

	databases, err := openAppDatabases(t.Context(), fixture.config)
	if err != nil {
		t.Fatalf("openAppDatabases() error = %v", err)
	}
	defer closeStartupResources(databases.db, databases.riverPool)
	if err := databases.db.Ping(t.Context()); err != nil {
		t.Fatalf("ping startup pool: %v", err)
	}
	if err := databases.riverPool.Ping(t.Context()); err != nil {
		t.Fatalf("ping startup River pool: %v", err)
	}
	assertProviderEndpointConfigurationAccess(t, databases.db)
	assertLibraryMaterializationMappingAccess(t, databases.db)
	assertWorkspaceRuntimeTableAccess(t, databases.db)
	assertWorkspaceRuntimeSequenceAccess(t, databases.db)
	assertWorkspaceTenancyWriteSmoke(t, databases.db)
	assertControlAuditAppendOnlySmoke(t, databases.db)
}

func assertLibraryMaterializationMappingAccess(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	var allowed bool
	if err := db.QueryRow(t.Context(), `
SELECT has_table_privilege(
  current_user,
  'library_asset_media_assets',
  'SELECT,INSERT,UPDATE,DELETE'
)`).Scan(&allowed); err != nil {
		t.Fatalf("assert library materialization mapping privileges: %v", err)
	}
	if !allowed {
		t.Fatal("runtime role lacks library_asset_media_assets SELECT, INSERT, UPDATE, or DELETE")
	}
}

func assertProviderEndpointConfigurationAccess(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	var endpointID int64
	if err := db.QueryRow(ctx, `
INSERT INTO provider_endpoints (api_type, protocol, name, base_url)
VALUES ('custom', 'image-pipeline', 'runtime privilege test', 'https://runtime-privilege.test')
RETURNING id`).Scan(&endpointID); err != nil {
		t.Fatalf("insert provider endpoint with runtime role: %v", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM provider_endpoints WHERE id = $1`, endpointID); err != nil {
		t.Fatalf("delete provider endpoint with runtime role: %v", err)
	}
}

// workspaceRuntimeTableNames mirrors the #809 workspace tables owned by the
// API runtime privilege policy with full SELECT/INSERT/UPDATE/DELETE.
// control_audit_records is intentionally absent: it is append-only (see
// assertWorkspaceRuntimeTableAccess).
var workspaceRuntimeTableNames = []string{
	"billing_accounts",
	"board_documents",
	"board_revisions",
	"event_deliveries",
	"event_delivery_attempts",
	"event_subscriptions",
	"library_materializations",
	"media_asset_derivatives",
	"organization_memberships",
	"organizations",
	"task_billing_settlements",
	"task_billing_snapshots",
	"upload_sessions",
	"workspace_events",
	"workspace_memberships",
	"workspaces",
}

// workspaceRuntimeSequenceNames mirrors the identity sequences owned by the
// API runtime privilege policy. Tables without an identity column
// (organization_memberships, workspace_memberships, board_documents,
// task_billing_snapshots, task_billing_settlements) have no sequence here.
var workspaceRuntimeSequenceNames = []string{
	"billing_accounts_internal_id_seq",
	"board_revisions_internal_id_seq",
	"control_audit_records_internal_id_seq",
	"event_deliveries_internal_id_seq",
	"event_delivery_attempts_internal_id_seq",
	"event_subscriptions_internal_id_seq",
	"library_materializations_internal_id_seq",
	"media_asset_derivatives_internal_id_seq",
	"organizations_internal_id_seq",
	"upload_sessions_internal_id_seq",
	"workspace_events_internal_id_seq",
	"workspaces_internal_id_seq",
}

func assertWorkspaceRuntimeTableAccess(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	for _, tableName := range workspaceRuntimeTableNames {
		var allowed bool
		if err := db.QueryRow(ctx, `SELECT has_table_privilege(current_user, $1, 'SELECT,INSERT,UPDATE,DELETE')`, tableName).Scan(&allowed); err != nil {
			t.Fatalf("assert workspace table privileges for %s: %v", tableName, err)
		}
		if !allowed {
			t.Fatalf("runtime role lacks SELECT, INSERT, UPDATE, or DELETE on %s", tableName)
		}
	}
	var appendAllowed, mutateAllowed bool
	if err := db.QueryRow(ctx, `SELECT has_table_privilege(current_user, 'control_audit_records', 'SELECT,INSERT')`).Scan(&appendAllowed); err != nil {
		t.Fatalf("assert control audit append privileges: %v", err)
	}
	if !appendAllowed {
		t.Fatal("runtime role lacks SELECT or INSERT on control_audit_records")
	}
	if err := db.QueryRow(ctx, `SELECT has_table_privilege(current_user, 'control_audit_records', 'UPDATE,DELETE')`).Scan(&mutateAllowed); err != nil {
		t.Fatalf("assert control audit mutation denial: %v", err)
	}
	if mutateAllowed {
		t.Fatal("runtime role has forbidden UPDATE or DELETE on control_audit_records")
	}
}

func assertWorkspaceRuntimeSequenceAccess(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	for _, sequenceName := range workspaceRuntimeSequenceNames {
		var allowed bool
		if err := db.QueryRow(ctx, `SELECT has_sequence_privilege(current_user, $1, 'USAGE,SELECT,UPDATE')`, sequenceName).Scan(&allowed); err != nil {
			t.Fatalf("assert workspace sequence privileges for %s: %v", sequenceName, err)
		}
		if !allowed {
			t.Fatalf("runtime role lacks USAGE, SELECT, or UPDATE on %s", sequenceName)
		}
	}
}

func assertWorkspaceTenancyWriteSmoke(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	var workspaceID int64
	if err := db.QueryRow(ctx, `
INSERT INTO workspaces (public_id, owner_user_id, name)
VALUES ('ws_priv_smoke', 42, 'privilege smoke')
RETURNING internal_id`).Scan(&workspaceID); err != nil {
		t.Fatalf("insert workspace with runtime role: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE workspaces SET name = 'privilege smoke renamed', updated_at = now() WHERE public_id = 'ws_priv_smoke'`); err != nil {
		t.Fatalf("update workspace with runtime role: %v", err)
	}
	var billingID int64
	if err := db.QueryRow(ctx, `
INSERT INTO billing_accounts (public_id, workspace_id)
VALUES ('ba_priv_smoke', $1)
RETURNING internal_id`, workspaceID).Scan(&billingID); err != nil {
		t.Fatalf("insert billing account with runtime role: %v", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM billing_accounts WHERE internal_id = $1`, billingID); err != nil {
		t.Fatalf("delete billing account with runtime role: %v", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM workspaces WHERE public_id = 'ws_priv_smoke'`); err != nil {
		t.Fatalf("delete workspace with runtime role: %v", err)
	}
}

func assertControlAuditAppendOnlySmoke(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin control audit smoke transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var auditID int64
	if err := tx.QueryRow(ctx, `
INSERT INTO control_audit_records (
  public_id, operator_user_id, operation_type, capability,
  target_type, target_id, outcome, error_message,
  request_correlation_id, idempotency_key, metadata
)
VALUES (
  'audit_priv_smoke', 42, 'privilege.smoke', 'control:test',
  'smoke', 'smoke-1', 'success', '',
  'corr-priv-smoke', '', '{}'
)
RETURNING internal_id`).Scan(&auditID); err != nil {
		t.Fatalf("insert control audit record with runtime role: %v", err)
	}
	if auditID <= 0 {
		t.Fatalf("control audit insert returned invalid internal_id %d", auditID)
	}
	if _, err := tx.Exec(ctx, `UPDATE control_audit_records SET target_id = 'tampered' WHERE public_id = 'audit_priv_smoke'`); err == nil {
		t.Fatal("runtime UPDATE on control_audit_records unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("control audit UPDATE error = %v, want permission denied", err)
	}
}

func TestOpenAppDatabasesFailsBeforeRiverPoolAndClosesPoolOnPrivilegeDrift(t *testing.T) {
	fixture := newStartupDatabaseFixture(t)
	ctx := t.Context()
	if _, err := fixture.ownerDB.Exec(ctx, `GRANT UPDATE ON billing_skus TO `+fixture.roleSQL); err != nil {
		t.Fatalf("inject forbidden Billing privilege: %v", err)
	}

	runtimeDB, err := database.NewPostgres(ctx, fixture.config.Database)
	if err != nil {
		t.Fatalf("open limited-role pool: %v", err)
	}
	riverOpened := false
	_, err = openAppDatabasesWithOpeners(ctx, fixture.config, appDatabaseOpeners{
		postgres: func(context.Context, config.DatabaseConfig) (*pgxpool.Pool, error) {
			return runtimeDB, nil
		},
		river: func(context.Context, string) (*pgxpool.Pool, error) {
			riverOpened = true
			return nil, errors.New("River opener must not run after privilege drift")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Billing") || !strings.Contains(err.Error(), "forbidden UPDATE") {
		t.Fatalf("openAppDatabasesWithOpeners() error = %v, want Billing forbidden UPDATE", err)
	}
	if riverOpened {
		t.Fatal("River pool opener ran before privilege verification failed")
	}
	if pingErr := runtimeDB.Ping(ctx); pingErr == nil || !strings.Contains(strings.ToLower(pingErr.Error()), "clos") {
		t.Fatalf("limited-role pool remained open after startup failure: %v", pingErr)
	}
}

func newStartupDatabaseFixture(t *testing.T) startupDatabaseFixture {
	t.Helper()
	db, pool := testdb.OpenWithPool(t)
	ctx := t.Context()
	if err := workerriver.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate River schema: %v", err)
	}

	rawDSN := testenv.Require(t, "API_TEST_DATABASE_DSN")
	parsed, err := pgx.ParseConfig(rawDSN)
	if err != nil {
		t.Fatalf("parse API_TEST_DATABASE_DSN: %v", err)
	}
	var schemaName string
	if err := db.QueryRow(ctx, `SELECT current_schema()`).Scan(&schemaName); err != nil {
		t.Fatalf("load isolated schema: %v", err)
	}
	runtimeRole := fmt.Sprintf("startup_runtime_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	runtimePassword := fmt.Sprintf("startup-test-%d", time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	databaseSQL := pgx.Identifier{parsed.Database}.Sanitize()
	schemaSQL := pgx.Identifier{schemaName}.Sanitize()
	if _, err := db.Exec(ctx, `CREATE ROLE `+roleSQL+` LOGIN PASSWORD `+"'"+strings.ReplaceAll(runtimePassword, "'", "''")+"'"); err != nil {
		t.Fatalf("create runtime role: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.Exec(cleanupCtx, `DROP OWNED BY `+roleSQL)
		_, _ = db.Exec(cleanupCtx, `DROP ROLE IF EXISTS `+roleSQL)
	})
	if _, err := db.Exec(ctx, `GRANT CONNECT ON DATABASE `+databaseSQL+` TO `+roleSQL); err != nil {
		t.Fatalf("grant runtime database connection: %v", err)
	}
	if _, err := db.Exec(ctx,
		`ALTER ROLE `+roleSQL+` IN DATABASE `+databaseSQL+` SET search_path TO `+schemaSQL+`, public`,
	); err != nil {
		t.Fatalf("set runtime role search_path: %v", err)
	}
	if err := dbprivileges.Configure(ctx, db, runtimeRole, runtimePrivilegePolicies()...); err != nil {
		t.Fatalf("configure runtime privilege policies: %v", err)
	}

	return startupDatabaseFixture{
		ownerDB: db,
		config: &config.Config{Database: config.DatabaseConfig{
			Host:     parsed.Host,
			Port:     strconv.Itoa(int(parsed.Port)),
			User:     runtimeRole,
			Password: runtimePassword,
			Database: parsed.Database,
			SSLMode:  testDatabaseSSLMode(rawDSN),
		}},
		roleSQL: roleSQL,
	}
}

func testDatabaseSSLMode(rawDSN string) string {
	parsed, err := url.Parse(rawDSN)
	if err == nil {
		if sslMode := strings.TrimSpace(parsed.Query().Get("sslmode")); sslMode != "" {
			return sslMode
		}
	}
	for _, field := range strings.Fields(rawDSN) {
		if sslMode, ok := strings.CutPrefix(field, "sslmode="); ok && sslMode != "" {
			return sslMode
		}
	}
	return "disable"
}
