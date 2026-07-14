package usage

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"runtime"
	"sort"
	"strconv"
	"testing"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	creditsloadtest "ideaboard/api/internal/credits/loadtestsupport"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	minimumUsageLoadTransactions       = 1_000_000
	usageLoadUserCount                 = 100
	usageLoadTargetUserID        int64 = 50_000
	usageLoadSamples                   = 20
	usageCaptureIndex                  = "idx_credit_transactions_capture_user_created"
)

// TestUsageMillionTransactionDataset is an explicit PostgreSQL load harness.
// It is intentionally excluded from the default suite because it seeds one
// million immutable Capture facts and matching durable Task Billing snapshots.
func TestUsageMillionTransactionDataset(t *testing.T) {
	if os.Getenv("CREDITS_LOAD_TEST") != "1" {
		t.Skip("set CREDITS_LOAD_TEST=1 to run the million-transaction Usage harness")
	}
	transactionCount := usageLoadTransactionCount(t)
	targetCount := transactionCount / 10
	db, pool := testdb.OpenWithPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	anchor := usageLoadAnchor(t)
	captureSeedStarted := time.Now()
	if err := creditsloadtest.SeedCaptureFacts(ctx, db, creditsloadtest.CaptureFactsConfig{
		BaseUserID:       usageLoadTargetUserID,
		UserCount:        usageLoadUserCount,
		TransactionCount: transactionCount,
		TargetCount:      targetCount,
		Anchor:           anchor,
	}); err != nil {
		t.Fatalf("seed Usage load Capture facts: %v", err)
	}
	captureSeedDuration := time.Since(captureSeedStarted)

	taskSeedStarted := time.Now()
	seedUsageLoadTaskSnapshots(t, ctx, db, pool, transactionCount, targetCount, anchor)
	taskSeedDuration := time.Since(taskSeedStarted)

	verifyStarted := time.Now()
	verifyUsageLoadFacts(t, ctx, db, transactionCount, targetCount)
	verifyDuration := time.Since(verifyStarted)

	if _, err := db.ExecContext(ctx, `ANALYZE credit_transactions`); err != nil {
		t.Fatalf("analyze Usage load facts: %v", err)
	}
	if _, err := db.ExecContext(ctx, `ANALYZE task_billing_snapshots`); err != nil {
		t.Fatalf("analyze Usage load Task Billing facts: %v", err)
	}
	location, err := time.LoadLocation(ShanghaiTimezone)
	if err != nil {
		t.Fatalf("load Shanghai timezone: %v", err)
	}
	yearStart := dateInLocation(anchor, location).AddDate(0, 0, -(Range1Year.Days() - 1))
	plan := explainUsageCaptureTrend(t, ctx, db, usageLoadTargetUserID, location, yearStart, yearStart.AddDate(0, 0, Range1Year.Days()))
	if !planUsesIndex(plan, usageCaptureIndex) {
		encoded, _ := json.MarshalIndent(plan, "", "  ")
		t.Fatalf("normal planner did not use %s:\n%s", usageCaptureIndex, encoded)
	}

	service := NewService(
		credits.NewService(pool),
		taskbilling.NewUsageStore(db),
		ServiceOptions{Now: func() time.Time { return anchor }},
	)
	results := make(map[Range]usageLoadLatency)
	for _, usageRange := range []Range{Range7Days, Range30Days, Range1Year} {
		loaded, err := service.loadCurrent(ctx, usageLoadTargetUserID, usageRange)
		if err != nil {
			t.Fatalf("warm uncached %s usage: %v", usageRange, err)
		}
		if len(loaded.Trend) != usageRange.Days() || loaded.Lifetime.TaskCount != int64(targetCount) {
			t.Fatalf("warm %s usage shape=%d tasks=%d, want %d/%d", usageRange, len(loaded.Trend), loaded.Lifetime.TaskCount, usageRange.Days(), targetCount)
		}

		durations := make([]time.Duration, 0, usageLoadSamples)
		for sample := 0; sample < usageLoadSamples; sample++ {
			started := time.Now()
			loaded, err := service.loadCurrent(ctx, usageLoadTargetUserID, usageRange)
			durations = append(durations, time.Since(started))
			if err != nil {
				t.Fatalf("uncached %s usage sample %d: %v", usageRange, sample, err)
			}
			if len(loaded.Trend) != usageRange.Days() {
				t.Fatalf("uncached %s sample buckets=%d, want %d", usageRange, len(loaded.Trend), usageRange.Days())
			}
		}
		results[usageRange] = summarizeUsageLoadLatency(durations)
	}

	var postgresVersion string
	if err := db.QueryRowContext(ctx, `SHOW server_version`).Scan(&postgresVersion); err != nil {
		t.Fatalf("read PostgreSQL version: %v", err)
	}
	t.Logf(
		"usage load result: captures=%d task_snapshots=%d users=%d target_user=%d target_facts=%d postgres=%s go=%s os=%s arch=%s cpus=%d capture_seed=%s task_seed=%s verify=%s plan_index=%s range_7d_p50=%s range_7d_p95=%s range_30d_p50=%s range_30d_p95=%s range_1y_p50=%s range_1y_p95=%s samples=%d",
		transactionCount,
		transactionCount,
		usageLoadUserCount,
		usageLoadTargetUserID,
		targetCount,
		postgresVersion,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
		runtime.NumCPU(),
		captureSeedDuration,
		taskSeedDuration,
		verifyDuration,
		usageCaptureIndex,
		results[Range7Days].p50,
		results[Range7Days].p95,
		results[Range30Days].p50,
		results[Range30Days].p95,
		results[Range1Year].p50,
		results[Range1Year].p95,
		usageLoadSamples,
	)
}

func usageLoadTransactionCount(t *testing.T) int {
	t.Helper()
	raw := os.Getenv("CREDITS_LOAD_TEST_TRANSACTIONS")
	if raw == "" {
		return minimumUsageLoadTransactions
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count < minimumUsageLoadTransactions {
		t.Fatalf("CREDITS_LOAD_TEST_TRANSACTIONS must be an integer >= %d", minimumUsageLoadTransactions)
	}
	return count
}

func usageLoadAnchor(t *testing.T) time.Time {
	t.Helper()
	location, err := time.LoadLocation(ShanghaiTimezone)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	return time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, location)
}

func seedUsageLoadTaskSnapshots(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	pool *pgxpool.Pool,
	transactionCount int,
	targetCount int,
	anchor time.Time,
) {
	t.Helper()
	const sku = "usage:load-test"
	billingService := billing.NewService(pool)
	if _, err := billingService.CreateSKU(ctx, billing.CreateSKUInput{
		SKU: sku, DisplayName: "Usage Load Test", Enabled: true, ActorID: 1, Reason: "usage load fixture",
	}); err != nil {
		t.Fatalf("create Usage load SKU: %v", err)
	}
	price, err := billingService.CreatePrice(ctx, billing.CreatePriceInput{
		SKU: sku, UnitCredits: 0, Enabled: true, ActorID: 1, Reason: "usage load fixture",
	})
	if err != nil {
		t.Fatalf("create Usage load price: %v", err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE task_billing_snapshots DISABLE TRIGGER task_billing_snapshots_validate_insert`); err != nil {
		t.Fatalf("disable Task Billing load trigger: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO task_billing_snapshots (
  task_id, user_id, feature_slug, model, sku, display_name, price_version_id, price_version,
  unit_credits, quantity, total_credits, quoted_at, created_at
)
SELECT
  'usage_load_task_' || series,
  CASE
    WHEN series <= $2::bigint THEN $3::bigint
    ELSE $3::bigint + 1 + ((series - $2::bigint - 1) % ($4::bigint - 1))
  END,
  'pattern-gen',
  'usage-load-model',
  $5,
  'Usage Load Test',
  $6,
  $7,
  0,
  1,
  0,
  $8::timestamptz - (
    CASE WHEN series <= $2::bigint THEN (series - 1) % 365 ELSE (series - $2::bigint - 1) % 730 END
  ) * interval '1 day',
  $8::timestamptz - (
    CASE WHEN series <= $2::bigint THEN (series - 1) % 365 ELSE (series - $2::bigint - 1) % 730 END
  ) * interval '1 day'
FROM generate_series(1, $1::bigint) AS series`, transactionCount, targetCount, usageLoadTargetUserID, usageLoadUserCount, sku, price.PriceVersionID, price.Version, anchor); err != nil {
		t.Fatalf("seed Usage load Task Billing snapshots: %v", err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE task_billing_snapshots ENABLE TRIGGER task_billing_snapshots_validate_insert`); err != nil {
		t.Fatalf("enable Task Billing load trigger: %v", err)
	}
}

func verifyUsageLoadFacts(t *testing.T, ctx context.Context, db *sql.DB, transactionCount, targetCount int) {
	t.Helper()
	var captures int
	var targetCaptures int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)::integer,
       COUNT(*) FILTER (WHERE wallet_user_id = $1)::integer
FROM credit_transactions
WHERE transaction_id LIKE 'usage_load_tx_%'`, usageLoadTargetUserID).Scan(&captures, &targetCaptures); err != nil {
		t.Fatalf("count Usage load Captures: %v", err)
	}
	if captures != transactionCount || targetCaptures != targetCount {
		t.Fatalf("Usage load Capture counts=%d/%d, want %d/%d", captures, targetCaptures, transactionCount, targetCount)
	}
	var snapshots int
	var targetSnapshots int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)::integer,
       COUNT(*) FILTER (WHERE user_id = $1)::integer
FROM task_billing_snapshots
WHERE task_id LIKE 'usage_load_task_%'`, usageLoadTargetUserID).Scan(&snapshots, &targetSnapshots); err != nil {
		t.Fatalf("count Usage load Task Billing snapshots: %v", err)
	}
	if snapshots != transactionCount || targetSnapshots != targetCount {
		t.Fatalf("Usage load Task counts=%d/%d, want %d/%d", snapshots, targetSnapshots, transactionCount, targetCount)
	}
	var invalid int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)::integer
FROM (
  SELECT transactions.transaction_id
  FROM credit_transactions AS transactions
  JOIN credit_postings AS postings ON postings.transaction_id = transactions.transaction_id
  WHERE transactions.transaction_id LIKE 'usage_load_tx_%'
  GROUP BY transactions.transaction_id, transactions.posting_count
  HAVING COUNT(*) <> transactions.posting_count OR SUM(postings.amount) <> 0
) AS invalid`).Scan(&invalid); err != nil {
		t.Fatalf("verify Usage load balanced facts: %v", err)
	}
	if invalid != 0 {
		t.Fatalf("unbalanced Usage load transactions = %d", invalid)
	}
}

func explainUsageCaptureTrend(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID int64,
	location *time.Location,
	start time.Time,
	endExclusive time.Time,
) any {
	t.Helper()
	var raw []byte
	err := db.QueryRowContext(ctx, `
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
SELECT ((created_at AT TIME ZONE $2)::date)::text AS capture_day,
       SUM(command_amount)::bigint AS amount
FROM credit_transactions
WHERE wallet_user_id = $1
  AND command_type = 'capture'
  AND created_at >= $3
  AND created_at < $4
GROUP BY (created_at AT TIME ZONE $2)::date
ORDER BY capture_day`, userID, location.String(), start, endExclusive).Scan(&raw)
	if err != nil {
		t.Fatalf("EXPLAIN Usage Capture trend: %v", err)
	}
	var plan any
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("decode Usage Capture plan: %v", err)
	}
	return plan
}

func planUsesIndex(value any, indexName string) bool {
	switch typed := value.(type) {
	case map[string]any:
		if typed["Index Name"] == indexName {
			return true
		}
		for _, child := range typed {
			if planUsesIndex(child, indexName) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if planUsesIndex(child, indexName) {
				return true
			}
		}
	}
	return false
}

type usageLoadLatency struct {
	p50 time.Duration
	p95 time.Duration
}

func summarizeUsageLoadLatency(durations []time.Duration) usageLoadLatency {
	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	return usageLoadLatency{
		p50: sorted[(len(sorted)-1)*50/100],
		p95: sorted[(len(sorted)-1)*95/100],
	}
}
