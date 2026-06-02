package taskqueue

import (
	"context"
	"testing"
	"time"
)

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

	var expiredRows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_cancellations WHERE task_id = $1`, "expired-cancel").Scan(&expiredRows); err != nil {
		t.Fatalf("count expired cancellation failed: %v", err)
	}
	if expiredRows != 0 {
		t.Fatalf("expired cancellation rows=%d, want 0", expiredRows)
	}
}
