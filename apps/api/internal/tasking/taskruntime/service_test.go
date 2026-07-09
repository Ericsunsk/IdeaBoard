package taskruntime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/testdb"

	"github.com/lib/pq"
)

type recordingTaskStatusPublisher struct {
	ch chan events.TaskStatusEvent
}

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
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 77)

	task := &tasks.Task{
		TaskID:                     "task-reset-pending",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-reset",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.ResetRunningTask(ctx, ResetRunningTaskInput{Assignment: taskRuntimeAssignment(task), Reason: "worker context interrupted"}); err != nil {
		t.Fatalf("ResetRunningTask returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
	if updated.AccountID != nil {
		t.Fatalf("account_id = %d, want nil", tasks.IDValue(updated.AccountID))
	}
	if updated.ErrorMsg != "worker context interrupted" {
		t.Fatalf("error_msg = %q, want %q", updated.ErrorMsg, "worker context interrupted")
	}
	if updated.CompletedAt != nil {
		t.Fatalf("completed_at should remain nil, got %v", updated.CompletedAt)
	}
}

func TestResetRunningTaskRejectsTerminalTask(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 77)

	task := &tasks.Task{
		TaskID:                     "task-reset-terminal",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusCompleted,
		CompletedAt:                ptrTime(time.Now()),
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.ResetRunningTask(ctx, ResetRunningTaskInput{Assignment: taskRuntimeAssignmentWithLease(task, "lease-terminal"), Reason: "worker context interrupted"})
	if !errors.Is(err, ErrTaskTerminal) {
		t.Fatalf("expected ErrTaskTerminal, got %v", err)
	}
}

func TestFailTaskPersistsProvidedResultEnvelope(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID: "task-fail-custom-result",
		UserID: 1,
		Status: tasks.StatusPending,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
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

func TestRequeueTaskForFailoverPersistsProvidedResultEnvelope(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 101)

	task := &tasks.Task{
		TaskID:                     "task-requeue-custom-result",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-requeue-custom-result",
		RetryCount:                 0,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"task_id":"task-requeue-custom-result","status":"failed","parsed_output_status":"invalid","error":"contract failed"}`
	requeued, err := service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{
		Assignment: taskRuntimeAssignment(task),
		Reason:     "contract failed",
		Result:     result,
	})
	if err != nil {
		t.Fatalf("RequeueTaskForFailover returned error: %v", err)
	}
	if !requeued {
		t.Fatalf("expected task to be requeued")
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
	if updated.Result != result {
		t.Fatalf("result = %q, want provided envelope", updated.Result)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func newTaskRuntimeServiceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func seedTaskRuntimeAccount(t testing.TB, db *sql.DB, id int64) int64 {
	t.Helper()
	channelID := taskRuntimeProviderEndpointID(id)
	if _, err := db.Exec(`
INSERT INTO provider_endpoints (id, api_type, name, base_url, status)
VALUES ($1, 'openai-compatible', $2, $3, 1)
ON CONFLICT (id) DO NOTHING`,
		channelID,
		fmt.Sprintf("runtime-test-channel-%d", channelID),
		fmt.Sprintf("https://example.com/runtime-test-channel-%d", channelID),
	); err != nil {
		t.Fatalf("seed channel %d failed: %v", channelID, err)
	}
	if _, err := db.Exec(`
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

func seedTaskRuntimeCredential(t testing.TB, db *sql.DB, accountID int64) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`
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

func seedTaskRuntimeActiveLease(t testing.TB, db *sql.DB, task *tasks.Task, credentialID int64) {
	t.Helper()
	if _, err := db.Exec(`
INSERT INTO task_leases (
  task_id, lease_token, river_job_id, account_id, provider_endpoint_id, credential_id,
  resolved_model, acquired_at
) VALUES ($1, $2, 1, $3, $4, $5, $6, now())`,
		task.TaskID,
		task.LeaseToken,
		tasks.IDValue(task.AccountID),
		tasks.IDValue(task.AssignedProviderEndpointID),
		credentialID,
		task.ResolvedModel,
	); err != nil {
		t.Fatalf("seed active task lease for %s failed: %v", task.TaskID, err)
	}
}

func seedTaskRuntimeUser(t testing.TB, db *sql.DB, id int64) {
	t.Helper()
	if _, err := db.Exec(`
INSERT INTO users (id, username, storage_uid, role, status)
VALUES ($1, $2, $3, 'user', 1)
ON CONFLICT (id) DO NOTHING`,
		id,
		fmt.Sprintf("runtime-test-user-%d", id),
		fmt.Sprintf("RT%04d", id%10000),
	); err != nil {
		t.Fatalf("seed user %d failed: %v", id, err)
	}
}

func taskRuntimeProviderEndpointID(accountID int64) int64 {
	return accountID + 20000
}

func taskRuntimeAssignment(task *tasks.Task) taskqueue.TaskAssignment {
	return taskRuntimeAssignmentWithLease(task, task.LeaseToken)
}

func taskRuntimeAssignmentWithLease(task *tasks.Task, leaseToken string) taskqueue.TaskAssignment {
	return taskqueue.TaskAssignment{
		TaskID:             task.TaskID,
		AccountID:          tasks.IDValue(task.AccountID),
		ProviderEndpointID: tasks.IDValue(task.AssignedProviderEndpointID),
		CredentialID:       tasks.IDValue(task.AssignedCredentialID),
		Status:             taskqueue.AssignStatusAssigned,
		LeaseToken:         leaseToken,
	}
}

func TestRequeueTaskForFailoverRequeuesWhenBelowRetryLimit(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 99)

	task := &tasks.Task{
		TaskID:                     "task-requeue",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-requeue",
		RetryCount:                 defaultTaskRetryLimit - 2,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	requeued, err := service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{Assignment: taskRuntimeAssignment(task), Reason: "retryable error"})
	if err != nil {
		t.Fatalf("RequeueTaskForFailover returned error: %v", err)
	}
	if !requeued {
		t.Fatalf("expected task to be requeued")
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
	if updated.AccountID != nil {
		t.Fatalf("account_id = %d, want nil", tasks.IDValue(updated.AccountID))
	}
	if updated.RetryCount != defaultTaskRetryLimit-1 {
		t.Fatalf("retry_count = %d, want %d", updated.RetryCount, defaultTaskRetryLimit-1)
	}
	if updated.CompletedAt != nil {
		t.Fatalf("completed_at should be nil when requeued")
	}
}

func TestConfirmTaskLeasePublishesRunningStatus(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	assigner := taskqueue.NewPostgresTaskAssigner(db)
	service := NewService(nil, taskRepo, assigner, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)
	accountID := seedTaskRuntimeAccount(t, db, 42)
	credentialID := seedTaskRuntimeCredential(t, db, accountID)

	task := &tasks.Task{
		TaskID:                     "task-publish",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-publish",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	seedTaskRuntimeActiveLease(t, db, task, credentialID)

	if err := service.ConfirmTaskLease(ctx, taskRuntimeAssignment(task)); err != nil {
		t.Fatalf("ConfirmTaskLease returned error: %v", err)
	}

	select {
	case event := <-publisher.ch:
		if event.TaskID != task.TaskID || event.Status != tasks.StatusRunning {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, task.TaskID, tasks.StatusRunning)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for running event")
	}
}

func TestRecordTaskCancelledPublishesCancelledStatus(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)
	accountID := seedTaskRuntimeAccount(t, db, 53)

	task := &tasks.Task{
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		TaskID:                     "task-cancelled-event",
		UserID:                     1,
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-cancelled-event",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
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
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)

	task := &tasks.Task{
		TaskID:     "task-river-final-pending",
		UserID:     1,
		Status:     tasks.StatusPending,
		LeaseToken: "",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
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
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)
	accountID := seedTaskRuntimeAccount(t, db, 61)

	task := &tasks.Task{
		TaskID:                     "task-river-final-running",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "river:202:lease",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
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

func TestConvergeTaskAfterRiverFinalFailureRejectsRunningTaskWithDifferentRiverLease(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 62)

	task := &tasks.Task{
		TaskID:                     "task-river-final-stale",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "river:303:lease",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
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

func TestFailTaskRejectsPartialAssignment(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID: "task-fail-partial-assignment",
		UserID: 1,
		Status: tasks.StatusPending,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.FailTask(ctx, FailTaskInput{
		Assignment: taskqueue.TaskAssignment{TaskID: task.TaskID},
		Reason:     "provider failed",
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

func TestConfirmTaskLeaseRejectsStaleLeaseWithoutPublishing(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	assigner := taskqueue.NewPostgresTaskAssigner(db)
	service := NewService(nil, taskRepo, assigner, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)
	accountID := seedTaskRuntimeAccount(t, db, 42)
	credentialID := seedTaskRuntimeCredential(t, db, accountID)

	task := &tasks.Task{
		TaskID:                     "task-stale-lease",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AssignedCredentialID:       tasks.IDPtr(credentialID),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-current",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	seedTaskRuntimeActiveLease(t, db, task, credentialID)

	err := service.ConfirmTaskLease(ctx, taskRuntimeAssignmentWithLease(task, "lease-stale"))
	if !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("expected ErrTaskStateConflict, got %v", err)
	}

	select {
	case event := <-publisher.ch:
		t.Fatalf("unexpected published event: %+v", event)
	default:
	}
}

func TestCompleteTaskRejectsLeaseTokenMismatch(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 42)

	task := &tasks.Task{
		TaskID:                     "task-complete-token-mismatch",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "current-lease-token",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.CompleteTask(ctx, CompleteTaskInput{Assignment: taskRuntimeAssignmentWithLease(task, "stale-lease-token"), Result: `{"status":"success"}`})
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

func TestRequeueTaskForFailoverFailsWhenRetryLimitReached(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 100)

	task := &tasks.Task{
		TaskID:                     "task-retry-limit",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-retry-limit",
		RetryCount:                 defaultTaskRetryLimit - 1,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"task_id":"task-retry-limit","status":"failed","parsed_output_status":"invalid","error":"retry limit reached"}`
	requeued, err := service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{Assignment: taskRuntimeAssignment(task), Reason: "retry limit reached", Result: result})
	if err != nil {
		t.Fatalf("RequeueTaskForFailover returned error: %v", err)
	}
	if requeued {
		t.Fatalf("expected task to stop requeue at retry limit")
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusFailed {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusFailed)
	}
	if tasks.IDValue(updated.AccountID) != accountID {
		t.Fatalf("account_id = %d, want %d", tasks.IDValue(updated.AccountID), accountID)
	}
	if tasks.IDValue(updated.AssignedProviderEndpointID) != taskRuntimeProviderEndpointID(accountID) {
		t.Fatalf("assigned_provider_endpoint_id = %d, want %d", tasks.IDValue(updated.AssignedProviderEndpointID), taskRuntimeProviderEndpointID(accountID))
	}
	if updated.RetryCount != defaultTaskRetryLimit {
		t.Fatalf("retry_count = %d, want %d", updated.RetryCount, defaultTaskRetryLimit)
	}
	if updated.CompletedAt == nil {
		t.Fatalf("completed_at should be set when retry limit reached")
	}
	if updated.Result != result {
		t.Fatalf("result = %q, want provided envelope", updated.Result)
	}

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, updated.CreatedAt, updated.CreatedAt)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("daily stat rows = %d, want 1", len(rows))
	}
	if rows[0].Failed != 1 || rows[0].Total != 1 {
		t.Fatalf("daily stat failed=%d total=%d, want 1/1", rows[0].Failed, rows[0].Total)
	}
}

func TestRequeueTaskForFailoverPublishesPendingOrFailed(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)
	account13 := seedTaskRuntimeAccount(t, db, 13)
	account14 := seedTaskRuntimeAccount(t, db, 14)

	requeueTask := &tasks.Task{
		TaskID:                     "task-requeue-publish-pending",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(account13)),
		AccountID:                  tasks.IDPtr(account13),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-publish-pending",
		RetryCount:                 0,
	}
	if err := taskRepo.Create(ctx, requeueTask); err != nil {
		t.Fatalf("seed requeue task failed: %v", err)
	}
	requeued, err := service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{Assignment: taskRuntimeAssignment(requeueTask), Reason: "retry"})
	if err != nil {
		t.Fatalf("RequeueTaskForFailover pending returned error: %v", err)
	}
	if !requeued {
		t.Fatalf("expected requeued=true")
	}
	select {
	case event := <-publisher.ch:
		if event.Status != tasks.StatusPending {
			t.Fatalf("requeue event status=%s, want %s", event.Status, tasks.StatusPending)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting requeue pending event")
	}

	finalTask := &tasks.Task{
		TaskID:                     "task-requeue-publish-failed",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(account14)),
		AccountID:                  tasks.IDPtr(account14),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-publish-failed",
		RetryCount:                 defaultTaskRetryLimit - 1,
	}
	if err := taskRepo.Create(ctx, finalTask); err != nil {
		t.Fatalf("seed final requeue task failed: %v", err)
	}
	requeued, err = service.RequeueTaskForFailover(ctx, RequeueForFailoverInput{Assignment: taskRuntimeAssignment(finalTask), Reason: "stop retry"})
	if err != nil {
		t.Fatalf("RequeueTaskForFailover failed returned error: %v", err)
	}
	if requeued {
		t.Fatalf("expected requeued=false when final fail")
	}
	select {
	case event := <-publisher.ch:
		if event.Status != tasks.StatusFailed {
			t.Fatalf("final fail event status=%s, want %s", event.Status, tasks.StatusFailed)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting final failed event")
	}
}

func TestCompleteTaskUpsertsDailyStat(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 201)

	task := &tasks.Task{
		TaskID:                     "task-completed-daily",
		UserID:                     7,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-completed-daily",
		CreatedAt:                  time.Now().Add(-90 * time.Second),
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"model":"gpt-image-1","username":"alice"}`
	if err := service.CompleteTask(ctx, CompleteTaskInput{Assignment: taskRuntimeAssignment(task), Result: result}); err != nil {
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
	if rows[0].ModelName != "gpt-image-1" || rows[0].UserKey != "alice" {
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
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 202)
	seedTaskRuntimeUser(t, db, 7)

	task := &tasks.Task{
		TaskID:                     "task-completed-outputs",
		UserID:                     7,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-completed-outputs",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"status":"success","image_url":"/uploads/tasks/7/task-completed-outputs/out-a.png","image_urls":["/uploads/tasks/7/task-completed-outputs/out-a.png","https://assets.example.com/uploads/tasks/7/task-completed-outputs/out-b.png"]}`
	taskOutputs := []tasks.TaskOutput{
		{
			URL:            "/uploads/tasks/7/task-completed-outputs/out-a.png",
			StorageKey:     "uploads/tasks/7/task-completed-outputs/out-a.png",
			MIMEType:       "image/png",
			StorageProfile: "builtin-local",
		},
		{
			URL:            "https://assets.example.com/uploads/tasks/7/task-completed-outputs/out-b.png",
			StorageKey:     "uploads/tasks/7/task-completed-outputs/out-b.png",
			MIMEType:       "image/png",
			StorageProfile: "builtin-local",
		},
	}
	if err := service.CompleteTask(ctx, CompleteTaskInput{
		Assignment: taskRuntimeAssignment(task),
		Result:     result,
		Outputs:    taskOutputs,
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
	for _, output := range outputs {
		if output.OutputKind != "asset" {
			t.Fatalf("output kind=%q, want asset", output.OutputKind)
		}
		if output.MediaAssetID == nil || *output.MediaAssetID == 0 {
			t.Fatalf("output missing media asset id: %+v", output)
		}
	}
	var persistedOutputURL string
	if err := db.QueryRowContext(ctx, `SELECT url FROM task_outputs WHERE task_id = $1 ORDER BY sort_index ASC LIMIT 1`, task.TaskID).Scan(&persistedOutputURL); err != nil {
		t.Fatalf("read raw task output failed: %v", err)
	}
	if persistedOutputURL != "" {
		t.Fatalf("asset task_outputs.url=%q, want empty relation field", persistedOutputURL)
	}
	var duplicatedColumns int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_name = 'task_outputs'
  AND column_name = ANY($1::text[])
`, pq.Array([]string{"storage_key", "mime_type", "width", "height", "size_bytes", "checksum_sha256", "storage_profile"})).Scan(&duplicatedColumns); err != nil {
		t.Fatalf("inspect task_outputs columns failed: %v", err)
	}
	if duplicatedColumns != 0 {
		t.Fatalf("task_outputs still has %d duplicated MediaAsset columns", duplicatedColumns)
	}
	var assetCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_assets WHERE owner_user_id = $1`, task.UserID).Scan(&assetCount); err != nil {
		t.Fatalf("count media assets failed: %v", err)
	}
	if assetCount != 2 {
		t.Fatalf("media asset count=%d, want 2", assetCount)
	}
	var refCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_refs WHERE task_id = $1 AND ref_type = 'task_output'`, task.TaskID).Scan(&refCount); err != nil {
		t.Fatalf("count media refs failed: %v", err)
	}
	if refCount != 2 {
		t.Fatalf("media ref count=%d, want 2", refCount)
	}
}

func TestCompleteTaskReusesDuplicateMediaAsset(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 205)
	seedTaskRuntimeUser(t, db, 7)

	task := &tasks.Task{
		TaskID:                     "task-duplicate-output-asset",
		UserID:                     7,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-duplicate-output-asset",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	sharedKey := "uploads/tasks/7/task-duplicate-output-asset/shared.png"
	outputs := []tasks.TaskOutput{
		{URL: "/uploads/" + sharedKey, StorageKey: sharedKey, MIMEType: "image/png", StorageProfile: "builtin-local"},
		{URL: "/uploads/" + sharedKey, StorageKey: sharedKey, MIMEType: "image/png", StorageProfile: "builtin-local"},
	}
	if err := service.CompleteTask(ctx, CompleteTaskInput{
		Assignment: taskRuntimeAssignment(task),
		Result:     `{"status":"success"}`,
		Outputs:    outputs,
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
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_assets WHERE owner_user_id = $1`, task.UserID).Scan(&assetCount); err != nil {
		t.Fatalf("count media assets failed: %v", err)
	}
	if assetCount != 1 {
		t.Fatalf("media asset count=%d, want 1", assetCount)
	}
}

func TestCompleteTaskPersistsRemoteOnlyOutput(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 206)
	seedTaskRuntimeUser(t, db, 7)

	task := &tasks.Task{
		TaskID:                     "task-remote-only-output",
		UserID:                     7,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-remote-only-output",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.CompleteTask(ctx, CompleteTaskInput{
		Assignment: taskRuntimeAssignment(task),
		Result:     `{"status":"success"}`,
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
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_assets WHERE owner_user_id = $1`, task.UserID).Scan(&assetCount); err != nil {
		t.Fatalf("count media assets failed: %v", err)
	}
	if assetCount != 0 {
		t.Fatalf("media asset count=%d, want 0", assetCount)
	}
}

func TestCompleteTaskDoesNotInferOutputsFromResultURL(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 204)

	task := &tasks.Task{
		TaskID:                     "task-foreign-origin",
		UserID:                     7,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-foreign-origin",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.CompleteTask(ctx, CompleteTaskInput{Assignment: taskRuntimeAssignment(task), Result: `{"status":"success","image_url":"https://evil.example.com/uploads/tasks/7/task-foreign-origin/out-a.png"}`}); err != nil {
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
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 203)

	task := &tasks.Task{
		TaskID:                     "task-inline-result",
		UserID:                     7,
		AssignedProviderEndpointID: tasks.IDPtr(taskRuntimeProviderEndpointID(accountID)),
		AccountID:                  tasks.IDPtr(accountID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-inline-result",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.CompleteTask(ctx, CompleteTaskInput{
		Assignment: taskRuntimeAssignment(task),
		Result:     `{"status":"success","image_url":"/uploads/tasks/7/task-inline-result/out-a.png"}`,
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
