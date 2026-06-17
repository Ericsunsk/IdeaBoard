package taskruntime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"ideaboard/api/internal/events"
	"ideaboard/api/internal/runtime/taskqueue"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/taskstore"
	"ideaboard/api/internal/testdb"
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

func TestResetTaskRunningToPendingResetsState(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 77)

	task := &tasks.Task{
		TaskID:            "task-reset-pending",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-reset",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.ResetTaskRunningToPending(ctx, taskRuntimeAssignment(task), "worker context interrupted"); err != nil {
		t.Fatalf("ResetTaskRunningToPending returned error: %v", err)
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

func TestResetTaskRunningToPendingRejectsTerminalTask(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 77)

	task := &tasks.Task{
		TaskID:            "task-reset-terminal",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusCompleted,
		CompletedAt:       ptrTime(time.Now()),
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.ResetTaskRunningToPending(ctx, taskRuntimeAssignmentWithLease(task, "lease-terminal"), "worker context interrupted")
	if !errors.Is(err, ErrTaskTerminal) {
		t.Fatalf("expected ErrTaskTerminal, got %v", err)
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
	channelID := taskRuntimeChannelID(id)
	if _, err := db.Exec(`
INSERT INTO channels (id, type, name, api_url, status)
VALUES ($1, 'openai-compatible', $2, $3, 1)
ON CONFLICT (id) DO NOTHING`,
		channelID,
		fmt.Sprintf("runtime-test-channel-%d", channelID),
		fmt.Sprintf("https://example.com/runtime-test-channel-%d", channelID),
	); err != nil {
		t.Fatalf("seed channel %d failed: %v", channelID, err)
	}
	if _, err := db.Exec(`
INSERT INTO api_accounts (id, name, channel_id, platform, api_type, proxy_url, max_concurrent, timeout, extra_config, status, weight)
VALUES ($1, $2, $3, 'openai-compatible', 'openai-compatible', 'https://example.com', 3, 120, '', 1, 1)
ON CONFLICT (id) DO NOTHING`,
		id,
		fmt.Sprintf("runtime-test-account-%d", id),
		channelID,
	); err != nil {
		t.Fatalf("seed account %d failed: %v", id, err)
	}
	return id
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

func taskRuntimeChannelID(accountID int64) int64 {
	return accountID + 20000
}

func taskRuntimeAssignment(task *tasks.Task) taskqueue.TaskAssignment {
	return taskRuntimeAssignmentWithLease(task, task.LeaseToken)
}

func taskRuntimeAssignmentWithLease(task *tasks.Task, leaseToken string) taskqueue.TaskAssignment {
	return taskqueue.TaskAssignment{
		TaskID:     task.TaskID,
		AccountID:  tasks.IDValue(task.AccountID),
		ChannelID:  tasks.IDValue(task.AssignedChannelID),
		Status:     taskqueue.AssignStatusAssigned,
		LeaseToken: leaseToken,
	}
}

func TestRequeueTaskForFailoverRequeuesWhenBelowRetryLimit(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 99)

	task := &tasks.Task{
		TaskID:            "task-requeue",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-requeue",
		RetryCount:        defaultTaskRetryLimit - 2,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	requeued, err := service.RequeueTaskForFailover(ctx, taskRuntimeAssignment(task), "retryable error")
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

	task := &tasks.Task{
		TaskID:            "task-publish",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-publish",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.ConfirmTaskLease(ctx, taskqueue.TaskAssignment{
		TaskID:     task.TaskID,
		AccountID:  tasks.IDValue(task.AccountID),
		ChannelID:  taskRuntimeChannelID(accountID),
		Status:     taskqueue.AssignStatusAssigned,
		LeaseToken: task.LeaseToken,
	}); err != nil {
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
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		TaskID:            "task-cancelled-event",
		UserID:            1,
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-cancelled-event",
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
		TaskID:            "task-river-final-running",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "river:202:lease",
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
		TaskID:            "task-river-final-stale",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "river:303:lease",
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

func TestConfirmTaskLeaseRejectsStaleLeaseWithoutPublishing(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	assigner := taskqueue.NewPostgresTaskAssigner(db)
	service := NewService(nil, taskRepo, assigner, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)
	accountID := seedTaskRuntimeAccount(t, db, 42)

	task := &tasks.Task{
		TaskID:            "task-stale-lease",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-current",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.ConfirmTaskLease(ctx, taskqueue.TaskAssignment{
		TaskID:     task.TaskID,
		AccountID:  tasks.IDValue(task.AccountID),
		ChannelID:  taskRuntimeChannelID(accountID),
		Status:     taskqueue.AssignStatusAssigned,
		LeaseToken: "lease-stale",
	})
	if err == nil {
		t.Fatal("expected stale lease confirmation to fail")
	}

	select {
	case event := <-publisher.ch:
		t.Fatalf("unexpected published event: %+v", event)
	default:
	}
}

func TestMarkTaskCompletedRejectsLeaseTokenMismatch(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 42)

	task := &tasks.Task{
		TaskID:            "task-complete-token-mismatch",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "current-lease-token",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.MarkTaskCompleted(ctx, taskRuntimeAssignmentWithLease(task, "stale-lease-token"), `{"status":"success"}`)
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
		TaskID:            "task-retry-limit",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-retry-limit",
		RetryCount:        defaultTaskRetryLimit - 1,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	requeued, err := service.RequeueTaskForFailover(ctx, taskRuntimeAssignment(task), "retry limit reached")
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
	if tasks.IDValue(updated.AssignedChannelID) != taskRuntimeChannelID(accountID) {
		t.Fatalf("assigned_channel_id = %d, want %d", tasks.IDValue(updated.AssignedChannelID), taskRuntimeChannelID(accountID))
	}
	if updated.RetryCount != defaultTaskRetryLimit {
		t.Fatalf("retry_count = %d, want %d", updated.RetryCount, defaultTaskRetryLimit)
	}
	if updated.CompletedAt == nil {
		t.Fatalf("completed_at should be set when retry limit reached")
	}
	if updated.Result == "" {
		t.Fatalf("result should be populated for failed task")
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
		TaskID:            "task-requeue-publish-pending",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(account13)),
		AccountID:         tasks.IDPtr(account13),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-publish-pending",
		RetryCount:        0,
	}
	if err := taskRepo.Create(ctx, requeueTask); err != nil {
		t.Fatalf("seed requeue task failed: %v", err)
	}
	requeued, err := service.RequeueTaskForFailover(ctx, taskRuntimeAssignment(requeueTask), "retry")
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
		TaskID:            "task-requeue-publish-failed",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(account14)),
		AccountID:         tasks.IDPtr(account14),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-publish-failed",
		RetryCount:        defaultTaskRetryLimit - 1,
	}
	if err := taskRepo.Create(ctx, finalTask); err != nil {
		t.Fatalf("seed final requeue task failed: %v", err)
	}
	requeued, err = service.RequeueTaskForFailover(ctx, taskRuntimeAssignment(finalTask), "stop retry")
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

func TestMarkTaskCompletedUpsertsDailyStat(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 201)

	task := &tasks.Task{
		TaskID:            "task-completed-daily",
		UserID:            7,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-completed-daily",
		CreatedAt:         time.Now().Add(-90 * time.Second),
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"model":"gpt-image-1","username":"alice"}`
	if err := service.MarkTaskCompleted(ctx, taskRuntimeAssignment(task), result); err != nil {
		t.Fatalf("MarkTaskCompleted returned error: %v", err)
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

func TestMarkTaskCompletedPersistsTaskOutputs(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 202)
	seedTaskRuntimeUser(t, db, 7)

	task := &tasks.Task{
		TaskID:            "task-completed-outputs",
		UserID:            7,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-completed-outputs",
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
	if err := service.MarkTaskCompletedWithOutputs(ctx, taskRuntimeAssignment(task), result, taskOutputs); err != nil {
		t.Fatalf("MarkTaskCompletedWithOutputs returned error: %v", err)
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
		if output.MediaAssetID == nil || *output.MediaAssetID == 0 {
			t.Fatalf("output missing media asset id: %+v", output)
		}
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

func TestMarkTaskCompletedDoesNotInferOutputsFromResultURL(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 204)

	task := &tasks.Task{
		TaskID:            "task-foreign-origin",
		UserID:            7,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-foreign-origin",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.MarkTaskCompleted(ctx, taskRuntimeAssignment(task), `{"status":"success","image_url":"https://evil.example.com/uploads/tasks/7/task-foreign-origin/out-a.png"}`); err != nil {
		t.Fatalf("MarkTaskCompleted returned error: %v", err)
	}
	outputsByTaskID, err := taskRepo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("ListOutputsByTaskIDs returned error: %v", err)
	}
	if len(outputsByTaskID[task.TaskID]) != 0 {
		t.Fatalf("outputs len=%d, want 0", len(outputsByTaskID[task.TaskID]))
	}
}

func TestMarkTaskCompletedRejectsOutputWithoutStorageKey(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)
	accountID := seedTaskRuntimeAccount(t, db, 203)

	task := &tasks.Task{
		TaskID:            "task-inline-result",
		UserID:            7,
		AssignedChannelID: tasks.IDPtr(taskRuntimeChannelID(accountID)),
		AccountID:         tasks.IDPtr(accountID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-inline-result",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.MarkTaskCompletedWithOutputs(ctx, taskRuntimeAssignment(task), `{"status":"success","image_url":"/uploads/tasks/7/task-inline-result/out-a.png"}`, []tasks.TaskOutput{
		{URL: "/uploads/tasks/7/task-inline-result/out-a.png"},
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
