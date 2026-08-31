package bootstrap

import (
	"context"
	"database/sql"
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
	"ideaboard/api/internal/runtime/worker"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lib/pq"
)

type startupDatabaseFixture struct {
	ownerDB *sql.DB
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
	if err := databases.db.PingContext(t.Context()); err != nil {
		t.Fatalf("ping startup SQL pool: %v", err)
	}
	if err := databases.riverPool.Ping(t.Context()); err != nil {
		t.Fatalf("ping startup River pool: %v", err)
	}
	assertProviderEndpointConfigurationAccess(t, databases.db)
	assertLibraryMaterializationMappingAccess(t, databases.db)
}

func assertLibraryMaterializationMappingAccess(t *testing.T, db *sql.DB) {
	t.Helper()
	var allowed bool
	if err := db.QueryRowContext(t.Context(), `
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

func assertProviderEndpointConfigurationAccess(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := t.Context()
	var endpointID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO provider_endpoints (api_type, protocol, name, base_url)
VALUES ('custom', 'image-url-json-to-image', 'runtime privilege test', 'https://runtime-privilege.test')
RETURNING id`).Scan(&endpointID); err != nil {
		t.Fatalf("insert provider endpoint with runtime role: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO provider_endpoint_capabilities (provider_endpoint_id, capability)
VALUES ($1, 'upscale')`, endpointID); err != nil {
		t.Fatalf("insert provider endpoint capability with runtime role: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO provider_endpoint_models (provider_endpoint_id, model_name, supports_2k, supports_4k)
VALUES ($1, 'upscale-model', TRUE, TRUE)`, endpointID); err != nil {
		t.Fatalf("insert provider endpoint model with runtime role: %v", err)
	}
	var capabilityCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*)
FROM provider_endpoint_capabilities
WHERE provider_endpoint_id = $1`, endpointID).Scan(&capabilityCount); err != nil {
		t.Fatalf("read provider endpoint capabilities with runtime role: %v", err)
	}
	if capabilityCount != 1 {
		t.Fatalf("provider endpoint capability count = %d, want 1", capabilityCount)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM provider_endpoints WHERE id = $1`, endpointID); err != nil {
		t.Fatalf("delete provider endpoint with runtime role: %v", err)
	}
}

func TestOpenAppDatabasesFailsBeforeRiverPoolAndClosesSQLPoolOnPrivilegeDrift(t *testing.T) {
	fixture := newStartupDatabaseFixture(t)
	ctx := t.Context()
	if _, err := fixture.ownerDB.ExecContext(ctx, `GRANT UPDATE ON billing_skus TO `+fixture.roleSQL); err != nil {
		t.Fatalf("inject forbidden Billing privilege: %v", err)
	}

	runtimeDB, err := database.NewPostgres(fixture.config.Database)
	if err != nil {
		t.Fatalf("open limited-role SQL pool: %v", err)
	}
	riverOpened := false
	_, err = openAppDatabasesWithOpeners(ctx, fixture.config, appDatabaseOpeners{
		postgres: func(config.DatabaseConfig) (*sql.DB, error) {
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
	if pingErr := runtimeDB.PingContext(ctx); pingErr == nil || !strings.Contains(pingErr.Error(), "database is closed") {
		t.Fatalf("limited-role SQL pool remained open after startup failure: %v", pingErr)
	}
}

func newStartupDatabaseFixture(t *testing.T) startupDatabaseFixture {
	t.Helper()
	db, pool := testdb.OpenWithPool(t)
	ctx := t.Context()
	if err := worker.MigrateRiver(ctx, pool); err != nil {
		t.Fatalf("migrate River schema: %v", err)
	}

	rawDSN := strings.TrimSpace(os.Getenv("API_TEST_DATABASE_DSN"))
	parsed, err := pgx.ParseConfig(rawDSN)
	if err != nil {
		t.Fatalf("parse API_TEST_DATABASE_DSN: %v", err)
	}
	var schemaName string
	if err := db.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schemaName); err != nil {
		t.Fatalf("load isolated schema: %v", err)
	}
	runtimeRole := fmt.Sprintf("startup_runtime_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	runtimePassword := fmt.Sprintf("startup-test-%d", time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	databaseSQL := pgx.Identifier{parsed.Database}.Sanitize()
	schemaSQL := pgx.Identifier{schemaName}.Sanitize()
	if _, err := db.ExecContext(ctx, `CREATE ROLE `+roleSQL+` LOGIN PASSWORD `+pq.QuoteLiteral(runtimePassword)); err != nil {
		t.Fatalf("create runtime role: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `DROP OWNED BY `+roleSQL)
		_, _ = db.ExecContext(cleanupCtx, `DROP ROLE IF EXISTS `+roleSQL)
	})
	if _, err := db.ExecContext(ctx, `GRANT CONNECT ON DATABASE `+databaseSQL+` TO `+roleSQL); err != nil {
		t.Fatalf("grant runtime database connection: %v", err)
	}
	if _, err := db.ExecContext(ctx,
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
