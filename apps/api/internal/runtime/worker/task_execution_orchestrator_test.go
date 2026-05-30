package worker

import (
	"api/internal/accounts"
	"api/internal/runtime/queuetypes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"api/internal/runtime/taskqueue"
	"api/internal/runtime/taskruntime"
	"api/internal/tasks"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUserFacingProviderErrorRecognizesWrappedDeadlineExceeded(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (渠道: gptnb): %w",
		fmt.Errorf("HTTP 请求失败: %w", context.DeadlineExceeded),
	)

	got := userFacingProviderError(err)
	if got != "上游图像服务响应超时" {
		t.Fatalf("expected timeout message, got %q", got)
	}
}

func TestUserFacingProviderErrorFallsBackForNonTimeout(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (渠道: gptnb): %w", fmt.Errorf("HTTP 请求失败: boom"))

	got := userFacingProviderError(err)
	if got != "上游图像服务调用失败" {
		t.Fatalf("expected generic provider message, got %q", got)
	}
}

func TestExecuteDropsDuplicateTaskWhenAlreadyRunningOnSameAccount(t *testing.T) {
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := accounts.NewTaskRepository(db)
	account := createAsynqTestAccount(t, accountRepo, "dup-running-account")

	if err := taskRepo.Create(context.Background(), &tasks.Task{
		TaskID:    "dup-running-task",
		UserID:    1,
		ChannelID: 1,
		AccountID: account.ID,
		Status:    tasks.StatusRunning,
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForAsynq())
	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.ApiAccount, *queuetypes.TaskPayload) (string, error) {
		providerCalled = true
		return `{"image_url":"https://example.com/image.png"}`, nil
	})

	err := orchestrator.execute(context.Background(), &queuetypes.TaskPayload{
		TaskID:     "dup-running-task",
		AccountIDs: []uint64{account.ID},
	}, taskqueue.TaskAssignment{TaskID: "dup-running-task", AccountID: account.ID, Status: taskqueue.AssignStatusAssigned, LeaseToken: "lease-direct-dup"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if providerCalled {
		t.Fatal("duplicate running task should not call provider")
	}
}

func TestExecuteReturnsErrorWhenMarkCompletedPersistenceFails(t *testing.T) {
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := accounts.NewTaskRepository(db)
	account := createAsynqTestAccount(t, accountRepo, "complete-fail-account")

	if err := taskRepo.Create(context.Background(), &tasks.Task{
		TaskID:    "complete-fail-task",
		UserID:    1,
		ChannelID: 1,
		Status:    tasks.StatusPending,
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_mark_completed BEFORE UPDATE ON tasks
WHEN NEW.status = 'completed'
BEGIN
	SELECT RAISE(ABORT, 'forced mark completed failure');
END;`).Error; err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}

	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForAsynq())
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.ApiAccount, *queuetypes.TaskPayload) (string, error) {
		return `{"image_url":"https://example.com/image.png"}`, nil
	})

	err := orchestrator.execute(context.Background(), &queuetypes.TaskPayload{
		TaskID:     "complete-fail-task",
		AccountIDs: []uint64{account.ID},
	}, taskqueue.TaskAssignment{TaskID: "complete-fail-task", AccountID: account.ID, Status: taskqueue.AssignStatusAssigned, LeaseToken: "lease-direct-complete"})
	if err == nil {
		t.Fatal("expected execute to return error when mark completed persistence fails")
	}
	if !strings.Contains(err.Error(), "forced mark completed failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteReturnsErrorWhenMarkFailedPersistenceFails(t *testing.T) {
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := accounts.NewTaskRepository(db)
	account := createAsynqTestAccount(t, accountRepo, "failed-fail-account")

	if err := taskRepo.Create(context.Background(), &tasks.Task{
		TaskID:    "failed-fail-task",
		UserID:    1,
		ChannelID: 1,
		Status:    tasks.StatusPending,
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_mark_failed BEFORE UPDATE ON tasks
WHEN NEW.status = 'failed'
BEGIN
	SELECT RAISE(ABORT, 'forced mark failed failure');
END;`).Error; err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}

	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForAsynq())
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.ApiAccount, *queuetypes.TaskPayload) (string, error) {
		return "", errors.New("provider boom")
	})

	err := orchestrator.execute(context.Background(), &queuetypes.TaskPayload{
		TaskID:     "failed-fail-task",
		AccountIDs: []uint64{account.ID},
	}, taskqueue.TaskAssignment{TaskID: "failed-fail-task", AccountID: account.ID, Status: taskqueue.AssignStatusAssigned, LeaseToken: "lease-direct-failed"})
	if err == nil {
		t.Fatal("expected execute to return error when mark failed persistence fails")
	}
	if !strings.Contains(err.Error(), "forced mark failed failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newTaskExecutionOrchestratorTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:task_execution_orchestrator_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&accounts.ApiAccount{}, &tasks.Task{}, &tasks.TaskOutput{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return db
}
