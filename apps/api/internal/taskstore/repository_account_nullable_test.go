package taskstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/testdb"
)

func TestCreateTaskStoresUnassignedAccountAsNull(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)

	task := &tasks.Task{
		TaskID: "task-unassigned-account",
		UserID: 1,
		Status: tasks.StatusPending,
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create unassigned task failed: %v", err)
	}

	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if loaded.AccountID != nil {
		t.Fatalf("loaded account_id=%d, want nil", tasks.IDValue(loaded.AccountID))
	}

	var isNull bool
	if err := db.QueryRowContext(ctx, `SELECT account_id IS NULL FROM tasks WHERE task_id = $1`, task.TaskID).Scan(&isNull); err != nil {
		t.Fatalf("read raw account_id failed: %v", err)
	}
	if !isNull {
		t.Fatal("raw account_id is not NULL")
	}
}

func TestCreateTaskRejectsUnknownAccountID(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)

	err := repo.Create(ctx, &tasks.Task{
		TaskID:            "task-unknown-account",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(199999),
		AccountID:         tasks.IDPtr(99999),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-unknown-account",
	})
	if err == nil {
		t.Fatal("create task with unknown account_id succeeded")
	}
}

func TestCreateTaskAcceptsKnownAccountID(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 88)
	channelID := accountID + 10000

	task := &tasks.Task{
		TaskID:            "task-known-account",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(channelID),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-known-account",
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task with known account failed: %v", err)
	}
	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if tasks.IDValue(loaded.AccountID) != accountID {
		t.Fatalf("account_id=%d, want %d", tasks.IDValue(loaded.AccountID), accountID)
	}
	if tasks.IDValue(loaded.AssignedChannelID) != channelID {
		t.Fatalf("assigned_channel_id=%d, want %d", tasks.IDValue(loaded.AssignedChannelID), channelID)
	}
}

func TestTaskAttemptRequiresKnownAccountAndChannel(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 89)
	channelID := accountID + 10000

	task := &tasks.Task{
		TaskID:            "task-attempt-fk",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(channelID),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-attempt-fk",
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	if _, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:            task.TaskID,
		LeaseToken:        "lease-invalid-attempt",
		AccountID:         0,
		ChannelID:         channelID,
		ProviderStartedAt: time.Now(),
	}); err == nil {
		t.Fatal("task attempt without account_id succeeded")
	}
	if _, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:            task.TaskID,
		LeaseToken:        task.LeaseToken,
		AccountID:         accountID,
		ChannelID:         channelID,
		ProviderStartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("task attempt with known account/channel failed: %v", err)
	}
}

func TestCreateTaskAttemptRequiresCurrentLeaseIdentity(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 93)
	channelID := accountID + 10000

	task := &tasks.Task{
		TaskID:            "task-attempt-lease-identity",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(channelID),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-attempt-current",
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	_, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:            task.TaskID,
		LeaseToken:        "lease-attempt-stale",
		AccountID:         accountID,
		ChannelID:         channelID,
		ProviderStartedAt: time.Now(),
	})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale lease attempt error=%v, want sql.ErrNoRows", err)
	}
	var attemptSeq int
	if err := db.QueryRowContext(ctx, `SELECT attempt_seq FROM tasks WHERE task_id = $1`, task.TaskID).Scan(&attemptSeq); err != nil {
		t.Fatalf("read attempt_seq failed: %v", err)
	}
	if attemptSeq != 0 {
		t.Fatalf("attempt_seq=%d, want 0 after stale attempt", attemptSeq)
	}

	ref, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:            task.TaskID,
		LeaseToken:        task.LeaseToken,
		AccountID:         accountID,
		ChannelID:         channelID,
		ProviderStartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("current lease attempt failed: %v", err)
	}
	if ref.AttemptNo != 1 {
		t.Fatalf("attempt_no=%d, want 1", ref.AttemptNo)
	}
}

func TestFinishTaskAttemptOnlyUpdatesRunningAttempt(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 95)
	channelID := accountID + 10000

	task := &tasks.Task{
		TaskID:            "task-attempt-terminal-guard",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(channelID),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-attempt-terminal-guard",
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	ref, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:            task.TaskID,
		LeaseToken:        task.LeaseToken,
		AccountID:         accountID,
		ChannelID:         channelID,
		ProviderStartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("create attempt failed: %v", err)
	}
	finishedAt := time.Now()
	rows, err := repo.FinishTaskAttempt(ctx, tasks.TaskAttemptFinish{
		ID:          ref.ID,
		Status:      tasks.TaskAttemptStatusRequeued,
		CompletedAt: finishedAt,
	})
	if err != nil || rows != 1 {
		t.Fatalf("finish running attempt rows=%d err=%v, want rows=1 nil err", rows, err)
	}
	rows, err = repo.FinishTaskAttempt(ctx, tasks.TaskAttemptFinish{
		ID:          ref.ID,
		Status:      tasks.TaskAttemptStatusFailed,
		ErrorMsg:    "late failure",
		CompletedAt: finishedAt.Add(time.Second),
	})
	if err != nil || rows != 0 {
		t.Fatalf("finish terminal attempt rows=%d err=%v, want rows=0 nil err", rows, err)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM task_attempts WHERE id = $1`, ref.ID).Scan(&status); err != nil {
		t.Fatalf("load attempt failed: %v", err)
	}
	if status != tasks.TaskAttemptStatusRequeued {
		t.Fatalf("attempt status=%q, want %q", status, tasks.TaskAttemptStatusRequeued)
	}
}

func TestCompleteWithAttemptRollsBackWhenAttemptFinishFails(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 92)
	channelID := accountID + 10000

	task := &tasks.Task{
		TaskID:            "task-complete-attempt-rollback",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(channelID),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-complete-attempt-rollback",
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	rows, err := repo.CompleteWithOutputsAndLeaseToken(
		ctx,
		task.TaskID,
		[]string{tasks.StatusRunning},
		accountID,
		channelID,
		task.LeaseToken,
		`{"status":"success"}`,
		time.Now(),
		nil,
		&tasks.TaskAttemptFinish{
			ID:          999999,
			Status:      tasks.TaskAttemptStatusCompleted,
			CompletedAt: time.Now(),
		},
	)
	if err == nil {
		t.Fatal("complete task with missing attempt succeeded")
	}
	if rows != 0 {
		t.Fatalf("rows=%d, want 0 when transaction rolls back", rows)
	}

	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if loaded.Status != tasks.StatusRunning {
		t.Fatalf("status=%s, want %s", loaded.Status, tasks.StatusRunning)
	}
	if loaded.LeaseToken != task.LeaseToken {
		t.Fatalf("lease_token=%q, want %q", loaded.LeaseToken, task.LeaseToken)
	}
}

func TestRequeueWithAttemptRollsBackWhenAttemptFinishFails(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 94)
	channelID := accountID + 10000

	task := &tasks.Task{
		TaskID:            "task-requeue-attempt-rollback",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(channelID),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-requeue-attempt-rollback",
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	rows, err := repo.RequeueTaskRunningWithLease(
		ctx,
		task.TaskID,
		[]string{tasks.StatusRunning},
		accountID,
		channelID,
		task.LeaseToken,
		"retry",
		&tasks.TaskAttemptFinish{
			ID:          999999,
			Status:      tasks.TaskAttemptStatusRequeued,
			CompletedAt: time.Now(),
		},
	)
	if err == nil {
		t.Fatal("requeue task with missing attempt succeeded")
	}
	if rows != 0 {
		t.Fatalf("rows=%d, want 0 when transaction rolls back", rows)
	}

	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if loaded.Status != tasks.StatusRunning {
		t.Fatalf("status=%s, want %s", loaded.Status, tasks.StatusRunning)
	}
	if loaded.LeaseToken != task.LeaseToken {
		t.Fatalf("lease_token=%q, want %q", loaded.LeaseToken, task.LeaseToken)
	}
}

func TestResetRunningTaskClearsAssignedAccountAndChannel(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 90)
	channelID := accountID + 10000

	task := &tasks.Task{
		TaskID:            "task-reset-assigned-channel",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(channelID),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-reset-assigned-channel",
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	rows, err := repo.ResetTaskRunningToPendingWithLease(ctx, task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, task.LeaseToken, "retry", nil)
	if err != nil {
		t.Fatalf("reset task failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("reset rows=%d, want 1", rows)
	}
	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if loaded.AccountID != nil {
		t.Fatalf("account_id=%d, want nil", tasks.IDValue(loaded.AccountID))
	}
	if loaded.AssignedChannelID != nil {
		t.Fatalf("assigned_channel_id=%d, want nil", tasks.IDValue(loaded.AssignedChannelID))
	}
}

func TestMarkTaskCancelledStoresSystemCancelActorAsNull(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)

	task := &tasks.Task{
		TaskID: "task-system-cancel",
		UserID: 1,
		Status: tasks.StatusPending,
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	if rows, err := repo.MarkTaskCancelled(ctx, tasks.TaskCancellationSnapshot{
		TaskID:      task.TaskID,
		CancelledAt: time.Now(),
		Reason:      "system cancel",
	}, nil); err != nil || rows != 1 {
		t.Fatalf("mark cancelled rows=%d err=%v, want rows=1 nil err", rows, err)
	}

	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if loaded.CancelledBy != nil {
		t.Fatalf("cancelled_by=%d, want nil", tasks.IDValue(loaded.CancelledBy))
	}
}

func TestMarkTaskCancelledRetainsAssignmentSnapshot(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 91)
	channelID := accountID + 10000

	task := &tasks.Task{
		TaskID:            "task-cancel-retains-assignment",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(channelID),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-cancel-retains-assignment",
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	if rows, err := repo.MarkTaskCancelled(ctx, tasks.TaskCancellationSnapshot{
		TaskID:      task.TaskID,
		CancelledAt: time.Now(),
		Reason:      "system cancel",
	}, nil); err != nil || rows != 1 {
		t.Fatalf("mark cancelled rows=%d err=%v, want rows=1 nil err", rows, err)
	}

	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if loaded.Status != tasks.StatusCancelled {
		t.Fatalf("status=%s, want %s", loaded.Status, tasks.StatusCancelled)
	}
	if tasks.IDValue(loaded.AccountID) != accountID {
		t.Fatalf("account_id=%d, want %d", tasks.IDValue(loaded.AccountID), accountID)
	}
	if tasks.IDValue(loaded.AssignedChannelID) != channelID {
		t.Fatalf("assigned_channel_id=%d, want %d", tasks.IDValue(loaded.AssignedChannelID), channelID)
	}
	if loaded.ExecutionMetrics == nil {
		t.Fatal("execution metrics missing after cancellation")
	}
	if loaded.ExecutionMetrics.CurrentStage != tasks.ExecutionStageCancelled {
		t.Fatalf("current_stage=%q, want %q", loaded.ExecutionMetrics.CurrentStage, tasks.ExecutionStageCancelled)
	}
	if loaded.ExecutionMetrics.FailureCategory != tasks.FailureCategoryCancelled {
		t.Fatalf("failure_category=%q, want %q", loaded.ExecutionMetrics.FailureCategory, tasks.FailureCategoryCancelled)
	}
}
