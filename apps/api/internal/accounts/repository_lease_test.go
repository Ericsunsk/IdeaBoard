package accounts

import (
	"context"
	"testing"

	"api/internal/tasks"
)

func TestTaskAutoMigrateIncludesLeaseTokenColumn(t *testing.T) {
	db := newAccountsTestDB(t)
	if !db.Migrator().HasColumn(&tasks.Task{}, "lease_token") {
		t.Fatal("expected tasks table to contain lease_token column after AutoMigrate")
	}
}

func TestListRunningTaskLeasesIncludesLeaseToken(t *testing.T) {
	ctx := context.Background()
	db := newAccountsTestDB(t)
	repo := NewTaskRepository(db)

	if err := repo.Create(ctx, &tasks.Task{
		TaskID:     "running-with-token",
		UserID:     1,
		ChannelID:  1,
		AccountID:  11,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-token-1",
	}); err != nil {
		t.Fatalf("seed running task with token failed: %v", err)
	}
	if err := repo.Create(ctx, &tasks.Task{
		TaskID:    "running-empty-token",
		UserID:    1,
		ChannelID: 1,
		AccountID: 22,
		Status:    tasks.StatusRunning,
	}); err != nil {
		t.Fatalf("seed running task without token failed: %v", err)
	}
	if err := repo.Create(ctx, &tasks.Task{
		TaskID:     "pending-with-token",
		UserID:     1,
		ChannelID:  1,
		AccountID:  33,
		Status:     tasks.StatusPending,
		LeaseToken: "lease-token-pending",
	}); err != nil {
		t.Fatalf("seed non-running task failed: %v", err)
	}

	leases, err := repo.ListRunningTaskLeases(ctx, 10)
	if err != nil {
		t.Fatalf("ListRunningTaskLeases failed: %v", err)
	}
	if len(leases) != 2 {
		t.Fatalf("running leases len=%d, want 2", len(leases))
	}

	if leases[0].TaskID != "running-with-token" || leases[0].AccountID != 11 || leases[0].LeaseToken != "lease-token-1" {
		t.Fatalf("first lease=%+v, want task_id=running-with-token account_id=11 lease_token=lease-token-1", leases[0])
	}
	if leases[1].TaskID != "running-empty-token" || leases[1].AccountID != 22 || leases[1].LeaseToken != "" {
		t.Fatalf("second lease=%+v, want task_id=running-empty-token account_id=22 empty lease_token", leases[1])
	}
}

