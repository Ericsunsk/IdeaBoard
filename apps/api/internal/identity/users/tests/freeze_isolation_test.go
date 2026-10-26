package tests

import (
	"context"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/testdb"
)

// TestBanFreezesOnlySoleOwnedAccountsAndKeepsSharedFundsMoving pins the
// #916 freeze isolation contract: banning a user freezes only
// non-organization accounts they solely own. Organization wallets and other
// users' wallets stay active, pre-existing Holds on the banned user's own
// account still converge, and deletion detaches shared management without
// freezing shared funds.
func TestBanFreezesOnlySoleOwnedAccountsAndKeepsSharedFundsMoving(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newLifecycleService(pool, creditService, []byte("0123456789abcdef0123456789abcdef"))
	ctx := context.Background()

	banned, err := service.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-freeze-iso-1", DisplayName: "freeze-user",
		ClaimIdentity: "freeze-user@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("ProvisionIdentity(banned) error = %v", err)
	}
	other, err := service.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-freeze-iso-2", DisplayName: "other-user",
		ClaimIdentity: "other-user@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("ProvisionIdentity(other) error = %v", err)
	}
	bannedAccount, err := creditService.ResolvePersonalAccount(ctx, banned.User.ID)
	if err != nil {
		t.Fatal(err)
	}

	// A studio account solely owned by the other user, with the banned user
	// as an additional admin manager holding no ownership.
	var studioAccountID int64
	if err := pool.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, other.User.ID).Scan(&studioAccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO billing_account_members (billing_account_id, user_id, role) VALUES ($1, $2, 'admin') ON CONFLICT DO NOTHING`, studioAccountID, banned.User.ID); err != nil {
		t.Fatal(err)
	}

	// An organization pool the banned user co-manages: personal money must
	// never flow into it, and freezing the member must never freeze it.
	var orgID int64
	if err := pool.QueryRow(ctx, `INSERT INTO organizations (public_id, name) VALUES ('org_freeze_iso', 'freeze org') RETURNING internal_id`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	var orgAccountID int64
	if err := pool.QueryRow(ctx, `INSERT INTO billing_accounts (public_id, organization_id) VALUES ('ba_freeze_iso_org', $1) RETURNING internal_id`, orgID).Scan(&orgAccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO billing_account_members (billing_account_id, user_id, role) VALUES ($1, $2, 'owner'), ($1, $3, 'owner')`, orgAccountID, banned.User.ID, other.User.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := creditService.Administrative().Grant(ctx, ports.GrantCommand{
		BillingAccountID: orgAccountID,
		Amount:           domain.CreditAmount(500),
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "freeze-iso:org-grant",
			Actor:             domain.Actor{Type: domain.ActorAdmin, ID: "7"},
			Source:            "admin_grant",
			BusinessReference: domain.BusinessReference{Type: "admin_adjustment", ID: "freeze-iso-org"},
		},
	}); err != nil {
		t.Fatalf("Grant(org) error = %v", err)
	}

	// A pre-existing Hold on the banned user's own account must still
	// converge after the ban (payment obligations survive the freeze).
	hold, err := creditService.Settlement().Hold(ctx, ports.HoldCommand{
		BillingAccountID: bannedAccount,
		Amount:           domain.CreditAmount(20),
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "freeze-iso:hold",
			Actor:             domain.Actor{Type: domain.ActorUser, ID: "freeze-user"},
			Source:            "task_submission",
			Reason:            "pre-ban obligation",
			BusinessReference: domain.BusinessReference{Type: "task", ID: "task_freeze_iso"},
		},
	})
	if err != nil {
		t.Fatalf("Hold(pre-ban) error = %v", err)
	}

	if err := service.SetUserStatus(ctx, banned.User.ID, 0); err != nil {
		t.Fatalf("SetUserStatus(banned) error = %v", err)
	}

	personal, err := creditService.GetWallet(ctx, bannedAccount)
	if err != nil || personal.Status != domain.WalletStatusFrozen {
		t.Fatalf("personal wallet = %+v err = %v, want frozen", personal, err)
	}
	studio, err := creditService.GetWallet(ctx, studioAccountID)
	if err != nil || studio.Status != domain.WalletStatusActive {
		t.Fatalf("studio wallet = %+v err = %v, want active (other user's funds)", studio, err)
	}
	org, err := creditService.GetWallet(ctx, orgAccountID)
	if err != nil || org.Status != domain.WalletStatusActive || org.Available != 500 {
		t.Fatalf("org wallet = %+v err = %v, want active with 500", org, err)
	}
	// New spending on the frozen personal account is rejected...
	if _, err := creditService.Settlement().Hold(ctx, lifecycleHold(t, creditService, banned.User.ID, "freeze-iso:post-ban")); err == nil {
		t.Fatal("Hold(frozen personal account) succeeded, want rejection")
	}
	// ...while the pre-existing Hold still converges...
	if _, err := creditService.Settlement().Release(ctx, ports.ReleaseCommand{
		BillingAccountID:  bannedAccount,
		HoldTransactionID: hold.TransactionID,
		Amount:            domain.CreditAmount(20),
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "freeze-iso:release",
			Actor:             domain.Actor{Type: domain.ActorUser, ID: "freeze-user"},
			Source:            "task_submission",
			Reason:            "post-ban convergence",
			BusinessReference: domain.BusinessReference{Type: "task", ID: "task_freeze_iso"},
		},
	}); err != nil {
		t.Fatalf("Release(pre-ban hold) error = %v", err)
	}
	// ...and shared funds keep moving under the other manager.
	if _, err := creditService.Settlement().Hold(ctx, ports.HoldCommand{
		BillingAccountID: orgAccountID,
		Amount:           domain.CreditAmount(50),
		Metadata: domain.CommandMetadata{
			IdempotencyKey:    "freeze-iso:org-hold",
			Actor:             domain.Actor{Type: domain.ActorUser, ID: "other-user"},
			Source:            "task_submission",
			Reason:            "org spending continues",
			BusinessReference: domain.BusinessReference{Type: "task", ID: "task_freeze_iso_org"},
		},
	}); err != nil {
		t.Fatalf("Hold(org during member ban) error = %v", err)
	}

	if err := service.DeleteUser(ctx, banned.User.ID, &recordingAuthUserDeleter{}, &recordingUserUploadCleaner{}); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}
	var studioMembers, orgManagers int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_account_members WHERE billing_account_id = $1`, studioAccountID).Scan(&studioMembers); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM billing_account_members WHERE billing_account_id = $1`, orgAccountID).Scan(&orgManagers); err != nil {
		t.Fatal(err)
	}
	if studioMembers != 1 || orgManagers != 1 {
		t.Fatalf("shared memberships after deletion: studio=%d org=%d, want 1/1 (banned user detached)", studioMembers, orgManagers)
	}
	orgAfterDelete, err := creditService.GetWallet(ctx, orgAccountID)
	if err != nil || orgAfterDelete.Status != domain.WalletStatusActive {
		t.Fatalf("org wallet after deletion = %+v err = %v, want active", orgAfterDelete, err)
	}
}
