package taskqueue

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/tasks"
)

func TestPostgresTaskRuntimeStoreGetAccountLoadInfoUsesRunningTaskAttempts(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	store := NewPostgresTaskRuntimeStore(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "runtime-load", 5)
	channelID := loadAssignerAccountChannelID(t, db, accountID)
	createAssignerTask(t, db, "running-without-attempt", accountID, tasks.StatusRunning, "lease-no-attempt")
	createAssignerTask(t, db, "running-with-attempt", accountID, tasks.StatusRunning, "lease-running-attempt")
	createAssignerTask(t, db, "completed-with-attempt", accountID, tasks.StatusCompleted, "")
	insertRuntimeTaskAttempt(t, db, "running-with-attempt", accountID, channelID, tasks.TaskAttemptStatusRunning)
	insertRuntimeTaskAttempt(t, db, "completed-with-attempt", accountID, channelID, tasks.TaskAttemptStatusCompleted)

	loads, err := store.GetAccountLoadInfo(ctx, []int64{accountID})
	if err != nil {
		t.Fatalf("GetAccountLoadInfo failed: %v", err)
	}
	load := loads[accountID]
	if load == nil {
		t.Fatalf("account load for %d missing: %+v", accountID, loads)
	}
	if load.CurrentConcurrent != 1 {
		t.Fatalf("current concurrent=%d, want running task attempt count 1", load.CurrentConcurrent)
	}
	if load.MaxConcurrent != 5 {
		t.Fatalf("max concurrent=%d, want 5", load.MaxConcurrent)
	}
}

func TestPostgresTaskRuntimeStoreClampsAccountMaxConcurrent(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	store := NewPostgresTaskRuntimeStore(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "runtime-load-clamped", 1000)

	loads, err := store.GetAccountLoadInfo(ctx, []int64{accountID})
	if err != nil {
		t.Fatalf("GetAccountLoadInfo failed: %v", err)
	}
	load := loads[accountID]
	if load == nil {
		t.Fatalf("account load for %d missing: %+v", accountID, loads)
	}
	if load.MaxConcurrent != accountlimits.MaxMaxConcurrent {
		t.Fatalf("max concurrent=%d, want clamped %d", load.MaxConcurrent, accountlimits.MaxMaxConcurrent)
	}

	maxConcurrent, err := store.GetAccountMaxConcurrent(ctx, accountID)
	if err != nil {
		t.Fatalf("GetAccountMaxConcurrent failed: %v", err)
	}
	if maxConcurrent != accountlimits.MaxMaxConcurrent {
		t.Fatalf("account max concurrent=%d, want clamped %d", maxConcurrent, accountlimits.MaxMaxConcurrent)
	}
}

func TestRiverQueueCountsRunningTaskAttempts(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	queue := NewRiverQueueWithClient(nil, db, nil, RiverQueueDefault, 7)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "runtime-running-count", 3)
	channelID := loadAssignerAccountChannelID(t, db, accountID)
	createAssignerTask(t, db, "running-count-1", accountID, tasks.StatusRunning, "lease-running-count-1")
	createAssignerTask(t, db, "running-count-2", accountID, tasks.StatusRunning, "lease-running-count-2")
	createAssignerTask(t, db, "failed-count-ignored", accountID, tasks.StatusFailed, "")
	insertRuntimeTaskAttempt(t, db, "running-count-1", accountID, channelID, tasks.TaskAttemptStatusRunning)
	insertRuntimeTaskAttempt(t, db, "running-count-2", accountID, channelID, tasks.TaskAttemptStatusRunning)
	insertRuntimeTaskAttempt(t, db, "failed-count-ignored", accountID, channelID, tasks.TaskAttemptStatusFailed)

	count, err := queue.countRunningTaskAttempts(ctx)
	if err != nil {
		t.Fatalf("countRunningTaskAttempts failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("running task attempts=%d, want 2", count)
	}
}

func TestPostgresTaskRuntimeStoreMarkCancelledPrunesExpiredRows(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	store := NewPostgresTaskRuntimeStore(db)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
	INSERT INTO task_cancellations (task_id, expires_at)
	VALUES ($1, $2)`, "expired-cancel", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("insert expired cancellation failed: %v", err)
	}

	if err := store.MarkCancelled(ctx, "active-cancel", time.Hour); err != nil {
		t.Fatalf("MarkCancelled failed: %v", err)
	}

	expired, err := store.IsCancelled(ctx, "expired-cancel")
	if err != nil {
		t.Fatalf("IsCancelled expired failed: %v", err)
	}
	if expired {
		t.Fatal("expired cancellation should not be active")
	}
	active, err := store.IsCancelled(ctx, "active-cancel")
	if err != nil {
		t.Fatalf("IsCancelled active failed: %v", err)
	}
	if !active {
		t.Fatal("new cancellation should be active")
	}
	missing, err := store.IsCancelled(ctx, "missing-cancel")
	if err != nil {
		t.Fatalf("IsCancelled missing failed: %v", err)
	}
	if missing {
		t.Fatal("missing cancellation should not be active")
	}

	var expiredRows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_cancellations WHERE task_id = $1`, "expired-cancel").Scan(&expiredRows); err != nil {
		t.Fatalf("count expired cancellation failed: %v", err)
	}
	if expiredRows != 0 {
		t.Fatalf("expired cancellation rows=%d, want 0", expiredRows)
	}
}

func insertRuntimeTaskAttempt(t *testing.T, db *sql.DB, taskID string, accountID int64, channelID int64, status string) {
	t.Helper()
	_, err := db.Exec(`
INSERT INTO task_attempts (
  task_id, attempt_no, river_job_id, lease_token, account_id, channel_id,
  feature_slug, requested_model, resolved_model, api_type, status, provider_started_at
)
VALUES ($1, 1, 1, $2, $3, $4, 'image-generate', 'gpt-image-1', 'gpt-image-1', 'openai-compatible', $5, now())`,
		taskID,
		"attempt-"+taskID,
		accountID,
		channelID,
		status,
	)
	if err != nil {
		t.Fatalf("insert task attempt for %q failed: %v", taskID, err)
	}
}
