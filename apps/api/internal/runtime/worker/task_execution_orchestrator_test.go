package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/taskruntime"
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

func TestExecuteRequeuesTaskWhenLeaseConfirmationFails(t *testing.T) {
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "confirm-fail-account")

	if err := taskRepo.Create(context.Background(), &tasks.Task{
		TaskID:            "confirm-fail-task",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(account.ChannelID),
		AccountID:         tasks.IDPtr(account.ID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-confirm-fail",
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	assigner := &fakeAssigner{confirmErr: errors.New("lease confirmation failed")}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.ApiAccount, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		providerCalled = true
		return providerOutputJSON(`{"image_url":"https://example.com/image.png"}`), nil
	})

	err := orchestrator.execute(context.Background(), &tasks.TaskPayload{
		TaskID:     "confirm-fail-task",
		AccountIDs: []int64{account.ID},
	}, taskqueue.TaskAssignment{TaskID: "confirm-fail-task", AccountID: account.ID, ChannelID: account.ChannelID, Status: taskqueue.AssignStatusAssigned, LeaseToken: "lease-confirm-fail"})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if providerCalled {
		t.Fatal("task with unconfirmed lease should not call provider")
	}
	updated, err := taskRepo.GetByTaskID(context.Background(), "confirm-fail-task")
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
	if updated.CancelledAt != nil {
		t.Fatalf("cancelled_at = %v, want nil", updated.CancelledAt)
	}
	if updated.AccountID != nil {
		t.Fatalf("account_id = %d, want nil", tasks.IDValue(updated.AccountID))
	}
	if updated.AssignedChannelID != nil {
		t.Fatalf("assigned_channel_id = %d, want nil", tasks.IDValue(updated.AssignedChannelID))
	}
}

func TestExecuteReturnsErrorWhenMarkCompletedPersistenceFails(t *testing.T) {
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "complete-fail-account")

	if err := taskRepo.Create(context.Background(), &tasks.Task{
		TaskID:            "complete-fail-task",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(account.ChannelID),
		AccountID:         tasks.IDPtr(account.ID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-direct-complete",
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
CREATE OR REPLACE FUNCTION fail_mark_completed()
RETURNS trigger AS $$
BEGIN
	IF NEW.status = 'completed' THEN
		RAISE EXCEPTION 'forced mark completed failure';
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER fail_mark_completed BEFORE UPDATE ON tasks
FOR EACH ROW EXECUTE FUNCTION fail_mark_completed();
`); err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}

	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(_ context.Context, runtimeAccount *accounts.ApiAccount, _ *tasks.TaskPayload, _ taskqueue.TaskAssignment) (providerCallOutput, error) {
		if runtimeAccount.ApiKey != "k" {
			t.Fatalf("runtime account api key = %q, want secret", runtimeAccount.ApiKey)
		}
		return providerOutputJSON(`{"image_url":"https://example.com/image.png"}`), nil
	})

	err := orchestrator.execute(context.Background(), &tasks.TaskPayload{
		TaskID:     "complete-fail-task",
		AccountIDs: []int64{account.ID},
	}, taskqueue.TaskAssignment{TaskID: "complete-fail-task", AccountID: account.ID, ChannelID: account.ChannelID, Status: taskqueue.AssignStatusAssigned, LeaseToken: "lease-direct-complete"})
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
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "cancel-after-provider-account")

	task := &tasks.Task{
		TaskID:            "cancel-after-provider-task",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(account.ChannelID),
		AccountID:         tasks.IDPtr(account.ID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-cancel-after-provider",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	cancelStore := newFakeCancelStoreForRiver()
	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, cancelStore)
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(ctx context.Context, _ *accounts.ApiAccount, task *tasks.TaskPayload, _ taskqueue.TaskAssignment) (providerCallOutput, error) {
		if err := cancelStore.MarkCancelled(ctx, task.TaskID, time.Minute); err != nil {
			t.Fatalf("mark cancelled failed: %v", err)
		}
		return providerOutputJSON(`{"image_url":"https://example.com/image.png"}`), nil
	})

	assignment := taskqueue.TaskAssignment{
		TaskID:     task.TaskID,
		AccountID:  account.ID,
		ChannelID:  account.ChannelID,
		Status:     taskqueue.AssignStatusAssigned,
		LeaseToken: "lease-cancel-after-provider",
	}
	if err := orchestrator.execute(ctx, &tasks.TaskPayload{
		TaskID:     task.TaskID,
		AccountIDs: []int64{account.ID},
	}, assignment); err != nil {
		t.Fatalf("execute returned error: %v", err)
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
	if tasks.IDValue(updated.AccountID) != account.ID {
		t.Fatalf("account_id = %d, want %d", tasks.IDValue(updated.AccountID), account.ID)
	}
	if tasks.IDValue(updated.AssignedChannelID) != account.ChannelID {
		t.Fatalf("assigned_channel_id = %d, want %d", tasks.IDValue(updated.AssignedChannelID), account.ChannelID)
	}
	if updated.LeaseToken != "" {
		t.Fatalf("lease_token = %q, want empty", updated.LeaseToken)
	}
	if !cancelStore.cleared[task.TaskID] {
		t.Fatal("cancel marker was not cleared")
	}
}

func TestExecuteReleasesRunningLeaseWhenCancelledBeforeProvider(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "cancel-before-provider-account")

	task := &tasks.Task{
		TaskID:            "cancel-before-provider-task",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(account.ChannelID),
		AccountID:         tasks.IDPtr(account.ID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-cancel-before-provider",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	cancelStore := newFakeCancelStoreForRiver()
	if err := cancelStore.MarkCancelled(ctx, task.TaskID, time.Minute); err != nil {
		t.Fatalf("mark cancelled failed: %v", err)
	}
	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, cancelStore)
	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.ApiAccount, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		providerCalled = true
		return providerOutputJSON(`{"image_url":"https://example.com/image.png"}`), nil
	})

	assignment := taskqueue.TaskAssignment{
		TaskID:     task.TaskID,
		AccountID:  account.ID,
		ChannelID:  account.ChannelID,
		Status:     taskqueue.AssignStatusAssigned,
		LeaseToken: task.LeaseToken,
	}
	if err := orchestrator.execute(ctx, &tasks.TaskPayload{
		TaskID:     task.TaskID,
		AccountIDs: []int64{account.ID},
	}, assignment); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if providerCalled {
		t.Fatal("cancelled task should not call provider")
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
	if tasks.IDValue(updated.AccountID) != account.ID {
		t.Fatalf("account_id = %d, want %d", tasks.IDValue(updated.AccountID), account.ID)
	}
	if tasks.IDValue(updated.AssignedChannelID) != account.ChannelID {
		t.Fatalf("assigned_channel_id = %d, want %d", tasks.IDValue(updated.AssignedChannelID), account.ChannelID)
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
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "failed-fail-account")

	if err := taskRepo.Create(context.Background(), &tasks.Task{
		TaskID:            "failed-fail-task",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(account.ChannelID),
		AccountID:         tasks.IDPtr(account.ID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-direct-failed",
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
CREATE OR REPLACE FUNCTION fail_mark_failed()
RETURNS trigger AS $$
BEGIN
	IF NEW.status = 'failed' THEN
		RAISE EXCEPTION 'forced mark failed failure';
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER fail_mark_failed BEFORE UPDATE ON tasks
FOR EACH ROW EXECUTE FUNCTION fail_mark_failed();
`); err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}

	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.ApiAccount, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		return providerCallOutput{}, errors.New("provider boom")
	})

	err := orchestrator.execute(context.Background(), &tasks.TaskPayload{
		TaskID:     "failed-fail-task",
		AccountIDs: []int64{account.ID},
	}, taskqueue.TaskAssignment{TaskID: "failed-fail-task", AccountID: account.ID, ChannelID: account.ChannelID, Status: taskqueue.AssignStatusAssigned, LeaseToken: "lease-direct-failed"})
	if err == nil {
		t.Fatal("expected execute to return error when mark failed persistence fails")
	}
	if !strings.Contains(err.Error(), "forced mark failed failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteFailsImmediatelyWhenFailoverHasNoCandidates(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "failover-exhausted-account")

	task := &tasks.Task{
		TaskID:            "failover-exhausted-task",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(account.ChannelID),
		AccountID:         tasks.IDPtr(account.ID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-failover-exhausted",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	assigner := &fakeAssigner{}
	requeuer := &fakeRequeuer{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	orchestrator := newTaskExecutionOrchestrator(accountRepo, requeuer, runtime, func(context.Context, *accounts.ApiAccount, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		return providerOutputJSON(""), newUpstreamHTTPError(429, "rate limited")
	})

	err := orchestrator.execute(ctx, &tasks.TaskPayload{
		TaskID:     task.TaskID,
		AccountIDs: []int64{account.ID},
	}, taskqueue.TaskAssignment{TaskID: task.TaskID, AccountID: account.ID, ChannelID: account.ChannelID, Status: taskqueue.AssignStatusAssigned, LeaseToken: task.LeaseToken})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if requeuer.calls != 0 {
		t.Fatalf("requeue calls = %d, want 0", requeuer.calls)
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
	if updated.CompletedAt == nil {
		t.Fatal("completed_at should be set")
	}
	if tasks.IDValue(updated.AccountID) != account.ID {
		t.Fatalf("account_id = %d, want %d", tasks.IDValue(updated.AccountID), account.ID)
	}
}

func newTaskExecutionOrchestratorTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func providerOutputJSON(result string) providerCallOutput {
	now := time.Now()
	return providerCallOutput{
		Result: result,
		Timeline: providerCallTimeline{
			ProviderStartedAt:  &now,
			ProviderFinishedAt: &now,
			FinalizeStartedAt:  &now,
			FinalizeFinishedAt: &now,
		},
	}
}
