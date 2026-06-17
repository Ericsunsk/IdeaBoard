package taskstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/testdb"
)

func TestTaskSchemaIncludesLeaseTokenColumn(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "db", "schema", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	if !strings.Contains(string(raw), "lease_token VARCHAR(128) NOT NULL DEFAULT ''") {
		t.Fatal("expected tasks table to contain lease_token column")
	}
}

func TestRunningTasksMissingLeaseTokenPreflightOnlyMatchesRunningMissingLeaseToken(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	account11 := seedRepositoryTestAccount(t, db, 11)
	account22 := seedRepositoryTestAccount(t, db, 22)
	account33 := seedRepositoryTestAccount(t, db, 33)
	account55 := seedRepositoryTestAccount(t, db, 55)

	seedTasks(t, repo, ctx,
		&tasks.Task{TaskID: "running-missing-1", UserID: 1, AccountID: tasks.IDPtr(account11), Status: tasks.StatusRunning},
		&tasks.Task{TaskID: "running-missing-2", UserID: 1, AccountID: tasks.IDPtr(account22), Status: tasks.StatusRunning},
		&tasks.Task{TaskID: "running-with-token", UserID: 1, AccountID: tasks.IDPtr(account33), Status: tasks.StatusRunning, LeaseToken: "lease-token"},
		&tasks.Task{TaskID: "pending-missing-token", UserID: 1, Status: tasks.StatusPending},
		&tasks.Task{TaskID: "failed-missing-token", UserID: 1, AccountID: tasks.IDPtr(account55), Status: tasks.StatusFailed},
	)

	count, err := repo.CountRunningTasksMissingLeaseToken(ctx)
	if err != nil {
		t.Fatalf("CountRunningTasksMissingLeaseToken failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("missing running lease count=%d, want 2", count)
	}

	leases, err := repo.ListRunningTasksMissingLeaseToken(ctx, 10)
	if err != nil {
		t.Fatalf("ListRunningTasksMissingLeaseToken failed: %v", err)
	}
	if len(leases) != 2 {
		t.Fatalf("missing running leases len=%d, want 2", len(leases))
	}
	if leases[0].TaskID != "running-missing-1" || leases[0].AccountID != 11 || leases[0].LeaseToken != "" {
		t.Fatalf("first missing lease=%+v, want running-missing-1 account_id=11 empty token", leases[0])
	}
	if leases[1].TaskID != "running-missing-2" || leases[1].AccountID != 22 || leases[1].LeaseToken != "" {
		t.Fatalf("second missing lease=%+v, want running-missing-2 account_id=22 empty token", leases[1])
	}
}

func TestRunningTasksMissingLeaseTokenPreflightDoesNotMutate(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 11)

	seedTasks(t, repo, ctx,
		&tasks.Task{TaskID: "running-missing-token", UserID: 1, AccountID: tasks.IDPtr(accountID), Status: tasks.StatusRunning},
	)

	if _, err := repo.CountRunningTasksMissingLeaseToken(ctx); err != nil {
		t.Fatalf("CountRunningTasksMissingLeaseToken failed: %v", err)
	}
	if _, err := repo.ListRunningTasksMissingLeaseToken(ctx, 10); err != nil {
		t.Fatalf("ListRunningTasksMissingLeaseToken failed: %v", err)
	}

	task, err := repo.GetByTaskID(ctx, "running-missing-token")
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if task.Status != tasks.StatusRunning || tasks.IDValue(task.AccountID) != accountID || task.LeaseToken != "" {
		t.Fatalf("task mutated by preflight: status=%q account_id=%d lease_token=%q", task.Status, tasks.IDValue(task.AccountID), task.LeaseToken)
	}
}

func TestResetRunningTasksMissingLeaseTokenToPendingRepairsOnlyTargets(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	account11 := seedRepositoryTestAccount(t, db, 11)
	account22 := seedRepositoryTestAccount(t, db, 22)
	account33 := seedRepositoryTestAccount(t, db, 33)

	seedTasks(t, repo, ctx,
		&tasks.Task{TaskID: "running-missing-1", UserID: 1, AccountID: tasks.IDPtr(account11), Status: tasks.StatusRunning},
		&tasks.Task{TaskID: "running-missing-2", UserID: 1, AccountID: tasks.IDPtr(account22), Status: tasks.StatusRunning},
		&tasks.Task{TaskID: "running-with-token", UserID: 1, AccountID: tasks.IDPtr(account33), Status: tasks.StatusRunning, LeaseToken: "lease-token"},
		&tasks.Task{TaskID: "pending-missing-token", UserID: 1, Status: tasks.StatusPending},
	)

	rows, err := repo.ResetRunningTasksMissingLeaseTokenToPending(ctx, 10)
	if err != nil {
		t.Fatalf("ResetRunningTasksMissingLeaseTokenToPending failed: %v", err)
	}
	if rows != 2 {
		t.Fatalf("repaired rows=%d, want 2", rows)
	}

	assertTaskState(t, repo, ctx, "running-missing-1", tasks.StatusPending, 0, "")
	assertTaskState(t, repo, ctx, "running-missing-2", tasks.StatusPending, 0, "")
	assertTaskRepairNote(t, repo, ctx, "running-missing-1")
	assertTaskRepairNote(t, repo, ctx, "running-missing-2")
	assertTaskState(t, repo, ctx, "running-with-token", tasks.StatusRunning, 33, "lease-token")
	assertTaskState(t, repo, ctx, "pending-missing-token", tasks.StatusPending, 0, "")
}

func TestResetRunningTasksMissingLeaseTokenToPendingHonorsLimit(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	account11 := seedRepositoryTestAccount(t, db, 11)
	account22 := seedRepositoryTestAccount(t, db, 22)

	seedTasks(t, repo, ctx,
		&tasks.Task{TaskID: "running-missing-1", UserID: 1, AccountID: tasks.IDPtr(account11), Status: tasks.StatusRunning},
		&tasks.Task{TaskID: "running-missing-2", UserID: 1, AccountID: tasks.IDPtr(account22), Status: tasks.StatusRunning},
	)

	rows, err := repo.ResetRunningTasksMissingLeaseTokenToPending(ctx, 1)
	if err != nil {
		t.Fatalf("ResetRunningTasksMissingLeaseTokenToPending failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("repaired rows=%d, want 1", rows)
	}

	assertTaskState(t, repo, ctx, "running-missing-1", tasks.StatusPending, 0, "")
	assertTaskState(t, repo, ctx, "running-missing-2", tasks.StatusRunning, 22, "")
	assertTaskRepairNote(t, repo, ctx, "running-missing-1")

	count, err := repo.CountRunningTasksMissingLeaseToken(ctx)
	if err != nil {
		t.Fatalf("CountRunningTasksMissingLeaseToken failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("remaining missing running lease count=%d, want 1", count)
	}
}

func TestCompleteWithOutputsAndLeaseTokenRejectsWrongLeaseToken(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 11)

	task := &tasks.Task{
		TaskID:     "lease-conflict-wrong-token",
		UserID:     1,
		AccountID:  tasks.IDPtr(accountID),
		Status:     tasks.StatusRunning,
		LeaseToken: "correct-token",
	}
	seedTasks(t, repo, ctx, task)

	updated, err := repo.CompleteWithOutputsAndLeaseToken(
		ctx, "lease-conflict-wrong-token",
		[]string{tasks.StatusRunning},
		accountID, accountID+10000,
		"wrong-token", // wrong lease token
		"result", time.Now(),
		nil, nil,
	)
	if err != nil {
		t.Fatalf("CompleteWithOutputsAndLeaseToken with wrong token errored: %v", err)
	}
	if updated != 0 {
		t.Fatalf("updated=%d with wrong lease token, want 0", updated)
	}

	taskAfter, err := repo.GetByTaskID(ctx, "lease-conflict-wrong-token")
	if err != nil {
		t.Fatalf("GetByTaskID: %v", err)
	}
	if taskAfter.Status != tasks.StatusRunning {
		t.Fatalf("status=%q, want running (should not complete with wrong token)", taskAfter.Status)
	}
}

func TestCompleteWithOutputsAndLeaseTokenConcurrentOnlyOneSucceeds(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 11)

	task := &tasks.Task{
		TaskID:     "lease-conflict-concurrent",
		UserID:     1,
		AccountID:  tasks.IDPtr(accountID),
		Status:     tasks.StatusRunning,
		LeaseToken: "token-a",
	}
	seedTasks(t, repo, ctx, task)

	results := make(chan int64, 2)
	for i := 0; i < 2; i++ {
		go func() {
			updated, err := repo.CompleteWithOutputsAndLeaseToken(
				context.Background(), "lease-conflict-concurrent",
				[]string{tasks.StatusRunning},
				accountID, accountID+10000,
				"token-a", // both use correct token, but only one should succeed
				"result", time.Now(),
				nil, nil,
			)
			if err != nil {
				results <- -1
				return
			}
			results <- updated
		}()
	}

	sum := int64(0)
	for i := 0; i < 2; i++ {
		r := <-results
		if r >= 0 {
			sum += r
		}
	}
	if sum != 1 {
		t.Fatalf("total updated rows=%d, want exactly 1 (both goroutines used same token but only one row matches)", sum)
	}

	taskAfter, err := repo.GetByTaskID(ctx, "lease-conflict-concurrent")
	if err != nil {
		t.Fatalf("GetByTaskID: %v", err)
	}
	if taskAfter.Status != tasks.StatusCompleted {
		t.Fatalf("status=%q, want completed", taskAfter.Status)
	}
}

func TestResetRunningTasksMissingLeaseTokenToPendingRequiresPositiveLimit(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 11)

	seedTasks(t, repo, ctx,
		&tasks.Task{TaskID: "running-missing-token", UserID: 1, AccountID: tasks.IDPtr(accountID), Status: tasks.StatusRunning},
	)

	rows, err := repo.ResetRunningTasksMissingLeaseTokenToPending(ctx, 0)
	if err == nil {
		t.Fatal("ResetRunningTasksMissingLeaseTokenToPending with zero limit succeeded, want error")
	}
	if rows != 0 {
		t.Fatalf("repaired rows=%d, want 0", rows)
	}
	assertTaskState(t, repo, ctx, "running-missing-token", tasks.StatusRunning, 11, "")
}

func seedTasks(t *testing.T, repo *Repository, ctx context.Context, items ...*tasks.Task) {
	t.Helper()
	for _, item := range items {
		if item.AccountID != nil && item.AssignedChannelID == nil {
			item.AssignedChannelID = tasks.IDPtr(tasks.IDValue(item.AccountID) + 10000)
		}
		if err := repo.Create(ctx, item); err != nil {
			t.Fatalf("seed task %q failed: %v", item.TaskID, err)
		}
	}
}

func assertTaskState(t *testing.T, repo *Repository, ctx context.Context, taskID string, status string, accountID int64, leaseToken string) {
	t.Helper()
	task, err := repo.GetByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("load task %q failed: %v", taskID, err)
	}
	if task.Status != status || tasks.IDValue(task.AccountID) != accountID || task.LeaseToken != leaseToken {
		t.Fatalf("task %q state = status=%q account_id=%d lease_token=%q, want status=%q account_id=%d lease_token=%q",
			taskID, task.Status, tasks.IDValue(task.AccountID), task.LeaseToken, status, accountID, leaseToken)
	}
}

func assertTaskRepairNote(t *testing.T, repo *Repository, ctx context.Context, taskID string) {
	t.Helper()
	task, err := repo.GetByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("load task %q failed: %v", taskID, err)
	}
	if task.ErrorMsg == "" {
		t.Fatalf("task %q error_msg is empty, want repair note", taskID)
	}
	if task.CompletedAt != nil {
		t.Fatalf("task %q completed_at=%v, want nil", taskID, task.CompletedAt)
	}
}
