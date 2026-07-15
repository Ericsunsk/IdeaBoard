package privileges

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
)

type queryCountingDB struct {
	DBTX
	counts map[string]int
}

func newQueryCountingDB(db DBTX) *queryCountingDB {
	return &queryCountingDB{DBTX: db, counts: make(map[string]int)}
}

func (db *queryCountingDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	db.record(query)
	return db.DBTX.QueryContext(ctx, query, args...)
}

func (db *queryCountingDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	db.record(query)
	return db.DBTX.QueryRowContext(ctx, query, args...)
}

func (db *queryCountingDB) record(query string) {
	for _, marker := range []string{
		"ideaboard:resolve-explicit-functions",
		"ideaboard:verify-schema-privileges",
		"ideaboard:verify-table-privileges",
		"ideaboard:verify-sequence-privileges",
		"ideaboard:verify-function-privileges",
	} {
		if strings.Contains(query, marker) {
			db.counts[marker]++
		}
	}
}

func TestPolicyConfiguresAndVerifiesAllPostgresObjectKinds(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	for _, statement := range []string{
		`CREATE TABLE "policy.table" (id BIGINT PRIMARY KEY)`,
		`CREATE TABLE policy_table_two (id BIGINT PRIMARY KEY)`,
		`CREATE SEQUENCE policy_sequence_b`,
		`CREATE SEQUENCE policy_sequence_a`,
		`CREATE FUNCTION policy_allowed(input TEXT) RETURNS TEXT LANGUAGE SQL AS 'SELECT input'`,
		`CREATE FUNCTION policy_allowed_two() RETURNS INTEGER LANGUAGE SQL AS 'SELECT 2'`,
		`CREATE FUNCTION policy_blocked() RETURNS INTEGER LANGUAGE SQL AS 'SELECT 1'`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("prepare privilege fixture: %v", err)
		}
	}

	runtimeRole := fmt.Sprintf("policy_runtime_%d_%d", os.Getpid(), time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	if _, err := db.ExecContext(ctx, `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create runtime role: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `DROP OWNED BY `+roleSQL)
		_, _ = db.ExecContext(cleanupCtx, `DROP ROLE IF EXISTS `+roleSQL)
	})

	platformPolicy := Policy{
		Name:               "platform",
		RequireSchemaUsage: true,
		Objects: []ObjectRule{{
			Kind: KindTable, Names: []string{"policy.table", "policy_table_two"},
			Required:        []Privilege{PrivilegeSelect, PrivilegeInsert},
			Forbidden:       []Privilege{PrivilegeUpdate, PrivilegeDelete, PrivilegeTruncate, PrivilegeReferences, PrivilegeTrigger},
			PublicForbidden: []Privilege{PrivilegeSelect},
		}},
	}
	commandPolicy := Policy{
		Name: "commands",
		Objects: []ObjectRule{
			{
				Kind: KindSequence, NamePrefix: "policy_sequence_",
				Required: []Privilege{PrivilegeUsage, PrivilegeSelect}, Forbidden: []Privilege{PrivilegeUpdate},
				PublicForbidden: []Privilege{PrivilegeUsage},
			},
			{
				Kind: KindFunction, Names: []string{"policy_allowed(text)", "policy_allowed_two()"},
				Required: []Privilege{PrivilegeExecute}, PublicForbidden: []Privilege{PrivilegeExecute},
			},
			{
				Kind: KindFunction, Names: []string{"policy_blocked()"}, Optional: true,
				Forbidden: []Privilege{PrivilegeExecute}, PublicForbidden: []Privilege{PrivilegeExecute},
			},
			{
				Kind: KindTable, NamePrefix: "does_not_exist_", Optional: true,
				Forbidden: []Privilege{PrivilegeInsert},
			},
		},
	}
	if err := Verify(ctx, db, Policy{Name: "sequence-public-default", Objects: []ObjectRule{{
		Kind: KindSequence, NamePrefix: "policy_sequence_", PublicForbidden: []Privilege{PrivilegeUsage},
	}}}); err != nil {
		t.Fatalf("Verify() default sequence PUBLIC ACL error = %v", err)
	}

	if err := Configure(ctx, db, runtimeRole, platformPolicy, commandPolicy); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	if err := Verify(ctx, db, platformPolicy, commandPolicy); err == nil {
		t.Fatal("Verify() accepted the schema owner despite forbidden privileges")
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open runtime connection: %v", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SET ROLE `+roleSQL); err != nil {
		t.Fatalf("SET ROLE: %v", err)
	}
	countingDB := newQueryCountingDB(conn)
	if err := Verify(ctx, countingDB, platformPolicy, commandPolicy); err != nil {
		t.Fatalf("Verify() runtime error = %v", err)
	}
	wantQueryCounts := map[string]int{
		"ideaboard:resolve-explicit-functions": 2,
		"ideaboard:verify-schema-privileges":   1,
		"ideaboard:verify-table-privileges":    1,
		"ideaboard:verify-sequence-privileges": 1,
		"ideaboard:verify-function-privileges": 1,
	}
	for marker, want := range wantQueryCounts {
		if got := countingDB.counts[marker]; got != want {
			t.Fatalf("Verify() query count for %s = %d, want %d", marker, got, want)
		}
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO "policy.table" (id) VALUES (1)`); err != nil {
		t.Fatalf("required table INSERT privilege unavailable: %v", err)
	}
	var insertedID int64
	if err := conn.QueryRowContext(ctx, `SELECT id FROM "policy.table" WHERE id = 1`).Scan(&insertedID); err != nil || insertedID != 1 {
		t.Fatalf("required table SELECT result = %d, error = %v", insertedID, err)
	}

	if _, err := conn.ExecContext(ctx, `SELECT policy_allowed('ok')`); err != nil {
		t.Fatalf("required function privilege unavailable: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT policy_blocked()`); err == nil {
		t.Fatal("forbidden function privilege remained available")
	}
	if _, err := conn.ExecContext(ctx, `UPDATE "policy.table" SET id = id`); err == nil {
		t.Fatal("forbidden table privilege remained available")
	}
	if _, err := conn.ExecContext(ctx, `SELECT nextval('policy_sequence_a')`); err != nil {
		t.Fatalf("required sequence privilege unavailable: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT setval('policy_sequence_a', 10)`); err == nil {
		t.Fatal("forbidden sequence UPDATE privilege remained available")
	}
	driftCases := []struct {
		name      string
		statement string
		want      string
	}{
		{name: "table required", statement: `REVOKE SELECT ON TABLE "policy.table" FROM ` + roleSQL, want: "lacks required SELECT"},
		{name: "table forbidden", statement: `GRANT UPDATE ON TABLE "policy.table" TO ` + roleSQL, want: "has forbidden UPDATE"},
		{name: "table PUBLIC", statement: `GRANT SELECT ON TABLE "policy.table" TO PUBLIC`, want: "PUBLIC has forbidden SELECT"},
		{name: "sequence required", statement: `REVOKE USAGE ON SEQUENCE policy_sequence_a FROM ` + roleSQL, want: "lacks required USAGE"},
		{name: "sequence forbidden", statement: `GRANT UPDATE ON SEQUENCE policy_sequence_a TO ` + roleSQL, want: "has forbidden UPDATE"},
		{name: "sequence PUBLIC", statement: `GRANT USAGE ON SEQUENCE policy_sequence_a TO PUBLIC`, want: "PUBLIC has forbidden USAGE"},
		{name: "function required", statement: `REVOKE EXECUTE ON FUNCTION policy_allowed(TEXT) FROM ` + roleSQL, want: "lacks required EXECUTE"},
		{name: "function forbidden", statement: `GRANT EXECUTE ON FUNCTION policy_blocked() TO ` + roleSQL, want: "has forbidden EXECUTE"},
		{name: "function PUBLIC", statement: `GRANT EXECUTE ON FUNCTION policy_allowed(TEXT) TO PUBLIC`, want: "PUBLIC has forbidden EXECUTE"},
	}
	for _, driftCase := range driftCases {
		t.Run(driftCase.name, func(t *testing.T) {
			if _, err := db.ExecContext(ctx, driftCase.statement); err != nil {
				t.Fatalf("introduce privilege drift: %v", err)
			}
			if err := Verify(ctx, conn, platformPolicy, commandPolicy); err == nil || !strings.Contains(err.Error(), driftCase.want) {
				t.Fatalf("Verify() drift error = %v, want containing %q", err, driftCase.want)
			}
			if err := Configure(ctx, db, runtimeRole, platformPolicy, commandPolicy); err != nil {
				t.Fatalf("restore privilege policy: %v", err)
			}
			if err := Verify(ctx, conn, platformPolicy, commandPolicy); err != nil {
				t.Fatalf("Verify() after restore error = %v", err)
			}
		})
	}
	if _, err := conn.ExecContext(ctx, `RESET ROLE`); err != nil {
		t.Fatalf("RESET ROLE: %v", err)
	}
}

func TestConfigureFailsBeforeMutationWhenRequiredObjectIsMissing(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `CREATE TABLE policy_existing (id BIGINT)`); err != nil {
		t.Fatalf("create existing table: %v", err)
	}
	runtimeRole := fmt.Sprintf("policy_missing_%d_%d", os.Getpid(), time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	if _, err := db.ExecContext(ctx, `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create runtime role: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP OWNED BY `+roleSQL)
		_, _ = db.ExecContext(context.Background(), `DROP ROLE IF EXISTS `+roleSQL)
	})

	policy := Policy{Name: "required-objects", Objects: []ObjectRule{
		{Kind: KindTable, Names: []string{"policy_existing"}, Required: []Privilege{PrivilegeSelect}},
		{Kind: KindTable, NamePrefix: "policy_missing_", Optional: true, Required: []Privilege{PrivilegeSelect}},
	}}
	err := Configure(ctx, db, runtimeRole, policy)
	if err == nil || !strings.Contains(err.Error(), `policy "required-objects"`) || !strings.Contains(err.Error(), "matched no objects") {
		t.Fatalf("Configure() error = %v, want missing required object", err)
	}
	var granted bool
	if err := db.QueryRowContext(ctx,
		`SELECT has_table_privilege($1, 'policy_existing', 'SELECT')`, runtimeRole,
	).Scan(&granted); err != nil {
		t.Fatalf("check pre-mutation privilege: %v", err)
	}
	if granted {
		t.Fatal("Configure() mutated privileges before resolving every required object")
	}
}

func TestVerifyPrivilegeBatchFailsClosedOnQueryError(t *testing.T) {
	db := testdb.Open(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	batch := buildVerificationBatches([]resolvedRule{{
		kind: KindTable, name: "users", required: []Privilege{PrivilegeSelect}, policyNames: []string{"runtime"},
	}})[0]
	err := verifyPrivilegeBatch(ctx, db, "runtime", "public", batch)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("verifyPrivilegeBatch() error = %v, want context cancellation", err)
	}
}
