package accounts

import (
	"fmt"
	"testing"
	"time"

	"api/internal/tasks"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTaskRuntimeServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:task_runtime_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&tasks.Task{}); err != nil {
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
	service := NewTaskRuntimeService(nil, taskRepo, nil)

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
	service := NewTaskRuntimeService(nil, taskRepo, nil)
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
	service := NewTaskRuntimeService(nil, taskRepo, nil)

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

func TestMarkTaskCompletedUpsertsDailyStat(t *testing.T) {
	db := newTaskRuntimeServiceTestDB(t)
	taskRepo := NewTaskRepository(db)
	service := NewTaskRuntimeService(nil, taskRepo, nil)

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
