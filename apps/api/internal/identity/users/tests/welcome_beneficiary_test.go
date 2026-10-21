package tests

import (
	"context"
	"errors"
	"testing"

	"ideaboard/api/internal/credits"
	creditsdomain "ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
)

// TestWelcomeGrantCreditsInitialPersonalAccount pins the #921 Welcome path:
// eligibility follows the verified personal identity, the Grant settles
// through the same Billing Account/Wallet/Commands chain into the
// claimant's explicit initial personal managed account, and the result
// names that account. Re-provisioning the same identity never mints a
// second Grant.
func TestWelcomeGrantCreditsInitialPersonalAccount(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newLifecycleService(pool, creditService, []byte("0123456789abcdef0123456789abcdef"))
	ctx := context.Background()
	identity := VerifiedIdentity{
		AuthUserID: "auth-welcome-beneficiary", DisplayName: "welcome-beneficiary",
		ClaimIdentity: "welcome-beneficiary@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	}

	first, err := service.ProvisionIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("ProvisionIdentity() error = %v", err)
	}
	if !first.WelcomeGranted {
		t.Fatalf("ProvisionIdentity() WelcomeGranted = false, want first-claim grant")
	}
	personalAccount, err := creditService.ResolvePersonalAccount(ctx, first.User.ID)
	if err != nil {
		t.Fatalf("ResolvePersonalAccount() error = %v", err)
	}
	if first.Wallet.BillingAccountID != personalAccount {
		t.Fatalf("provisioned Wallet.BillingAccountID = %d, want initial personal account %d",
			first.Wallet.BillingAccountID, personalAccount)
	}
	if first.Wallet.Available != 100 || first.Wallet.Reserved != 0 {
		t.Fatalf("provisioned wallet = %+v, want available=100 reserved=0", first.Wallet)
	}

	var grantAccount, grantCount int64
	if err := pool.QueryRow(ctx, `SELECT billing_account_id, COUNT(*)
FROM credit_transactions
WHERE business_reference_type = 'welcome_claim'
GROUP BY billing_account_id`).Scan(&grantAccount, &grantCount); err != nil {
		t.Fatalf("read welcome Grant facts: %v", err)
	}
	if grantAccount != personalAccount || grantCount != 1 {
		t.Fatalf("welcome Grant account=%d rows=%d, want account %d rows 1",
			grantAccount, grantCount, personalAccount)
	}

	replay, err := service.ProvisionIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("replayed ProvisionIdentity() error = %v", err)
	}
	if replay.WelcomeGranted || replay.User.ID != first.User.ID || replay.Wallet.BillingAccountID != personalAccount {
		t.Fatalf("replay = %+v, want same user/account without a new grant", replay)
	}
	if replay.Wallet.Available != 100 {
		t.Fatalf("replay wallet available = %d, want unchanged 100", replay.Wallet.Available)
	}
}

// TestWelcomeGrantSurvivesBillingAccountOwnershipTransfer verifies that
// moving account management (owner handover) never mints a new Welcome
// Grant: re-provisioning either identity still replays, both personal
// balances stay exactly at their single grants, and no new welcome_claim
// Ledger fact appears.
func TestWelcomeGrantSurvivesBillingAccountOwnershipTransfer(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newLifecycleService(pool, creditService, []byte("0123456789abcdef0123456789abcdef"))
	ctx := context.Background()
	accounts := tenancypg.NewBillingAccountStore(pool)

	owner, err := service.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-transfer-owner", DisplayName: "transfer-owner",
		ClaimIdentity: "transfer-owner@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("provision owner: %v", err)
	}
	successor, err := service.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-transfer-successor", DisplayName: "transfer-successor",
		ClaimIdentity: "transfer-successor@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("provision successor: %v", err)
	}
	ownerAccount := owner.Wallet.BillingAccountID
	successorAccount := successor.Wallet.BillingAccountID
	if ownerAccount <= 0 || successorAccount <= 0 || ownerAccount == successorAccount {
		t.Fatalf("personal accounts owner=%d successor=%d, want two distinct accounts",
			ownerAccount, successorAccount)
	}

	if _, err := accounts.AddBillingAccountMember(ctx, ownerAccount, owner.User.ID, successor.User.ID, tenancy.BillingRoleAdmin); err != nil {
		t.Fatalf("AddBillingAccountMember() error = %v", err)
	}
	if err := accounts.TransferBillingAccountOwnership(ctx, ownerAccount, owner.User.ID, successor.User.ID); err != nil {
		t.Fatalf("TransferBillingAccountOwnership() error = %v", err)
	}

	ownerReplay, err := service.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-transfer-owner", DisplayName: "transfer-owner",
		ClaimIdentity: "transfer-owner@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("replay owner after transfer: %v", err)
	}
	successorReplay, err := service.ProvisionIdentity(ctx, VerifiedIdentity{
		AuthUserID: "auth-transfer-successor", DisplayName: "transfer-successor",
		ClaimIdentity: "transfer-successor@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	})
	if err != nil {
		t.Fatalf("replay successor after transfer: %v", err)
	}
	if ownerReplay.WelcomeGranted || successorReplay.WelcomeGranted {
		t.Fatalf("transfer minted a new grant: owner=%+v successor=%+v", ownerReplay, successorReplay)
	}
	if ownerReplay.Wallet.Available != 100 || successorReplay.Wallet.Available != 100 {
		t.Fatalf("balances after transfer owner=%d successor=%d, want 100/100",
			ownerReplay.Wallet.Available, successorReplay.Wallet.Available)
	}

	var welcomeClaims, welcomeGrants int
	if err := pool.QueryRow(ctx, `SELECT
  (SELECT COUNT(*) FROM credit_welcome_claims),
  (SELECT COUNT(*) FROM credit_transactions WHERE business_reference_type = 'welcome_claim')`,
	).Scan(&welcomeClaims, &welcomeGrants); err != nil {
		t.Fatal(err)
	}
	if welcomeClaims != 2 || welcomeGrants != 2 {
		t.Fatalf("welcome claims=%d grants=%d, want 2/2 (transfer mints nothing)", welcomeClaims, welcomeGrants)
	}
}

// TestWelcomeGrantKeyDriftFailsClosed verifies the HMAC key lifecycle: a
// mismatched Welcome identity key fails provisioning instead of minting a
// second claim for the same verified identity under a new fingerprint.
func TestWelcomeGrantKeyDriftFailsClosed(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newLifecycleService(pool, creditService, []byte("0123456789abcdef0123456789abcdef"))
	ctx := context.Background()
	identity := VerifiedIdentity{
		AuthUserID: "auth-key-drift", DisplayName: "key-drift",
		ClaimIdentity: "key-drift@example.com", ClaimIdentityKind: "canonical_email",
		ClaimIdentityVerified: true, Role: "user",
	}
	if _, err := service.ProvisionIdentity(ctx, identity); err != nil {
		t.Fatalf("ProvisionIdentity() error = %v", err)
	}

	drifted := newLifecycleService(pool, creditService, []byte("fedcba9876543210fedcba9876543210"))
	if _, err := drifted.ProvisionIdentity(ctx, identity); !errors.Is(err, creditsdomain.ErrWelcomeIdentityKeyMismatch) {
		t.Fatalf("drifted ProvisionIdentity() error = %v, want ErrWelcomeIdentityKeyMismatch", err)
	}

	var welcomeClaims int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM credit_welcome_claims`).Scan(&welcomeClaims); err != nil {
		t.Fatal(err)
	}
	if welcomeClaims != 1 {
		t.Fatalf("welcome claims=%d, want 1 (drift mints nothing)", welcomeClaims)
	}
}
