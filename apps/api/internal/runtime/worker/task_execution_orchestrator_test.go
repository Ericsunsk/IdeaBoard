package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"ideaboard/api/internal/accountpool/accounts"
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

func TestExecuteProviderFailureRoutesThroughConvergenceDecision(t *testing.T) {
	ctx := context.Background()
	db := newTaskExecutionOrchestratorTestDB(t)
	accountRepo := accounts.NewAccountRepository(db)
	taskRepo := taskstore.NewRepository(db)
	account := createRiverTestAccount(t, db, accountRepo, "convergence-primary-account")
	fallbackAccount := createRiverTestAccount(t, db, accountRepo, "convergence-fallback-account")

	task := &tasks.Task{
		TaskID:            "convergence-provider-failure-task",
		UserID:            1,
		AssignedChannelID: tasks.IDPtr(account.ChannelID),
		AccountID:         tasks.IDPtr(account.ID),
		Status:            tasks.StatusRunning,
		LeaseToken:        "lease-convergence-provider-failure",
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
	orchestrator := newTaskExecutionOrchestrator(accountRepo, requeuer, runtime, func(context.Context, *accounts.ApiAccount, *tasks.TaskPayload, taskqueue.TaskAssignment) (providerCallOutput, error) {
		return providerOutputJSON(""), newUpstreamHTTPError(429, "rate limited")
	})
	orchestrator.convergenceDecider = decider

	err := orchestrator.execute(ctx, &tasks.TaskPayload{
		TaskID:     task.TaskID,
		AccountIDs: []int64{account.ID, fallbackAccount.ID},
	}, taskqueue.TaskAssignment{TaskID: task.TaskID, AccountID: account.ID, ChannelID: account.ChannelID, Status: taskqueue.AssignStatusAssigned, LeaseToken: task.LeaseToken})
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
