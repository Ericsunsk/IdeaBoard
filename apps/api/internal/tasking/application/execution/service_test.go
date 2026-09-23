package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ideaboard/api/internal/events"
	tasks "ideaboard/api/internal/tasking/domain"
	taskstore "ideaboard/api/internal/tasking/infrastructure/postgres"
	"ideaboard/api/internal/testdb"
)

type recordingTaskStatusPublisher struct {
	ch chan events.TaskStatusEvent
}

const taskExecutionTestParams = `{"schema_version":5}`

func newRecordingTaskStatusPublisher() *recordingTaskStatusPublisher {
	return &recordingTaskStatusPublisher{ch: make(chan events.TaskStatusEvent, 8)}
}

func (p *recordingTaskStatusPublisher) PublishTaskStatus(ctx context.Context, event events.TaskStatusEvent) error {
	event = events.NormalizeTaskStatusEvent(event)
	select {
	case p.ch <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestResetRunningTaskResetsState(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	accountID := seedTaskExecutionAccount(t, db, 77)

	task := &tasks.Task{
		TaskID:                     "task-reset-pending",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-reset",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.ResetRunningTask(ctx, ResetRunningTaskInput{TaskID: task.TaskID, ExecutionToken: task.LeaseToken, Reason: "worker context interrupted"}); err != nil {
		t.Fatalf("ResetRunningTask returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
	// Token-fenced reset clears the fence, not the assignment history:
	// account columns stay for audit while lease_token gates the next claim.
	if tasks.IDValue(updated.AccountID) != accountID {
		t.Fatalf("account_id = %d, want retained %d", tasks.IDValue(updated.AccountID), accountID)
	}
	if updated.ErrorMsg != "worker context interrupted" {
		t.Fatalf("error_msg = %q, want %q", updated.ErrorMsg, "worker context interrupted")
	}
	if updated.CompletedAt != nil {
		t.Fatalf("completed_at should remain nil, got %v", updated.CompletedAt)
	}
}

func TestRequeueRunningTaskIncrementsRetryCountAndClosesAttempt(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	task := &tasks.Task{TaskID: "task-requeue-count", UserID: 1, Status: tasks.StatusPending}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	token := "exec-token-requeue-count"
	if _, err := service.ClaimAndStartExecutionAttempt(ctx, tasks.ExecutionAttemptStart{
		TaskID: task.TaskID, ExecutionToken: token, FeatureSlug: "render",
		RequestedModel: "openai/gpt-image-1", Provider: "openai", StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("ClaimAndStartExecutionAttempt failed: %v", err)
	}

	finish := &tasks.TaskAttemptFinish{
		TaskID: task.TaskID, Status: tasks.TaskAttemptStatusRequeued,
		FailureStage: "provider", FailureCategory: "timeout", ErrorMsg: "temporary provider failure",
		CompletedAt: time.Now(),
	}
	if err := service.RequeueRunningTask(ctx, ResetRunningTaskInput{
		TaskID: task.TaskID, ExecutionToken: token, Reason: finish.ErrorMsg, AttemptFinish: finish,
	}); err != nil {
		t.Fatalf("RequeueRunningTask failed: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusPending || updated.RetryCount != 1 || updated.LeaseToken != "" {
		t.Fatalf("task status/retry_count/lease = %s/%d/%q, want pending/1/empty", updated.Status, updated.RetryCount, updated.LeaseToken)
	}
	closed, err := taskRepo.GetExecutionAttemptByToken(ctx, token)
	if err != nil {
		t.Fatalf("reload execution attempt failed: %v", err)
	}
	if closed.Status != tasks.TaskAttemptStatusRequeued {
		t.Fatalf("attempt status = %q, want %q", closed.Status, tasks.TaskAttemptStatusRequeued)
	}
}

func TestResetRunningTaskRejectsTerminalTask(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	accountID := seedTaskExecutionAccount(t, db, 77)

	task := &tasks.Task{
		TaskID:                     "task-reset-terminal",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusCompleted,
		CompletedAt:                ptrTime(time.Now()),
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.ResetRunningTask(ctx, ResetRunningTaskInput{TaskID: task.TaskID, ExecutionToken: "lease-terminal", Reason: "worker context interrupted"})
	if !errors.Is(err, ErrTaskTerminal) {
		t.Fatalf("expected ErrTaskTerminal, got %v", err)
	}
}

func TestResetRunningTaskEmptyTokenLeavesStateUntouched(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	accountID := seedTaskExecutionAccount(t, db, 77)

	task := &tasks.Task{
		TaskID:                     "task-reset-empty-token",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-empty-token",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	// Empty token means nothing was claimed by this worker (pre-claim
	// failure path): the reset must not touch another worker's claim.
	if err := service.ResetRunningTask(ctx, ResetRunningTaskInput{TaskID: task.TaskID, Reason: "pre-claim failure"}); err != nil {
		t.Fatalf("ResetRunningTask returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusRunning || updated.LeaseToken != "lease-empty-token" {
		t.Fatalf("status/lease = %s/%q, want running/lease-empty-token", updated.Status, updated.LeaseToken)
	}
}

func TestFailTaskPersistsProvidedResultEnvelope(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)

	task := &tasks.Task{
		TaskID: "task-fail-custom-result",
		UserID: 1,
		Status: tasks.StatusPending,
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"task_id":"task-fail-custom-result","status":"failed","parsed_output_status":"invalid","error":"contract failed"}`
	if err := service.FailTask(ctx, FailTaskInput{
		TaskID: task.TaskID,
		Reason: "contract failed",
		Result: result,
	}); err != nil {
		t.Fatalf("FailTask returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusFailed {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusFailed)
	}
	if updated.Result != result {
		t.Fatalf("result = %q, want provided envelope", updated.Result)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func newTaskExecutionServiceTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Open(t)
}

func seedTaskExecutionAccount(t testing.TB, db *pgxpool.Pool, id int64) int64 {
	t.Helper()
	channelID := taskExecutionProviderEndpointID(id)
	if _, err := db.Exec(t.Context(), `
INSERT INTO provider_endpoints (id, api_type, name, base_url, status)
VALUES ($1, 'openai-compatible', $2, $3, 1)
ON CONFLICT (id) DO NOTHING`,
		channelID,
		fmt.Sprintf("runtime-test-channel-%d", channelID),
		fmt.Sprintf("https://example.com/runtime-test-channel-%d", channelID),
	); err != nil {
		t.Fatalf("seed channel %d failed: %v", channelID, err)
	}
	if _, err := db.Exec(t.Context(), `
INSERT INTO api_accounts (id, name, provider_endpoint_id, max_concurrent, timeout, extra_config, status, weight)
VALUES ($1, $2, $3, 3, 120, '', 1, 1)
ON CONFLICT (id) DO NOTHING`,
		id,
		fmt.Sprintf("runtime-test-account-%d", id),
		channelID,
	); err != nil {
		t.Fatalf("seed account %d failed: %v", id, err)
	}
	return id
}

func seedTaskExecutionCredential(t testing.TB, db *pgxpool.Pool, accountID int64) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(t.Context(), `SELECT id FROM api_account_secrets WHERE account_id=$1 AND status='active' ORDER BY id DESC LIMIT 1`, accountID).Scan(&id); err == nil {
		return id
	} else if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("load credential for account %d failed: %v", accountID, err)
	}
	if err := db.QueryRow(t.Context(), `
INSERT INTO api_account_secrets (account_id, secret_material_kind, secret_payload, key_version, fingerprint, status)
VALUES ($1, 'encrypted', 'encrypted-secret', 'test-key', $2, 'active')
RETURNING id`,
		accountID,
		fmt.Sprintf("runtime-test-fingerprint-%d", accountID),
	).Scan(&id); err != nil {
		t.Fatalf("seed credential for account %d failed: %v", accountID, err)
	}
	return id
}

func seedTaskExecutionTask(t testing.TB, db *pgxpool.Pool, repo *taskstore.Repository, ctx context.Context, task *tasks.Task) error {
	t.Helper()
	if task.Params == "" {
		task.Params = taskExecutionTestParams
	}
	if task.Status == tasks.StatusRunning || task.AccountID != nil {
		accountID := tasks.IDValue(task.AccountID)
		if accountID <= 0 {
			t.Fatalf("running/assigned task %q requires account_id", task.TaskID)
		}
		if task.AssignedProviderEndpointID == nil {
			task.AssignedProviderEndpointID = testPtr(taskExecutionProviderEndpointID(accountID))
		}
		if task.AssignedCredentialID == nil {
			task.AssignedCredentialID = testPtr(seedTaskExecutionCredential(t, db, accountID))
		}
	}
	now := time.Now()
	switch task.Status {
	case tasks.StatusCompleted:
		if task.CompletedAt == nil {
			task.CompletedAt = &now
		}
	case tasks.StatusFailed:
		if task.CompletedAt == nil {
			task.CompletedAt = &now
		}
		if task.ErrorMsg == "" {
			task.ErrorMsg = "test failure"
		}
	case tasks.StatusCancelled:
		if task.CompletedAt == nil {
			task.CompletedAt = &now
		}
		if task.CancelledAt == nil {
			task.CancelledAt = &now
		}
		if task.CancelReason == "" {
			task.CancelReason = "test cancellation"
		}
	}
	if err := repo.Create(ctx, task); err != nil {
		return err
	}
	if task.Status == tasks.StatusRunning && task.LeaseToken != "" {
		seedTaskExecutionActiveLease(t, db, task)
	}
	return nil
}

func seedTaskExecutionActiveLease(t testing.TB, db *pgxpool.Pool, task *tasks.Task) {
	t.Helper()
	var exists bool
	if err := db.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM task_execution_attempts WHERE task_id=$1 AND status='running')`, task.TaskID).Scan(&exists); err != nil {
		t.Fatalf("check active execution attempt for %s failed: %v", task.TaskID, err)
	}
	if exists {
		return
	}
	model := strings.TrimSpace(task.ResolvedModel)
	if model == "" {
		model = "gpt-image-1"
	}
	if _, err := db.Exec(t.Context(), `
INSERT INTO task_execution_attempts (
  task_id, river_job_id, attempt_no, execution_token,
  feature_slug, requested_model
) VALUES ($1, 1, 1, $2, 'image-generate', $3)`,
		task.TaskID,
		task.LeaseToken,
		model,
	); err != nil {
		t.Fatalf("seed active execution attempt for %s failed: %v", task.TaskID, err)
	}
}

func seedTaskExecutionUser(t testing.TB, db *pgxpool.Pool, id int64) {
	t.Helper()
	if _, err := db.Exec(t.Context(), `
INSERT INTO users (id, display_name, storage_uid, role, status)
VALUES ($1, $2, $3, 'user', 1)
ON CONFLICT (id) DO NOTHING`,
		id,
		fmt.Sprintf("runtime-test-user-%d", id),
		fmt.Sprintf("RT%04d", id%10000),
	); err != nil {
		t.Fatalf("seed user %d failed: %v", id, err)
	}
}

func taskExecutionProviderEndpointID(accountID int64) int64 {
	return accountID + 20000
}

func TestRecordTaskCancelledPublishesCancelledStatus(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)
	accountID := seedTaskExecutionAccount(t, db, 53)

	task := &tasks.Task{
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		TaskID:                     "task-cancelled-event",
		UserID:                     1,
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-cancelled-event",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	cancelledAt := time.Now()
	if err := service.RecordTaskCancelled(ctx, tasks.TaskCancellationSnapshot{
		TaskID:      task.TaskID,
		CancelledAt: cancelledAt,
		Reason:      "user cancelled",
	}); err != nil {
		t.Fatalf("RecordTaskCancelled returned error: %v", err)
	}

	select {
	case event := <-publisher.ch:
		if event.TaskID != task.TaskID || event.Status != tasks.StatusCancelled {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, task.TaskID, tasks.StatusCancelled)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for cancelled event")
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusCancelled {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusCancelled)
	}
	if updated.CancelledAt == nil {
		t.Fatal("cancelled_at should be set")
	}
	if updated.CompletedAt == nil {
		t.Fatal("completed_at should be set for cancelled task")
	}
}

func TestConvergeTaskAfterRiverFinalFailureFailsPendingTask(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)

	task := &tasks.Task{
		TaskID:     "task-river-final-pending",
		UserID:     1,
		Status:     tasks.StatusPending,
		LeaseToken: "",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.ConvergeTaskAfterRiverFinalFailure(ctx, task.TaskID, 101, "river final error"); err != nil {
		t.Fatalf("ConvergeTaskAfterRiverFinalFailure returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusFailed {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusFailed)
	}
	if updated.CompletedAt == nil {
		t.Fatal("completed_at should be set")
	}
	select {
	case event := <-publisher.ch:
		if event.TaskID != task.TaskID || event.Status != tasks.StatusFailed {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, task.TaskID, tasks.StatusFailed)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for failed event")
	}
}

func TestConvergeTaskAfterRiverFinalFailureFailsRunningTaskWithSameRiverLease(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)
	accountID := seedTaskExecutionAccount(t, db, 61)

	task := &tasks.Task{
		TaskID:                     "task-river-final-running",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "river:202:lease",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.ConvergeTaskAfterRiverFinalFailure(ctx, task.TaskID, 202, "river final error"); err != nil {
		t.Fatalf("ConvergeTaskAfterRiverFinalFailure returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusFailed {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusFailed)
	}
	if updated.LeaseToken != "" {
		t.Fatalf("lease_token = %q, want empty", updated.LeaseToken)
	}
	select {
	case event := <-publisher.ch:
		if event.TaskID != task.TaskID || event.Status != tasks.StatusFailed {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, task.TaskID, tasks.StatusFailed)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for failed event")
	}
}

func TestConvergeTaskAfterRiverFinalFailureRecoversRunningTaskWithoutLease(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)
	accountID := seedTaskExecutionAccount(t, db, 61)

	task := &tasks.Task{
		TaskID:                     "task-startup-recovery-unleased",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.ConvergeTaskAfterRiverFinalFailure(ctx, task.TaskID, 0, "startup recovery"); err != nil {
		t.Fatalf("startup recovery convergence failed: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusFailed || updated.CompletedAt == nil {
		t.Fatalf("recovered task=%+v, want failed terminal fact", updated)
	}
	select {
	case event := <-publisher.ch:
		if event.TaskID != task.TaskID || event.Status != tasks.StatusFailed {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, task.TaskID, tasks.StatusFailed)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for failed event")
	}
}

func TestConvergeTaskAfterRiverFinalFailureRejectsRunningTaskWithDifferentRiverLease(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	accountID := seedTaskExecutionAccount(t, db, 62)

	task := &tasks.Task{
		TaskID:                     "task-river-final-stale",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "river:303:lease",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.ConvergeTaskAfterRiverFinalFailure(ctx, task.TaskID, 202, "river final error")
	if !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("expected ErrTaskStateConflict, got %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusRunning {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusRunning)
	}
	if updated.LeaseToken != task.LeaseToken {
		t.Fatalf("lease_token = %q, want %q", updated.LeaseToken, task.LeaseToken)
	}
}

func TestFailTaskRejectsEmptyTaskID(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)

	task := &tasks.Task{
		TaskID: "task-fail-partial-assignment",
		UserID: 1,
		Status: tasks.StatusPending,
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.FailTask(ctx, FailTaskInput{
		TaskID: "",
		Reason: "provider failed",
	})
	if !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("expected ErrTaskStateConflict, got %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
}

func TestCompleteTaskRejectsLeaseTokenMismatch(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	accountID := seedTaskExecutionAccount(t, db, 42)

	task := &tasks.Task{
		TaskID:                     "task-complete-token-mismatch",
		UserID:                     1,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "current-lease-token",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.CompleteTask(ctx, CompleteTaskInput{TaskID: task.TaskID, ExecutionToken: "stale-lease-token", Result: `{"status":"success"}`})
	if !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("expected ErrTaskStateConflict, got %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusRunning || updated.LeaseToken != task.LeaseToken {
		t.Fatalf("task changed on token mismatch: status=%s lease=%s", updated.Status, updated.LeaseToken)
	}
}

func TestCompleteTaskUpsertsDailyStat(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	accountID := seedTaskExecutionAccount(t, db, 201)

	task := &tasks.Task{
		TaskID:                     "task-completed-daily",
		UserID:                     7,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-completed-daily",
		CreatedAt:                  time.Now().Add(-90 * time.Second),
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"model":"gpt-image-1","username":"alice"}`
	if err := service.CompleteTask(ctx, CompleteTaskInput{TaskID: task.TaskID, ExecutionToken: task.LeaseToken, Result: result, ResolvedModel: "gpt-image-1"}); err != nil {
		t.Fatalf("CompleteTask returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, updated.CreatedAt, updated.CreatedAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("daily stat rows = %d, want 1", len(rows))
	}
	if rows[0].ModelName != "gpt-image-1" || rows[0].UserKey != "7" {
		t.Fatalf("daily stat model/user = %s/%s", rows[0].ModelName, rows[0].UserKey)
	}
	if rows[0].Total != 1 || rows[0].Completed != 1 || rows[0].Failed != 0 {
		t.Fatalf("daily stat total/completed/failed = %d/%d/%d", rows[0].Total, rows[0].Completed, rows[0].Failed)
	}
	if rows[0].LatencyCount != 1 {
		t.Fatalf("daily stat latency_count = %d, want 1", rows[0].LatencyCount)
	}
	if rows[0].LatencySecondsSum <= 0 {
		t.Fatalf("daily stat latency_seconds_sum = %f, want > 0", rows[0].LatencySecondsSum)
	}
}

func TestCompleteTaskPersistsTaskOutputs(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	accountID := seedTaskExecutionAccount(t, db, 202)
	seedTaskExecutionUser(t, db, 7)

	task := &tasks.Task{
		TaskID:                     "task-completed-outputs",
		UserID:                     7,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-completed-outputs",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"status":"success","image_url":"/uploads/tasks/7/task-completed-outputs/out-a.png","image_urls":["/uploads/tasks/7/task-completed-outputs/out-a.png","https://assets.example.com/uploads/tasks/7/task-completed-outputs/out-b.png"]}`
	taskOutputs := []tasks.TaskOutput{
		{
			URL:            "/uploads/tasks/7/task-completed-outputs/out-a.png",
			StorageKey:     "uploads/tasks/7/task-completed-outputs/out-a.png",
			MIMEType:       "image/png",
			StorageProfile: "builtin-local",
			ResultIndex:    0,
		},
		{
			URL:            "https://assets.example.com/uploads/tasks/7/task-completed-outputs/out-b.png",
			StorageKey:     "uploads/tasks/7/task-completed-outputs/out-b.png",
			MIMEType:       "image/png",
			StorageProfile: "builtin-local",
			ResultIndex:    2,
		},
	}
	if err := service.CompleteTask(ctx, CompleteTaskInput{TaskID: task.TaskID, ExecutionToken: task.LeaseToken,
		Result:  result,
		Outputs: taskOutputs,
	}); err != nil {
		t.Fatalf("CompleteTask returned error: %v", err)
	}

	outputsByTaskID, err := taskRepo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("ListOutputsByTaskIDs returned error: %v", err)
	}
	outputs := outputsByTaskID[task.TaskID]
	if len(outputs) != 2 {
		t.Fatalf("outputs len=%d, want 2", len(outputs))
	}
	if outputs[0].URL != "/uploads/tasks/7/task-completed-outputs/out-a.png" || outputs[0].StorageKey != "uploads/tasks/7/task-completed-outputs/out-a.png" {
		t.Fatalf("unexpected first output: %+v", outputs[0])
	}
	if outputs[1].URL != "https://assets.example.com/uploads/tasks/7/task-completed-outputs/out-b.png" || outputs[1].StorageKey != "uploads/tasks/7/task-completed-outputs/out-b.png" {
		t.Fatalf("unexpected second output: %+v", outputs[1])
	}
	if outputs[0].ResultIndex != 0 || outputs[1].ResultIndex != 2 {
		t.Fatalf("result indices = %d/%d, want 0/2", outputs[0].ResultIndex, outputs[1].ResultIndex)
	}
	for _, output := range outputs {
		if output.OutputKind != "asset" {
			t.Fatalf("output kind=%q, want asset", output.OutputKind)
		}
		if output.MediaAssetID == nil || *output.MediaAssetID == 0 {
			t.Fatalf("output missing media asset id: %+v", output)
		}
		if output.AssetID == "" {
			t.Fatalf("output missing public asset id: %+v", output)
		}
	}
	var persistedOutputURL string
	if err := db.QueryRow(ctx, `SELECT url FROM task_outputs WHERE task_id = $1 ORDER BY result_index ASC LIMIT 1`, task.TaskID).Scan(&persistedOutputURL); err != nil {
		t.Fatalf("read raw task output failed: %v", err)
	}
	if persistedOutputURL != "" {
		t.Fatalf("asset task_outputs.url=%q, want empty relation field", persistedOutputURL)
	}
	var duplicatedColumns int
	if err := db.QueryRow(ctx, `
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_name = 'task_outputs'
  AND column_name = ANY($1::text[])
`, ([]string{"storage_key", "mime_type", "width", "height", "size_bytes", "checksum_sha256", "storage_profile"})).Scan(&duplicatedColumns); err != nil {
		t.Fatalf("inspect task_outputs columns failed: %v", err)
	}
	if duplicatedColumns != 0 {
		t.Fatalf("task_outputs still has %d duplicated MediaAsset columns", duplicatedColumns)
	}
	var assetCount int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM media_assets WHERE owner_user_id = $1`, task.UserID).Scan(&assetCount); err != nil {
		t.Fatalf("count media assets failed: %v", err)
	}
	if assetCount != 2 {
		t.Fatalf("media asset count=%d, want 2", assetCount)
	}
	var refCount int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM media_refs WHERE task_id = $1 AND ref_type = 'task_output'`, task.TaskID).Scan(&refCount); err != nil {
		t.Fatalf("count media refs failed: %v", err)
	}
	if refCount != 2 {
		t.Fatalf("media ref count=%d, want 2", refCount)
	}
}

func TestCompleteTaskReusesDuplicateMediaAsset(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	accountID := seedTaskExecutionAccount(t, db, 205)
	seedTaskExecutionUser(t, db, 7)

	task := &tasks.Task{
		TaskID:                     "task-duplicate-output-asset",
		UserID:                     7,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-duplicate-output-asset",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	sharedKey := "uploads/tasks/7/task-duplicate-output-asset/shared.png"
	outputs := []tasks.TaskOutput{
		{URL: "/uploads/" + sharedKey, StorageKey: sharedKey, MIMEType: "image/png", StorageProfile: "builtin-local", ResultIndex: 0},
		{URL: "/uploads/" + sharedKey, StorageKey: sharedKey, MIMEType: "image/png", StorageProfile: "builtin-local", ResultIndex: 1},
	}
	if err := service.CompleteTask(ctx, CompleteTaskInput{TaskID: task.TaskID, ExecutionToken: task.LeaseToken,
		Result:  `{"status":"success"}`,
		Outputs: outputs,
	}); err != nil {
		t.Fatalf("CompleteTask returned error: %v", err)
	}

	outputsByTaskID, err := taskRepo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("ListOutputsByTaskIDs returned error: %v", err)
	}
	persisted := outputsByTaskID[task.TaskID]
	if len(persisted) != 2 {
		t.Fatalf("outputs len=%d, want 2", len(persisted))
	}
	if persisted[0].MediaAssetID == nil || persisted[1].MediaAssetID == nil || *persisted[0].MediaAssetID != *persisted[1].MediaAssetID {
		t.Fatalf("duplicate outputs did not reuse media asset: %+v", persisted)
	}
	var assetCount int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM media_assets WHERE owner_user_id = $1`, task.UserID).Scan(&assetCount); err != nil {
		t.Fatalf("count media assets failed: %v", err)
	}
	if assetCount != 1 {
		t.Fatalf("media asset count=%d, want 1", assetCount)
	}
}

func TestCompleteTaskPersistsRemoteOnlyOutput(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	accountID := seedTaskExecutionAccount(t, db, 206)
	seedTaskExecutionUser(t, db, 7)

	task := &tasks.Task{
		TaskID:                     "task-remote-only-output",
		UserID:                     7,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-remote-only-output",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.CompleteTask(ctx, CompleteTaskInput{TaskID: task.TaskID, ExecutionToken: task.LeaseToken,
		Result: `{"status":"success"}`,
		Outputs: []tasks.TaskOutput{{
			OutputKind: "remote",
			Role:       "image",
			URL:        "https://provider.example/result.png",
			SourceURL:  "https://provider.example/source.png",
		}},
	}); err != nil {
		t.Fatalf("CompleteTask returned error: %v", err)
	}

	outputsByTaskID, err := taskRepo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("ListOutputsByTaskIDs returned error: %v", err)
	}
	outputs := outputsByTaskID[task.TaskID]
	if len(outputs) != 1 {
		t.Fatalf("outputs len=%d, want 1", len(outputs))
	}
	if outputs[0].OutputKind != "remote" || outputs[0].URL != "https://provider.example/result.png" || outputs[0].MediaAssetID != nil {
		t.Fatalf("unexpected remote output: %+v", outputs[0])
	}
	if outputs[0].StorageKey != "" || outputs[0].MIMEType != "" || outputs[0].SizeBytes != 0 {
		t.Fatalf("remote output has asset metadata: %+v", outputs[0])
	}
	var assetCount int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM media_assets WHERE owner_user_id = $1`, task.UserID).Scan(&assetCount); err != nil {
		t.Fatalf("count media assets failed: %v", err)
	}
	if assetCount != 0 {
		t.Fatalf("media asset count=%d, want 0", assetCount)
	}
}

func TestCompleteTaskDoesNotInferOutputsFromResultURL(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	accountID := seedTaskExecutionAccount(t, db, 204)

	task := &tasks.Task{
		TaskID:                     "task-foreign-origin",
		UserID:                     7,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-foreign-origin",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.CompleteTask(ctx, CompleteTaskInput{TaskID: task.TaskID, ExecutionToken: task.LeaseToken, Result: `{"status":"success","image_url":"https://evil.example.com/uploads/tasks/7/task-foreign-origin/out-a.png"}`}); err != nil {
		t.Fatalf("CompleteTask returned error: %v", err)
	}
	outputsByTaskID, err := taskRepo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("ListOutputsByTaskIDs returned error: %v", err)
	}
	if len(outputsByTaskID[task.TaskID]) != 0 {
		t.Fatalf("outputs len=%d, want 0", len(outputsByTaskID[task.TaskID]))
	}
}

func TestCompleteTaskRejectsOutputWithoutStorageKey(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(taskRepo, nil)
	accountID := seedTaskExecutionAccount(t, db, 203)

	task := &tasks.Task{
		TaskID:                     "task-inline-result",
		UserID:                     7,
		AssignedProviderEndpointID: testPtr(taskExecutionProviderEndpointID(accountID)),
		AccountID:                  testPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-inline-result",
	}
	if err := seedTaskExecutionTask(t, db, taskRepo, ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.CompleteTask(ctx, CompleteTaskInput{TaskID: task.TaskID, ExecutionToken: task.LeaseToken,
		Result: `{"status":"success","image_url":"/uploads/tasks/7/task-inline-result/out-a.png"}`,
		Outputs: []tasks.TaskOutput{
			{URL: "/uploads/tasks/7/task-inline-result/out-a.png"},
		},
	})
	if err == nil {
		t.Fatal("expected missing storage_key to be rejected")
	}
	updated, getErr := taskRepo.GetByTaskID(ctx, task.TaskID)
	if getErr != nil {
		t.Fatalf("reload task failed: %v", getErr)
	}
	if updated.Status != tasks.StatusRunning {
		t.Fatalf("status=%s, want running after rejected completion", updated.Status)
	}
}
