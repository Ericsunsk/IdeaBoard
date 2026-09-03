package postgres

import (
	"context"
	"testing"

	tasks "ideaboard/api/internal/tasking/domain"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
	"ideaboard/api/internal/testdb"
)

func TestListRunningTasksForStartupRecoveryIsReadOnly(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 92)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	runningTask := seedRepositoryRunningTaskWithActiveLease(
		t,
		repo,
		db,
		ctx,
		"startup-recovery-running",
		accountID,
		channelID,
		credentialID,
		"lease-startup-recovery-running",
		"lease-startup-recovery-running",
	)
	pendingTask := &tasks.Task{
		TaskID: "startup-recovery-pending",
		UserID: 1,
		Status: tasks.StatusPending,
	}
	seedTasks(t, repo, ctx, pendingTask)

	recoveredTasks, err := repo.ListRunningTasksForStartupRecovery(ctx)
	if err != nil {
		t.Fatalf("ListRunningTasksForStartupRecovery failed: %v", err)
	}
	if len(recoveredTasks) != 1 || recoveredTasks[0].TaskID != runningTask.TaskID {
		t.Fatalf("cancelled tasks=%v, want task_id=%s", recoveredTasks, runningTask.TaskID)
	}
	if recoveredTasks[0].RiverJobID != 1 {
		t.Fatalf("recovered river_job_id=%d, want 1", recoveredTasks[0].RiverJobID)
	}

	loaded, err := repo.GetByTaskID(ctx, runningTask.TaskID)
	if err != nil {
		t.Fatalf("load running task failed: %v", err)
	}
	if loaded.Status != tasks.StatusRunning {
		t.Fatalf("status=%q, want %q", loaded.Status, tasks.StatusRunning)
	}
	if loaded.LeaseToken != runningTask.LeaseToken {
		t.Fatalf("lease_token=%q, want %q", loaded.LeaseToken, runningTask.LeaseToken)
	}
	assertRepositoryActiveLeaseCount(t, db, runningTask.TaskID, 1)
	assertTaskState(t, repo, ctx, pendingTask.TaskID, tasks.StatusPending, 0, "")
	secondRead, err := repo.ListRunningTasksForStartupRecovery(ctx)
	if err != nil || len(secondRead) != 1 || secondRead[0] != recoveredTasks[0] {
		t.Fatalf("second read=%v err=%v, want idempotent inventory", secondRead, err)
	}
}

func TestRecoveredFailureRejectsTaskThatAcquiredActiveLease(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 93)
	providerEndpointID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := seedRepositoryRunningTaskWithActiveLease(
		t,
		repo,
		db,
		ctx,
		"startup-recovery-late-lease",
		accountID,
		providerEndpointID,
		credentialID,
		"river:93:late-lease",
		"river:93:late-lease",
	)

	rows, err := repo.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind:         runtimecontracts.TaskConvergenceRecoveredFailed,
		TaskID:       task.TaskID,
		ErrorMessage: "startup recovery",
	})
	if err != nil {
		t.Fatalf("recovered convergence returned error: %v", err)
	}
	if rows != 0 {
		t.Fatalf("rows=%d, want 0 after active lease appeared", rows)
	}
	assertTaskState(t, repo, ctx, task.TaskID, tasks.StatusRunning, accountID, task.LeaseToken)
	assertRepositoryActiveLeaseCount(t, db, task.TaskID, 1)
}
