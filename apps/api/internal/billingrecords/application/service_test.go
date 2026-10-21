package application

import (
	"context"
	"errors"
	"fmt"
	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/ports"
	"strings"
	"testing"
	"time"

	billingrecordsports "ideaboard/api/internal/billingrecords/ports"
	"ideaboard/api/internal/consumptionread"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestServiceListReturnsOnlyCurrentUsersFinalBillingRecords(t *testing.T) {
	pool := testdb.Open(t)
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

	page, err := NewService(NewRepository(pool)).List(ctx, billingrecordsports.ListInput{BillingAccountID: mustBillingAccount(t, pool, 42)})
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
	assertRecord(t, page.Records[0], billingrecordsports.KindTopUp, "Idea 充值", 40, 155)
	assertRecord(t, page.Records[1], billingrecordsports.KindReward, "兑换奖励", 15, 115)
	assertRecord(t, page.Records[2], billingrecordsports.KindSystemAdjustment, "系统调整", -5, 100)
	assertRecord(t, page.Records[3], billingrecordsports.KindAIUsage, "局部重绘", -20, 100)
	assertRecord(t, page.Records[4], billingrecordsports.KindSystemAdjustment, "系统调整", -10, 120)
	assertRecord(t, page.Records[5], billingrecordsports.KindSystemGrant, "系统赠送", 30, 130)
	assertRecord(t, page.Records[6], billingrecordsports.KindReward, "欢迎奖励", 100, 100)
	for _, record := range page.Records {
		if record.RecordID == other.TransactionID || record.CreditDelta == 0 {
			t.Fatalf("foreign or zero record leaked: %+v", record)
		}
	}
}

func TestServiceListUsesStableFilteredCursor(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	creditService := credits.NewService(pool)
	seedWallet(t, ctx, pool, creditService, 42)
	first := grant(t, ctx, creditService, 42, 10, metadata("cursor:1", "admin", "7", "admin_grant", "admin_adjustment", "cursor-1"))
	_ = hold(t, ctx, creditService, 42, 1, metadata("cursor:hold", "user", "42", "task_submission", "task", "cursor-task"))
	second := grant(t, ctx, creditService, 42, 20, metadata("cursor:2", "admin", "7", "admin_grant", "admin_adjustment", "cursor-2"))
	third := grant(t, ctx, creditService, 42, 30, metadata("cursor:3", "admin", "7", "admin_grant", "admin_adjustment", "cursor-3"))

	service := NewService(NewRepository(pool))
	page1, err := service.List(ctx, billingrecordsports.ListInput{BillingAccountID: mustBillingAccount(t, pool, 42), Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Records) != 2 || page1.Records[0].RecordID != third.TransactionID || page1.Records[1].RecordID != second.TransactionID || page1.NextBeforeSequence == 0 {
		t.Fatalf("page1=%+v", page1)
	}
	page2, err := service.List(ctx, billingrecordsports.ListInput{BillingAccountID: mustBillingAccount(t, pool, 42), BeforeSequence: page1.NextBeforeSequence, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Records) != 1 || page2.Records[0].RecordID != first.TransactionID || page2.NextBeforeSequence != 0 {
		t.Fatalf("page2=%+v", page2)
	}
}

func TestServiceListRejectsBalancesOutsideJavaScriptSafeIntegerRange(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	creditService := credits.NewService(pool)
	seedWallet(t, ctx, pool, creditService, 42)
	const commandAmount int64 = 9_000_000_000_000_000
	grant(t, ctx, creditService, 42, commandAmount, metadata("unsafe:1", "admin", "7", "admin_grant", "admin_adjustment", "unsafe-1"))
	grant(t, ctx, creditService, 42, commandAmount, metadata("unsafe:2", "admin", "7", "admin_grant", "admin_adjustment", "unsafe-2"))

	_, err := NewService(NewRepository(pool)).List(ctx, billingrecordsports.ListInput{BillingAccountID: mustBillingAccount(t, pool, 42)})
	if !errors.Is(err, billingrecordsports.ErrUnsafeNumber) {
		t.Fatalf("List() error=%v, want billingrecordsports.ErrUnsafeNumber", err)
	}
}

func TestServiceListUsesWalletSequenceIndex(t *testing.T) {
	pool := testdb.Open(t)
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
	rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF) "+listRecordsSQL, mustBillingAccount(t, pool, 42), int64(0), billingrecordsports.DefaultPageLimit+1, int64(0), int64(0))
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

func assertRecord(t *testing.T, record billingrecordsports.Record, kind billingrecordsports.Kind, title string, delta, balance int64) {
	t.Helper()
	if record.Kind != kind || record.Title != title || record.CreditDelta != delta || (record.AvailableCreditsAfter == nil || *record.AvailableCreditsAfter != balance) || record.OccurredAt.IsZero() {
		t.Fatalf("record=%+v", record)
	}
}

func mustBillingAccount(t *testing.T, pool *pgxpool.Pool, userID int64) int64 {
	t.Helper()
	var accountID int64
	if err := pool.QueryRow(context.Background(), `SELECT ensure_personal_billing_account_for_user($1)`, userID).Scan(&accountID); err != nil {
		t.Fatalf("resolve personal billing account %d: %v", userID, err)
	}
	return accountID
}

func seedWallet(t *testing.T, ctx context.Context, pool *pgxpool.Pool, service *credits.Service, userID int64) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	accountID, err := service.LifecycleInTx(tx).ResolvePersonalAccount(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.LifecycleInTx(tx).ProvisionAccountWallet(ctx, accountID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func accountOf(t *testing.T, ctx context.Context, service *credits.Service, userID int64) int64 {
	t.Helper()
	accountID, err := service.ResolvePersonalAccount(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	return accountID
}

func grant(t *testing.T, ctx context.Context, service *credits.Service, userID, amount int64, commandMetadata domain.CommandMetadata) ports.CommandResult {
	t.Helper()
	creditAmount, _ := domain.NewCreditAmount(amount)
	result, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: accountOf(t, ctx, service, userID), Amount: creditAmount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func deduct(t *testing.T, ctx context.Context, service *credits.Service, userID, amount int64, commandMetadata domain.CommandMetadata) ports.CommandResult {
	t.Helper()
	creditAmount, _ := domain.NewCreditAmount(amount)
	result, err := service.Deduct(ctx, ports.DeductionCommand{BillingAccountID: accountOf(t, ctx, service, userID), Amount: creditAmount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func hold(t *testing.T, ctx context.Context, service *credits.Service, userID, amount int64, commandMetadata domain.CommandMetadata) ports.CommandResult {
	t.Helper()
	creditAmount, _ := domain.NewCreditAmount(amount)
	result, err := service.Hold(ctx, ports.HoldCommand{BillingAccountID: accountOf(t, ctx, service, userID), Amount: creditAmount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func capture(t *testing.T, ctx context.Context, service *credits.Service, userID int64, holdID string, amountValue int64, commandMetadata domain.CommandMetadata) ports.CommandResult {
	t.Helper()
	amount, err := domain.NewCreditAmount(amountValue)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Capture(ctx, ports.CaptureCommand{BillingAccountID: accountOf(t, ctx, service, userID), HoldTransactionID: holdID, Amount: amount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func refund(t *testing.T, ctx context.Context, service *credits.Service, userID int64, captureID string, amount int64, commandMetadata domain.CommandMetadata) ports.CommandResult {
	t.Helper()
	creditAmount, _ := domain.NewCreditAmount(amount)
	result, err := service.Refund(ctx, ports.RefundCommand{BillingAccountID: accountOf(t, ctx, service, userID), ChargeTransactionID: captureID, Amount: creditAmount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func correct(t *testing.T, ctx context.Context, service *credits.Service, userID int64, originalID string, amount int64, commandMetadata domain.CommandMetadata) ports.CommandResult {
	t.Helper()
	creditAmount, _ := domain.NewCreditAmount(amount)
	result, err := service.Correct(ctx, ports.CorrectionCommand{BillingAccountID: accountOf(t, ctx, service, userID), OriginalTransactionID: originalID, Amount: creditAmount, Metadata: commandMetadata})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func metadata(key, actorType, actorID, source, referenceType, referenceID string) domain.CommandMetadata {
	return domain.CommandMetadata{
		IdempotencyKey:    key,
		Actor:             domain.Actor{Type: domain.ActorType(actorType), ID: actorID},
		Source:            source,
		Reason:            "billing records integration test",
		BusinessReference: domain.BusinessReference{Type: referenceType, ID: referenceID},
	}
}

func seedTaskSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int64, taskID, holdID, displayName string, amount int64) {
	t.Helper()
	seedTaskSnapshotInScope(t, ctx, pool, userID, userID, 0, taskID, holdID, displayName, amount)
}

func seedTaskSnapshotInScope(t *testing.T, ctx context.Context, pool *pgxpool.Pool, accountOwnerID, actorID, workspaceID int64, taskID, holdID, displayName string, amount int64) {
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
  task_id, user_id, billing_account_id, service, sku, display_name, price_version_id,
  price_version, unit_credits, quantity, total_credits, quoted_at, hold_transaction_id, workspace_id
) VALUES ($1, $2, (SELECT ensure_personal_billing_account_for_user($10)), 'inpaint', $3, $4, $5, $6, $7, 1, $7, $8, $9, NULLIF($11::bigint, 0))`,
		taskID, actorID, sku, displayName, priceVersionID, priceVersion, amount, time.Now(), holdID, accountOwnerID, workspaceID,
	); err != nil {
		t.Fatal(err)
	}
}

// Filters must be applied before pagination, including while unrelated grants,
// other members, other Workspaces, and another account interleave in the Ledger.
func TestServiceListConsumptionScopesAndPagination(t *testing.T) {
	pool := testdb.Open(t)
	ctx := t.Context()
	creditService := credits.NewService(pool)
	for _, userID := range []int64{42, 99} {
		seedWallet(t, ctx, pool, creditService, userID)
		grant(t, ctx, creditService, userID, 1000, metadata(fmt.Sprintf("scope-grant:%d", userID), "admin", "7", "admin_grant", "admin_adjustment", fmt.Sprintf("scope-grant:%d", userID)))
	}
	accountID := mustBillingAccount(t, pool, 42)
	createCapture := func(owner, actor, workspace int64, id string) string {
		h := hold(t, ctx, creditService, owner, 10, metadata("hold:"+id, "user", fmt.Sprint(actor), "task_submission", "task", id))
		seedTaskSnapshotInScope(t, ctx, pool, owner, actor, workspace, id, h.TransactionID, id, 10)
		return capture(t, ctx, creditService, owner, h.TransactionID, 10, metadata("capture:"+id, "user", fmt.Sprint(actor), "task_terminal", "task", id)).TransactionID
	}
	first := createCapture(42, 42, 11, "scope-first")
	otherWorkspace := createCapture(42, 42, 12, "scope-other-workspace")
	otherMember := createCapture(42, 99, 11, "scope-other-member")
	foreign := createCapture(99, 42, 11, "scope-foreign-account")
	service := NewService(NewRepository(pool))
	for _, tc := range []struct {
		name       string
		input      billingrecordsports.ListInput
		ids        []string
		hasBalance bool
	}{
		{"member sees own account consumption across Workspaces", billingrecordsports.ListInput{ActorUserID: 42}, []string{otherWorkspace, first}, false},
		{"Workspace admin sees local members", billingrecordsports.ListInput{WorkspaceID: 11}, []string{otherMember, first}, false},
		{"combined filters intersect", billingrecordsports.ListInput{WorkspaceID: 11, ActorUserID: 42}, []string{first}, false},
		{"financial manager sees account", billingrecordsports.ListInput{}, []string{otherMember, otherWorkspace, first}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			input.BillingAccountID = accountID
			input.Limit = 1
			var ids []string
			for {
				page, err := service.List(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				for _, record := range page.Records {
					if record.RecordID == foreign {
						t.Fatal("foreign account leaked")
					}
					if (record.AvailableCreditsAfter != nil) != tc.hasBalance {
						t.Fatalf("balance disclosure mismatch: %+v", record)
					}
					if record.Kind == billingrecordsports.KindAIUsage {
						ids = append(ids, record.RecordID)
					} else if !tc.hasBalance {
						t.Fatalf("non-consumption leaked: %+v", record)
					}
				}
				if page.NextBeforeSequence == 0 {
					break
				}
				if input.BeforeSequence != 0 && page.NextBeforeSequence >= input.BeforeSequence {
					t.Fatal("cursor did not advance")
				}
				input.BeforeSequence = page.NextBeforeSequence
			}
			if fmt.Sprint(ids) != fmt.Sprint(tc.ids) {
				t.Fatalf("ids=%v, want %v", ids, tc.ids)
			}
			// Snapshot-only fixtures intentionally have no live Task row: both
			// summary and trend must retain the same attribution as records.
			assertCaptureUsage(t, ctx, creditService, accountID, domain.UsageFilter{
				ActorUserID: input.ActorUserID, WorkspaceID: input.WorkspaceID,
			}, int64(len(tc.ids))*10)
		})
	}
}

func TestServiceListWorkspaceIncludesSearchAndExcludesUnknownOrigins(t *testing.T) {
	pool := testdb.Open(t)
	ctx := t.Context()
	creditService := credits.NewService(pool)
	seedWallet(t, ctx, pool, creditService, 42)
	grant(t, ctx, creditService, 42, 100, metadata("search-funds", "admin", "7", "admin_grant", "admin_adjustment", "search-funds"))
	accountID := mustBillingAccount(t, pool, 42)
	operationID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	h := hold(t, ctx, creditService, 42, 10, metadata("search-hold", "user", "42", "vision_search", "ai_search_operation", operationID))
	if _, err := pool.Exec(ctx, `INSERT INTO ai_search_operations(operation_id,user_id,workspace_id,billing_account_id,idempotency_key,request_hash,sku,price_version_id,unit_credits,hold_transaction_id) VALUES($1,42,11,$2,'scoped-search',repeat('a',64),'ai-search:vision',1,10,$3)`, operationID, accountID, h.TransactionID); err != nil {
		t.Fatal(err)
	}
	expected := capture(t, ctx, creditService, 42, h.TransactionID, 10, metadata("search-capture", "user", "42", "vision_search", "ai_search_operation", operationID))
	unknown := hold(t, ctx, creditService, 42, 10, metadata("unknown-hold", "user", "42", "vision_search", "ai_search_operation", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"))
	capture(t, ctx, creditService, 42, unknown.TransactionID, 10, metadata("unknown-capture", "user", "42", "vision_search", "ai_search_operation", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"))
	// A selective Workspace must use the origin scope index rather than
	// scanning every operation for each Capture when accounts grow.
	if _, err := pool.Exec(ctx, `
INSERT INTO ai_search_operations(operation_id,user_id,workspace_id,billing_account_id,idempotency_key,request_hash,sku,price_version_id,unit_credits,hold_transaction_id)
SELECT md5(series::text)::uuid,42,12,$1,'other-workspace-'||series,repeat('a',64),'ai-search:vision',1,10,$2
FROM generate_series(1,4000) series`, accountID, h.TransactionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE ai_search_operations"); err != nil {
		t.Fatal(err)
	}
	for name, query := range map[string]string{
		"summary": "SELECT SUM(command_amount) FROM " + consumptionread.Transactions(2) + " AS captures WHERE billing_account_id = $1 AND command_type = 'capture'",
		"records": listRecordsSQL,
	} {
		args := []any{accountID, int64(11)}
		if name == "records" {
			args = []any{accountID, int64(0), 1, int64(0), int64(11)}
		}
		rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF) "+query, args...)
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(line + "\n")
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s scoped-consumption plan:\n%s", name, plan.String())
		if !strings.Contains(plan.String(), "uq_ai_search_operations_workspace_key") || strings.Contains(plan.String(), "Seq Scan on ai_search_operations") {
			t.Fatalf("%s failed to bound operation lookup by Workspace:\n%s", name, plan.String())
		}
	}
	service := NewService(NewRepository(pool))
	for _, workspaceID := range []int64{11, 12} {
		page, err := service.List(ctx, billingrecordsports.ListInput{BillingAccountID: accountID, WorkspaceID: workspaceID, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		wantCredits := int64(10)
		if workspaceID == 12 {
			wantCredits = 0
			if len(page.Records) != 0 {
				t.Fatalf("foreign Workspace leaked: %+v", page)
			}
		} else if len(page.Records) != 1 || page.Records[0].RecordID != expected.TransactionID || page.Records[0].Kind != billingrecordsports.KindUsage || page.Records[0].AvailableCreditsAfter != nil || page.NextBeforeSequence != 0 {
			t.Fatalf("search page=%+v", page)
		}
		assertCaptureUsage(t, ctx, creditService, accountID, domain.UsageFilter{WorkspaceID: workspaceID}, wantCredits)
	}
	assertCaptureUsage(t, ctx, creditService, accountID, domain.UsageFilter{}, 20)
}

func assertCaptureUsage(t *testing.T, ctx context.Context, service *credits.Service, accountID int64, filter domain.UsageFilter, want int64) {
	t.Helper()
	total, peak, firstDay, err := service.GetCaptureUsage(ctx, accountID, filter, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	daily, err := service.GetDailyCaptureUsage(ctx, accountID, filter, time.UTC, time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var dailyTotal, dailyPeak int64
	for _, amount := range daily {
		dailyTotal += amount
		if amount > dailyPeak {
			dailyPeak = amount
		}
	}
	if total != want || dailyTotal != want || peak != dailyPeak || firstDay.IsZero() != (want == 0) {
		t.Fatalf("capture scope %+v: total=%d peak=%d first=%v daily=%v; want total=%d", filter, total, peak, firstDay, daily, want)
	}
}

func TestConsumptionReadsRejectMismatchedAndMissingAttribution(t *testing.T) {
	pool := testdb.Open(t)
	ctx := t.Context()
	creditService := credits.NewService(pool)
	for _, owner := range []int64{42, 99} {
		seedWallet(t, ctx, pool, creditService, owner)
		grant(t, ctx, creditService, owner, 100, metadata(fmt.Sprintf("origin-funds:%d", owner), "admin", "7", "admin_grant", "admin_adjustment", fmt.Sprintf("origin-funds:%d", owner)))
	}
	accountID := mustBillingAccount(t, pool, 42)
	foreignAccountID := mustBillingAccount(t, pool, 99)
	foreignHold := hold(t, ctx, creditService, 99, 10, metadata("foreign-origin-hold", "user", "42", "task_submission", "task", "foreign-snapshot"))
	seedTaskSnapshotInScope(t, ctx, pool, 99, 42, 11, "foreign-snapshot", foreignHold.TransactionID, "Foreign Task", 10)
	operationID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	if _, err := pool.Exec(ctx, `INSERT INTO ai_search_operations(operation_id,user_id,workspace_id,billing_account_id,idempotency_key,request_hash,sku,price_version_id,unit_credits,hold_transaction_id) VALUES($1,42,11,$2,'foreign-origin',repeat('a',64),'ai-search:vision',1,10,$3)`, operationID, foreignAccountID, foreignHold.TransactionID); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []struct{ kind, id string }{
		{"task", "foreign-snapshot"},
		{"task", "missing-snapshot"},
		{"ai_search_operation", operationID},
		{"ai_search_operation", "malformed-operation-reference"},
		{"unknown_origin", "unknown-origin"},
	} {
		h := hold(t, ctx, creditService, 42, 10, metadata("origin-hold:"+origin.id, "user", "42", "task_submission", origin.kind, origin.id))
		capture(t, ctx, creditService, 42, h.TransactionID, 10, metadata("origin-capture:"+origin.id, "user", "42", "task_terminal", origin.kind, origin.id))
	}

	repository := NewRepository(pool)
	for _, filter := range []domain.UsageFilter{{}, {ActorUserID: 42}, {WorkspaceID: 11}, {ActorUserID: 42, WorkspaceID: 11}} {
		want := int64(50)
		if filter.WorkspaceID != 0 {
			want = 0
		}
		assertCaptureUsage(t, ctx, creditService, accountID, filter, want)
		records, err := repository.List(ctx, billingrecordsports.ListInput{BillingAccountID: accountID, ActorUserID: filter.ActorUserID, WorkspaceID: filter.WorkspaceID}, 100)
		if err != nil {
			t.Fatal(err)
		}
		var total int64
		for _, record := range records {
			if record.commandType == "capture" {
				total += record.amount
			}
			if record.snapshotDisplayName.Valid {
				t.Fatal("foreign snapshot display name leaked")
			}
		}
		if total != want {
			t.Fatalf("records filter=%+v: consumption=%d, want %d", filter, total, want)
		}
	}
}
