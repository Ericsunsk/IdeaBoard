package migrations

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestAuthRuntimeSettingsFunctionExposesOnlyTypedSettings(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	conn := openIsolatedMigrationConn(t, ctx)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var registrationEnabled bool
	if err := conn.QueryRowContext(ctx, `
SELECT registration_enabled
FROM get_auth_runtime_settings()
`).Scan(&registrationEnabled); err != nil {
		t.Fatalf("load default Auth runtime settings: %v", err)
	}
	if registrationEnabled {
		t.Fatal("registration should fail closed when the setting is absent")
	}

	if _, err := conn.ExecContext(ctx, `
INSERT INTO system_configs (config_key, value)
VALUES ('auth_registration_enabled', 'true')
`); err != nil {
		t.Fatalf("enable registration setting: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `
SELECT registration_enabled
FROM get_auth_runtime_settings()
`).Scan(&registrationEnabled); err != nil {
		t.Fatalf("load enabled Auth runtime settings: %v", err)
	}
	if !registrationEnabled {
		t.Fatal("typed Auth runtime settings did not return the enabled value")
	}

	var publicCanExecute bool
	if err := conn.QueryRowContext(ctx, `
SELECT COALESCE(bool_or(
  privileges.grantee = 0
  AND privileges.privilege_type = 'EXECUTE'
), FALSE)
FROM pg_proc AS functions
JOIN pg_namespace AS schemas ON schemas.oid = functions.pronamespace
CROSS JOIN LATERAL aclexplode(
  COALESCE(functions.proacl, acldefault('f', functions.proowner))
) AS privileges
WHERE schemas.nspname = current_schema()
  AND functions.proname = 'get_auth_runtime_settings'
`).Scan(&publicCanExecute); err != nil {
		t.Fatalf("inspect Auth runtime settings privileges: %v", err)
	}
	if publicCanExecute {
		t.Fatal("PUBLIC must not execute the Auth runtime settings function")
	}

	var configuredSearchPath string
	if err := conn.QueryRowContext(ctx, `
SELECT array_to_string(functions.proconfig, ',')
FROM pg_proc AS functions
JOIN pg_namespace AS schemas ON schemas.oid = functions.pronamespace
WHERE schemas.nspname = current_schema()
  AND functions.proname = 'get_auth_runtime_settings'
`).Scan(&configuredSearchPath); err != nil {
		t.Fatalf("inspect Auth runtime settings search_path: %v", err)
	}
	for _, expected := range []string{currentSchema(t, ctx, conn), "pg_catalog", "pg_temp"} {
		if !strings.Contains(configuredSearchPath, expected) {
			t.Fatalf("Auth runtime settings search_path=%q, missing %q", configuredSearchPath, expected)
		}
	}

	raw, err := migrationFS.ReadFile("20260713010000_auth_runtime_settings.sql")
	if err != nil {
		t.Fatalf("read Auth runtime settings migration: %v", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin Auth runtime settings rollback: %v", err)
	}
	for _, statement := range parseSQLStatements(extractDownSQLForTest(string(raw))) {
		if _, err := tx.ExecContext(ctx, statement.SQL); err != nil {
			_ = tx.Rollback()
			t.Fatalf("roll back Auth runtime settings migration: %v", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM schema_migrations
WHERE version = 20260713010000
`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("remove Auth runtime settings migration marker: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit Auth runtime settings rollback: %v", err)
	}

	assertRegprocedureExists(t, ctx, conn, "get_auth_runtime_settings()", false)
	if err := upWithConn(ctx, conn); err != nil {
		t.Fatalf("reapply Auth runtime settings migration: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `
SELECT registration_enabled
FROM get_auth_runtime_settings()
`).Scan(&registrationEnabled); err != nil {
		t.Fatalf("load reapplied Auth runtime settings: %v", err)
	}
	if !registrationEnabled {
		t.Fatal("reapplied Auth runtime settings lost the stored value")
	}
}

func currentSchema(t *testing.T, ctx context.Context, conn *sql.Conn) string {
	t.Helper()
	var schemaName string
	if err := conn.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schemaName); err != nil {
		t.Fatalf("load current schema: %v", err)
	}
	return schemaName
}
