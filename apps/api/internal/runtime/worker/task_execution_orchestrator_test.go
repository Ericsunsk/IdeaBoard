package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/taskruntime"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/testdb"
)

func TestUserFacingProviderErrorRecognizesWrappedDeadlineExceeded(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (服务端点: gptnb): %w",
		fmt.Errorf("HTTP 请求失败: %w", context.DeadlineExceeded),
	)

	got := userFacingProviderError(err)
	if got != "上游图像服务响应超时" {
		t.Fatalf("expected timeout message, got %q", got)
	}
}

func TestUserFacingProviderErrorFallsBackForNonTimeout(t *testing.T) {
	err := fmt.Errorf("调用 API 失败 (服务端点: gptnb): %w", fmt.Errorf("HTTP 请求失败: boom"))

	got := userFacingProviderError(err)
	if got != "上游图像服务调用失败" {
		t.Fatalf("expected generic provider message, got %q", got)
	}
}

func TestUserFacingProviderErrorPreservesInputInvalidDetails(t *testing.T) {
	err := newExecutionFailureError(
		tasks.ExecutionStageAssigned,
		tasks.FailureCategoryInputInvalid,
		errors.New("图案生成不需要输入图片，请移除图片输入后重试"),
	)

	got := userFacingProviderError(err)
	if got != "图案生成不需要输入图片，请移除图片输入后重试" {
		t.Fatalf("expected specific input validation message, got %q", got)
	}
}

func TestExecuteRequeuesTaskWhenLeaseConfirmationFails(t *testing.T) {
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "confirm-fail-account")

	if err := taskRepo.Create(context.Background(), &tasks.Task{
		TaskID:                     "confirm-fail-task",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AssignedCredentialID:       tasks.IDPtr(loadRiverTestCredentialID(t, db, account.ID)),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-confirm-fail",
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	assigner := &fakeAssigner{confirmErr: errors.New("lease confirmation failed")}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.RuntimeAccountAccess, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		providerCalled = true
		return providerOutputJSON(`{"image_url":"https://example.com/image.png"}`), nil
	})

	assignment := riverTestAssignment(t, db, account, "confirm-fail-task", "lease-confirm-fail")
	err := orchestrator.execute(context.Background(), &tasks.TaskPayload{
		TaskID: "confirm-fail-task",
	}, assignment)
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
	if updated.AssignedProviderEndpointID != nil {
		t.Fatalf("assigned_provider_endpoint_id = %d, want nil", tasks.IDValue(updated.AssignedProviderEndpointID))
	}
}

func TestExecuteDropsTaskWhenLeaseConfirmationAlreadyConverged(t *testing.T) {
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "confirm-converged-account")

	task := &tasks.Task{
		TaskID:                     "confirm-converged-task",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AssignedCredentialID:       tasks.IDPtr(loadRiverTestCredentialID(t, db, account.ID)),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-confirm-converged",
	}
	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	assigner := &fakeAssigner{confirmErr: taskqueue.ErrTaskLeaseConflict}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	requeuer := &fakeRequeuer{}
	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, requeuer, runtime, func(context.Context, *accounts.RuntimeAccountAccess, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		providerCalled = true
		return providerOutputJSON(`{"image_url":"https://example.com/image.png"}`), nil
	})

	assignment := riverTestAssignment(t, db, account, task.TaskID, task.LeaseToken)
	err := orchestrator.execute(context.Background(), &tasks.TaskPayload{
		TaskID: task.TaskID,
	}, assignment)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if providerCalled {
		t.Fatal("converged lease conflict should not call provider")
	}
	if requeuer.calls != 0 {
		t.Fatalf("requeue calls = %d, want 0", requeuer.calls)
	}
	updated, err := taskRepo.GetByTaskID(context.Background(), task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusRunning {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusRunning)
	}
}

func TestExecuteRequeuesWhenAssignmentProviderEndpointNoLongerMatchesAccount(t *testing.T) {
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "endpoint-mismatch-account")

	task := &tasks.Task{
		TaskID:                     "endpoint-mismatch-task",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AssignedCredentialID:       tasks.IDPtr(loadRiverTestCredentialID(t, db, account.ID)),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-endpoint-mismatch",
	}
	if err := taskRepo.Create(context.Background(), task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	requeuer := &fakeRequeuer{}
	providerCalled := false
	orchestrator := newTaskExecutionOrchestrator(accountRepo, requeuer, runtime, func(context.Context, *accounts.RuntimeAccountAccess, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		providerCalled = true
		return providerOutputJSON(`{"image_url":"https://example.com/image.png"}`), nil
	})

	assignment := riverTestAssignment(t, db, account, task.TaskID, task.LeaseToken)
	assignment.ProviderEndpointID += 1000
	err := orchestrator.execute(context.Background(), &tasks.TaskPayload{
		TaskID: task.TaskID,
	}, assignment)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if providerCalled {
		t.Fatal("endpoint mismatch should not call provider")
	}
	if requeuer.calls != 1 {
		t.Fatalf("requeue calls = %d, want 1", requeuer.calls)
	}
}

func TestExecuteReturnsErrorWhenMarkCompletedPersistenceFails(t *testing.T) {
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "complete-fail-account")

	if err := taskRepo.Create(context.Background(), &tasks.Task{
		TaskID:                     "complete-fail-task",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AssignedCredentialID:       tasks.IDPtr(loadRiverTestCredentialID(t, db, account.ID)),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-direct-complete",
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
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(_ context.Context, runtimeAccount *accounts.RuntimeAccountAccess, _ *tasks.TaskPayload, _ taskqueue.TaskAssignment) (providerCallOutput, error) {
		if runtimeAccount.APIKey != "k" {
			t.Fatalf("runtime account api key = %q, want secret", runtimeAccount.APIKey)
		}
		return providerOutputJSON(`{"image_url":"https://example.com/image.png"}`), nil
	})

	assignment := riverTestAssignment(t, db, account, "complete-fail-task", "lease-direct-complete")
	err := orchestrator.execute(context.Background(), &tasks.TaskPayload{
		TaskID: "complete-fail-task",
	}, assignment)
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
		TaskID:                     "cancel-after-provider-task",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AssignedCredentialID:       tasks.IDPtr(loadRiverTestCredentialID(t, db, account.ID)),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-cancel-after-provider",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	cancelStore := newFakeCancelStoreForRiver()
	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, cancelStore)
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(ctx context.Context, _ *accounts.RuntimeAccountAccess, task *tasks.TaskPayload, _ taskqueue.TaskAssignment) (providerCallOutput, error) {
		if err := cancelStore.MarkCancelled(ctx, task.TaskID, time.Minute); err != nil {
			t.Fatalf("mark cancelled failed: %v", err)
		}
		return providerOutputJSON(`{"image_url":"https://example.com/image.png"}`), nil
	})

	assignment := riverTestAssignment(t, db, account, task.TaskID, "lease-cancel-after-provider")
	if err := orchestrator.execute(ctx, &tasks.TaskPayload{
		TaskID: task.TaskID,
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
	if tasks.IDValue(updated.AssignedProviderEndpointID) != account.ProviderEndpointID {
		t.Fatalf("assigned_provider_endpoint_id = %d, want %d", tasks.IDValue(updated.AssignedProviderEndpointID), account.ProviderEndpointID)
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
		TaskID:                     "cancel-before-provider-task",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AssignedCredentialID:       tasks.IDPtr(loadRiverTestCredentialID(t, db, account.ID)),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-cancel-before-provider",
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
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.RuntimeAccountAccess, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		providerCalled = true
		return providerOutputJSON(`{"image_url":"https://example.com/image.png"}`), nil
	})

	assignment := riverTestAssignment(t, db, account, task.TaskID, task.LeaseToken)
	if err := orchestrator.execute(ctx, &tasks.TaskPayload{
		TaskID: task.TaskID,
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
	if tasks.IDValue(updated.AssignedProviderEndpointID) != account.ProviderEndpointID {
		t.Fatalf("assigned_provider_endpoint_id = %d, want %d", tasks.IDValue(updated.AssignedProviderEndpointID), account.ProviderEndpointID)
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
		TaskID:                     "failed-fail-task",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AssignedCredentialID:       tasks.IDPtr(loadRiverTestCredentialID(t, db, account.ID)),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-direct-failed",
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
	orchestrator := newTaskExecutionOrchestrator(accountRepo, &fakeRequeuer{}, runtime, func(context.Context, *accounts.RuntimeAccountAccess, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		return providerCallOutput{}, errors.New("provider boom")
	})

	assignment := riverTestAssignment(t, db, account, "failed-fail-task", "lease-direct-failed")
	err := orchestrator.execute(context.Background(), &tasks.TaskPayload{
		TaskID: "failed-fail-task",
	}, assignment)
	if err == nil {
		t.Fatal("expected execute to return error when mark failed persistence fails")
	}
	if !strings.Contains(err.Error(), "forced mark failed failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteProviderFailureRoutesThroughConvergenceDecision(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "convergence-primary-account")

	task := &tasks.Task{
		TaskID:                     "convergence-provider-failure-task",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AssignedCredentialID:       tasks.IDPtr(loadRiverTestCredentialID(t, db, account.ID)),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-convergence-provider-failure",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	requeuer := &fakeRequeuer{}
	assigner := &fakeAssigner{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	decider := &recordingTaskExecutionConvergenceDecider{
		outcomes: []taskExecutionConvergenceOutcome{taskExecutionConvergenceFinalFail},
	}
	orchestrator := newTaskExecutionOrchestrator(accountRepo, requeuer, runtime, func(context.Context, *accounts.RuntimeAccountAccess, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		return providerOutputJSON(""), newUpstreamHTTPError(429, "rate limited")
	})
	orchestrator.convergenceDecider = decider

	assignment := riverTestAssignment(t, db, account, task.TaskID, task.LeaseToken)
	err := orchestrator.execute(ctx, &tasks.TaskPayload{
		TaskID: task.TaskID,
	}, assignment)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	if len(decider.inputs) != 1 {
		t.Fatalf("convergence decisions = %d, want 1", len(decider.inputs))
	}
	decisionInput := decider.inputs[0]
	if !decisionInput.ProviderFailure.IsFailoverable {
		t.Fatal("provider failure should be marked failoverable")
	}
	if decisionInput.ProviderFailure.Reason != "上游服务请求过于频繁，请稍后重试" {
		t.Fatalf("reason = %q", decisionInput.ProviderFailure.Reason)
	}
	if decisionInput.ProviderFailure.Stage != tasks.ExecutionStageProvider {
		t.Fatalf("failure stage = %q, want %q", decisionInput.ProviderFailure.Stage, tasks.ExecutionStageProvider)
	}
	if decisionInput.ProviderFailure.Category != tasks.FailureCategoryTimeout {
		t.Fatalf("failure category = %q, want %q", decisionInput.ProviderFailure.Category, tasks.FailureCategoryTimeout)
	}
	if decisionInput.Failover.RemainingAccountCandidates != 1 {
		t.Fatalf("remaining account candidates = %d, want 1", decisionInput.Failover.RemainingAccountCandidates)
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
	failedAccount, err := accountRepo.GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("reload account failed: %v", err)
	}
	if failedAccount.FailCount != 1 {
		t.Fatalf("fail_count = %d, want 1", failedAccount.FailCount)
	}

	var attemptStatus, attemptReason, attemptFailureStage, attemptFailureCategory string
	if err := db.QueryRowContext(ctx, `
SELECT status, error_msg, failure_stage, failure_category
FROM task_attempts
WHERE task_id = $1`, task.TaskID).Scan(&attemptStatus, &attemptReason, &attemptFailureStage, &attemptFailureCategory); err != nil {
		t.Fatalf("read task attempt failed: %v", err)
	}
	if attemptStatus != tasks.TaskAttemptStatusFailed {
		t.Fatalf("attempt status = %s, want %s", attemptStatus, tasks.TaskAttemptStatusFailed)
	}
	if attemptReason != decisionInput.ProviderFailure.Reason {
		t.Fatalf("attempt reason = %q, want %q", attemptReason, decisionInput.ProviderFailure.Reason)
	}
	if attemptFailureStage != tasks.ExecutionStageProvider {
		t.Fatalf("attempt failure stage = %q, want %q", attemptFailureStage, tasks.ExecutionStageProvider)
	}
	if attemptFailureCategory != tasks.FailureCategoryTimeout {
		t.Fatalf("attempt failure category = %q, want %q", attemptFailureCategory, tasks.FailureCategoryTimeout)
	}

	metricsByTaskID, err := taskRepo.ListExecutionMetricsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("read execution metrics failed: %v", err)
	}
	metrics := metricsByTaskID[task.TaskID]
	if metrics == nil {
		t.Fatal("execution metrics should exist")
		return
	}
	if metrics.CurrentStage != tasks.ExecutionStageFailed {
		t.Fatalf("metrics current stage = %q, want %q", metrics.CurrentStage, tasks.ExecutionStageFailed)
	}
	if metrics.FailureStage != tasks.ExecutionStageProvider {
		t.Fatalf("metrics failure stage = %q, want %q", metrics.FailureStage, tasks.ExecutionStageProvider)
	}
	if metrics.FailureCategory != tasks.FailureCategoryTimeout {
		t.Fatalf("metrics failure category = %q, want %q", metrics.FailureCategory, tasks.FailureCategoryTimeout)
	}
}

func TestExecuteTemporaryProviderFailureRequeuesWithoutFailureResult(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "retry-primary-account")

	task := &tasks.Task{
		TaskID:                     "temporary-provider-failure-task",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AssignedCredentialID:       tasks.IDPtr(loadRiverTestCredentialID(t, db, account.ID)),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-temporary-provider-failure",
		RetryCount:                 0,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	assigner := &fakeAssigner{}
	requeuer := &fakeRequeuer{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	orchestrator := newTaskExecutionOrchestrator(accountRepo, requeuer, runtime, func(context.Context, *accounts.RuntimeAccountAccess, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		return providerOutputJSON(""), newUpstreamHTTPError(429, "rate limited")
	})

	assignment := riverTestAssignment(t, db, account, task.TaskID, task.LeaseToken)
	err := orchestrator.execute(ctx, &tasks.TaskPayload{
		TaskID: task.TaskID,
	}, assignment)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if requeuer.calls != 1 {
		t.Fatalf("requeue calls = %d, want 1", requeuer.calls)
	}

	updated, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if updated.Status != tasks.StatusPending {
		t.Fatalf("status = %s, want %s", updated.Status, tasks.StatusPending)
	}
	if updated.Result != "" {
		t.Fatalf("result = %q, want empty while retrying", updated.Result)
	}
	if updated.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", updated.RetryCount)
	}
	if updated.CompletedAt != nil {
		t.Fatalf("completed_at should remain nil, got %v", updated.CompletedAt)
	}
}

func TestExecuteProviderParseFailureFinalFailsWithFailureResultEnvelope(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "parse-primary-account")

	task := &tasks.Task{
		TaskID:                     "provider-parse-retry-task",
		UserID:                     1,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AssignedCredentialID:       tasks.IDPtr(loadRiverTestCredentialID(t, db, account.ID)),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-provider-parse-retry",
		RetryCount:                 0,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}

	failureResult := `{"task_id":"provider-parse-retry-task","status":"failed","parsed_output_status":"invalid","error":"图案生成输出不符合 workflow contract"}`
	assigner := &fakeAssigner{}
	requeuer := &fakeRequeuer{}
	runtime := taskruntime.NewService(accountRepo, taskRepo, assigner, newFakeCancelStoreForRiver())
	orchestrator := newTaskExecutionOrchestrator(accountRepo, requeuer, runtime, func(context.Context, *accounts.RuntimeAccountAccess, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		return providerOutputJSON(failureResult), newExecutionFailureError(
			tasks.ExecutionStageProvider,
			tasks.FailureCategoryProviderError,
			newProviderOutputParseError(errors.New("图案生成输出不符合 workflow contract")),
		)
	})

	assignment := riverTestAssignment(t, db, account, task.TaskID, task.LeaseToken)
	err := orchestrator.execute(ctx, &tasks.TaskPayload{
		TaskID: task.TaskID,
	}, assignment)
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
	if updated.Result != failureResult {
		t.Fatalf("result = %q, want parse failure envelope", updated.Result)
	}
	if !strings.Contains(updated.ErrorMsg, "workflow contract") {
		t.Fatalf("error_msg = %q, want parse contract reason", updated.ErrorMsg)
	}
	if updated.CompletedAt == nil {
		t.Fatal("completed_at should be set")
	}
}

func newTaskExecutionOrchestratorTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func providerOutputJSON(result string) providerCallOutput {
	now := time.Now()
	output := providerCallOutput{
		Result: result,
		Timeline: providerCallTimeline{
			ProviderStartedAt:  &now,
			ProviderFinishedAt: &now,
			FinalizeStartedAt:  &now,
			FinalizeFinishedAt: &now,
		},
	}
	var envelope struct {
		ImageURL string `json:"image_url"`
	}
	if json.Unmarshal([]byte(result), &envelope) == nil && strings.TrimSpace(envelope.ImageURL) != "" {
		output.Outputs = []tasks.TaskOutput{{
			OutputKind:  "remote",
			Role:        "image",
			URL:         envelope.ImageURL,
			ResultIndex: 0,
		}}
	}
	return output
}

type recordingTaskExecutionConvergenceDecider struct {
	inputs   []taskExecutionConvergenceInput
	outcomes []taskExecutionConvergenceOutcome
}

func (d *recordingTaskExecutionConvergenceDecider) DecideTaskExecutionConvergence(input taskExecutionConvergenceInput) taskExecutionConvergenceDecision {
	d.inputs = append(d.inputs, input)
	decision := taskExecutionConvergencePolicy{}.DecideTaskExecutionConvergence(input)
	index := len(d.inputs) - 1
	if index < len(d.outcomes) {
		decision.Outcome = d.outcomes[index]
	}
	return decision
}
