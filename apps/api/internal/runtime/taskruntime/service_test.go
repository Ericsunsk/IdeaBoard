package taskruntime

import (
	"context"
	"database/sql"
	"errors"
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

	task := &tasks.Task{
		TaskID:     "task-reset-pending",
		UserID:     1,
		ChannelID:  1,
		AccountID:  77,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-reset",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.ResetTaskRunningToPending(ctx, task.TaskID, task.AccountID, task.LeaseToken, "worker context interrupted"); err != nil {
		t.Fatalf("ResetTaskRunningToPending returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
	if updated.AccountID != 0 {
		t.Fatalf("account_id = %d, want 0", updated.AccountID)
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

	task := &tasks.Task{
		TaskID:      "task-reset-terminal",
		UserID:      1,
		ChannelID:   1,
		AccountID:   77,
		Status:      tasks.StatusCompleted,
		LeaseToken:  "lease-terminal",
		CompletedAt: ptrTime(time.Now()),
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.ResetTaskRunningToPending(ctx, task.TaskID, task.AccountID, task.LeaseToken, "worker context interrupted")
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

func TestRequeueTaskForFailoverRequeuesWhenBelowRetryLimit(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:     "task-requeue",
		UserID:     1,
		ChannelID:  1,
		AccountID:  99,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-requeue",
		RetryCount: defaultTaskRetryLimit - 2,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	requeued, err := service.RequeueTaskForFailover(ctx, task.TaskID, task.AccountID, task.LeaseToken, "retryable error")
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
	if updated.AccountID != 0 {
		t.Fatalf("account_id = %d, want 0", updated.AccountID)
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

	task := &tasks.Task{
		TaskID:     "task-publish",
		UserID:     1,
		ChannelID:  1,
		AccountID:  42,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-publish",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.ConfirmTaskLease(ctx, taskqueue.TaskAssignment{
		TaskID:     task.TaskID,
		AccountID:  task.AccountID,
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

func TestConfirmTaskLeaseRejectsStaleLeaseWithoutPublishing(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	assigner := taskqueue.NewPostgresTaskAssigner(db)
	service := NewService(nil, taskRepo, assigner, nil)
	publisher := newRecordingTaskStatusPublisher()
	service.SetTaskStatusPublisher(publisher)

	task := &tasks.Task{
		TaskID:     "task-stale-lease",
		UserID:     1,
		ChannelID:  1,
		AccountID:  42,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-current",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.ConfirmTaskLease(ctx, taskqueue.TaskAssignment{
		TaskID:     task.TaskID,
		AccountID:  task.AccountID,
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

	task := &tasks.Task{
		TaskID:     "task-complete-token-mismatch",
		UserID:     1,
		ChannelID:  1,
		AccountID:  42,
		Status:     tasks.StatusRunning,
		LeaseToken: "current-lease-token",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.MarkTaskCompleted(ctx, task.TaskID, "stale-lease-token", `{"status":"success"}`)
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

	task := &tasks.Task{
		TaskID:     "task-retry-limit",
		UserID:     1,
		ChannelID:  1,
		AccountID:  100,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-retry-limit",
		RetryCount: defaultTaskRetryLimit - 1,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	requeued, err := service.RequeueTaskForFailover(ctx, task.TaskID, task.AccountID, task.LeaseToken, "retry limit reached")
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
	if updated.AccountID != 0 {
		t.Fatalf("account_id = %d, want 0", updated.AccountID)
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

	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, updated.CreatedAt.Format("2006-01-02"), updated.CreatedAt.Format("2006-01-02"))
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

	requeueTask := &tasks.Task{
		TaskID:     "task-requeue-publish-pending",
		UserID:     1,
		ChannelID:  1,
		AccountID:  13,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-publish-pending",
		RetryCount: 0,
	}
	if err := taskRepo.Create(ctx, requeueTask); err != nil {
		t.Fatalf("seed requeue task failed: %v", err)
	}
	requeued, err := service.RequeueTaskForFailover(ctx, requeueTask.TaskID, requeueTask.AccountID, requeueTask.LeaseToken, "retry")
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
		TaskID:     "task-requeue-publish-failed",
		UserID:     1,
		ChannelID:  1,
		AccountID:  14,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-publish-failed",
		RetryCount: defaultTaskRetryLimit - 1,
	}
	if err := taskRepo.Create(ctx, finalTask); err != nil {
		t.Fatalf("seed final requeue task failed: %v", err)
	}
	requeued, err = service.RequeueTaskForFailover(ctx, finalTask.TaskID, finalTask.AccountID, finalTask.LeaseToken, "stop retry")
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

	task := &tasks.Task{
		TaskID:     "task-completed-daily",
		UserID:     7,
		ChannelID:  1,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-completed-daily",
		CreatedAt:  time.Now().Add(-90 * time.Second),
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"model":"gpt-image-1","username":"alice"}`
	if err := service.MarkTaskCompleted(ctx, task.TaskID, task.LeaseToken, result); err != nil {
		t.Fatalf("MarkTaskCompleted returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	day := updated.CreatedAt.Format("2006-01-02")
	rows, err := taskRepo.ListDailyStatsByDateRange(ctx, day, day)
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

	task := &tasks.Task{
		TaskID:     "task-completed-outputs",
		UserID:     7,
		ChannelID:  1,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-completed-outputs",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"status":"success","image_url":"/uploads/2026/05/15/out-a.png","image_urls":["/uploads/2026/05/15/out-a.png","https://cdn.example.com/uploads/2026/05/15/out-b.png"]}`
	if err := service.MarkTaskCompleted(ctx, task.TaskID, task.LeaseToken, result); err != nil {
		t.Fatalf("MarkTaskCompleted returned error: %v", err)
	}

	outputsByTaskID, err := taskRepo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("ListOutputsByTaskIDs returned error: %v", err)
	}
	outputs := outputsByTaskID[task.TaskID]
	if len(outputs) != 2 {
		t.Fatalf("outputs len=%d, want 2", len(outputs))
	}
	if outputs[0].URL != "/uploads/2026/05/15/out-a.png" || outputs[0].StorageKey != "uploads/2026/05/15/out-a.png" {
		t.Fatalf("unexpected first output: %+v", outputs[0])
	}
	if outputs[1].URL != "https://cdn.example.com/uploads/2026/05/15/out-b.png" || outputs[1].StorageKey != "uploads/2026/05/15/out-b.png" {
		t.Fatalf("unexpected second output: %+v", outputs[1])
	}
}

func TestMarkTaskCompletedRejectsInlineImageResult(t *testing.T) {
	ctx := context.Background()
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := taskstore.NewRepository(db)
	service := NewService(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:     "task-inline-result",
		UserID:     7,
		ChannelID:  1,
		Status:     tasks.StatusRunning,
		LeaseToken: "lease-inline-result",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.MarkTaskCompleted(ctx, task.TaskID, task.LeaseToken, `{"status":"success","image_url":"data:image/png;base64,aGVsbG8="}`)
	if err == nil {
		t.Fatal("expected inline image result to be rejected")
	}
	updated, getErr := taskRepo.GetByTaskID(ctx, task.TaskID)
	if getErr != nil {
		t.Fatalf("reload task failed: %v", getErr)
	}
	if updated.Status != tasks.StatusRunning {
		t.Fatalf("status=%s, want running after rejected completion", updated.Status)
	}
}
