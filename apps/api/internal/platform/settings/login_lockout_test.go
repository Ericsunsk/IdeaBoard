package settings

import (
	"errors"
	"testing"

	"ideaboard/api/internal/platform/errorrender"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestLoginLockoutSettingsRequireExplicitRuntimeGrant(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	roleName := testdb.CreateRestrictedRole(t, db, "auth_lockout")
	roleSQL := pgx.Identifier{roleName}.Sanitize()
	var schemaName string
	if err := db.QueryRow(ctx, `SELECT current_schema()`).Scan(&schemaName); err != nil {
		t.Fatalf("resolve test schema: %v", err)
	}
	schemaSQL := pgx.Identifier{schemaName}.Sanitize()
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- schemaSQL and roleSQL are sanitized PostgreSQL identifiers
	if _, err := db.Exec(ctx, `GRANT USAGE ON SCHEMA `+schemaSQL+` TO `+roleSQL); err != nil {
		t.Fatalf("grant runtime schema usage: %v", err)
	}
	runtimeDB := testdb.OpenPoolAsRole(t, db, roleName)
	_, err := runtimeDB.Exec(ctx, `SELECT * FROM get_auth_login_lockout_settings()`)
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "42501" {
		t.Fatalf("ungranted runtime execute error = %v, want insufficient_privilege", err)
	}
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- roleSQL is a sanitized PostgreSQL identifier
	if _, err := db.Exec(ctx, `GRANT EXECUTE ON FUNCTION get_auth_login_lockout_settings() TO `+roleSQL); err != nil {
		t.Fatalf("grant custom runtime execute: %v", err)
	}
	var threshold, duration int
	if err := runtimeDB.QueryRow(ctx, `SELECT * FROM get_auth_login_lockout_settings()`).Scan(&threshold, &duration); err != nil {
		t.Fatalf("read settings through granted seam: %v", err)
	}
	if threshold != 10 || duration != 900 {
		t.Fatalf("runtime settings = %d/%d, want 10/900", threshold, duration)
	}
	var canReadConfigs bool
	if err := runtimeDB.QueryRow(ctx, `SELECT has_table_privilege(current_user, 'system_configs', 'SELECT')`).Scan(&canReadConfigs); err != nil {
		t.Fatalf("inspect raw settings privilege: %v", err)
	}
	if canReadConfigs {
		t.Fatal("custom runtime role can read raw system settings")
	}
}

func TestRuntimeSettingsLoginLockoutDefaultsWhenConfigsAreMissing(t *testing.T) {
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	got, err := service.GetLoginLockout(t.Context())
	if err != nil || got != DefaultLoginLockoutSetting() {
		t.Fatalf("login lockout default = %+v, err=%v", got, err)
	}
	if got != (LoginLockoutSetting{Threshold: 10, DurationSeconds: 900}) {
		t.Fatalf("login lockout default = %+v, want threshold=10 duration=900", got)
	}
}

func TestRuntimeSettingsLoginLockoutUpdatesAndReadsBack(t *testing.T) {
	ctx := t.Context()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	updated, err := service.UpdateLoginLockout(ctx, LoginLockoutSetting{Threshold: 5, DurationSeconds: 300})
	if err != nil {
		t.Fatalf("update login lockout failed: %v", err)
	}
	if updated != (LoginLockoutSetting{Threshold: 5, DurationSeconds: 300}) {
		t.Fatalf("login lockout = %+v, want persisted payload", updated)
	}

	got, err := service.GetLoginLockout(ctx)
	if err != nil {
		t.Fatalf("get login lockout failed: %v", err)
	}
	if got != updated {
		t.Fatalf("login lockout readback = %+v, want %+v", got, updated)
	}
}

func TestRuntimeSettingsLoginLockoutUpdateRejectsOutOfRange(t *testing.T) {
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	for _, setting := range []LoginLockoutSetting{
		{Threshold: 2, DurationSeconds: 900},
		{Threshold: 101, DurationSeconds: 900},
		{Threshold: 10, DurationSeconds: 59},
		{Threshold: 10, DurationSeconds: 86401},
		{Threshold: 0, DurationSeconds: 0},
	} {
		if _, err := service.UpdateLoginLockout(t.Context(), setting); !errorrender.IsValidationError(err) {
			t.Fatalf("UpdateLoginLockout(%+v) error = %v, want validation error", setting, err)
		}
	}

	for _, setting := range []LoginLockoutSetting{
		{Threshold: 3, DurationSeconds: 60},
		{Threshold: 100, DurationSeconds: 86400},
	} {
		if _, err := service.UpdateLoginLockout(t.Context(), setting); err != nil {
			t.Fatalf("UpdateLoginLockout(%+v) error = %v, want edge values accepted", setting, err)
		}
	}
}

func TestRuntimeSettingsLoginLockoutFallsBackToDefaultOnInvalidRows(t *testing.T) {
	ctx := t.Context()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	if _, err := db.Exec(ctx, `INSERT INTO system_configs (config_key, value) VALUES ('auth_login_lockout_threshold', 'not-a-number'), ('auth_login_lockout_duration_seconds', '1') ON CONFLICT (config_key) DO UPDATE SET value = EXCLUDED.value`); err != nil {
		t.Fatalf("seed invalid login lockout rows: %v", err)
	}

	got, err := service.GetLoginLockout(ctx)
	if err != nil || got != DefaultLoginLockoutSetting() {
		t.Fatalf("login lockout fallback = %+v, err=%v, want defaults", got, err)
	}
}
