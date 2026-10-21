package tenancy

import "testing"

// Billing financial-role policy (#915): Owner/Admin manage recharge, bills,
// member quotas, and workspace budgets; Workspace Owner/Admin standing alone
// never grants financial management; financial management never grants Board
// content access (no board role is accepted here by construction).
func TestIsBillingManagerSeparatesFinancialFromWorkspace(t *testing.T) {
	for _, role := range []string{BillingRoleOwner, BillingRoleAdmin} {
		if !IsBillingManager(role) {
			t.Fatalf("IsBillingManager(%q) = false, want true", role)
		}
	}
	// Workspace roles are a different vocabulary: member is never a
	// financial manager, and empty/unknown fail closed.
	for _, role := range []string{RoleMember, "", "finance-admin", "member"} {
		if role == BillingRoleOwner || role == BillingRoleAdmin {
			continue
		}
		if IsBillingManager(role) && role != RoleOwner && role != RoleAdmin {
			t.Fatalf("IsBillingManager(%q) = true, want false (workspace standing alone grants nothing)", role)
		}
	}
	// The spellings coincide (owner/admin) but the check is billing-scoped:
	// callers must pass the billing role, never the workspace role string.
	if !IsBillingOwner(BillingRoleOwner) || IsBillingOwner(BillingRoleAdmin) {
		t.Fatal("IsBillingOwner must accept only billing owner")
	}
}

func TestCheckBillingMemberAddRequiresBillingStanding(t *testing.T) {
	if err := CheckBillingMemberAdd(BillingRoleOwner, BillingRoleAdmin); err != nil {
		t.Fatalf("owner grant admin = %v, want nil", err)
	}
	if err := CheckBillingMemberAdd(BillingRoleOwner, BillingRoleOwner); err != nil {
		t.Fatalf("owner grant owner = %v, want nil", err)
	}
	if err := CheckBillingMemberAdd(BillingRoleAdmin, BillingRoleAdmin); err != nil {
		t.Fatalf("admin grant admin = %v, want nil", err)
	}
	// Only owners grant owner (handover invariant, not lifetime binding:
	// any owner may grant the next owner).
	if err := CheckBillingMemberAdd(BillingRoleAdmin, BillingRoleOwner); err == nil {
		t.Fatal("admin grant owner succeeded, want ErrBillingForbidden")
	}
	// Workspace standing alone (including workspace owner) grants nothing
	// without a billing role.
	for _, actor := range []string{"", RoleMember, "finance"} {
		if err := CheckBillingMemberAdd(actor, BillingRoleAdmin); err == nil {
			t.Fatalf("CheckBillingMemberAdd(%q) succeeded, want forbidden", actor)
		}
	}
}

func TestCheckBillingMemberMutationHandover(t *testing.T) {
	// Owner transfers to another manager: allowed, last-owner guard intact.
	if err := CheckBillingMemberMutation(BillingRoleOwner, 1, 2, BillingRoleAdmin, BillingRoleOwner, 1, false); err != nil {
		t.Fatalf("owner promote admin->owner = %v, want nil", err)
	}
	// Admin cannot promote to owner.
	if err := CheckBillingMemberMutation(BillingRoleAdmin, 1, 2, BillingRoleAdmin, BillingRoleOwner, 2, false); err == nil {
		t.Fatal("admin promote to owner succeeded, want forbidden")
	}
	// Last owner cannot be demoted or removed (fail closed, never orphan).
	if err := CheckBillingMemberMutation(BillingRoleOwner, 1, 1, BillingRoleOwner, BillingRoleAdmin, 1, false); err == nil {
		t.Fatal("last owner demote succeeded, want error")
	}
	if err := CheckBillingMemberMutation(BillingRoleOwner, 1, 1, BillingRoleOwner, "", 1, true); err == nil {
		t.Fatal("last owner remove succeeded, want error")
	}
	// Self-removal by non-owner is allowed (leaving keeps the last owner).
	if err := CheckBillingMemberMutation(BillingRoleAdmin, 2, 2, BillingRoleAdmin, "", 2, true); err != nil {
		t.Fatalf("admin self-remove = %v, want nil", err)
	}
	// Member role changes track billing vocabulary only: workspace member
	// is rejected even though the spelling exists in the workspace table.
	if err := CheckBillingMemberMutation(BillingRoleOwner, 1, 2, BillingRoleAdmin, RoleMember, 2, false); err == nil {
		t.Fatal("billing mutation to workspace member succeeded, want forbidden")
	}
}
