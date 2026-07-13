package credits

import (
	"context"
	"strings"
	"testing"

	"ideaboard/api/internal/testdb"
)

func TestCaptureUsageAggregationUsesPartialCoveringIndex(t *testing.T) {
	_, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	service := NewService(pool)
	const userID int64 = 771
	if _, err := service.Grant(ctx, GrantCommand{UserID: userID, Amount: mustCreditAmount(t, 10), Metadata: testMetadata("usage-index-grant", "usage-index-grant")}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	hold, err := service.Hold(ctx, HoldCommand{UserID: userID, Amount: mustCreditAmount(t, 10), Metadata: testMetadata("usage-index-hold", "usage-index-hold")})
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	if _, err := service.Capture(ctx, CaptureCommand{UserID: userID, HoldTransactionID: hold.TransactionID, Metadata: testMetadata("usage-index-capture", "usage-index-capture")}); err != nil {
		t.Fatalf("capture: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin explain transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatalf("disable sequential scan: %v", err)
	}
	rows, err := tx.Query(ctx, `
EXPLAIN (COSTS OFF)
WITH daily_captures AS (
  SELECT (created_at AT TIME ZONE $2)::date AS capture_day,
         SUM(command_amount)::numeric AS amount
  FROM credit_transactions
  WHERE wallet_user_id = $1
    AND command_type = 'capture'
  GROUP BY (created_at AT TIME ZONE $2)::date
)
SELECT COALESCE(SUM(amount), 0)::bigint,
       COALESCE(MAX(amount), 0)::bigint,
       COALESCE(MIN(capture_day)::text, '')
FROM daily_captures`, userID, "Asia/Shanghai")
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
	if !strings.Contains(plan.String(), "idx_credit_transactions_capture_user_created") {
		t.Fatalf("capture aggregation plan did not use partial covering index:\n%s", plan.String())
	}
}
