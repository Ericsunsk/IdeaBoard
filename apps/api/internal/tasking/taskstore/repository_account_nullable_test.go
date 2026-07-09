package taskstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/tasks"
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
		TaskID:                     "task-unknown-account",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(199999),
		AccountID:                  tasks.IDPtr(99999),
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
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-known-account",
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
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10089",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-fk",
	}
	if err := repo.Create(ctx, task); err != nil {
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
	if err := repo.Create(ctx, pendingTask); err != nil {
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
	if err := repo.Create(ctx, requeuedPendingTask); err != nil {
		t.Fatalf("create requeued pending task failed: %v", err)
	}

	runningTask := &tasks.Task{
		TaskID:                     "attempt-pressure-running",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10090",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-pressure-running",
	}
	if err := repo.Create(ctx, runningTask); err != nil {
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
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10090",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-pressure-failed",
	}
	if err := repo.Create(ctx, failedTask); err != nil {
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
	if _, err := repo.FinishTaskAttempt(ctx, tasks.TaskAttemptFinish{
		ID:             failedRef.ID,
		Status:         tasks.TaskAttemptStatusFailed,
		TotalLatencyMS: 70_000,
		CompletedAt:    now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("finish failed attempt failed: %v", err)
	}

	retryTask := &tasks.Task{
		TaskID:                     "attempt-pressure-requeued",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10090",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-pressure-requeued",
	}
	if err := repo.Create(ctx, retryTask); err != nil {
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
	if _, err := repo.FinishTaskAttempt(ctx, tasks.TaskAttemptFinish{
		ID:             retryRef.ID,
		Status:         tasks.TaskAttemptStatusRequeued,
		TotalLatencyMS: 45_000,
		CompletedAt:    now.Add(-30 * time.Second),
	}); err != nil {
		t.Fatalf("finish retry attempt failed: %v", err)
	}

	rows, err := repo.ListMonitorExecutionPressureByRange(ctx, now.Add(-10*time.Minute), now)
	if err != nil {
		t.Fatalf("list monitor pressure failed: %v", err)
	}
	byKey := make(map[string]MonitorExecutionPressureAggregate, len(rows))
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
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10093",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-current",
	}
	if err := repo.Create(ctx, task); err != nil {
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
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10095",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-attempt-terminal-guard",
	}
	if err := repo.Create(ctx, task); err != nil {
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
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		ResolvedModel:              "gpt-image-1",
		APIType:                    "openai-compatible",
		ProviderEndpointBaseURL:    "https://example.com/repository-test-channel-10096",
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-retry-history-a",
	}
	if err := repo.Create(ctx, task); err != nil {
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
	rows, err := repo.RequeueTaskRunningWithLease(ctx, task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, credentialID, task.LeaseToken, "retry provider", "", &tasks.TaskAttemptFinish{
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
    resolved_model = 'gpt-image-1',
    api_type = 'openai-compatible',
    provider_endpoint_base_url = $5,
    lease_token = $6
WHERE task_id = $1`,
		task.TaskID,
		accountID,
		channelID,
		credentialID,
		task.ProviderEndpointBaseURL,
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
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-complete-attempt-rollback",
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
		TaskID:                     "task-requeue-attempt-rollback",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-requeue-attempt-rollback",
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
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-reset-assigned-channel",
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	rows, err := repo.ResetTaskRunningToPendingWithLease(ctx, task.TaskID, []string{tasks.StatusRunning}, accountID, channelID, loadRepositoryTestCredentialID(t, db, accountID), task.LeaseToken, "retry", nil)
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
		TaskID:                     "task-cancel-retains-assignment",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(channelID),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-cancel-retains-assignment",
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
