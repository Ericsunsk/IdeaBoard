package usage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/billing"
	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/tasking/taskbilling"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestServiceGetCurrentReportsDurableLogicalTasks(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	const userID int64 = 901
	seedUsageUser(t, ctx, db, userID, "US0901")
	seedUsageUser(t, ctx, db, 902, "US0902")

	creditService := credits.NewService(pool)
	firstCapture := captureUsageCredits(t, ctx, creditService, userID, 20, "usage-capture-1")
	latestCapture := captureUsageCredits(t, ctx, creditService, userID, 100, "usage-capture-2")
	_ = captureUsageCredits(t, ctx, creditService, 902, 10, "usage-other-user-capture")
	if _, err := creditService.Refund(ctx, credits.RefundCommand{
		UserID:              userID,
		ChargeTransactionID: firstCapture.TransactionID,
		Amount:              mustUsageCreditAmount(t, 20),
		Metadata:            usageCreditMetadata("usage-refund"),
	}); err != nil {
		t.Fatalf("refund usage capture: %v", err)
	}

	taskIDs := []string{
		"usage-free-completed",
		"usage-free-failed",
		"usage-free-cancelled",
		"usage-free-pending",
	}
	billingWriter, request := newDurableUsageBilling(t, ctx, pool, creditService)
	seedDurableUsageSnapshots(t, ctx, db, pool, billingWriter, request, userID, taskIDs)
	seedDurableUsageSnapshots(t, ctx, db, pool, billingWriter, request, 902, []string{"usage-other-user"})
	setUsageTaskStatus(t, ctx, db, taskIDs[0], "completed")
	setUsageTaskStatus(t, ctx, db, taskIDs[1], "failed")
	setUsageTaskStatus(t, ctx, db, taskIDs[2], "cancelled")

	newUsageService := func() *Service {
		return NewService(creditService, taskbilling.NewUsageStore(db), ServiceOptions{Now: func() time.Time {
			return latestCapture.CreatedAt
		}})
	}
	service := newUsageService()

	beforeDelete, err := service.GetCurrent(ctx, userID, Range30Days)
	if err != nil {
		t.Fatalf("GetCurrent() error = %v", err)
	}
	if beforeDelete.Lifetime.TotalCredits != 120 || beforeDelete.Lifetime.PeakDailyCredits != 120 {
		t.Fatalf("capture summary = %+v, want total=120 peak=120", beforeDelete.Lifetime)
	}
	if beforeDelete.Lifetime.AverageDailyCredits <= 0 {
		t.Fatalf("daily average = %v, want positive value", beforeDelete.Lifetime.AverageDailyCredits)
	}
	if beforeDelete.Lifetime.TaskCount != 4 || beforeDelete.Lifetime.ActiveDays != 1 {
		t.Fatalf("task summary = %+v, want count=4 active_days=1", beforeDelete.Lifetime)
	}
	if len(beforeDelete.Trend) != Range30Days.Days() {
		t.Fatalf("trend buckets = %d, want %d", len(beforeDelete.Trend), Range30Days.Days())
	}
	var trendTotal int64
	for _, day := range beforeDelete.Trend {
		trendTotal += day.Credits
	}
	if trendTotal != 120 {
		t.Fatalf("trend total = %d, want gross captures 120", trendTotal)
	}

}

func seedUsageUser(t *testing.T, ctx context.Context, db *sql.DB, userID int64, storageUID string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, display_name, storage_uid, role, status)
VALUES ($1, $2, $3, 'user', 1)`, userID, fmt.Sprintf("usage-user-%d", userID), storageUID); err != nil {
		t.Fatalf("seed user %d: %v", userID, err)
	}
}

func captureUsageCredits(t *testing.T, ctx context.Context, service *credits.Service, userID, amount int64, key string) credits.CommandResult {
	t.Helper()
	if _, err := service.Grant(ctx, credits.GrantCommand{
		UserID:   userID,
		Amount:   mustUsageCreditAmount(t, amount),
		Metadata: usageCreditMetadata("usage-grant-" + key),
	}); err != nil {
		t.Fatalf("grant user %d: %v", userID, err)
	}
	hold, err := service.Hold(ctx, credits.HoldCommand{
		UserID:   userID,
		Amount:   mustUsageCreditAmount(t, amount),
		Metadata: usageCreditMetadata("usage-hold-" + key),
	})
	if err != nil {
		t.Fatalf("hold user %d: %v", userID, err)
	}
	capture, err := service.Capture(ctx, credits.CaptureCommand{
		UserID:            userID,
		HoldTransactionID: hold.TransactionID,
		Amount:            mustUsageCreditAmount(t, amount),
		Metadata:          usageCreditMetadata("usage-capture-" + key),
	})
	if err != nil {
		t.Fatalf("capture user %d: %v", userID, err)
	}
	return capture
}

func mustUsageCreditAmount(t *testing.T, value int64) credits.CreditAmount {
	t.Helper()
	amount, err := credits.NewCreditAmount(value)
	if err != nil {
		t.Fatalf("credit amount %d: %v", value, err)
	}
	return amount
}

func usageCreditMetadata(key string) credits.CommandMetadata {
	return credits.CommandMetadata{
		IdempotencyKey: key,
		Actor:          credits.Actor{Type: credits.ActorService, ID: "usage-test"},
		Source:         "usage_test",
		Reason:         "usage summary fixture",
		BusinessReference: credits.BusinessReference{
			Type: "usage_test",
			ID:   key,
		},
	}
}

func seedDurableUsageSnapshots(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	pool *pgxpool.Pool,
	billingWriter *taskbilling.Service,
	request taskbilling.Request,
	userID int64,
	taskIDs []string,
) {
	t.Helper()
	for _, taskID := range taskIDs {
		seedUsageTask(t, ctx, db, taskID, userID)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin task billing fixture: %v", err)
		}
		if _, err := billingWriter.HoldAndPersist(ctx, tx, taskID, userID, request); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("persist task billing snapshot %s: %v", taskID, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit task billing snapshot %s: %v", taskID, err)
		}
	}
}

func newDurableUsageBilling(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	creditService *credits.Service,
) (*taskbilling.Service, taskbilling.Request) {
	t.Helper()
	billingService := billing.NewService(pool)
	request, err := taskbilling.AIImageRequest(workflow.FeatureSlugPromptGeneration, "1k", 1)
	if err != nil {
		t.Fatalf("build task billing request: %v", err)
	}
	sku, err := taskbilling.ResolveSKU(request)
	if err != nil {
		t.Fatalf("resolve usage SKU: %v", err)
	}
	if _, err := billingService.PublishPrice(ctx, usagePriceInput(sku, "Usage Free", 0)); err != nil {
		t.Fatalf("publish usage price: %v", err)
	}
	return taskbilling.NewService(billingService, creditService), request
}

func usagePriceInput(sku billing.SKU, displayName string, unitCredits int64) billing.PublishPriceInput {
	namespace, _, _ := strings.Cut(string(sku), ":")
	return billing.PublishPriceInput{
		Definition: billing.BillableItemDefinition{
			SKU: sku, Namespace: namespace, ProductKind: "test-feature", DisplayName: displayName,
		},
		UnitCredits: unitCredits,
		ActorID:     1,
	}
}

func seedUsageTask(t *testing.T, ctx context.Context, db *sql.DB, taskID string, userID int64) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO tasks (task_id, user_id, prompt, user_prompt, params, status, error_msg, result, created_at, updated_at)
VALUES ($1, $2, 'usage task', 'usage task', '{}', 'pending', '', '', clock_timestamp(), clock_timestamp())`, taskID, userID); err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
}

func setUsageTaskStatus(t *testing.T, ctx context.Context, db *sql.DB, taskID, status string) {
	t.Helper()
	var query string
	switch status {
	case "completed":
		query = `UPDATE tasks SET status = 'completed', completed_at = clock_timestamp() WHERE task_id = $1`
	case "failed":
		query = `UPDATE tasks SET status = 'failed', error_msg = 'fixture failure', completed_at = clock_timestamp() WHERE task_id = $1`
	case "cancelled":
		query = `UPDATE tasks SET status = 'cancelled', completed_at = clock_timestamp(), cancelled_at = clock_timestamp(), cancel_reason = 'fixture cancellation' WHERE task_id = $1`
	default:
		t.Fatalf("unsupported fixture status %q", status)
	}
	if _, err := db.ExecContext(ctx, query, taskID); err != nil {
		t.Fatalf("set task %s status %s: %v", taskID, status, err)
	}
}
