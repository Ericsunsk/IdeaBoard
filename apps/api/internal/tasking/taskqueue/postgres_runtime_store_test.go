package taskqueue

import (
	"context"
	"testing"
	"time"

	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/tasking/tasks"
)

func TestRuntimeMaxConcurrentTreatsNonPositiveValuesAsNoCapacity(t *testing.T) {
	tests := []struct {
		name  string
		value int
		want  int
	}{
		{name: "negative", value: -1, want: 0},
		{name: "zero", value: 0, want: 0},
		{name: "configured", value: 5, want: 5},
		{name: "too high", value: accountlimits.MaxMaxConcurrent + 1, want: accountlimits.MaxMaxConcurrent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runtimeMaxConcurrent(tt.value); got != tt.want {
				t.Fatalf("runtimeMaxConcurrent(%d)=%d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestPostgresTaskRuntimeStoreGetAccountLoadInfoUsesActiveTaskLeases(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	store := NewPostgresTaskRuntimeStore(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "runtime-load", 5)
	createAssignerTask(t, db, "running-without-lease", accountID, tasks.StatusRunning, "")
	createAssignerTask(t, db, "running-with-active-lease", accountID, tasks.StatusRunning, "lease-running-active")
	createAssignerTask(t, db, "running-with-released-lease", accountID, tasks.StatusRunning, "lease-running-released")
	if _, err := db.Exec(`UPDATE task_leases SET released_at=clock_timestamp(), release_reason='completed' WHERE task_id='running-with-released-lease'`); err != nil {
		t.Fatal(err)
	}

	loads, err := store.GetAccountLoadInfo(ctx, []int64{accountID})
	if err != nil {
		t.Fatalf("GetAccountLoadInfo failed: %v", err)
	}
	load := loads[accountID]
	if load == nil {
		t.Fatalf("account load for %d missing: %+v", accountID, loads)
		return
	}
	if load.CurrentConcurrent != 1 {
		t.Fatalf("current concurrent=%d, want active task lease count 1", load.CurrentConcurrent)
	}
	if load.MaxConcurrent != 5 {
		t.Fatalf("max concurrent=%d, want 5", load.MaxConcurrent)
	}
}

func TestPostgresTaskRuntimeStoreUsesAccountMaxConcurrentLimit(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	store := NewPostgresTaskRuntimeStore(db)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "runtime-load-limit", accountlimits.MaxMaxConcurrent)

	loads, err := store.GetAccountLoadInfo(ctx, []int64{accountID})
	if err != nil {
		t.Fatalf("GetAccountLoadInfo failed: %v", err)
	}
	load := loads[accountID]
	if load == nil {
		t.Fatalf("account load for %d missing: %+v", accountID, loads)
		return
	}
	if load.MaxConcurrent != accountlimits.MaxMaxConcurrent {
		t.Fatalf("max concurrent=%d, want configured limit %d", load.MaxConcurrent, accountlimits.MaxMaxConcurrent)
	}

	maxConcurrent, err := store.GetAccountMaxConcurrent(ctx, accountID)
	if err != nil {
		t.Fatalf("GetAccountMaxConcurrent failed: %v", err)
	}
	if maxConcurrent != accountlimits.MaxMaxConcurrent {
		t.Fatalf("account max concurrent=%d, want configured limit %d", maxConcurrent, accountlimits.MaxMaxConcurrent)
	}
}

func TestRiverQueueCountsActiveTaskLeases(t *testing.T) {
	db := newPostgresAssignerTestDB(t)
	queue := NewRiverQueueWithClient(nil, db, nil, RiverQueueDefault, 7)
	ctx := context.Background()

	accountID := createAssignerAccount(t, db, "runtime-running-count", 3)
	createAssignerTask(t, db, "running-count-1", accountID, tasks.StatusRunning, "lease-running-count-1")
	createAssignerTask(t, db, "running-count-2", accountID, tasks.StatusRunning, "lease-running-count-2")
	createAssignerTask(t, db, "running-count-released", accountID, tasks.StatusRunning, "lease-running-count-released")
	if _, err := db.Exec(`UPDATE task_leases SET released_at=clock_timestamp(), release_reason='completed' WHERE task_id='running-count-released'`); err != nil {
		t.Fatal(err)
	}

	count, err := queue.countActiveTaskLeases(ctx)
	if err != nil {
		t.Fatalf("countActiveTaskLeases failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("active task leases=%d, want 2", count)
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
