package privileges

import (
	"context"
	"reflect"
	"strings"
	"testing"

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
