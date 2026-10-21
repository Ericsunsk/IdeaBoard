package tests

import (
	"errors"
	"testing"

	"ideaboard/api/internal/credits"
	rewardsdomain "ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"
	"ideaboard/api/internal/testdb"
)

// TestRewardRedemptionCreditsBeneficiaryPersonalAccount pins the #921
// beneficiary rule at the Rewards service seam: a successful redemption
// Grants through credits.Commands into the caller's deterministic initial
// personal Billing Account, records that account on the result, and never
// touches another user's personal funds. Repeating the same claim replays
// the first redemption without a second Grant.
func TestRewardRedemptionCreditsBeneficiaryPersonalAccount(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "受益账户奖励", Code: "BENEFICIARY-2026", CreditAmount: 50,
		IdempotencyKey: "reward-code-beneficiary", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	provisionRewardWallet(t, pool, creditService, 3)
	provisionRewardWallet(t, pool, creditService, 7)

	beneficiaryAccount, err := creditService.ResolvePersonalAccount(t.Context(), 3)
	if err != nil {
		t.Fatalf("ResolvePersonalAccount() error = %v", err)
	}
	otherAccount, err := creditService.ResolvePersonalAccount(t.Context(), 7)
	if err != nil {
		t.Fatalf("ResolvePersonalAccount() error = %v", err)
	}
	if beneficiaryAccount == otherAccount {
		t.Fatal("personal accounts must stay isolated per user")
	}

	first, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "BENEFICIARY-2026"})
	if err != nil {
		t.Fatalf("Redeem() error = %v", err)
	}
	if first.BillingAccountID != beneficiaryAccount {
		t.Fatalf("RedeemResult.BillingAccountID = %d, want beneficiary personal account %d",
			first.BillingAccountID, beneficiaryAccount)
	}
	if first.AvailableBalance != 50 || first.CreditedAmount != 50 || first.Replayed {
		t.Fatalf("first Redeem() = %+v, want credited 50 with beneficiary balance", first)
	}

	// The same claim from the same user replays without a second Grant,
	// and the replay names the same beneficiary account.
	replay, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "  beneficiary-2026 "})
	if err != nil {
		t.Fatalf("replay Redeem() error = %v", err)
	}
	if !replay.Replayed || replay.Redemption.ID != first.Redemption.ID ||
		replay.BillingAccountID != beneficiaryAccount || replay.AvailableBalance != 50 {
		t.Fatalf("replay Redeem() = %+v, want same redemption on beneficiary %d", replay, beneficiaryAccount)
	}

	// Another user's personal funds are never implicitly touched.
	otherWallet, err := rewardWallet(t, creditService, 7)
	if err != nil {
		t.Fatalf("GetWallet() error = %v", err)
	}
	if otherWallet.Available != 0 {
		t.Fatalf("other personal wallet available = %d, want 0", otherWallet.Available)
	}

	var redemptionCount, grants, beneficiaryGrants int
	if err := pool.QueryRow(t.Context(), `SELECT
  (SELECT redemption_count FROM reward_codes WHERE id = $1),
  (SELECT COUNT(*) FROM credit_transactions WHERE business_reference_type = 'reward_code_redemption'),
  (SELECT COUNT(*) FROM credit_transactions
    WHERE business_reference_type = 'reward_code_redemption' AND billing_account_id = $2)`,
		created.RewardCode.ID, beneficiaryAccount,
	).Scan(&redemptionCount, &grants, &beneficiaryGrants); err != nil {
		t.Fatal(err)
	}
	if redemptionCount != 1 || grants != 1 || beneficiaryGrants != 1 {
		t.Fatalf("facts redemption_count=%d grants=%d beneficiary_grants=%d, want 1/1/1",
			redemptionCount, grants, beneficiaryGrants)
	}
}

// TestRewardRedemptionFrozenWalletGrantsNothing verifies the frozen
// beneficiary wallet fails closed with no capacity consumed and no Ledger
// Grant, keeping the redemption + Grant atomicity on the unhappy path.
func TestRewardRedemptionFrozenWalletGrantsNothing(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	created, err := service.CreateRewardCode(t.Context(), ports.CreateRewardCodeCommand{
		DisplayName: "冻结钱包奖励", Code: "FROZEN-BENEFICIARY-2026", CreditAmount: 12,
		IdempotencyKey: "reward-code-frozen-beneficiary", ActorUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisionRewardWallet(t, pool, creditService, 3)

	freezeTx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := creditService.LifecycleInTx(freezeTx).SetSoleOwnedWalletsFrozen(t.Context(), 3, true); err != nil {
		_ = freezeTx.Rollback(t.Context())
		t.Fatalf("SetSoleOwnedWalletsFrozen() error = %v", err)
	}
	if err := freezeTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Redeem(t.Context(), ports.RedeemCommand{UserID: 3, Code: "FROZEN-BENEFICIARY-2026"}); !errors.Is(err, rewardsdomain.ErrWalletFrozen) {
		t.Fatalf("Redeem() error = %v, want frozen beneficiary wallet failure", err)
	}

	var count, redemptions, grants int
	if err := pool.QueryRow(t.Context(), `SELECT
  (SELECT redemption_count FROM reward_codes WHERE id = $1),
  (SELECT COUNT(*) FROM reward_redemptions WHERE reward_code_id = $1),
  (SELECT COUNT(*) FROM credit_transactions WHERE business_reference_type = 'reward_code_redemption')`,
		created.RewardCode.ID,
	).Scan(&count, &redemptions, &grants); err != nil {
		t.Fatal(err)
	}
	if count != 0 || redemptions != 0 || grants != 0 {
		t.Fatalf("frozen facts count=%d redemptions=%d grants=%d, want 0/0/0", count, redemptions, grants)
	}
}
