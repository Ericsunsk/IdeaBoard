package postgres

import (
	"context"
	"database/sql"
	"errors"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
	statisticscontracts "ideaboard/api/internal/tasking/ports/statistics"
	"testing"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
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
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
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
		TaskID:                     "task-unknown-account",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(199999),
		AccountID:                  testPtr(99999),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-unknown-account",
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
		TaskID:                     "task-known-account",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-known-account",
	}
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
		t.Fatalf("create task with known account failed: %v", err)
	}
	loaded, err := repo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task failed: %v", err)
	}
	if tasks.IDValue(loaded.AccountID) != accountID {
		t.Fatalf("account_id=%d, want %d", tasks.IDValue(loaded.AccountID), accountID)
	}
	if tasks.IDValue(loaded.AssignedProviderEndpointID) != channelID {
		t.Fatalf("assigned_provider_endpoint_id=%d, want %d", tasks.IDValue(loaded.AssignedProviderEndpointID), channelID)
	}
}

func TestTaskAttemptRequiresKnownAccountAndChannel(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 89)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)

	task := &tasks.Task{
		TaskID:                     "task-attempt-fk",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AssignedCredentialID:       testPtr(credentialID),
		AccountID:                  testPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10089",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-fk",
	}
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	insertRepositoryActiveTaskLease(t, db, task.TaskID, accountID, channelID, credentialID, task.LeaseToken)

	if _, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:             task.TaskID,
		LeaseToken:         "lease-invalid-attempt",
		AccountID:          0,
		ProviderEndpointID: channelID,
		ProviderStartedAt:  time.Now(),
	}); err == nil {
		t.Fatal("task attempt without account_id succeeded")
	}
	if _, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:             task.TaskID,
		LeaseToken:         task.LeaseToken,
		AccountID:          accountID,
		ProviderEndpointID: channelID,
		CredentialID:       credentialID,
		ProviderStartedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("task attempt with known account/channel failed: %v", err)
	}
}

func TestListMonitorExecutionPressureByRangeUsesQueueAndAttemptFacts(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 90)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	now := time.Now().Truncate(time.Millisecond)

	pendingTask := &tasks.Task{
		TaskID:    "monitor-pressure-pending",
		UserID:    1,
		Status:    tasks.StatusPending,
		CreatedAt: now.Add(-90 * time.Second),
	}
	if err := createRepositoryTestTask(t, repo, ctx, pendingTask); err != nil {
		t.Fatalf("create pending task failed: %v", err)
	}

	requeuedPendingTask := &tasks.Task{
		TaskID:     "monitor-pressure-requeued-pending",
		UserID:     1,
		Status:     tasks.StatusPending,
		RetryCount: 2,
		CreatedAt:  now.Add(-5 * time.Minute),
		UpdatedAt:  now.Add(-10 * time.Second),
	}
	if err := createRepositoryTestTask(t, repo, ctx, requeuedPendingTask); err != nil {
		t.Fatalf("create requeued pending task failed: %v", err)
	}

	runningTask := &tasks.Task{
		TaskID:                     "attempt-pressure-running",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AssignedCredentialID:       testPtr(credentialID),
		AccountID:                  testPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10090",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-pressure-running",
	}
	if err := createRepositoryTestTask(t, repo, ctx, runningTask); err != nil {
		t.Fatalf("create running task failed: %v", err)
	}
	insertRepositoryActiveTaskLease(t, db, runningTask.TaskID, accountID, channelID, credentialID, runningTask.LeaseToken)
	runningRef, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:             runningTask.TaskID,
		LeaseToken:         runningTask.LeaseToken,
		AccountID:          accountID,
		ProviderEndpointID: channelID,
		CredentialID:       credentialID,
		ProviderStartedAt:  now.Add(-30 * time.Second),
	})
	if err != nil {
		t.Fatalf("create running attempt failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE task_attempts SET created_at = $1 WHERE id = $2`, now.Add(-30*time.Second), runningRef.ID); err != nil {
		t.Fatalf("patch running attempt created_at failed: %v", err)
	}

	failedTask := &tasks.Task{
		TaskID:                     "attempt-pressure-failed",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AssignedCredentialID:       testPtr(credentialID),
		AccountID:                  testPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10090",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-pressure-failed",
	}
	if err := createRepositoryTestTask(t, repo, ctx, failedTask); err != nil {
		t.Fatalf("create failed task failed: %v", err)
	}
	insertRepositoryActiveTaskLease(t, db, failedTask.TaskID, accountID, channelID, credentialID, failedTask.LeaseToken)
	failedRef, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:             failedTask.TaskID,
		LeaseToken:         failedTask.LeaseToken,
		AccountID:          accountID,
		ProviderEndpointID: channelID,
		CredentialID:       credentialID,
		ProviderStartedAt:  now.Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create failed attempt failed: %v", err)
	}
	failedFinish := taskAttemptFinishForRef(
		failedRef,
		tasks.TaskAttemptStatusFailed,
		now.Add(-time.Minute),
	)
	failedFinish.TotalLatencyMS = 70_000
	if _, err := repo.FinishTaskAttempt(ctx, failedFinish); err != nil {
		t.Fatalf("finish failed attempt failed: %v", err)
	}

	retryTask := &tasks.Task{
		TaskID:                     "attempt-pressure-requeued",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AssignedCredentialID:       testPtr(credentialID),
		AccountID:                  testPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10090",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-pressure-requeued",
	}
	if err := createRepositoryTestTask(t, repo, ctx, retryTask); err != nil {
		t.Fatalf("create retry task failed: %v", err)
	}
	insertRepositoryActiveTaskLease(t, db, retryTask.TaskID, accountID, channelID, credentialID, retryTask.LeaseToken)
	retryRef, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:             retryTask.TaskID,
		LeaseToken:         retryTask.LeaseToken,
		AccountID:          accountID,
		ProviderEndpointID: channelID,
		CredentialID:       credentialID,
		ProviderStartedAt:  now.Add(-45 * time.Second),
	})
	if err != nil {
		t.Fatalf("create retry attempt failed: %v", err)
	}
	retryFinish := taskAttemptFinishForRef(
		retryRef,
		tasks.TaskAttemptStatusRequeued,
		now.Add(-30*time.Second),
	)
	retryFinish.TotalLatencyMS = 45_000
	if _, err := repo.FinishTaskAttempt(ctx, retryFinish); err != nil {
		t.Fatalf("finish retry attempt failed: %v", err)
	}

	rows, err := repo.ListMonitorExecutionPressureByRange(ctx, now.Add(-10*time.Minute), now)
	if err != nil {
		t.Fatalf("list monitor pressure failed: %v", err)
	}
	byKey := make(map[string]statisticscontracts.MonitorExecutionPressureAggregate, len(rows))
	for _, row := range rows {
		byKey[row.Key] = row
	}
	if byKey["backlog_pending"].Total != 1 {
		t.Fatalf("backlog_pending=%+v, want total 1", byKey["backlog_pending"])
	}
	if byKey["normal_pending"].Total != 1 || byKey["normal_pending"].Retry != 0 {
		t.Fatalf("normal_pending=%+v, want total 1 and retry 0", byKey["normal_pending"])
	}
	if byKey["normal_running"].Total != 1 {
		t.Fatalf("normal_running=%+v, want total 1", byKey["normal_running"])
	}
	if byKey["slow_failed"].Total != 1 || byKey["slow_failed"].Failed != 1 {
		t.Fatalf("slow_failed=%+v, want total/failed 1", byKey["slow_failed"])
	}
	if byKey["retrying"].Total != 1 || byKey["retrying"].Retry != 1 {
		t.Fatalf("retrying=%+v, want total/retry 1", byKey["retrying"])
	}
}

func TestCreateTaskAttemptRequiresCurrentLeaseIdentity(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 93)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)

	task := &tasks.Task{
		TaskID:                     "task-attempt-lease-identity",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AssignedCredentialID:       testPtr(credentialID),
		AccountID:                  testPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10093",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-current",
	}
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	insertRepositoryActiveTaskLease(t, db, task.TaskID, accountID, channelID, credentialID, task.LeaseToken)

	_, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:             task.TaskID,
		LeaseToken:         "lease-attempt-stale",
		AccountID:          accountID,
		ProviderEndpointID: channelID,
		CredentialID:       credentialID,
		ProviderStartedAt:  time.Now(),
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
		TaskID:             task.TaskID,
		LeaseToken:         task.LeaseToken,
		AccountID:          accountID,
		ProviderEndpointID: channelID,
		CredentialID:       credentialID,
		ProviderStartedAt:  time.Now(),
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
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)

	task := &tasks.Task{
		TaskID:                     "task-attempt-terminal-guard",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AssignedCredentialID:       testPtr(credentialID),
		AccountID:                  testPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10095",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-terminal-guard",
	}
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	insertRepositoryActiveTaskLease(t, db, task.TaskID, accountID, channelID, credentialID, task.LeaseToken)
	ref, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:             task.TaskID,
		LeaseToken:         task.LeaseToken,
		AccountID:          accountID,
		ProviderEndpointID: channelID,
		CredentialID:       credentialID,
		ProviderStartedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("create attempt failed: %v", err)
	}
	finishedAt := time.Now()
	rows, err := repo.FinishTaskAttempt(
		ctx,
		taskAttemptFinishForRef(ref, tasks.TaskAttemptStatusRequeued, finishedAt),
	)
	if err != nil || rows != 1 {
		t.Fatalf("finish running attempt rows=%d err=%v, want rows=1 nil err", rows, err)
	}
	lateFinish := taskAttemptFinishForRef(
		ref,
		tasks.TaskAttemptStatusFailed,
		finishedAt.Add(time.Second),
	)
	lateFinish.ErrorMsg = "late failure"
	rows, err = repo.FinishTaskAttempt(ctx, lateFinish)
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

func taskAttemptFinishForRef(
	ref tasks.TaskAttemptRef,
	status string,
	completedAt time.Time,
) tasks.TaskAttemptFinish {
	return tasks.TaskAttemptFinish{
		ID: ref.ID, TaskID: ref.TaskID, LeaseToken: ref.LeaseToken,
		AccountID: ref.AccountID, ProviderEndpointID: ref.ProviderEndpointID,
		CredentialID: ref.CredentialID, Status: status, CompletedAt: completedAt,
	}
}

func TestRetryAttemptHistoryUsesReleasedLeaseThenNewLease(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 96)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	now := time.Now().Truncate(time.Millisecond)

	task := &tasks.Task{
		TaskID:                     "task-attempt-retry-history",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AssignedCredentialID:       testPtr(credentialID),
		AccountID:                  testPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10096",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-retry-history-a",
	}
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	insertRepositoryActiveTaskLease(t, db, task.TaskID, accountID, channelID, credentialID, task.LeaseToken)

	firstRef, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:             task.TaskID,
		LeaseToken:         task.LeaseToken,
		AccountID:          accountID,
		ProviderEndpointID: channelID,
		CredentialID:       credentialID,
		ProviderStartedAt:  now.Add(-2 * time.Second),
	})
	if err != nil {
		t.Fatalf("create first attempt failed: %v", err)
	}
	rows, err := repo.convergeRequeueForTest(ctx, task.TaskID, accountID, channelID, credentialID, task.LeaseToken, "retry provider", "", &tasks.TaskAttemptFinish{
		ID:              firstRef.ID,
		Status:          tasks.TaskAttemptStatusRequeued,
		FailureStage:    tasks.ExecutionStageProvider,
		FailureCategory: tasks.FailureCategoryProviderError,
		ErrorMsg:        "retry provider",
		CompletedAt:     now,
	})
	if err != nil || rows != 1 {
		t.Fatalf("requeue first attempt rows=%d err=%v, want rows=1 nil err", rows, err)
	}

	secondLease := "lease-retry-history-b"
	if _, err := db.ExecContext(ctx, `
UPDATE tasks
SET status = 'running',
    account_id = $2,
    assigned_provider_endpoint_id = $3,
    assigned_credential_id = $4,
    lease_token = $5
WHERE task_id = $1`,
		task.TaskID,
		accountID,
		channelID,
		credentialID,
		secondLease,
	); err != nil {
		t.Fatalf("assign second lease snapshot failed: %v", err)
	}
	insertRepositoryActiveTaskLease(t, db, task.TaskID, accountID, channelID, credentialID, secondLease)

	secondRef, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID:             task.TaskID,
		LeaseToken:         secondLease,
		AccountID:          accountID,
		ProviderEndpointID: channelID,
		CredentialID:       credentialID,
		ProviderStartedAt:  now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("create second attempt failed: %v", err)
	}
	if secondRef.AttemptNo != 2 {
		t.Fatalf("second attempt_no=%d, want 2", secondRef.AttemptNo)
	}
	if firstRef.LeaseID == 0 || secondRef.LeaseID == 0 || firstRef.LeaseID == secondRef.LeaseID {
		t.Fatalf("lease ids first=%d second=%d, want distinct non-zero", firstRef.LeaseID, secondRef.LeaseID)
	}

	var firstStatus, secondStatus string
	var firstCompletedValid, firstCancelledValid, secondCompletedValid bool
	if err := db.QueryRowContext(ctx, `
SELECT
  MAX(status) FILTER (WHERE id = $1),
  bool_or(completed_at IS NOT NULL) FILTER (WHERE id = $1),
  bool_or(cancelled_at IS NOT NULL) FILTER (WHERE id = $1),
  MAX(status) FILTER (WHERE id = $2),
  bool_or(completed_at IS NOT NULL) FILTER (WHERE id = $2)
FROM task_attempts
WHERE task_id = $3`, firstRef.ID, secondRef.ID, task.TaskID).Scan(
		&firstStatus,
		&firstCompletedValid,
		&firstCancelledValid,
		&secondStatus,
		&secondCompletedValid,
	); err != nil {
		t.Fatalf("read attempt history failed: %v", err)
	}
	if firstStatus != tasks.TaskAttemptStatusRequeued || !firstCompletedValid || firstCancelledValid {
		t.Fatalf("first attempt status=%q completed=%v cancelled=%v, want requeued completed only", firstStatus, firstCompletedValid, firstCancelledValid)
	}
	if secondStatus != tasks.TaskAttemptStatusRunning || secondCompletedValid {
		t.Fatalf("second attempt status=%q completed=%v, want running without completed_at", secondStatus, secondCompletedValid)
	}
}

func TestCompleteWithAttemptRollsBackWhenAttemptFinishFails(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 92)
	channelID := accountID + 10000

	task := &tasks.Task{
		TaskID:                     "task-complete-attempt-rollback",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-complete-attempt-rollback",
	}
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	rows, err := repo.ConvergeTask(ctx, completedTaskConvergenceIntent(
		task.TaskID,
		accountID,
		channelID,
		loadRepositoryTestCredentialID(t, db, accountID),
		task.LeaseToken,
		`{"status":"success"}`,
		time.Now(),
		nil,
		&tasks.TaskAttemptFinish{
			ID:          999999,
			Status:      tasks.TaskAttemptStatusCompleted,
			CompletedAt: time.Now(),
		},
	))
	if !errors.Is(err, runtimecontracts.ErrTaskLeaseStateConflict) {
		t.Fatalf("complete task with missing attempt err=%v, want ErrTaskLeaseStateConflict", err)
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

func TestCompleteRollsBackWhenAttemptBelongsToAnotherTask(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 191)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	taskA := seedRepositoryRunningTaskWithActiveLease(t, repo, db, ctx, "terminal-cross-task-a", accountID, channelID, credentialID, "lease-cross-a", "lease-cross-a")
	taskB := seedRepositoryRunningTaskWithActiveLease(t, repo, db, ctx, "terminal-cross-task-b", accountID, channelID, credentialID, "lease-cross-b", "lease-cross-b")
	attemptA, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID: taskA.TaskID, LeaseToken: taskA.LeaseToken, AccountID: accountID,
		ProviderEndpointID: channelID, CredentialID: credentialID, ProviderStartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	attemptB, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID: taskB.TaskID, LeaseToken: taskB.LeaseToken, AccountID: accountID,
		ProviderEndpointID: channelID, CredentialID: credentialID, ProviderStartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := repo.ConvergeTask(ctx, completedTaskConvergenceIntent(
		taskA.TaskID, accountID, channelID, credentialID,
		taskA.LeaseToken, `{}`, time.Now(), nil,
		&tasks.TaskAttemptFinish{ID: attemptB.ID, Status: tasks.TaskAttemptStatusCompleted, CompletedAt: time.Now()},
	))
	if err == nil || rows != 0 {
		t.Fatalf("cross-task attempt rows=%d err=%v, want rollback", rows, err)
	}
	assertTaskAndAttemptStillRunning(t, db, taskA.TaskID, attemptA.ID)
	assertTaskAndAttemptStillRunning(t, db, taskB.TaskID, attemptB.ID)
}

func TestCompleteRollsBackWhenAttemptBelongsToStaleLease(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 192)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, ctx, "terminal-stale-lease", accountID, channelID, credentialID, "lease-stale-old", "lease-stale-old")
	oldAttempt, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID: task.TaskID, LeaseToken: task.LeaseToken, AccountID: accountID,
		ProviderEndpointID: channelID, CredentialID: credentialID, ProviderStartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE task_leases
SET released_at = now(), release_reason = 'reset'
WHERE task_id = $1 AND lease_token = $2`,
		task.TaskID, task.LeaseToken); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE tasks
SET lease_token = 'lease-stale-new'
WHERE task_id = $1`, task.TaskID); err != nil {
		t.Fatal(err)
	}
	insertRepositoryActiveTaskLease(t, db, task.TaskID, accountID, channelID, credentialID, "lease-stale-new")

	rows, err := repo.ConvergeTask(ctx, completedTaskConvergenceIntent(
		task.TaskID, accountID, channelID, credentialID,
		"lease-stale-new", `{}`, time.Now(), nil,
		&tasks.TaskAttemptFinish{ID: oldAttempt.ID, Status: tasks.TaskAttemptStatusCompleted, CompletedAt: time.Now()},
	))
	if err == nil || rows != 0 {
		t.Fatalf("stale-lease attempt rows=%d err=%v, want rollback", rows, err)
	}
	assertTaskAndAttemptStillRunning(t, db, task.TaskID, oldAttempt.ID)
	var activeLeaseToken string
	if err := db.QueryRowContext(ctx, `
SELECT lease_token FROM task_leases WHERE task_id = $1 AND released_at IS NULL`, task.TaskID).Scan(&activeLeaseToken); err != nil {
		t.Fatal(err)
	}
	if activeLeaseToken != "lease-stale-new" {
		t.Fatalf("active lease token=%q, want new lease preserved", activeLeaseToken)
	}
}

func TestCompleteRollsBackWhenAttemptStatusConflictsWithTerminalPolicy(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 193)
	channelID := accountID + 10000
	credentialID := loadRepositoryTestCredentialID(t, db, accountID)
	task := seedRepositoryRunningTaskWithActiveLease(t, repo, db, ctx, "terminal-status-mismatch", accountID, channelID, credentialID, "lease-status-mismatch", "lease-status-mismatch")
	attempt, err := repo.CreateTaskAttempt(ctx, tasks.TaskAttemptStart{
		TaskID: task.TaskID, LeaseToken: task.LeaseToken, AccountID: accountID,
		ProviderEndpointID: channelID, CredentialID: credentialID, ProviderStartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := repo.ConvergeTask(ctx, completedTaskConvergenceIntent(
		task.TaskID, accountID, channelID, credentialID,
		task.LeaseToken, `{}`, time.Now(), nil,
		&tasks.TaskAttemptFinish{ID: attempt.ID, Status: tasks.TaskAttemptStatusFailed, CompletedAt: time.Now()},
	))
	if err == nil || rows != 0 {
		t.Fatalf("attempt status mismatch rows=%d err=%v, want rollback", rows, err)
	}
	assertTaskAndAttemptStillRunning(t, db, task.TaskID, attempt.ID)
}

func assertTaskAndAttemptStillRunning(t *testing.T, db *sql.DB, taskID string, attemptID int64) {
	t.Helper()
	var taskStatus, attemptStatus string
	if err := db.QueryRow(`
SELECT tasks.status, attempts.status
FROM tasks
JOIN task_attempts attempts ON attempts.task_id = tasks.task_id
WHERE tasks.task_id = $1 AND attempts.id = $2`, taskID, attemptID).Scan(&taskStatus, &attemptStatus); err != nil {
		t.Fatal(err)
	}
	if taskStatus != tasks.StatusRunning || attemptStatus != tasks.TaskAttemptStatusRunning {
		t.Fatalf("task/attempt status=%q/%q, want running/running", taskStatus, attemptStatus)
	}
}

func TestRequeueWithAttemptRollsBackWhenAttemptFinishFails(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewRepository(db)
	accountID := seedRepositoryTestAccount(t, db, 94)
	channelID := accountID + 10000

	task := &tasks.Task{
		TaskID:                     "task-requeue-attempt-rollback",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-requeue-attempt-rollback",
	}
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	rows, err := repo.convergeRequeueForTest(
		ctx,
		task.TaskID,
		accountID,
		channelID,
		loadRepositoryTestCredentialID(t, db, accountID),
		task.LeaseToken,
		"retry",
		"",
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
		TaskID:                     "task-reset-assigned-channel",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-reset-assigned-channel",
	}
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	insertRepositoryActiveTaskLease(t, db, task.TaskID, accountID, channelID, loadRepositoryTestCredentialID(t, db, accountID), task.LeaseToken)
	rows, err := repo.convergeResetForTest(ctx, task.TaskID, accountID, channelID, loadRepositoryTestCredentialID(t, db, accountID), task.LeaseToken, "retry", nil)
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
	if loaded.AssignedProviderEndpointID != nil {
		t.Fatalf("assigned_provider_endpoint_id=%d, want nil", tasks.IDValue(loaded.AssignedProviderEndpointID))
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
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
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
		TaskID:                     "task-cancel-retains-assignment",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(channelID),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-cancel-retains-assignment",
	}
	if err := createRepositoryTestTask(t, repo, ctx, task); err != nil {
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
	if tasks.IDValue(loaded.AssignedProviderEndpointID) != channelID {
		t.Fatalf("assigned_provider_endpoint_id=%d, want %d", tasks.IDValue(loaded.AssignedProviderEndpointID), channelID)
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
