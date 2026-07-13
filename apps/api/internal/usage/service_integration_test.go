package usage

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/credits"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/testdb"
)

func TestServiceGetLifetimeAggregatesOnlyCurrentUsersSettledCaptures(t *testing.T) {
	db, pool := testdb.OpenWithPool(t)
	ctx := context.Background()
	const userID int64 = 901
	seedUsageUser(t, ctx, db, userID, "US0901")
	seedUsageUser(t, ctx, db, 902, "US0902")

	creditService := credits.NewService(pool)
	firstCapture := captureUsageCredits(t, ctx, creditService, userID, 20, "usage-capture-1")
	_ = captureUsageCredits(t, ctx, creditService, userID, 100, "usage-capture-2")
	_ = captureUsageCredits(t, ctx, creditService, 902, 10, "usage-other-user-capture")
	if _, err := creditService.Refund(ctx, credits.RefundCommand{
		UserID:              userID,
		ChargeTransactionID: firstCapture.TransactionID,
		Amount:              mustUsageCreditAmount(t, 20),
		Metadata:            usageCreditMetadata("usage-refund"),
	}); err != nil {
		t.Fatalf("refund usage capture: %v", err)
	}

	firstCaptureAt := time.Date(2026, time.July, 10, 15, 50, 0, 0, time.UTC)  // 23:50 Shanghai
	secondCaptureAt := time.Date(2026, time.July, 10, 16, 10, 0, 0, time.UTC) // 00:10 Shanghai
	seedUsageTask(t, ctx, db, "usage-task-1", userID, firstCaptureAt)
	seedUsageTask(t, ctx, db, "usage-task-2", userID, secondCaptureAt)
	seedUsageTask(t, ctx, db, "usage-task-3", userID, secondCaptureAt.Add(2*time.Hour))
	seedUsageTask(t, ctx, db, "usage-task-other-user", 902, secondCaptureAt)

	location, err := time.LoadLocation(ShanghaiTimezone)
	if err != nil {
		t.Fatalf("load Shanghai location: %v", err)
	}
	service := NewService(creditService, taskstore.NewRepository(db), func() time.Time {
		return time.Date(2026, time.July, 13, 8, 0, 0, 0, location)
	})

	result, err := service.GetLifetime(ctx, userID)
	if err != nil {
		t.Fatalf("GetLifetime() error = %v", err)
	}
	if result.Lifetime.TotalCredits != 120 || result.Lifetime.PeakDailyCredits != 120 {
		t.Fatalf("capture summary = %+v, want total=120 peak=120", result.Lifetime)
	}
	if result.Lifetime.AverageDailyCredits <= 0 {
		t.Fatalf("daily average = %v, want positive value", result.Lifetime.AverageDailyCredits)
	}
	if result.Lifetime.TaskCount != 3 || result.Lifetime.ActiveDays != 2 {
		t.Fatalf("task summary = %+v, want count=3 active_days=2", result.Lifetime)
	}
}

func seedUsageUser(t *testing.T, ctx context.Context, db *sql.DB, userID int64, storageUID string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, username, storage_uid, role, status)
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

func seedUsageTask(t *testing.T, ctx context.Context, db *sql.DB, taskID string, userID int64, createdAt time.Time) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO tasks (task_id, user_id, prompt, user_prompt, params, status, error_msg, result, created_at, updated_at)
VALUES ($1, $2, 'usage task', 'usage task', '{}', 'pending', '', '', $3, $3)`, taskID, userID, createdAt); err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
}
