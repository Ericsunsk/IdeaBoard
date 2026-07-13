package taskbilling

import (
	"context"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTaskBillingUsageUsesUserCreatedIndexWithNormalPlanner(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	request, sku := seedTaskBillingUsageProduct(t, ctx, pool)
	seedTaskBillingUsageSnapshots(t, ctx, pool, 81, 32, "usage_plan_target", request, sku)
	seedTaskBillingUsageSnapshots(t, ctx, pool, 82, 4000, "usage_plan_other", request, sku)
	if _, err := pool.Exec(ctx, "ANALYZE task_billing_snapshots"); err != nil {
		t.Fatalf("analyze task billing snapshots: %v", err)
	}

	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	store := NewUsageStore(db)
	count, activeDays, err := store.GetTaskUsage(ctx, 81, location)
	if err != nil {
		t.Fatalf("GetTaskUsage() error = %v", err)
	}
	if count != 32 || activeDays != 1 {
		t.Fatalf("task usage count=%d active_days=%d, want 32/1", count, activeDays)
	}

	rows, err := pool.Query(ctx, `
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF)
SELECT COUNT(*)::bigint,
       COUNT(DISTINCT (created_at AT TIME ZONE $2)::date)::bigint
FROM task_billing_snapshots
WHERE user_id = $1`, 81, location.String())
	if err != nil {
		t.Fatalf("explain task billing usage: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan task billing plan: %v", err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read task billing plan: %v", err)
	}
	if !strings.Contains(plan.String(), "idx_task_billing_snapshots_user_created") {
		t.Fatalf("normal planner did not use Task Billing usage index:\n%s", plan.String())
	}
}

func seedTaskBillingUsageProduct(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) (Request, billing.SKU) {
	t.Helper()
	request, err := AIImageRequest(workflow.FeatureSlugPatternGen, "usage-index-model")
	if err != nil {
		t.Fatal(err)
	}
	sku, err := ResolveSKU(request)
	if err != nil {
		t.Fatal(err)
	}
	service := billing.NewService(pool)
	if _, err := service.CreateSKU(ctx, billing.CreateSKUInput{SKU: string(sku), DisplayName: "Usage Index", Enabled: true, ActorID: 1, Reason: "usage index fixture"}); err != nil {
		t.Fatalf("create usage index SKU: %v", err)
	}
	if _, err := service.CreatePrice(ctx, billing.CreatePriceInput{SKU: string(sku), UnitCredits: 0, Enabled: true, ActorID: 1, Reason: "usage index fixture"}); err != nil {
		t.Fatalf("create usage index price: %v", err)
	}
	return request, sku
}

func seedTaskBillingUsageSnapshots(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	userID int64,
	count int,
	prefix string,
	request Request,
	sku billing.SKU,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
INSERT INTO task_billing_snapshots (
  task_id, user_id, feature_slug, model, sku, price_version_id, price_version,
  unit_credits, quantity, total_credits, quoted_at, created_at
)
SELECT
  $3 || '_' || series,
  $1,
  $4,
  $5,
  price.sku,
	  price.price_version_id,
  price.version,
  price.unit_credits,
  1,
  0,
  clock_timestamp(),
  clock_timestamp() - (series::text || ' seconds')::interval
FROM (
	  SELECT price_version_id, sku, version, unit_credits
  FROM billing_price_versions
  WHERE sku = $6
  ORDER BY version DESC
  LIMIT 1
) AS price
CROSS JOIN generate_series(1, $2::integer) AS series`,
		userID, count, prefix, request.Feature, request.Model, string(sku)); err != nil {
		t.Fatalf("seed task billing snapshots: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO task_billing_settlements (task_id, state, settled_at)
SELECT $1 || '_' || series, 'free', clock_timestamp()
FROM generate_series(1, $2::integer) AS series`, prefix, count); err != nil {
		t.Fatalf("seed task billing settlements: %v", err)
	}
}
