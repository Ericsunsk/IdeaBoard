package accounts

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"api/internal/tasks"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResetTaskRunningToPendingResetsState(t *testing.T) {
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := NewTaskRepository(db)
	service := NewTaskRuntimeServiceWithQueue(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:    "task-reset-pending",
		UserID:    1,
		ChannelID: 1,
		AccountID: 77,
		Status:    tasks.StatusRunning,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.ResetTaskRunningToPending(task.TaskID, task.AccountID, "worker context interrupted"); err != nil {
		t.Fatalf("ResetTaskRunningToPending returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(task.TaskID)
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
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := NewTaskRepository(db)
	service := NewTaskRuntimeServiceWithQueue(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:      "task-reset-terminal",
		UserID:      1,
		ChannelID:   1,
		AccountID:   77,
		Status:      tasks.StatusCompleted,
		CompletedAt: ptrTime(time.Now()),
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.ResetTaskRunningToPending(task.TaskID, task.AccountID, "worker context interrupted")
	if !errors.Is(err, ErrTaskTerminal) {
		t.Fatalf("expected ErrTaskTerminal, got %v", err)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func newTaskRuntimeServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:task_runtime_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&tasks.Task{}, &tasks.TaskOutput{}); err != nil {
		t.Fatalf("auto migrate task failed: %v", err)
	}
	if err := db.AutoMigrate(&TaskDailyStat{}); err != nil {
		t.Fatalf("auto migrate task_daily_stats failed: %v", err)
	}
	if err := db.AutoMigrate(&TaskStatEvent{}, &TaskDailyStatBackfill{}); err != nil {
		t.Fatalf("auto migrate stat event/backfill failed: %v", err)
	}
	return db
}

func TestRequeueTaskForFailoverRequeuesWhenBelowRetryLimit(t *testing.T) {
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := NewTaskRepository(db)
	service := NewTaskRuntimeServiceWithQueue(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:     "task-requeue",
		UserID:     1,
		ChannelID:  1,
		AccountID:  99,
		Status:     tasks.StatusRunning,
		RetryCount: defaultTaskRetryLimit - 2,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	requeued, err := service.RequeueTaskForFailover(task.TaskID, task.AccountID, "retryable error")
	if err != nil {
		t.Fatalf("RequeueTaskForFailover returned error: %v", err)
	}
	if !requeued {
		t.Fatalf("expected task to be requeued")
	}

	updated, err := taskRepo.GetByTaskID(task.TaskID)
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

func TestTaskRuntimePublishesStatusChanges(t *testing.T) {
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := NewTaskRepository(db)
	service := NewTaskRuntimeServiceWithQueue(nil, taskRepo, nil, nil)
	hub := NewTaskEventHub()
	service.SetEventHub(hub)
	events, unsubscribe := hub.Subscribe("task-publish")
	defer unsubscribe()

	task := &tasks.Task{
		TaskID:    "task-publish",
		UserID:    1,
		ChannelID: 1,
		Status:    tasks.StatusPending,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	if err := service.MarkTaskRunning(task.TaskID, 42); err != nil {
		t.Fatalf("MarkTaskRunning returned error: %v", err)
	}

	select {
	case event := <-events:
		if event.TaskID != task.TaskID || event.Status != tasks.StatusRunning {
			t.Fatalf("event=(%s,%s), want=(%s,%s)", event.TaskID, event.Status, task.TaskID, tasks.StatusRunning)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for running event")
	}
}

func TestRequeueTaskForFailoverFailsWhenRetryLimitReached(t *testing.T) {
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := NewTaskRepository(db)
	service := NewTaskRuntimeServiceWithQueue(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:     "task-retry-limit",
		UserID:     1,
		ChannelID:  1,
		AccountID:  100,
		Status:     tasks.StatusRunning,
		RetryCount: defaultTaskRetryLimit - 1,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	requeued, err := service.RequeueTaskForFailover(task.TaskID, task.AccountID, "retry limit reached")
	if err != nil {
		t.Fatalf("RequeueTaskForFailover returned error: %v", err)
	}
	if requeued {
		t.Fatalf("expected task to stop requeue at retry limit")
	}

	updated, err := taskRepo.GetByTaskID(task.TaskID)
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

	rows, err := taskRepo.ListDailyStatsByDateRange(updated.CreatedAt.Format("2006-01-02"), updated.CreatedAt.Format("2006-01-02"))
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
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := NewTaskRepository(db)
	service := NewTaskRuntimeServiceWithQueue(nil, taskRepo, nil, nil)
	hub := NewTaskEventHub()
	service.SetEventHub(hub)

	requeueTask := &tasks.Task{
		TaskID:     "task-requeue-publish-pending",
		UserID:     1,
		ChannelID:  1,
		AccountID:  13,
		Status:     tasks.StatusRunning,
		RetryCount: 0,
	}
	if err := taskRepo.Create(requeueTask); err != nil {
		t.Fatalf("seed requeue task failed: %v", err)
	}
	pendingEvents, unsubPending := hub.Subscribe(requeueTask.TaskID)
	defer unsubPending()
	requeued, err := service.RequeueTaskForFailover(requeueTask.TaskID, requeueTask.AccountID, "retry")
	if err != nil {
		t.Fatalf("RequeueTaskForFailover pending returned error: %v", err)
	}
	if !requeued {
		t.Fatalf("expected requeued=true")
	}
	select {
	case event := <-pendingEvents:
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
		RetryCount: defaultTaskRetryLimit - 1,
	}
	if err := taskRepo.Create(finalTask); err != nil {
		t.Fatalf("seed final requeue task failed: %v", err)
	}
	failedEvents, unsubFailed := hub.Subscribe(finalTask.TaskID)
	defer unsubFailed()
	requeued, err = service.RequeueTaskForFailover(finalTask.TaskID, finalTask.AccountID, "stop retry")
	if err != nil {
		t.Fatalf("RequeueTaskForFailover failed returned error: %v", err)
	}
	if requeued {
		t.Fatalf("expected requeued=false when final fail")
	}
	select {
	case event := <-failedEvents:
		if event.Status != tasks.StatusFailed {
			t.Fatalf("final fail event status=%s, want %s", event.Status, tasks.StatusFailed)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting final failed event")
	}
}

func TestMarkTaskCompletedUpsertsDailyStat(t *testing.T) {
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := NewTaskRepository(db)
	service := NewTaskRuntimeServiceWithQueue(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:    "task-completed-daily",
		UserID:    7,
		ChannelID: 1,
		Status:    tasks.StatusRunning,
		CreatedAt: time.Now().Add(-90 * time.Second),
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"model":"gpt-image-1","username":"alice"}`
	if err := service.MarkTaskCompleted(task.TaskID, result); err != nil {
		t.Fatalf("MarkTaskCompleted returned error: %v", err)
	}

	updated, err := taskRepo.GetByTaskID(task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	day := updated.CreatedAt.Format("2006-01-02")
	rows, err := taskRepo.ListDailyStatsByDateRange(day, day)
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
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := NewTaskRepository(db)
	service := NewTaskRuntimeServiceWithQueue(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:    "task-completed-outputs",
		UserID:    7,
		ChannelID: 1,
		Status:    tasks.StatusRunning,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	result := `{"status":"success","image_url":"/uploads/2026/05/15/out-a.png","image_urls":["/uploads/2026/05/15/out-a.png","https://cdn.example.com/uploads/2026/05/15/out-b.png"]}`
	if err := service.MarkTaskCompleted(task.TaskID, result); err != nil {
		t.Fatalf("MarkTaskCompleted returned error: %v", err)
	}

	outputsByTaskID, err := taskRepo.ListOutputsByTaskIDs([]string{task.TaskID})
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
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := NewTaskRepository(db)
	service := NewTaskRuntimeServiceWithQueue(nil, taskRepo, nil, nil)

	task := &tasks.Task{
		TaskID:    "task-inline-result",
		UserID:    7,
		ChannelID: 1,
		Status:    tasks.StatusRunning,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	err := service.MarkTaskCompleted(task.TaskID, `{"status":"success","image_url":"data:image/png;base64,aGVsbG8="}`)
	if err == nil {
		t.Fatal("expected inline image result to be rejected")
	}
	updated, getErr := taskRepo.GetByTaskID(task.TaskID)
	if getErr != nil {
		t.Fatalf("reload task failed: %v", getErr)
	}
	if updated.Status != tasks.StatusRunning {
		t.Fatalf("status=%s, want running after rejected completion", updated.Status)
	}
}
