package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/runtime/queuetypes"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/runtime/taskqueue"
	"ideaboard/api/internal/runtime/taskruntime"
	"ideaboard/api/internal/tasks"
	"ideaboard/api/internal/testdb"
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
	account := createRiverTestAccount(t, accountRepo, "dup-running-account")

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
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.ApiAccount, *queuetypes.TaskPayload) (string, error) {
		providerCalled = true
		return `{"image_url":"https://example.com/image.png"}`, nil
	})

	err := orchestrator.execute(context.Background(), &queuetypes.TaskPayload{
		TaskID:     "dup-running-task",
		AccountIDs: []int64{account.ID},
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
	account := createRiverTestAccount(t, accountRepo, "complete-fail-account")

	if err := taskRepo.Create(context.Background(), &tasks.Task{
		TaskID:    "complete-fail-task",
		UserID:    1,
		ChannelID: 1,
		Status:    tasks.StatusPending,
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `CREATE TRIGGER fail_mark_completed BEFORE UPDATE ON tasks
FOR EACH ROW
BEGIN
	IF NEW.status = 'completed' THEN
		SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'forced mark completed failure';
	END IF;
END`); err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}

	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.ApiAccount, *queuetypes.TaskPayload) (string, error) {
		return `{"image_url":"https://example.com/image.png"}`, nil
	})

	err := orchestrator.execute(context.Background(), &queuetypes.TaskPayload{
		TaskID:     "complete-fail-task",
		AccountIDs: []int64{account.ID},
	}, taskqueue.TaskAssignment{TaskID: "complete-fail-task", AccountID: account.ID, Status: taskqueue.AssignStatusAssigned, LeaseToken: "lease-direct-complete"})
	if err == nil {
		t.Fatal("expected execute to return error when mark completed persistence fails")
	}
	if !strings.Contains(err.Error(), "forced mark completed failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteReleasesRunningLeaseWhenCancelledAfterProviderReturns(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := accounts.NewTaskRepository(db)
	account := createRiverTestAccount(t, accountRepo, "cancel-after-provider-account")

	task := &tasks.Task{
		TaskID:    "cancel-after-provider-task",
		UserID:    1,
		ChannelID: 1,
		Status:    tasks.StatusPending,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	cancelStore := newFakeCancelStoreForRiver()
	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, cancelStore)
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(ctx context.Context, _ *accounts.ApiAccount, task *queuetypes.TaskPayload) (string, error) {
		if err := cancelStore.MarkCancelled(ctx, task.TaskID, time.Minute); err != nil {
			t.Fatalf("mark cancelled failed: %v", err)
		}
		return `{"image_url":"https://example.com/image.png"}`, nil
	})

	assignment := taskqueue.TaskAssignment{
		TaskID:     task.TaskID,
		AccountID:  account.ID,
		Status:     taskqueue.AssignStatusAssigned,
		LeaseToken: "lease-cancel-after-provider",
	}
	if err := orchestrator.execute(ctx, &queuetypes.TaskPayload{
		TaskID:     task.TaskID,
		AccountIDs: []int64{account.ID},
	}, assignment); err != nil {
		t.Fatalf("execute returned error: %v", err)
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
	if updated.LeaseToken != "" {
		t.Fatalf("lease_token = %q, want empty", updated.LeaseToken)
	}
	if !cancelStore.cleared[task.TaskID] {
		t.Fatal("cancel marker was not cleared")
	}
}

func TestExecuteReturnsErrorWhenMarkFailedPersistenceFails(t *testing.T) {
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := accounts.NewTaskRepository(db)
	account := createRiverTestAccount(t, accountRepo, "failed-fail-account")

	if err := taskRepo.Create(context.Background(), &tasks.Task{
		TaskID:    "failed-fail-task",
		UserID:    1,
		ChannelID: 1,
		Status:    tasks.StatusPending,
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `CREATE TRIGGER fail_mark_failed BEFORE UPDATE ON tasks
FOR EACH ROW
BEGIN
	IF NEW.status = 'failed' THEN
		SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'forced mark failed failure';
	END IF;
END`); err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}

	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.ApiAccount, *queuetypes.TaskPayload) (string, error) {
		return "", errors.New("provider boom")
	})

	err := orchestrator.execute(context.Background(), &queuetypes.TaskPayload{
		TaskID:     "failed-fail-task",
		AccountIDs: []int64{account.ID},
	}, taskqueue.TaskAssignment{TaskID: "failed-fail-task", AccountID: account.ID, Status: taskqueue.AssignStatusAssigned, LeaseToken: "lease-direct-failed"})
	if err == nil {
		t.Fatal("expected execute to return error when mark failed persistence fails")
	}
	if !strings.Contains(err.Error(), "forced mark failed failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newTaskExecutionOrchestratorTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}
