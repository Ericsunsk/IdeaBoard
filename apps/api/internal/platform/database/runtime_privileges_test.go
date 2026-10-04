package database

import (
	"slices"
	"testing"

	dbprivileges "ideaboard/api/internal/platform/database/privileges"
)

func TestRuntimePrivilegePolicyCoversIdentityReconciliationWithLeastPrivilege(t *testing.T) {
	policy := RuntimePrivilegePolicy()
	find := func(kind dbprivileges.ObjectKind, name string) dbprivileges.ObjectRule {
		for _, rule := range policy.Objects {
			if rule.Kind != kind {
				continue
			}
			for _, candidate := range rule.Names {
				if candidate == name {
					return rule
				}
			}
		}
		t.Fatalf("missing %s rule for %q", kind, name)
		return dbprivileges.ObjectRule{}
	}

	assertPrivileges := func(label string, got, want []dbprivileges.Privilege) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Fatalf("%s privileges = %v, want %v", label, got, want)
		}
	}

	runs := find(dbprivileges.KindTable, "identity_admin_reconciliation_runs")
	assertPrivileges("runs required", runs.Required, []dbprivileges.Privilege{
		dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert, dbprivileges.PrivilegeUpdate,
	})

	runFindings := find(dbprivileges.KindTable, "identity_admin_reconciliation_run_findings")
	assertPrivileges("run_findings required", runFindings.Required, []dbprivileges.Privilege{
		dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert,
	})
	if !slices.Contains(runFindings.Forbidden, dbprivileges.PrivilegeUpdate) {
		t.Fatal("run_findings must forbid UPDATE")
	}

	for _, sequence := range []string{
		"identity_admin_reconciliation_runs_id_seq",
		"identity_admin_reconciliation_findings_id_seq",
	} {
		rule := find(dbprivileges.KindSequence, sequence)
		assertPrivileges(sequence+" required", rule.Required, []dbprivileges.Privilege{
			dbprivileges.PrivilegeUsage, dbprivileges.PrivilegeSelect,
		})
		if !slices.Contains(rule.Forbidden, dbprivileges.PrivilegeUpdate) {
			t.Fatalf("%s must forbid UPDATE", sequence)
		}
	}

	functions := find(dbprivileges.KindFunction, "start_identity_admin_reconciliation(text, bigint, text, timestamptz, boolean)")
	assertPrivileges("function required", functions.Required, []dbprivileges.Privilege{dbprivileges.PrivilegeExecute})
	assertPrivileges("function public forbidden", functions.PublicForbidden, []dbprivileges.Privilege{dbprivileges.PrivilegeExecute})
}
