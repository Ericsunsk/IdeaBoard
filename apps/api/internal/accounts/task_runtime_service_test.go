package accounts

import (
	"testing"

	"api/internal/tasks"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTaskRuntimeServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&tasks.Task{}); err != nil {
		t.Fatalf("auto migrate task failed: %v", err)
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
}
