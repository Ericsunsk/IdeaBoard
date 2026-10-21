package postgres

// Workspace statistics store integration coverage: aggregation
// correctness over the workspace tasks/billing/credit facts plus backfill
// idempotency (re-running a frozen day inserts nothing new).

import (
	"context"
	"strings"
	"testing"
	"time"

	statisticsapp "ideaboard/api/internal/tasking/application/statistics"
)

// Compile-time conformance with the application seam. The non-test store
// file stays free of the application import; the assertion lives here.
var _ statisticsapp.Store = (*StatisticsStore)(nil)

type statSeed struct {
	userID            int64
	workspacePublicID string
	todayStart        time.Time
	yesterdayStart    time.Time
	dayBeforeStart    time.Time
}

func seedStatisticsFixture(t *testing.T) (*StatisticsStore, statSeed) {
	t.Helper()
	ctx := context.Background()
	db := newTaskTestDB(t)
	workspacePublicID, userID := seedTaskWorkspace(t, db)

	now := time.Now()
	loc := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	dayBeforeStart := todayStart.AddDate(0, 0, -2)

	var workspaceInternalID int64
	if err := db.QueryRow(ctx, `SELECT internal_id FROM workspaces WHERE public_id = $1`, workspacePublicID).Scan(&workspaceInternalID); err != nil {
		t.Fatalf("resolve seeded workspace failed: %v", err)
	}
	var billingAccountID int64
	if err := db.QueryRow(ctx, `INSERT INTO billing_accounts (public_id, workspace_id) VALUES ($1, $2) RETURNING internal_id`,
		"ba_stattest"+randomHex(t, 4), workspaceInternalID).Scan(&billingAccountID); err != nil {
		t.Fatalf("seed billing account failed: %v", err)
	}

	insertTask := func(publicID, status string, created, updated, completed, cancelled *time.Time) {
		t.Helper()
		if _, err := db.Exec(ctx, `
INSERT INTO tasks (public_id, workspace_id, submitter_user_id, billing_account_id, service, workflow_input, status, priority, retry_count, lease_token, error_message, created_at, updated_at, completed_at, cancelled_at)
VALUES ($1,$2,$3,$4,'render','{}',$5,2,0,'','', $6,$7,$8,$9)`,
			publicID, workspaceInternalID, userID, billingAccountID, status, created, updated, completed, cancelled); err != nil {
			t.Fatalf("seed task %s failed: %v", publicID, err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO credit_wallets (billing_account_id) VALUES ($1) ON CONFLICT DO NOTHING`, billingAccountID); err != nil {
		t.Fatalf("seed credit wallet failed: %v", err)
	}
	var issuanceAccountID, sinkAccountID int64
	for _, seed := range []struct {
		role, shard string
		dest        *int64
	}{
		{"system_issuance", "stat-seed-issuance", &issuanceAccountID},
		{"system_sink", "stat-seed-sink", &sinkAccountID},
	} {
		if err := db.QueryRow(ctx, `
INSERT INTO credit_accounts (wallet_user_id, account_role, shard_key) VALUES (NULL, $1, $2)
ON CONFLICT DO NOTHING RETURNING id`, seed.role, seed.shard).Scan(seed.dest); err != nil {
			// ON CONFLICT DO NOTHING with no unique match on (role, shard)
			// falls through; resolve the pre-existing row instead.
			if err := db.QueryRow(ctx, `SELECT id FROM credit_accounts WHERE account_role = $1 AND shard_key = $2`, seed.role, seed.shard).Scan(seed.dest); err != nil {
				t.Fatalf("seed credit account %s failed: %v", seed.role, err)
			}
		}
	}
	insertTxn := func(key, txnID, commandType, businessRefType, businessRefID string, amount int64, created time.Time) {
		t.Helper()
		// The ledger balance trigger is deferred to commit, so the
		// transaction row and its two balancing postings must commit in
		// one explicit transaction (system legs keep user balances
		// untouched; only command_type/amount matter for statistics).
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("seed credit txn %s begin failed: %v", key, err)
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback(ctx)
			}
		}()
		if _, err := tx.Exec(ctx, `
INSERT INTO credit_transactions (transaction_id, billing_account_id, command_type, idempotency_key, request_hash, actor_type, actor_id, source, reason, business_reference_type, business_reference_id, command_amount, posting_count, available_balance_after, reserved_balance_after, created_at)
VALUES ($1,$2,$3,$4,$5,'system','test','test','test seed',$6,$7,$8,2,1000,0,$9)`,
			txnID, billingAccountID, commandType, key, strings.Repeat("ab", 32), businessRefType, businessRefID, amount, created); err != nil {
			t.Fatalf("seed credit txn %s failed: %v", key, err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO credit_postings (transaction_id, account_id, asset_code, amount) VALUES ($1,$2,'credits',$3), ($1,$4,'credits',$5)`,
			txnID, issuanceAccountID, -amount, sinkAccountID, amount); err != nil {
			t.Fatalf("seed credit postings %s failed: %v", key, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("seed credit txn %s commit failed: %v", key, err)
		}
		committed = true
	}
	// Paid snapshots need a matching hold in the credit ledger (IB204
	// trigger); free snapshots (total 0) skip the hold. Snapshots also
	// FK to an exact billing price version row.
	var paidPriceVersionID, freePriceVersionID int64
	if _, err := db.Exec(ctx, `
INSERT INTO billing_skus (sku, display_name, created_by, updated_by) VALUES ('test:render', 'Render', $1, $1)
ON CONFLICT (sku) DO NOTHING`, userID); err != nil {
		t.Fatalf("seed billing sku failed: %v", err)
	}
	if err := db.QueryRow(ctx, `
INSERT INTO billing_price_versions (sku, output_resolution, version, unit_credits, created_by)
VALUES ('test:render', '1k', 1, 10, $1) RETURNING price_version_id`, userID).Scan(&paidPriceVersionID); err != nil {
		t.Fatalf("seed paid price version failed: %v", err)
	}
	if err := db.QueryRow(ctx, `
INSERT INTO billing_price_versions (sku, output_resolution, version, unit_credits, created_by)
VALUES ('test:render', '1k', 2, 0, $1) RETURNING price_version_id`, userID).Scan(&freePriceVersionID); err != nil {
		t.Fatalf("seed free price version failed: %v", err)
	}
	insertSnapshot := func(taskID string, priceVersionID, priceVersion, unitCredits, quantity int64, quotedAt time.Time) {
		t.Helper()
		total := quantity * unitCredits
		var hold any
		if total > 0 {
			holdID := "hold_" + taskID
			insertTxn("seed-"+holdID, holdID, "hold", "task", taskID, total, quotedAt)
			hold = holdID
		}
		if _, err := db.Exec(ctx, `
INSERT INTO task_billing_snapshots (task_id, user_id, billing_account_id, service, sku, display_name, price_version_id, price_version, unit_credits, quantity, total_credits, quoted_at, hold_transaction_id)
VALUES ($1,$2,$3,'render','test:render','Render',$4,$5,$6,$7,$8,$9,$10)`,
			taskID, userID, billingAccountID, priceVersionID, priceVersion, unitCredits, quantity, total, quotedAt, hold); err != nil {
			t.Fatalf("seed snapshot %s failed: %v", taskID, err)
		}
	}

	// Task A: completed yesterday, 90s latency, quoted quantity 2 (paid).
	createdA := yesterdayStart.Add(time.Hour)
	completedA := createdA.Add(90 * time.Second)
	insertTask("task_stat_test_a", "completed", &createdA, &completedA, &completedA, nil)
	insertSnapshot("task_stat_test_a", paidPriceVersionID, 1, 10, 2, createdA)
	// Task B: failed yesterday, quoted quantity 1 (paid).
	createdB := yesterdayStart.Add(2 * time.Hour)
	failedB := createdB.Add(30 * time.Second)
	insertTask("task_stat_test_b", "failed", &createdB, &failedB, &failedB, nil)
	insertSnapshot("task_stat_test_b", paidPriceVersionID, 1, 10, 1, createdB)
	// Task C: cancelled the day before, no billing snapshot (quantity 1 fallback).
	cancelledC := dayBeforeStart.Add(3 * time.Hour)
	createdC := dayBeforeStart.Add(time.Hour)
	insertTask("task_stat_test_c", "cancelled", &createdC, &cancelledC, nil, &cancelledC)
	// Task D: pending now (live pressure bucket).
	createdD := now.Add(-10 * time.Second)
	insertTask("task_stat_test_d", "pending", &createdD, &createdD, nil, nil)
	// Task E: completed today, 60s latency, quoted quantity 3 (free).
	createdE := todayStart.Add(time.Hour)
	completedE := createdE.Add(60 * time.Second)
	insertTask("task_stat_test_e", "completed", &createdE, &completedE, &completedE, nil)
	insertSnapshot("task_stat_test_e", freePriceVersionID, 2, 0, 3, createdE)

	// Execution attempts for final-model attribution (#852): task A
	// completed on its first send; task B exhausted its gateway send
	// then its single bounded fallback. Task A also carries a stale
	// running row (best-effort finalize orphan) that must never shadow
	// the terminal send's model.
	insertAttempt := func(taskID string, attemptNo int, layer, status, resolvedModel, failureStage, failureCategory string, completed *time.Time) {
		t.Helper()
		if _, err := db.Exec(ctx, `
INSERT INTO task_execution_attempts (task_id, river_job_id, attempt_no, execution_token, service, requested_model, resolved_model, provider, retry_layer, gateway_retry_count, status, failure_stage, failure_category, error_msg, completed_at)
VALUES ($1, 0, $2, $3, 'render', $4, $4, 'openrouter', $5, 0, $6, $7, $8, '', $9)`,
			taskID, attemptNo, "tok_"+taskID+"_"+string(rune('0'+attemptNo)), resolvedModel, layer, status, failureStage, failureCategory, completed); err != nil {
			t.Fatalf("seed attempt %s#%d failed: %v", taskID, attemptNo, err)
		}
	}
	insertAttempt("task_stat_test_a", 1, "gateway", "completed", "stat-model-a", "", "", &completedA)
	insertAttempt("task_stat_test_a", 2, "gateway", "running", "stat-model-stale", "", "", nil)
	insertAttempt("task_stat_test_b", 1, "gateway", "failed", "stat-model-a", "provider", "provider_error", &failedB)
	insertAttempt("task_stat_test_b", 2, "model_fallback", "failed", "stat-model-b", "provider", "timeout", &failedB)

	insertTxn("stat-cap-1", "txn_stat_cap_1", "capture", "test", "cap-1", 100, yesterdayStart.Add(4*time.Hour))
	insertTxn("stat-ref-1", "txn_stat_ref_1", "refund", "test", "ref-1", 30, yesterdayStart.Add(5*time.Hour))
	insertTxn("stat-cap-2", "txn_stat_cap_2", "capture", "test", "cap-2", 50, todayStart.Add(2*time.Hour))

	return NewStatisticsStore(db), statSeed{
		userID:            userID,
		workspacePublicID: workspacePublicID,
		todayStart:        todayStart,
		yesterdayStart:    yesterdayStart,
		dayBeforeStart:    dayBeforeStart,
	}
}

func TestStatisticsStoreCountsAndCaptures(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips postgres integration")
	}
	ctx := context.Background()
	store, seed := seedStatisticsFixture(t)

	for status, want := range map[string]int64{
		"pending": 1, "running": 0, "completed": 2, "failed": 1, "cancelled": 1, "cancelled_with_results": 0,
	} {
		got, err := store.CountByStatus(ctx, status)
		if err != nil {
			t.Fatalf("CountByStatus(%s) failed: %v", status, err)
		}
		if got != want {
			t.Fatalf("CountByStatus(%s) = %d, want %d", status, got, want)
		}
	}

	if got, err := store.CountAllTaskCalls(ctx); err != nil || got != 6 {
		t.Fatalf("CountAllTaskCalls() = %d, %v, want 6", got, err)
	}

	captures, err := store.ListDailyCaptureCreditsByRange(ctx, seed.dayBeforeStart, seed.todayStart.AddDate(0, 0, 1), time.UTC)
	if err != nil {
		t.Fatalf("ListDailyCaptureCreditsByRange failed: %v", err)
	}
	yesterdayKey := seed.yesterdayStart.In(time.UTC).Format("2006-01-02")
	todayKey := seed.todayStart.In(time.UTC).Format("2006-01-02")
	if captures[yesterdayKey] != 70 {
		t.Fatalf("yesterday captures = %d, want 70", captures[yesterdayKey])
	}
	if captures[todayKey] != 50 {
		t.Fatalf("today captures = %d, want 50", captures[todayKey])
	}
}

func TestStatisticsStoreBackfillIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips postgres integration")
	}
	ctx := context.Background()
	store, seed := seedStatisticsFixture(t)

	// Freeze the day before (cancelled task without snapshot) and
	// yesterday (completed + failed with snapshots).
	first, err := store.BackfillDailyStatsByCreatedRange(ctx, seed.dayBeforeStart, seed.yesterdayStart, seed.dayBeforeStart, true)
	if err != nil {
		t.Fatalf("backfill day-before failed: %v", err)
	}
	if first.InsertedEvents != 1 || first.MarkedDays != 1 {
		t.Fatalf("backfill day-before = %+v, want inserted=1 marked=1", first)
	}
	second, err := store.BackfillDailyStatsByCreatedRange(ctx, seed.yesterdayStart, seed.todayStart, seed.yesterdayStart, true)
	if err != nil {
		t.Fatalf("backfill yesterday failed: %v", err)
	}
	if second.InsertedEvents != 2 || second.TouchedGroups == 0 || second.MarkedDays != 1 {
		t.Fatalf("backfill yesterday = %+v, want inserted=2 touched>0 marked=1", second)
	}

	// Re-running a frozen day inserts nothing and marks nothing.
	repeat, err := store.BackfillDailyStatsByCreatedRange(ctx, seed.yesterdayStart, seed.todayStart, seed.yesterdayStart, true)
	if err != nil {
		t.Fatalf("repeat backfill failed: %v", err)
	}
	if repeat.InsertedEvents != 0 || repeat.TouchedGroups != 0 || repeat.MarkedDays != 0 {
		t.Fatalf("repeat backfill = %+v, want all zeros", repeat)
	}

	markers, err := store.ListBackfilledDatesByRange(ctx, seed.dayBeforeStart, seed.todayStart)
	if err != nil {
		t.Fatalf("ListBackfilledDatesByRange failed: %v", err)
	}
	if len(markers) != 2 {
		t.Fatalf("backfilled dates = %d, want 2", len(markers))
	}

	// Frozen aggregates: yesterday groups completed 2 + failed 1 (snapshot
	// quantities), attributed to the final resolved model per task (#852):
	// task A completed on its first send (stat-model-a; the stale running
	// row never shadows the terminal send), task B exhausted its bounded
	// fallback (stat-model-b).
	rows, err := store.ListDailyStatsByDateRange(ctx, seed.dayBeforeStart, seed.todayStart)
	if err != nil {
		t.Fatalf("ListDailyStatsByDateRange failed: %v", err)
	}
	var yesterdayTotal, yesterdayCompleted, yesterdayFailed, dayBeforeCancelled int64
	yesterdayModels := map[string]int64{}
	for _, row := range rows {
		key := row.StatDate.Format("2006-01-02")
		switch key {
		case seed.yesterdayStart.Format("2006-01-02"):
			yesterdayTotal += row.Total
			yesterdayCompleted += row.Completed
			yesterdayFailed += row.Failed
			yesterdayModels[row.ModelName] += row.Total
		case seed.dayBeforeStart.Format("2006-01-02"):
			if row.ModelName != "未识别模型" {
				t.Fatalf("day-before model = %q, want fallback model (task C has no attempts)", row.ModelName)
			}
			dayBeforeCancelled += row.Cancelled
		}
	}
	if yesterdayTotal != 3 || yesterdayCompleted != 2 || yesterdayFailed != 1 {
		t.Fatalf("yesterday aggregates = total %d completed %d failed %d, want 3/2/1",
			yesterdayTotal, yesterdayCompleted, yesterdayFailed)
	}
	if yesterdayModels["stat-model-a"] != 2 || yesterdayModels["stat-model-b"] != 1 {
		t.Fatalf("yesterday model attribution = %+v, want stat-model-a:2 stat-model-b:1", yesterdayModels)
	}
	if _, stale := yesterdayModels["stat-model-stale"]; stale {
		t.Fatalf("stale running attempt shadowed terminal attribution: %+v", yesterdayModels)
	}
	if yesterdayTotal != 3 || yesterdayCompleted != 2 || yesterdayFailed != 1 {
		t.Fatalf("yesterday aggregates = total %d completed %d failed %d, want 3/2/1",
			yesterdayTotal, yesterdayCompleted, yesterdayFailed)
	}
	if dayBeforeCancelled != 1 {
		t.Fatalf("day-before cancelled = %d, want 1 (snapshot-less task falls back to quantity 1)", dayBeforeCancelled)
	}

	// Raw events + latency samples serve the intraday trend and slow-model
	// views: A (90s) and B (30s failed, no latency sample).
	events, err := store.ListStatEventsByOccurredRange(ctx, seed.yesterdayStart, seed.todayStart)
	if err != nil {
		t.Fatalf("ListStatEventsByOccurredRange failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("yesterday events = %d, want 2", len(events))
	}
	samples, err := store.ListStatEventLatencySamplesByDateRange(ctx, seed.dayBeforeStart, seed.todayStart)
	if err != nil {
		t.Fatalf("ListStatEventLatencySamplesByDateRange failed: %v", err)
	}
	if len(samples) != 1 || samples[0].LatencySeconds != 90 {
		t.Fatalf("latency samples = %+v, want single 90s sample", samples)
	}

	// Today backfills without marking: the current day stays re-backfillable.
	today, err := store.BackfillDailyStatsByCreatedRange(ctx, seed.todayStart, seed.todayStart.AddDate(0, 0, 1), seed.todayStart, false)
	if err != nil {
		t.Fatalf("backfill today failed: %v", err)
	}
	if today.InsertedEvents != 1 || today.MarkedDays != 0 {
		t.Fatalf("backfill today = %+v, want inserted=1 marked=0", today)
	}
}

func TestStatisticsStoreMonitorReads(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips postgres integration")
	}
	ctx := context.Background()
	store, seed := seedStatisticsFixture(t)
	now := time.Now()

	// Pending pressure is live immediately; terminal attempt rows land
	// outside the 6h pressure window, while the seeded stale running
	// orphan (task A#2, created now) honestly surfaces as one
	// normal_running attempt until a reaper converges it (#852 residual).
	pressure, err := store.ListMonitorExecutionPressureByRange(ctx, now.Add(-6*time.Hour), now)
	if err != nil {
		t.Fatalf("ListMonitorExecutionPressureByRange failed: %v", err)
	}
	var pendingTotal int64
	for _, row := range pressure {
		switch row.Key {
		case "normal_pending", "backlog_pending":
			pendingTotal += row.Total
		case "normal_running":
			if row.Total != 1 {
				t.Fatalf("attempt bucket normal_running = %d, want 1 (seeded stale running orphan)", row.Total)
			}
		case "fast_completed", "slow_completed", "stuck_running",
			"fast_failed", "slow_failed", "cancelled", "retrying", "other":
			if row.Total != 0 {
				t.Fatalf("attempt bucket %s = %d, want 0 without worker attempts", row.Key, row.Total)
			}
		default:
			t.Fatalf("unexpected pressure bucket %q", row.Key)
		}
	}
	if pendingTotal != 1 {
		t.Fatalf("pending pressure total = %d, want 1", pendingTotal)
	}

	// Attribution without metrics rows groups honestly under 'unknown'.
	bottlenecks, err := store.ListExecutionBottleneckStatsByCreatedRange(ctx, seed.dayBeforeStart, seed.todayStart.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("ListExecutionBottleneckStatsByCreatedRange failed: %v", err)
	}
	if len(bottlenecks) != 1 || bottlenecks[0].Source != "unknown" || bottlenecks[0].Total != 3 || bottlenecks[0].Failed != 1 {
		t.Fatalf("bottlenecks = %+v, want single unknown total=3 failed=1", bottlenecks)
	}
	failures, err := store.ListExecutionFailureCategoryStatsByCreatedRange(ctx, seed.dayBeforeStart, seed.todayStart.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("ListExecutionFailureCategoryStatsByCreatedRange failed: %v", err)
	}
	if len(failures) != 1 || failures[0].Category != "unknown" || failures[0].Total != 1 {
		t.Fatalf("failures = %+v, want single unknown total=1", failures)
	}

	// Golden signals sample zero rows until the worker records metrics.
	golden, err := store.ListMonitorGoldenSignalSamplesByRange(ctx, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("ListMonitorGoldenSignalSamplesByRange failed: %v", err)
	}
	if len(golden) != 0 {
		t.Fatalf("golden samples = %d, want 0 without metrics rows", len(golden))
	}
}

func TestStatisticsStoreServiceReadsLiveSummary(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips postgres integration")
	}
	ctx := context.Background()
	store, seed := seedStatisticsFixture(t)

	if _, err := store.BackfillDailyStatsByCreatedRange(ctx, seed.dayBeforeStart, seed.yesterdayStart, seed.dayBeforeStart, true); err != nil {
		t.Fatalf("backfill day-before failed: %v", err)
	}
	if _, err := store.BackfillDailyStatsByCreatedRange(ctx, seed.yesterdayStart, seed.todayStart, seed.yesterdayStart, true); err != nil {
		t.Fatalf("backfill yesterday failed: %v", err)
	}

	service := statisticsapp.NewService(store)
	resp, err := service.GetStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("GetStatistics failed: %v", err)
	}
	summary := resp.Summary
	if summary.TotalCalls != 6 || summary.Pending != 1 || summary.Running != 0 ||
		summary.Completed != 2 || summary.Failed != 1 || summary.Cancelled != 1 {
		t.Fatalf("summary = %+v, want total=6 pending=1 running=0 completed=2 failed=1 cancelled=1", summary)
	}
	if len(resp.Trend) != 7 {
		t.Fatalf("trend buckets = %d, want 7", len(resp.Trend))
	}
	if resp.Backfill.BackfilledDays != 2 {
		t.Fatalf("backfilled days = %d, want 2", resp.Backfill.BackfilledDays)
	}
	var yesterdayTotal int64
	yesterdayKey := seed.yesterdayStart.Format("2006-01-02")
	for _, item := range resp.Trend {
		if item.Date == yesterdayKey {
			yesterdayTotal = item.Total
		}
	}
	if yesterdayTotal != 3 {
		t.Fatalf("yesterday trend total = %d, want 3", yesterdayTotal)
	}

	backfill, err := service.BackfillStatistics(ctx, "7d")
	if err != nil {
		t.Fatalf("BackfillStatistics failed: %v", err)
	}
	// Five days are missing (four unmarked past days plus today, which is
	// always re-backfillable); the four past days get marked, today does not.
	if backfill.ScannedDays != 5 {
		t.Fatalf("backfill scanned = %d, want 5", backfill.ScannedDays)
	}
	if backfill.BackfilledDays != 4 {
		t.Fatalf("backfill backfilled = %d, want 4", backfill.BackfilledDays)
	}
}

func TestStatisticsStoreNilPoolFailsClosed(t *testing.T) {
	store := NewStatisticsStore(nil)
	ctx := context.Background()
	now := time.Now()
	if _, err := store.CountByStatus(ctx, "pending"); err == nil {
		t.Fatal("CountByStatus with nil pool = nil, want error")
	}
	if _, err := store.CountAllTaskCalls(ctx); err == nil {
		t.Fatal("CountAllTaskCalls with nil pool = nil, want error")
	}
	if _, err := store.ListDailyStatsByDateRange(ctx, now, now); err == nil {
		t.Fatal("ListDailyStatsByDateRange with nil pool = nil, want error")
	}
	if _, err := store.BackfillDailyStatsByCreatedRange(ctx, now, now, now, false); err == nil {
		t.Fatal("BackfillDailyStatsByCreatedRange with nil pool = nil, want error")
	}
	if _, err := store.ListMonitorGoldenSignalSamplesByRange(ctx, now.Add(-time.Hour), now); err == nil {
		t.Fatal("ListMonitorGoldenSignalSamplesByRange with nil pool = nil, want error")
	}
	if _, err := store.ListDailyCaptureCreditsByRange(ctx, now.Add(-time.Hour), now, time.UTC); err == nil {
		t.Fatal("ListDailyCaptureCreditsByRange with nil pool = nil, want error")
	}
}
