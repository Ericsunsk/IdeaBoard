package usage_test

import (
	"ideaboard/api/internal/consumptionread"
	creditsdomain "ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/credits/testhelpers"

	"ideaboard/api/internal/credits"

	"context"
	"ideaboard/api/internal/credits/ports"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCaptureUsageAggregationUsesShanghaiDayBoundary(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	const userID int64 = 771
	service := seedCaptureUsageWallet(t, ctx, pool, userID, 120, "boundary")
	seedCaptureUsageTransaction(t, ctx, pool, userID, 20, time.Date(2026, time.July, 10, 15, 50, 0, 0, time.UTC), "boundary-before")
	seedCaptureUsageTransaction(t, ctx, pool, userID, 100, time.Date(2026, time.July, 10, 16, 10, 0, 0, time.UTC), "boundary-after")
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}

	accountID := testhelpers.MustBillingAccount(t, pool, userID)
	total, peak, firstDay, err := service.GetCaptureUsage(ctx, accountID, creditsdomain.UsageFilter{}, location)
	if err != nil {
		t.Fatalf("GetCaptureUsage() error = %v", err)
	}
	wantFirstDay := time.Date(2026, time.July, 10, 0, 0, 0, 0, location)
	if total != 120 || peak != 100 || !firstDay.Equal(wantFirstDay) {
		t.Fatalf("capture usage total=%d peak=%d first=%s, want 120/100/%s", total, peak, firstDay, wantFirstDay)
	}

	daily, err := service.GetDailyCaptureUsage(
		ctx,
		accountID, creditsdomain.UsageFilter{},
		location,
		time.Date(2026, time.July, 10, 0, 0, 0, 0, location),
		time.Date(2026, time.July, 12, 0, 0, 0, 0, location),
	)
	if err != nil {
		t.Fatalf("GetDailyCaptureUsage() error = %v", err)
	}
	if daily["2026-07-10"] != 20 || daily["2026-07-11"] != 100 || len(daily) != 2 {
		t.Fatalf("daily capture usage = %+v, want Shanghai buckets 20/100", daily)
	}
}

func TestCaptureUsageAggregationUsesPartialCoveringIndex(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	const targetUserID int64 = 771
	const otherUserID int64 = 772
	seedCaptureUsageWallet(t, ctx, pool, targetUserID, 32, "plan-target")
	seedCaptureUsageWallet(t, ctx, pool, otherUserID, 4000, "plan-other")
	targetAccountID := testhelpers.MustBillingAccount(t, pool, targetUserID)
	seedCaptureUsageSeries(t, ctx, pool, targetUserID, 32, "plan_target")
	seedCaptureUsageSeries(t, ctx, pool, otherUserID, 4000, "plan_other")
	if _, err := pool.Exec(ctx, "ANALYZE credit_transactions"); err != nil {
		t.Fatalf("analyze credit transactions: %v", err)
	}

	rows, err := pool.Query(ctx, `
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF)
WITH daily_captures AS (
  SELECT (created_at AT TIME ZONE $2)::date AS capture_day,
         SUM(command_amount)::numeric AS amount
  FROM `+consumptionread.Transactions(3)+` AS captures
  WHERE billing_account_id = $1
    AND command_type = 'capture'
  GROUP BY (created_at AT TIME ZONE $2)::date
)
SELECT COALESCE(SUM(amount), 0)::bigint,
       COALESCE(MAX(amount), 0)::bigint,
       COALESCE(MIN(capture_day)::text, '')
FROM daily_captures`, targetAccountID, "Asia/Shanghai", int64(0))
	if err != nil {
		t.Fatalf("explain capture aggregation: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read plan: %v", err)
	}
	t.Logf("lifetime attributed-Ledger plan:\n%s", plan.String())
	if strings.Contains(plan.String(), "task_billing_snapshots") || strings.Contains(plan.String(), "ai_search_operations") {
		t.Fatalf("unscoped aggregate performed unused attribution work:\n%s", plan.String())
	}
	if !strings.Contains(plan.String(), "idx_credit_transactions_capture_user_created") {
		t.Fatalf("normal planner did not use partial covering index:\n%s", plan.String())
	}

	dailyRows, err := pool.Query(ctx, `
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF)
SELECT ((created_at AT TIME ZONE $2)::date)::text AS capture_day,
       SUM(command_amount)::bigint AS amount
FROM `+consumptionread.Transactions(5)+` AS captures
WHERE billing_account_id = $1
  AND command_type = 'capture'
  AND created_at >= $3
  AND created_at < $4
GROUP BY (created_at AT TIME ZONE $2)::date
ORDER BY capture_day`, targetAccountID, "Asia/Shanghai", time.Now().AddDate(0, 0, -29), time.Now().AddDate(0, 0, 1), int64(0))
	if err != nil {
		t.Fatalf("explain daily capture trend: %v", err)
	}
	defer dailyRows.Close()
	var dailyPlan strings.Builder
	for dailyRows.Next() {
		var line string
		if err := dailyRows.Scan(&line); err != nil {
			t.Fatalf("scan daily plan: %v", err)
		}
		dailyPlan.WriteString(line)
		dailyPlan.WriteByte('\n')
	}
	if err := dailyRows.Err(); err != nil {
		t.Fatalf("read daily plan: %v", err)
	}
	t.Logf("daily attributed-Ledger plan:\n%s", dailyPlan.String())
	if strings.Contains(dailyPlan.String(), "task_billing_snapshots") || strings.Contains(dailyPlan.String(), "ai_search_operations") {
		t.Fatalf("unscoped trend performed unused attribution work:\n%s", dailyPlan.String())
	}
	if !strings.Contains(dailyPlan.String(), "idx_credit_transactions_capture_user_created") {
		t.Fatalf("normal planner did not use capture index for daily trend:\n%s", dailyPlan.String())
	}
}

func seedCaptureUsageWallet(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	userID int64,
	amount int64,
	key string,
) *credits.Service {
	t.Helper()
	testhelpers.EnsureAdminOperationUser(t, pool, userID)
	service := credits.NewService(pool)
	if _, err := service.Grant(ctx, ports.GrantCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, userID), Amount: testhelpers.MustCreditAmount(t, amount), Metadata: testhelpers.TestMetadata("usage:"+key+":grant", "usage-"+key+"-grant")}); err != nil {
		t.Fatalf("grant usage wallet %d: %v", userID, err)
	}
	if _, err := service.Hold(ctx, ports.HoldCommand{BillingAccountID: testhelpers.MustBillingAccount(t, pool, userID), Amount: testhelpers.MustCreditAmount(t, amount), Metadata: testhelpers.TestMetadata("usage:"+key+":hold", "usage-"+key+"-hold")}); err != nil {
		t.Fatalf("hold usage wallet %d: %v", userID, err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO credit_accounts (account_role, shard_key)
VALUES ('system_revenue', $1)
ON CONFLICT (asset_code, account_role, shard_key) WHERE wallet_user_id IS NULL AND billing_account_id IS NULL DO NOTHING`, testhelpers.WalletSystemShard(testhelpers.MustBillingAccount(t, pool, userID))); err != nil {
		t.Fatalf("seed revenue account %d: %v", userID, err)
	}
	return service
}

func seedCaptureUsageTransaction(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	userID, amount int64,
	createdAt time.Time,
	key string,
) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin capture fixture: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	reservedID, revenueID := captureUsageAccountIDs(t, ctx, tx, userID)
	transactionID := "ctx_usage_" + key
	if _, err := tx.Exec(ctx, `
INSERT INTO credit_transactions (
  transaction_id, billing_account_id, command_type, idempotency_key, request_hash,
  actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
  command_amount, posting_count, available_balance_after, reserved_balance_after, created_at
) VALUES ($1, $2, 'capture', $3, repeat('a', 64), 'service', 'usage-test', 'usage_test',
          'usage summary fixture', 'usage_test', $3, $4, 2, 0, 0, $5)`,
		transactionID, testhelpers.MustBillingAccount(t, pool, userID), key, amount, createdAt); err != nil {
		t.Fatalf("insert capture transaction: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO credit_postings (transaction_id, account_id, amount)
VALUES ($1, $2, $3), ($1, $4, -$3)`, transactionID, revenueID, amount, reservedID); err != nil {
		t.Fatalf("insert capture postings: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit capture fixture: %v", err)
	}
}

func seedCaptureUsageSeries(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int64, count int, prefix string) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin capture series: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	reservedID, revenueID := captureUsageAccountIDs(t, ctx, tx, userID)
	if _, err := tx.Exec(ctx, `
INSERT INTO credit_transactions (
  transaction_id, billing_account_id, command_type, idempotency_key, request_hash,
  actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
  command_amount, posting_count, available_balance_after, reserved_balance_after, created_at
)
SELECT
  'ctx_usage_' || $3 || '_' || series,
  $1,
  'capture',
  'usage:' || $3 || ':' || series,
  repeat('b', 64),
  'service', 'usage-test', 'usage_test', 'usage planner fixture', 'usage_test',
  $3 || ':' || series,
  1, 2, 0, 0,
  clock_timestamp() - (series::text || ' seconds')::interval
FROM generate_series(1, $2::integer) AS series`, testhelpers.MustBillingAccount(t, pool, userID), count, prefix); err != nil {
		t.Fatalf("insert capture series transactions: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO credit_postings (transaction_id, account_id, amount)
SELECT 'ctx_usage_' || $1 || '_' || series, posting.account_id, posting.amount
FROM generate_series(1, $2::integer) AS series
CROSS JOIN LATERAL (
  VALUES ($3::bigint, 1::bigint), ($4::bigint, -1::bigint)
) AS posting(account_id, amount)`, prefix, count, revenueID, reservedID); err != nil {
		t.Fatalf("insert capture series postings: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit capture series: %v", err)
	}
}

func captureUsageAccountIDs(t *testing.T, ctx context.Context, query interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID int64) (reservedID, revenueID int64) {
	t.Helper()
	var accountID int64
	if err := query.QueryRow(ctx, `SELECT ensure_personal_billing_account_for_user($1)`, userID).Scan(&accountID); err != nil {
		t.Fatalf("resolve billing account: %v", err)
	}
	if err := query.QueryRow(ctx, `SELECT id FROM credit_accounts WHERE billing_account_id = $1 AND account_role = 'user_reserved'`, accountID).Scan(&reservedID); err != nil {
		t.Fatalf("load reserved account: %v", err)
	}
	if err := query.QueryRow(ctx, `SELECT id FROM credit_accounts WHERE wallet_user_id IS NULL AND billing_account_id IS NULL AND account_role = 'system_revenue' AND shard_key = $1`, testhelpers.WalletSystemShard(accountID)).Scan(&revenueID); err != nil {
		t.Fatalf("load revenue account: %v", err)
	}
	return reservedID, revenueID
}
