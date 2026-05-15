package accounts

import (
	"fmt"
	"testing"
	"time"

	"api/internal/tasks"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTaskRepositoryTestDB(t *testing.T) *TaskRepository {
	t.Helper()
	dsn := fmt.Sprintf("file:repo_fail_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&ApiAccount{}, &tasks.Task{}, &tasks.TaskOutput{}, &TaskDailyStat{}, &TaskStatEvent{}, &TaskDailyStatBackfill{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return NewTaskRepository(db)
}

func TestFailRunningByAccountIDUpsertsDailyStats(t *testing.T) {
	taskRepo := newTaskRepositoryTestDB(t)
	accountID := uint64(66)
	createdAt := time.Now().Add(-10 * time.Minute)

	if err := taskRepo.Create(&tasks.Task{
		TaskID:    "acc-fail-1",
		UserID:    10,
		ChannelID: 1,
		AccountID: accountID,
		Status:    tasks.StatusRunning,
		Result:    `{"model":"model-stop","username":"user-stop"}`,
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("seed task1 failed: %v", err)
	}
	if err := taskRepo.Create(&tasks.Task{
		TaskID:    "acc-fail-2",
		UserID:    10,
		ChannelID: 1,
		AccountID: accountID,
		Status:    tasks.StatusPending,
		Result:    `{"model":"model-stop","username":"user-stop"}`,
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("seed task2 failed: %v", err)
	}

	affected, err := taskRepo.FailRunningByAccountID(accountID, "stopped")
	if err != nil {
		t.Fatalf("FailRunningByAccountID failed: %v", err)
	}
	if affected != 2 {
		t.Fatalf("affected=%d, want 2", affected)
	}

	day := createdAt.Format("2006-01-02")
	rows, err := taskRepo.ListDailyStatsByDateRange(day, day)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("daily rows=%d, want 1", len(rows))
	}
	if rows[0].Failed != 2 || rows[0].Total != 2 || rows[0].Completed != 0 {
		t.Fatalf("unexpected daily row: %+v", rows[0])
	}

	events := int64(0)
	if err := taskRepo.db.Model(&TaskStatEvent{}).Count(&events).Error; err != nil {
		t.Fatalf("count stat events failed: %v", err)
	}
	if events != 2 {
		t.Fatalf("event count=%d, want 2", events)
	}
}

func TestUpsertDailyStatByTaskIsIdempotentByTaskID(t *testing.T) {
	taskRepo := newTaskRepositoryTestDB(t)
	createdAt := time.Now().Add(-1 * time.Minute)
	completedAt := createdAt.Add(10 * time.Second)
	task := &tasks.Task{
		TaskID:      "idem-1",
		UserID:      3,
		Status:      tasks.StatusCompleted,
		Result:      `{"model":"idem-model","username":"idem-user"}`,
		CreatedAt:   createdAt,
		CompletedAt: &completedAt,
	}

	if err := taskRepo.UpsertDailyStatByTask(task); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	if err := taskRepo.UpsertDailyStatByTask(task); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	day := createdAt.Format("2006-01-02")
	rows, err := taskRepo.ListDailyStatsByDateRange(day, day)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Total != 1 || rows[0].Completed != 1 {
		t.Fatalf("idempotent aggregate expected 1 row/1 total: %+v", rows)
	}
}

func TestUpsertDailyStatByTaskWithFallbackModelUsesProvidedSingleModel(t *testing.T) {
	taskRepo := newTaskRepositoryTestDB(t)
	createdAt := time.Now().Add(-2 * time.Minute)
	completedAt := createdAt.Add(15 * time.Second)
	task := &tasks.Task{
		TaskID:      "fallback-1",
		UserID:      7,
		AccountID:   12345,
		Status:      tasks.StatusCompleted,
		Result:      `{}`,
		CreatedAt:   createdAt,
		CompletedAt: &completedAt,
	}

	if err := taskRepo.UpsertDailyStatByTaskWithFallbackModel(task, "model-fallback"); err != nil {
		t.Fatalf("upsert with fallback failed: %v", err)
	}

	day := createdAt.Format("2006-01-02")
	rows, err := taskRepo.ListDailyStatsByDateRange(day, day)
	if err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d, want 1", len(rows))
	}
	if rows[0].ModelName != "model-fallback" {
		t.Fatalf("model_name=%q, want model-fallback", rows[0].ModelName)
	}
}
