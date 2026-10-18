package tests

// User redeem joins the ambient guard transaction (redeem carrier
// close-out: the rewardcodes admin path already joined, the user
// redeem path was the remaining detached gap).
//
// What is proven here, against the isolated fixture database:
//   - Redeem on a carrier context lands inside the carrier
//     transaction: the redemption and credit-grant rows are readable
//     from the still-open carrier and invisible on the pool, then
//     durable after the carrier commits (an owned commit would
//     already be visible on the pool, so the pair proves no new
//     transaction was opened).
//   - A carrier rollback leaves no redemption row, no grant row, and
//     no redeemed capacity behind.
//   - A same-carrier repeat redeem of the same code is a replay, not
//     a second grant: the carrier stays usable (the replay path must
//     NOT roll the borrowed transaction back) and the commit leaves
//     exactly one redemption with one grant.
//   - With no carrier the owned-transaction path runs unchanged (the
//     existing code service suite is that proof; it is not duplicated
//     here).

import (
	"context"
	"testing"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/rewards"
	"ideaboard/api/internal/rewards/ports"
	"ideaboard/api/internal/testdb"
)

func countRedemptionRows(t *testing.T, ctx context.Context, querier rewardCarrierQuerier) (redemptions, grants int) {
	t.Helper()
	if err := querier.QueryRow(ctx, `SELECT
  (SELECT COUNT(*) FROM reward_redemptions),
  (SELECT COUNT(*) FROM credit_transactions WHERE business_reference_type = 'reward_code_redemption')`).Scan(&redemptions, &grants); err != nil {
		t.Fatalf("count redemption rows: %v", err)
	}
	return redemptions, grants
}

func seedRedeemableCode(t *testing.T, service *rewards.Service, displayName, code, idempotencyKey string) int64 {
	t.Helper()
	created, err := service.CreateRewardCode(context.Background(), ports.CreateRewardCodeCommand{
		DisplayName: displayName, Code: code, CreditAmount: 50,
		MaxRedemptions: 0, IdempotencyKey: idempotencyKey, ActorUserID: 1,
	})
	if err != nil {
		t.Fatalf("CreateRewardCode(%q) error = %v", code, err)
	}
	return created.RewardCode.ID
}

func TestRedeemJoinsAmbientCarrierTx(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	ctx := context.Background()

	rewardCodeID := seedRedeemableCode(t, service, "redeem carrier commit", "REDEEM-CARRIER-81", "redeem-carrier-commit-001")
	provisionRewardWallet(t, pool, creditService, 3)

	tx, carrier := beginRewardCarrier(t, ctx, pool)
	result, err := service.Redeem(carrier, ports.RedeemCommand{UserID: 3, Code: "REDEEM-CARRIER-81"})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Redeem() on carrier error = %v", err)
	}
	if result.Replayed || result.CreditedAmount != 50 || result.Redemption.RewardCodeID != rewardCodeID {
		_ = tx.Rollback(ctx)
		t.Fatalf("Redeem() on carrier = %+v, want fresh grant of 50", result)
	}
	// Same-transaction visibility: both rows must be readable from
	// INSIDE the still-open carrier ...
	if redemptions, grants := countRedemptionRows(t, carrier, tx); redemptions != 1 || grants != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("rows inside carrier = redemptions %d grants %d, want 1/1", redemptions, grants)
	}
	// ... and invisible outside it before the carrier commits (an
	// owned commit would already be visible on the pool).
	if redemptions, grants := countRedemptionRows(t, ctx, pool); redemptions != 0 || grants != 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("rows on pool before commit = redemptions %d grants %d, want 0/0", redemptions, grants)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx: %v", err)
	}
	if redemptions, grants := countRedemptionRows(t, ctx, pool); redemptions != 1 || grants != 1 {
		t.Fatalf("rows on pool after commit = redemptions %d grants %d, want 1/1", redemptions, grants)
	}
	var redemptionCount int64
	if err := pool.QueryRow(ctx, `SELECT redemption_count FROM reward_codes WHERE id = $1`, rewardCodeID).Scan(&redemptionCount); err != nil {
		t.Fatalf("read redemption_count: %v", err)
	}
	if redemptionCount != 1 {
		t.Fatalf("redemption_count = %d, want 1", redemptionCount)
	}
}

func TestRedeemRollsBackWithAmbientCarrier(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	ctx := context.Background()

	rewardCodeID := seedRedeemableCode(t, service, "redeem carrier rollback", "REDEEM-ROLLBACK-82", "redeem-carrier-rollback-001")
	provisionRewardWallet(t, pool, creditService, 3)

	tx, carrier := beginRewardCarrier(t, ctx, pool)
	if _, err := service.Redeem(carrier, ports.RedeemCommand{UserID: 3, Code: "REDEEM-ROLLBACK-82"}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Redeem() on carrier error = %v", err)
	}
	// The guard rolls the whole carrier back, domain writes included.
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	if redemptions, grants := countRedemptionRows(t, ctx, pool); redemptions != 0 || grants != 0 {
		t.Fatalf("rows after carrier rollback = redemptions %d grants %d, want 0/0", redemptions, grants)
	}
	var redemptionCount int64
	if err := pool.QueryRow(ctx, `SELECT redemption_count FROM reward_codes WHERE id = $1`, rewardCodeID).Scan(&redemptionCount); err != nil {
		t.Fatalf("read redemption_count: %v", err)
	}
	if redemptionCount != 0 {
		t.Fatalf("redemption_count = %d, want 0", redemptionCount)
	}
}

func TestRedeemReplayOnAmbientCarrierDoesNotDoubleGrant(t *testing.T) {
	pool := testdb.Open(t)
	creditService := credits.NewService(pool)
	service := newTestRewardService(t, pool, creditService)
	ctx := context.Background()

	seedRedeemableCode(t, service, "redeem carrier replay", "REDEEM-REPLAY-83", "redeem-carrier-replay-001")
	provisionRewardWallet(t, pool, creditService, 3)

	tx, carrier := beginRewardCarrier(t, ctx, pool)
	first, err := service.Redeem(carrier, ports.RedeemCommand{UserID: 3, Code: "REDEEM-REPLAY-83"})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("first Redeem() on carrier error = %v", err)
	}
	if first.Replayed {
		_ = tx.Rollback(ctx)
		t.Fatalf("first Redeem() = %+v, want fresh grant", first)
	}
	// The repeat redeem must observe the still-uncommitted first
	// redemption through the shared carrier and replay it. The replay
	// path must NOT roll the borrowed transaction back: an owned-mode
	// release here would discard the carrier (and the guard's audit
	// row with it), and the commit below would fail on a closed
	// transaction.
	second, err := service.Redeem(carrier, ports.RedeemCommand{UserID: 3, Code: "REDEEM-REPLAY-83"})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("second Redeem() on carrier error = %v", err)
	}
	if !second.Replayed || second.Redemption.ID != first.Redemption.ID {
		_ = tx.Rollback(ctx)
		t.Fatalf("second Redeem() = %+v, want replay of redemption %d", second, first.Redemption.ID)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit carrier tx after replay: %v", err)
	}
	if redemptions, grants := countRedemptionRows(t, ctx, pool); redemptions != 1 || grants != 1 {
		t.Fatalf("rows on pool after commit = redemptions %d grants %d, want 1/1", redemptions, grants)
	}
}
