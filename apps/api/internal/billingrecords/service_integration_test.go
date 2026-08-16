package billingrecords

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestServiceListReturnsOnlyCurrentUsersFinalBillingRecords(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	creditService := credits.NewService(pool)
	seedWallet(t, ctx, pool, creditService, 42)
	seedWallet(t, ctx, pool, creditService, 99)

	welcome := grant(t, ctx, creditService, 42, 100, metadata("welcome:42", "system", "identity-lifecycle", "welcome_grant", "welcome_claim", "claim-42"))
	adminGrant := grant(t, ctx, creditService, 42, 30, metadata("grant:42", "admin", "7", "admin_grant", "admin_adjustment", "grant-42"))
	deduction := deduct(t, ctx, creditService, 42, 10, metadata("deduct:42", "admin", "7", "admin_deduction", "admin_adjustment", "deduct-42"))
	hold := hold(t, ctx, creditService, 42, 20, metadata("hold:42", "user", "42", "task_submission", "task", "task-42"))
	seedTaskSnapshot(t, ctx, pool, 42, "task-42", hold.TransactionID, "局部重绘", 20)
	capture := capture(t, ctx, creditService, 42, hold.TransactionID, 20, metadata("capture:42", "service", "task-runtime", "task_terminal", "task", "task-42"))
	_ = refund(t, ctx, creditService, 42, capture.TransactionID, 5, metadata("refund:42", "admin", "7", "admin_refund", "admin_adjustment", "refund-42"))
	correction := correct(t, ctx, creditService, 42, adminGrant.TransactionID, 5, metadata("correction:42", "admin", "7", "admin_correction", "admin_adjustment", "correction-42"))
	redemption := grant(t, ctx, creditService, 42, 15, metadata("redemption:42", "system", "rewards", "reward_redemption", "reward_redemption", "redemption-42"))
	topUp := grant(t, ctx, creditService, 42, 40, metadata("purchase:42", "system", "payments", "credit_purchase", "credit_purchase", "purchase-42"))
	other := grant(t, ctx, creditService, 99, 500, metadata("welcome:99", "system", "identity-lifecycle", "welcome_grant", "welcome_claim", "claim-99"))

	page, err := NewService(NewRepository(pool)).List(ctx, ListInput{UserID: 42})
	if err != nil {
		t.Fatalf("List() error=%v", err)
	}
	if len(page.Records) != 7 {
		t.Fatalf("records=%+v, want 7", page.Records)
	}
	wantIDs := []string{topUp.TransactionID, redemption.TransactionID, correction.TransactionID, capture.TransactionID, deduction.TransactionID, adminGrant.TransactionID, welcome.TransactionID}
	for index, wantID := range wantIDs {
		if page.Records[index].RecordID != wantID {
			t.Fatalf("record[%d]=%+v, want id=%s", index, page.Records[index], wantID)
		}
	}
	assertRecord(t, page.Records[0], KindTopUp, "Idea 充值", 40, 155)
	assertRecord(t, page.Records[1], KindReward, "兑换奖励", 15, 115)
	assertRecord(t, page.Records[2], KindSystemAdjustment, "系统调整", -5, 100)
	assertRecord(t, page.Records[3], KindAIUsage, "局部重绘", -20, 100)
	assertRecord(t, page.Records[4], KindSystemAdjustment, "系统调整", -10, 120)
	assertRecord(t, page.Records[5], KindSystemGrant, "系统赠送", 30, 130)
	assertRecord(t, page.Records[6], KindReward, "欢迎奖励", 100, 100)
	for _, record := range page.Records {
		if record.RecordID == other.TransactionID || record.CreditDelta == 0 {
			t.Fatalf("foreign or zero record leaked: %+v", record)
		}
	}
}

func TestServiceListUsesStableFilteredCursor(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	creditService := credits.NewService(pool)
	seedWallet(t, ctx, pool, creditService, 42)
	first := grant(t, ctx, creditService, 42, 10, metadata("cursor:1", "admin", "7", "admin_grant", "admin_adjustment", "cursor-1"))
	_ = hold(t, ctx, creditService, 42, 1, metadata("cursor:hold", "user", "42", "task_submission", "task", "cursor-task"))
	second := grant(t, ctx, creditService, 42, 20, metadata("cursor:2", "admin", "7", "admin_grant", "admin_adjustment", "cursor-2"))
	third := grant(t, ctx, creditService, 42, 30, metadata("cursor:3", "admin", "7", "admin_grant", "admin_adjustment", "cursor-3"))

	service := NewService(NewRepository(pool))
	page1, err := service.List(ctx, ListInput{UserID: 42, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Records) != 2 || page1.Records[0].RecordID != third.TransactionID || page1.Records[1].RecordID != second.TransactionID || page1.NextBeforeSequence == 0 {
		t.Fatalf("page1=%+v", page1)
	}
	page2, err := service.List(ctx, ListInput{UserID: 42, BeforeSequence: page1.NextBeforeSequence, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Records) != 1 || page2.Records[0].RecordID != first.TransactionID || page2.NextBeforeSequence != 0 {
		t.Fatalf("page2=%+v", page2)
	}
}

func TestServiceListRejectsBalancesOutsideJavaScriptSafeIntegerRange(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	creditService := credits.NewService(pool)
	seedWallet(t, ctx, pool, creditService, 42)
	const commandAmount int64 = 9_000_000_000_000_000
	grant(t, ctx, creditService, 42, commandAmount, metadata("unsafe:1", "admin", "7", "admin_grant", "admin_adjustment", "unsafe-1"))
	grant(t, ctx, creditService, 42, commandAmount, metadata("unsafe:2", "admin", "7", "admin_grant", "admin_adjustment", "unsafe-2"))

	_, err := NewService(NewRepository(pool)).List(ctx, ListInput{UserID: 42})
	if !errors.Is(err, ErrUnsafeNumber) {
		t.Fatalf("List() error=%v, want ErrUnsafeNumber", err)
	}
}

func TestServiceListUsesWalletSequenceIndex(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	creditService := credits.NewService(pool)
	seedWallet(t, ctx, pool, creditService, 42)
	seedWallet(t, ctx, pool, creditService, 99)
	for index := 0; index < 32; index++ {
		grant(t, ctx, creditService, 42, 1, metadata(fmt.Sprintf("plan-target:%d", index), "admin", "7", "admin_grant", "admin_adjustment", fmt.Sprintf("target-%d", index)))
	}
	for index := 0; index < 4000; index++ {
		grant(t, ctx, creditService, 99, 1, metadata(fmt.Sprintf("plan-other:%d", index), "admin", "7", "admin_grant", "admin_adjustment", fmt.Sprintf("other-%d", index)))
	}
	if _, err := pool.Exec(ctx, "ANALYZE credit_transactions"); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF) "+listRecordsSQL, int64(42), int64(0), DefaultPageLimit+1)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "idx_credit_transactions_wallet_id_desc") {
		t.Fatalf("planner did not use wallet cursor index:\n%s", plan.String())
	}
}

func assertRecord(t *testing.T, record Record, kind Kind, title string, delta, balance int64) {
	t.Helper()
	if record.Kind != kind || record.Title != title || record.CreditDelta != delta || record.AvailableCreditsAfter != balance || record.OccurredAt.IsZero() {
		t.Fatalf("record=%+v", record)
	}
}

func seedWallet(t *testing.T, ctx context.Context, pool *pgxpool.Pool, service *credits.Service, userID int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := service.LifecycleInTx(tx).ProvisionWallet(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func grant(t *testing.T, ctx context.Context, service *credits.Service, userID, amount int64, commandMetadata credits.CommandMetadata) credits.CommandResult {
	t.Helper()
	creditAmount, _ := credits.NewCreditAmount(amount)
	result, err := service.Grant(ctx, credits.GrantCommand{UserID: userID, Amount: creditAmount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func deduct(t *testing.T, ctx context.Context, service *credits.Service, userID, amount int64, commandMetadata credits.CommandMetadata) credits.CommandResult {
	t.Helper()
	creditAmount, _ := credits.NewCreditAmount(amount)
	result, err := service.Deduct(ctx, credits.DeductionCommand{UserID: userID, Amount: creditAmount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func hold(t *testing.T, ctx context.Context, service *credits.Service, userID, amount int64, commandMetadata credits.CommandMetadata) credits.CommandResult {
	t.Helper()
	creditAmount, _ := credits.NewCreditAmount(amount)
	result, err := service.Hold(ctx, credits.HoldCommand{UserID: userID, Amount: creditAmount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func capture(t *testing.T, ctx context.Context, service *credits.Service, userID int64, holdID string, amountValue int64, commandMetadata credits.CommandMetadata) credits.CommandResult {
	t.Helper()
	amount, err := credits.NewCreditAmount(amountValue)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Capture(ctx, credits.CaptureCommand{UserID: userID, HoldTransactionID: holdID, Amount: amount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func refund(t *testing.T, ctx context.Context, service *credits.Service, userID int64, captureID string, amount int64, commandMetadata credits.CommandMetadata) credits.CommandResult {
	t.Helper()
	creditAmount, _ := credits.NewCreditAmount(amount)
	result, err := service.Refund(ctx, credits.RefundCommand{UserID: userID, ChargeTransactionID: captureID, Amount: creditAmount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func correct(t *testing.T, ctx context.Context, service *credits.Service, userID int64, originalID string, amount int64, commandMetadata credits.CommandMetadata) credits.CommandResult {
	t.Helper()
	creditAmount, _ := credits.NewCreditAmount(amount)
	result, err := service.Correct(ctx, credits.CorrectionCommand{UserID: userID, OriginalTransactionID: originalID, Amount: creditAmount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func metadata(key, actorType, actorID, source, referenceType, referenceID string) credits.CommandMetadata {
	return credits.CommandMetadata{
		IdempotencyKey:    key,
		Actor:             credits.Actor{Type: credits.ActorType(actorType), ID: actorID},
		Source:            source,
		Reason:            "billing records integration test",
		BusinessReference: credits.BusinessReference{Type: referenceType, ID: referenceID},
	}
}

func seedTaskSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int64, taskID, holdID, displayName string, amount int64) {
	t.Helper()
	sku := "ai-image:" + strings.ToLower(strings.ReplaceAll(taskID, "_", "-"))
	var priceVersionID, priceVersion int64
	if err := pool.QueryRow(ctx, `
SELECT price_version_id, version
FROM publish_billing_price($1, $2, '', $3, 7)`, sku, displayName, amount).Scan(&priceVersionID, &priceVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO task_billing_snapshots (
  task_id, user_id, feature_slug, sku, display_name, price_version_id,
  price_version, unit_credits, quantity, total_credits, quoted_at, hold_transaction_id
) VALUES ($1, $2, 'partial-edit', $3, $4, $5, $6, $7, 1, $7, $8, $9)`,
		taskID, userID, sku, displayName, priceVersionID, priceVersion, amount, time.Now(), holdID,
	); err != nil {
		t.Fatal(err)
	}
}
