package privileges

import (
	"slices"
	"testing"
)

func TestRuntimePrivilegePolicyCoversIdentityReconciliationWithLeastPrivilege(t *testing.T) {
	policy := RuntimePrivilegePolicy()
	find := func(kind ObjectKind, name string) ObjectRule {
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
		return ObjectRule{}
	}

	assertPrivileges := func(label string, got, want []Privilege) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Fatalf("%s privileges = %v, want %v", label, got, want)
		}
	}

	runs := find(KindTable, "identity_admin_reconciliation_runs")
	assertPrivileges("runs required", runs.Required, []Privilege{
		PrivilegeSelect, PrivilegeInsert, PrivilegeUpdate,
	})

	runFindings := find(KindTable, "identity_admin_reconciliation_run_findings")
	assertPrivileges("run_findings required", runFindings.Required, []Privilege{
		PrivilegeSelect, PrivilegeInsert,
	})
	if !slices.Contains(runFindings.Forbidden, PrivilegeUpdate) {
		t.Fatal("run_findings must forbid UPDATE")
	}

	for _, sequence := range []string{
		"identity_admin_reconciliation_runs_id_seq",
		"identity_admin_reconciliation_findings_id_seq",
	} {
		rule := find(KindSequence, sequence)
		assertPrivileges(sequence+" required", rule.Required, []Privilege{
			PrivilegeUsage, PrivilegeSelect,
		})
		if !slices.Contains(rule.Forbidden, PrivilegeUpdate) {
			t.Fatalf("%s must forbid UPDATE", sequence)
		}
	}

	functions := find(KindFunction, "start_identity_admin_reconciliation(text, bigint, timestamptz, boolean)")
	assertPrivileges("function required", functions.Required, []Privilege{PrivilegeExecute})
	assertPrivileges("function public forbidden", functions.PublicForbidden, []Privilege{PrivilegeExecute})
}

func TestRuntimePrivilegePolicyCoversInviteAndNotificationTables(t *testing.T) {
	policy := RuntimePrivilegePolicy()
	find := func(kind ObjectKind, name string) ObjectRule {
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
		return ObjectRule{}
	}

	// Collaboration invite + notification facts (#957, #958) ride the
	// api-owned full-DML grant: same shape as every other api-owned table.
	for _, table := range []string{"workspace_member_invites", "user_notifications"} {
		rule := find(KindTable, table)
		assertInvitePrivileges(t, table+" required", rule.Required, []Privilege{
			PrivilegeSelect, PrivilegeInsert, PrivilegeUpdate, PrivilegeDelete,
		})
		for _, forbidden := range []Privilege{PrivilegeTruncate, PrivilegeReferences, PrivilegeTrigger} {
			if !slices.Contains(rule.Forbidden, forbidden) {
				t.Fatalf("%s must forbid %s", table, forbidden)
			}
		}
	}

	for _, sequence := range []string{
		"workspace_member_invites_internal_id_seq",
		"user_notifications_internal_id_seq",
	} {
		rule := find(KindSequence, sequence)
		assertInvitePrivileges(t, sequence+" required", rule.Required, []Privilege{
			PrivilegeUsage, PrivilegeSelect, PrivilegeUpdate,
		})
	}
}

func assertInvitePrivileges(t *testing.T, label string, got, want []Privilege) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s privileges = %v, want %v", label, got, want)
	}
}
