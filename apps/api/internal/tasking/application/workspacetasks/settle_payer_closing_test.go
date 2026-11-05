package workspacetasks

// Fresh DB closing assertions for #981 (part of #973).
//
// Covers the cross-member and fail-closed corners the tracer files leave
// open: A/B both submit (each bills own account, ledgers isolated),
// collaborator insufficient / frozen fail-closed with no residue and the
// other member unaffected, and closed-Organization Task submit refused
// with no residue. Org quota/shared-pool behavior itself is pinned by the
// Org regression suites; this file only adds the close gate for Tasks.
//
// Tests SKIP without API_TEST_DATABASE_DSN (house convention).

import (
	"context"
	"errors"
	"testing"

	creditsdomain "ideaboard/api/internal/credits/domain"
)

func TestClosingCrossMemberBillsOwnAccounts(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	ownerTask, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "981-close-cross-owner-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("owner Create: %v", err)
	}
	_ = env.service.StagedEvents()

	memberTask, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: collaboratorID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "981-close-cross-member-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("collaborator Create: %v", err)
	}
	_ = env.service.StagedEvents()

	if ownerTask.BillingAccountID != env.accountID {
		t.Fatalf("owner task account = %d, want owner %d", ownerTask.BillingAccountID, env.accountID)
	}
	if memberTask.BillingAccountID != collaboratorAccount {
		t.Fatalf("member task account = %d, want actor %d", memberTask.BillingAccountID, collaboratorAccount)
	}
	if ownerTask.BillingAccountID == memberTask.BillingAccountID {
		t.Fatal("A/B share one billing account, want per-actor isolation")
	}
	for _, tc := range []struct {
		name string
		id   string
		acct int64
		user int64
	}{
		{"owner", ownerTask.ID, env.accountID, env.userID},
		{"member", memberTask.ID, collaboratorAccount, collaboratorID},
	} {
		stored, err := env.store.GetTask(ctx, env.ws, tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if stored.BillingAccountID != tc.acct || stored.OwnerUserID != tc.user {
			t.Fatalf("%s triple = (%d,%d), want (%d,%d)", tc.name, stored.BillingAccountID, stored.OwnerUserID, tc.acct, tc.user)
		}
	}
	if got := env.ledgerCount(t, `billing_account_id = $1 AND command_type = 'hold'`, env.accountID); got != 1 {
		t.Fatalf("owner hold rows = %d, want 1 (own spend only)", got)
	}
	if got := env.ledgerCount(t, `billing_account_id = $1 AND command_type = 'hold'`, collaboratorAccount); got != 1 {
		t.Fatalf("actor hold rows = %d, want 1 (own spend only)", got)
	}
	if available, reserved := env.wallet(t); available != 990 || reserved != 10 {
		t.Fatalf("owner wallet = (%d,%d), want (990,10)", available, reserved)
	}
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 990 || reserved != 10 {
		t.Fatalf("actor wallet = (%d,%d), want (990,10)", available, reserved)
	}
}

func TestClosingCollaboratorInsufficientFailsClosed(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	_, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: collaboratorID,
		Service: "render", Quantity: 200, Resolution: "1k",
	}, "981-close-insufficient-"+randomAtomicHex(t, 4))
	if !errors.Is(err, ErrInsufficientCredits) {
		t.Fatalf("over-balance Create err = %v, want insufficient-credits", err)
	}
	if got := taskCount(t, env); got != 0 {
		t.Fatalf("tasks after rejected submit = %d, want 0", got)
	}
	if got := env.ledgerCount(t, `command_type = 'hold'`); got != 0 {
		t.Fatalf("hold rows after rejected submit = %d, want 0", got)
	}
	if available, reserved := collaboratorWallet(t, env, collaboratorAccount); available != 1000 || reserved != 0 {
		t.Fatalf("actor wallet after reject = (%d,%d), want (1000,0)", available, reserved)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("owner wallet after member reject = (%d,%d), want (1000,0) (counterparty untouched)", available, reserved)
	}
	if got := len(env.service.StagedEvents()); got != 0 {
		t.Fatalf("staged events after reject = %d, want 0", got)
	}

	ownerTask, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "981-close-insufficient-owner-"+randomAtomicHex(t, 4))
	if err != nil {
		t.Fatalf("owner Create after member reject: %v", err)
	}
	_ = env.service.StagedEvents()
	if ownerTask.BillingAccountID != env.accountID {
		t.Fatalf("owner task account = %d, want %d", ownerTask.BillingAccountID, env.accountID)
	}
}

func TestClosingCollaboratorFrozenWalletFailsClosed(t *testing.T) {
	env := openAtomicEnv(t, 10)
	ctx := context.Background()
	collaboratorID, collaboratorAccount := seedPersonalCollaborator(t, env)

	if _, err := env.pool.Exec(ctx, `SELECT set_billing_account_wallet_status($1, 'frozen')`, collaboratorAccount); err != nil {
		t.Fatalf("freeze actor wallet: %v", err)
	}
	_, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: collaboratorID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "981-close-frozen-"+randomAtomicHex(t, 4))
	if !errors.Is(err, ErrWalletRestricted) && !errors.Is(err, creditsdomain.ErrWalletFrozen) && !errors.Is(err, creditsdomain.ErrWalletRestricted) {
		t.Fatalf("frozen Create err = %v, want wallet-restricted/frozen", err)
	}
	if got := taskCount(t, env); got != 0 {
		t.Fatalf("tasks after frozen reject = %d, want 0", got)
	}
	if got := env.ledgerCount(t, `command_type = 'hold'`); got != 0 {
		t.Fatalf("hold rows after frozen reject = %d, want 0", got)
	}
	if got := env.ledgerCount(t, `billing_account_id = $1 AND command_type IN ('hold','capture','release')`, env.accountID); got != 0 {
		t.Fatalf("owner ledger rows after member frozen reject = %d, want 0", got)
	}
}

func TestClosingOrgClosedWorkspaceRejectsSubmit(t *testing.T) {
	env := openQuotaOrgEnv(t)
	ctx := context.Background()
	if _, err := env.pool.Exec(ctx, `UPDATE organizations SET closed_at = now() WHERE internal_id = (SELECT organization_id FROM workspaces WHERE public_id = $1)`, env.ws); err != nil {
		t.Fatalf("close org: %v", err)
	}
	_, err := env.service.Create(ctx, CreateInput{
		WorkspaceID: env.ws, OwnerUserID: env.userID,
		Service: "render", Quantity: 1, Resolution: "1k",
	}, "981-close-org-"+randomAtomicHex(t, 4))
	if !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrForbidden) {
		t.Fatalf("closed-org Create err = %v, want not-found/forbidden (fail-closed, no spend)", err)
	}
	if got := taskCount(t, env); got != 0 {
		t.Fatalf("tasks after closed-org reject = %d, want 0", got)
	}
	if got := env.ledgerCount(t, `command_type = 'hold'`); got != 0 {
		t.Fatalf("hold rows after closed-org reject = %d, want 0", got)
	}
	if available, reserved := env.wallet(t); available != 1000 || reserved != 0 {
		t.Fatalf("org wallet after closed reject = (%d,%d), want (1000,0)", available, reserved)
	}
}
