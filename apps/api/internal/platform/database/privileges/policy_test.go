package privileges

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/testdb"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type panicDB struct{}

func (panicDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("invalid policy reached Exec")
}

func (panicDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("invalid policy reached Query")
}

func (panicDB) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("invalid policy reached QueryRow")
}

func TestValidatePoliciesRejectsInvalidPolicyBeforeDatabaseAccess(t *testing.T) {
	tests := []struct {
		name     string
		policies []Policy
		want     string
	}{
		{name: "no policies", want: "at least one policy"},
		{name: "missing policy name", policies: []Policy{{RequireSchemaUsage: true}}, want: "name is empty"},
		{name: "empty policy", policies: []Policy{{Name: "empty"}}, want: `policy "empty" has no rules`},
		{
			name: "mixed selector",
			policies: []Policy{{Name: "mixed", Objects: []ObjectRule{{
				Kind: KindTable, Names: []string{"items"}, NamePrefix: "item_", Required: []Privilege{PrivilegeSelect},
			}}}},
			want: "exactly one of Names or NamePrefix",
		},
		{
			name: "missing selector",
			policies: []Policy{{Name: "missing", Objects: []ObjectRule{{
				Kind: KindTable, Required: []Privilege{PrivilegeSelect},
			}}}},
			want: "exactly one of Names or NamePrefix",
		},
		{
			name: "unsupported kind",
			policies: []Policy{{Name: "kind", Objects: []ObjectRule{{
				Kind: ObjectKind("view"), Names: []string{"items"}, Required: []Privilege{PrivilegeSelect},
			}}}},
			want: `unsupported object kind "view"`,
		},
		{
			name: "unsupported privilege",
			policies: []Policy{{Name: "privilege", Objects: []ObjectRule{{
				Kind: KindSequence, Names: []string{"items_id_seq"}, Required: []Privilege{PrivilegeInsert},
			}}}},
			want: `unsupported INSERT privilege for sequence`,
		},
		{
			name: "conflicting privilege",
			policies: []Policy{{Name: "conflict", Objects: []ObjectRule{{
				Kind: KindFunction, Names: []string{"do_work()"},
				Required: []Privilege{PrivilegeExecute}, Forbidden: []Privilege{PrivilegeExecute},
			}}}},
			want: "both required and forbidden",
		},
		{
			name: "duplicate object name",
			policies: []Policy{{Name: "duplicate", Objects: []ObjectRule{{
				Kind: KindTable, Names: []string{"items", "items"}, Required: []Privilege{PrivilegeSelect},
			}}}},
			want: `duplicate object name "items"`,
		},
		{
			name:     "duplicate policy name",
			policies: []Policy{{Name: "same", RequireSchemaUsage: true}, {Name: "same", RequireSchemaUsage: true}},
			want:     `duplicate policy name "same"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePolicies(tt.policies)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validatePolicies() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestConfigureRejectsInvalidPolicyBeforeDatabaseAccess(t *testing.T) {
	err := Configure(context.Background(), panicDB{}, "runtime", Policy{
		Name: "invalid",
		Objects: []ObjectRule{{
			Kind: KindTable, Names: []string{"items"}, NamePrefix: "item_", Required: []Privilege{PrivilegeSelect},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "exactly one of Names or NamePrefix") {
		t.Fatalf("Configure() error = %v", err)
	}
}

func TestConfigureRejectsStaticCrossPolicyConflictBeforeDatabaseAccess(t *testing.T) {
	err := Configure(context.Background(), panicDB{}, "runtime",
		Policy{Name: "reader", Objects: []ObjectRule{{
			Kind: KindTable, Names: []string{"items"}, Required: []Privilege{PrivilegeSelect},
		}}},
		Policy{Name: "writer", Objects: []ObjectRule{{
			Kind: KindTable, Names: []string{"items"}, Required: []Privilege{PrivilegeSelect, PrivilegeInsert},
		}}},
	)
	if err == nil || !strings.Contains(err.Error(), `policies "reader" and "writer"`) {
		t.Fatalf("Configure() error = %v", err)
	}
}

func TestValidatePoliciesNormalizesPrivilegeSets(t *testing.T) {
	policies, err := validatePolicies([]Policy{{
		Name:               "runtime",
		RequireSchemaUsage: true,
		Objects: []ObjectRule{{
			Kind:            KindTable,
			Names:           []string{"z_table", "a_table"},
			Required:        []Privilege{PrivilegeInsert, PrivilegeSelect},
			Forbidden:       []Privilege{PrivilegeTrigger, PrivilegeUpdate},
			PublicForbidden: []Privilege{PrivilegeSelect},
		}},
	}})
	if err != nil {
		t.Fatalf("validatePolicies() error = %v", err)
	}
	rule := policies[0].objects[0]
	if got, want := rule.names, []string{"a_table", "z_table"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	if got, want := rule.required, []Privilege{PrivilegeSelect, PrivilegeInsert}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required = %v, want %v", got, want)
	}
}

func TestMergeResolvedRulesRejectsCrossPolicyConflicts(t *testing.T) {
	base := resolvedRule{
		kind:        KindTable,
		name:        "items",
		required:    []Privilege{PrivilegeSelect},
		forbidden:   []Privilege{PrivilegeUpdate},
		optional:    false,
		policyNames: []string{"catalog"},
	}
	conflict := base
	conflict.required = []Privilege{PrivilegeSelect, PrivilegeInsert}
	conflict.policyNames = []string{"writer"}

	if _, err := mergeResolvedRules([]resolvedRule{base, conflict}); err == nil ||
		!strings.Contains(err.Error(), `policies "catalog" and "writer"`) {
		t.Fatalf("mergeResolvedRules() error = %v, want cross-policy conflict", err)
	}
}

func TestMergeResolvedRulesMergesIdenticalContracts(t *testing.T) {
	first := resolvedRule{
		kind: KindFunction, name: "execute_command", functionArguments: "text, bigint",
		required: []Privilege{PrivilegeExecute}, publicForbidden: []Privilege{PrivilegeExecute},
		policyNames: []string{"credits"},
	}
	second := first
	second.policyNames = []string{"tasking"}

	merged, err := mergeResolvedRules([]resolvedRule{second, first})
	if err != nil {
		t.Fatalf("mergeResolvedRules() error = %v", err)
	}
	if len(merged) != 1 || !reflect.DeepEqual(merged[0].policyNames, []string{"credits", "tasking"}) {
		t.Fatalf("mergeResolvedRules() = %+v", merged)
	}
}

func TestCompileStatementsQuotesEveryIdentifier(t *testing.T) {
	rules := []resolvedRule{
		{
			kind: KindTable, name: "order.items",
			required:        []Privilege{PrivilegeSelect, PrivilegeInsert},
			forbidden:       []Privilege{PrivilegeUpdate},
			publicForbidden: []Privilege{PrivilegeSelect},
			policyNames:     []string{"orders"},
		},
		{
			kind: KindFunction, name: `do "work"`, functionArguments: "text, bigint",
			required: []Privilege{PrivilegeExecute}, publicForbidden: []Privilege{PrivilegeExecute},
			policyNames: []string{"commands"},
		},
	}

	got := compileStatements(`odd schema`, `runtime "role`, []string{"platform"}, rules)
	want := []compiledStatement{
		{policyNames: []string{"platform"}, sql: `GRANT USAGE ON SCHEMA "odd schema" TO "runtime ""role"`},
		{policyNames: []string{"orders"}, sql: `REVOKE SELECT ON TABLE "odd schema"."order.items" FROM PUBLIC`},
		{policyNames: []string{"orders"}, sql: `REVOKE UPDATE ON TABLE "odd schema"."order.items" FROM "runtime ""role"`},
		{policyNames: []string{"orders"}, sql: `GRANT SELECT, INSERT ON TABLE "odd schema"."order.items" TO "runtime ""role"`},
		{policyNames: []string{"commands"}, sql: `REVOKE EXECUTE ON FUNCTION "odd schema"."do ""work"""(text, bigint) FROM PUBLIC`},
		{policyNames: []string{"commands"}, sql: `GRANT EXECUTE ON FUNCTION "odd schema"."do ""work"""(text, bigint) TO "runtime ""role"`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("compileStatements() =\n%#v\nwant\n%#v", got, want)
	}
}

type queryCountingDB struct {
	DBTX
	counts map[string]int
}

func newQueryCountingDB(db DBTX) *queryCountingDB {
	return &queryCountingDB{DBTX: db, counts: make(map[string]int)}
}

func (db *queryCountingDB) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	db.record(query)
	return db.DBTX.Query(ctx, query, args...)
}

func (db *queryCountingDB) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	db.record(query)
	return db.DBTX.QueryRow(ctx, query, args...)
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
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatalf("prepare privilege fixture: %v", err)
		}
	}

	runtimeRole := fmt.Sprintf("policy_runtime_%d_%d", os.Getpid(), time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	if _, err := db.Exec(ctx, `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create runtime role: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.Exec(cleanupCtx, `DROP OWNED BY `+roleSQL)
		_, _ = db.Exec(cleanupCtx, `DROP ROLE IF EXISTS `+roleSQL)
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

	conn, err := db.Acquire(ctx)
	if err != nil {
		t.Fatalf("open runtime connection: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SET ROLE `+roleSQL); err != nil {
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
	if _, err := conn.Exec(ctx, `INSERT INTO "policy.table" (id) VALUES (1)`); err != nil {
		t.Fatalf("required table INSERT privilege unavailable: %v", err)
	}
	var insertedID int64
	if err := conn.QueryRow(ctx, `SELECT id FROM "policy.table" WHERE id = 1`).Scan(&insertedID); err != nil || insertedID != 1 {
		t.Fatalf("required table SELECT result = %d, error = %v", insertedID, err)
	}

	if _, err := conn.Exec(ctx, `SELECT policy_allowed('ok')`); err != nil {
		t.Fatalf("required function privilege unavailable: %v", err)
	}
	if _, err := conn.Exec(ctx, `SELECT policy_blocked()`); err == nil {
		t.Fatal("forbidden function privilege remained available")
	}
	if _, err := conn.Exec(ctx, `UPDATE "policy.table" SET id = id`); err == nil {
		t.Fatal("forbidden table privilege remained available")
	}
	if _, err := conn.Exec(ctx, `SELECT nextval('policy_sequence_a')`); err != nil {
		t.Fatalf("required sequence privilege unavailable: %v", err)
	}
	if _, err := conn.Exec(ctx, `SELECT setval('policy_sequence_a', 10)`); err == nil {
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
			if _, err := db.Exec(ctx, driftCase.statement); err != nil {
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
	if _, err := conn.Exec(ctx, `RESET ROLE`); err != nil {
		t.Fatalf("RESET ROLE: %v", err)
	}
}

func TestConfigureFailsBeforeMutationWhenRequiredObjectIsMissing(t *testing.T) {
	db := testdb.Open(t)
	ctx := t.Context()
	if _, err := db.Exec(ctx, `CREATE TABLE policy_existing (id BIGINT)`); err != nil {
		t.Fatalf("create existing table: %v", err)
	}
	runtimeRole := fmt.Sprintf("policy_missing_%d_%d", os.Getpid(), time.Now().UnixNano())
	roleSQL := pgx.Identifier{runtimeRole}.Sanitize()
	if _, err := db.Exec(ctx, `CREATE ROLE `+roleSQL+` NOLOGIN`); err != nil {
		t.Fatalf("create runtime role: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DROP OWNED BY `+roleSQL)
		_, _ = db.Exec(context.Background(), `DROP ROLE IF EXISTS `+roleSQL)
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
	if err := db.QueryRow(ctx,
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
