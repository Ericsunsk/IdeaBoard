package worker

import (
	"context"
	"encoding/json"
	"errors"
	"ideaboard/api/internal/tasks"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/runtime/taskruntime"
	"ideaboard/api/internal/taskqueue"
	"ideaboard/api/internal/workflow"
)

type providerCallFunc func(ctx context.Context, account *accounts.ApiAccount, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment) (providerCallOutput, error)

type accountLookupStore interface {
	GetRuntimeByIDWithSecret(ctx context.Context, id int64) (*accounts.ApiAccount, error)
}

type taskExecutionOrchestrator struct {
	accountRepo  accountLookupStore
	enqueuer     taskqueue.TaskEnqueuer
	taskRuntime  *taskruntime.Service
	providerCall providerCallFunc
}

const taskCleanupTimeout = 5 * time.Second

func newTaskExecutionOrchestrator(
	accountRepo accountLookupStore,
	enqueuer taskqueue.TaskEnqueuer,
	taskRuntime *taskruntime.Service,
	providerCall providerCallFunc,
) *taskExecutionOrchestrator {
	return &taskExecutionOrchestrator{
		accountRepo:  accountRepo,
		enqueuer:     enqueuer,
		taskRuntime:  taskRuntime,
		providerCall: providerCall,
	}
}

func (o *taskExecutionOrchestrator) execute(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment) error {
	// ── Phase 1: pre-flight (account + cancellation check) ──
	accountID := assignment.AccountID
	slog.Info("worker executing task", "task_id", task.TaskID, "account_id", accountID)
	startTime := time.Now()

	account, err := o.accountRepo.GetRuntimeByIDWithSecret(ctx, accountID)
	if err != nil {
		if requeueErr := o.requeueTask(ctx, task, assignment, "账号不存在，重新调度"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if account.Status != 1 {
		if requeueErr := o.requeueTask(ctx, task, assignment, "账号已停用，重新调度"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	account.ChannelID = assignment.ChannelID

	cancelled, err := o.taskRuntime.IsTaskCancelled(ctx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", err)
		if requeueErr := o.requeueTask(ctx, task, assignment, "检查任务取消状态失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if cancelled {
		o.skipCancelledTask(ctx, task, nil)
		return nil
	}

	// ── Phase 2: lease confirmation + snapshot + attempt start ──
	taskCtx, stop := o.taskRuntime.WatchTaskCancellation(ctx, task.TaskID)
	defer stop()

	if err := o.taskRuntime.ConfirmTaskLease(ctx, assignment); err != nil {
		slog.Warn("confirm task running lease failed", "task_id", task.TaskID, "account_id", accountID, "err", err)
		cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
		defer cleanupCancel()
		if resetErr := o.taskRuntime.ResetRunningTask(cleanupCtx, taskruntime.ResetRunningTaskInput{
			Assignment: assignment,
			Reason:     "确认任务运行租约失败",
		}); resetErr != nil {
			if isTaskConvergedError(resetErr) {
				o.dropTask(ctx, task, "任务状态已收敛，丢弃脏队列任务")
				return nil
			}
			return resetErr
		}
		if requeueErr := o.requeueTask(ctx, task, taskqueue.TaskAssignment{}, "确认任务运行租约失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}

	snapshot := taskExecutionSnapshot(task, account, assignment)
	o.recordExecutionSnapshot(ctx, task.TaskID, snapshot)

	providerStartAt := time.Now()
	attemptRef, err := o.startTaskAttempt(ctx, task, assignment, snapshot, providerStartAt)
	if err != nil {
		slog.Warn("start task attempt failed", "task_id", task.TaskID, "err", err)
		cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
		defer cleanupCancel()
		if resetErr := o.taskRuntime.ResetRunningTask(cleanupCtx, taskruntime.ResetRunningTaskInput{
			Assignment: assignment,
			Reason:     "记录任务执行尝试失败",
		}); resetErr != nil {
			if isTaskConvergedError(resetErr) {
				o.dropTask(ctx, task, "任务状态已收敛，丢弃脏队列任务")
				return nil
			}
			return resetErr
		}
		if requeueErr := o.requeueTask(ctx, task, taskqueue.TaskAssignment{}, "记录任务执行尝试失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	o.recordExecutionMetrics(ctx, task.TaskID, tasks.TaskExecutionMetricsPatch{
		TaskID:            task.TaskID,
		CurrentStage:      tasks.StringPtr(tasks.ExecutionStageProvider),
		ProviderStartedAt: &providerStartAt,
		WorkerOverheadMS:  int64Ptr(nonNegativeMilliseconds(providerStartAt.Sub(startTime))),
	})

	// ── Phase 3: upstream provider call ──
	providerOutput, err := o.providerCall(taskCtx, account, task, assignment)
	o.recordProviderTimeline(ctx, task.TaskID, startTime, providerOutput.Timeline)
	// ── Phase 4: provider error handling (interrupt / fail / failover) ──
	if err != nil {
		return o.handleProviderFailure(ctx, taskCtx, task, assignment, accountID, startTime, attemptRef, providerOutput, err)
	}

	// ── Phase 5: post-success cancel check + finalization ──
	checkCtx, checkCancel := taskCleanupContext(ctx)
	cancelled, err = o.taskRuntime.IsTaskCancelled(checkCtx, task.TaskID)
	checkCancel()
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", err)
		if requeueErr := o.requeueTask(ctx, task, assignment, "检查任务取消状态失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if cancelled {
		cancelFinish, _ := buildTaskAttemptFinish(attemptRef, tasks.TaskAttemptStatusCancelled, "任务已取消", tasks.ExecutionStageFinalizing, tasks.FailureCategoryCancelled, startTime, providerOutput.Timeline, time.Now())
		o.skipCancelledTask(ctx, task, &cancelFinish)
		return nil
	}

	elapsed := time.Since(startTime).Seconds()
	completedFinish, _ := buildTaskAttemptFinish(attemptRef, tasks.TaskAttemptStatusCompleted, "", "", "", startTime, providerOutput.Timeline, time.Now())
	completed, completeErr := o.completeTask(ctx, task, assignment, providerOutput.Result, providerOutput.Outputs, elapsed, &completedFinish)
	if completeErr != nil {
		o.finishTaskAttempt(ctx, attemptRef, tasks.TaskAttemptStatusFailed, "任务结果保存失败，请稍后重试", tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, startTime, providerOutput.Timeline, time.Now())
		o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageFailed, tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, time.Now())
		return completeErr
	}
	finishedAt := completedFinish.CompletedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	if completed {
		o.recordFinalizeFinished(ctx, task.TaskID, providerOutput.Timeline, finishedAt)
		o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageCompleted, "", "", finishedAt)
		if err := o.taskRuntime.ResetAccountFailure(ctx, accountID); err != nil {
			slog.Warn("reset account failure counter failed", "account_id", accountID, "err", err)
		}
	}
	return nil
}

func (o *taskExecutionOrchestrator) handleProviderFailure(
	ctx context.Context,
	taskCtx context.Context,
	task *tasks.TaskPayload,
	assignment taskqueue.TaskAssignment,
	accountID int64,
	startTime time.Time,
	attemptRef tasks.TaskAttemptRef,
	providerOutput providerCallOutput,
	err error,
) error {
	if isContextInterrupted(taskCtx.Err()) || isContextInterrupted(ctx.Err()) {
		checkCtx, checkCancel := taskCleanupContext(ctx)
		cancelled, checkErr := o.taskRuntime.IsTaskCancelled(checkCtx, task.TaskID)
		checkCancel()
		if checkErr != nil {
			slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", checkErr)
		}
		if cancelled {
			cancelFinish, _ := buildTaskAttemptFinish(attemptRef, tasks.TaskAttemptStatusCancelled, "任务已取消", tasks.ExecutionStageProvider, tasks.FailureCategoryCancelled, startTime, providerOutput.Timeline, time.Now())
			o.skipCancelledTask(ctx, task, &cancelFinish)
			return nil
		}
		requeueFinish, _ := buildTaskAttemptFinish(attemptRef, tasks.TaskAttemptStatusRequeued, err.Error(), tasks.ExecutionStageProvider, tasks.FailureCategoryUnknown, startTime, providerOutput.Timeline, time.Now())
		return o.handleExecutionInterrupted(ctx, task, assignment, err, &requeueFinish)
	}

	slog.Warn("upstream provider call failed", "task_id", task.TaskID, "account_id", accountID, "err", err)
	elapsed := time.Since(startTime).Seconds()
	reason := userFacingProviderError(err)
	failureStage, failureCategory := classifyProviderFailure(err, providerOutput.Timeline)
	if !shouldFailoverProviderError(err) {
		failFinish, _ := buildTaskAttemptFinish(attemptRef, tasks.TaskAttemptStatusFailed, reason, failureStage, failureCategory, startTime, providerOutput.Timeline, time.Now())
		if markErr := o.completeFailedTaskWithMetricsAndAttempt(ctx, task, assignment, reason, elapsed, failureStage, failureCategory, &failFinish); markErr != nil {
			return markErr
		}
		return nil
	}

	finalFailFinish, _ := buildTaskAttemptFinish(attemptRef, tasks.TaskAttemptStatusFailed, reason, failureStage, failureCategory, startTime, providerOutput.Timeline, time.Now())
	requeueFinish, _ := buildTaskAttemptFinish(attemptRef, tasks.TaskAttemptStatusRequeued, reason, failureStage, failureCategory, startTime, providerOutput.Timeline, time.Now())
	disabled, failCount, statErr := o.taskRuntime.RecordAccountFailure(ctx, accountID)
	if statErr != nil {
		slog.Warn("record account failure state failed", "account_id", accountID, "err", statErr)
	}
	if disabled {
		slog.Warn("account disabled after too many failures", "account_id", accountID, "fail_count", failCount)
	}
	task.AccountIDs = removeAccountCandidate(task.AccountIDs, accountID)
	if len(task.AccountIDs) == 0 {
		if markErr := o.completeFailedTaskWithMetricsAndAttempt(ctx, task, assignment, reason, elapsed, failureStage, failureCategory, &finalFailFinish); markErr != nil {
			return markErr
		}
		return nil
	}

	requeued, requeueErr := o.taskRuntime.RequeueTaskForFailover(ctx, taskruntime.RequeueForFailoverInput{
		Assignment:             assignment,
		Reason:                 reason,
		RequeuedAttemptFinish:  &requeueFinish,
		FinalFailAttemptFinish: &finalFailFinish,
	})
	if requeueErr != nil {
		if errors.Is(requeueErr, taskruntime.ErrTaskNotFound) || errors.Is(requeueErr, taskruntime.ErrTaskTerminal) || errors.Is(requeueErr, taskruntime.ErrTaskStateConflict) {
			o.finishTaskAttempt(ctx, attemptRef, tasks.TaskAttemptStatusFailed, reason, failureStage, failureCategory, startTime, providerOutput.Timeline, time.Now())
			o.dropTask(ctx, task, "任务状态已收敛，丢弃脏队列任务")
			return nil
		}
		slog.Warn("reset task for failover retry failed", "task_id", task.TaskID, "err", requeueErr)
		if markErr := o.completeFailedTaskWithMetricsAndAttempt(ctx, task, assignment, reason, elapsed, failureStage, failureCategory, &finalFailFinish); markErr != nil {
			return markErr
		}
		return nil
	}
	if !requeued {
		slog.Info("task reached failover retry limit", "task_id", task.TaskID)
		o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageFailed, failureStage, failureCategory, finalFailFinish.CompletedAt)
		return nil
	}
	o.recordExecutionMetrics(ctx, task.TaskID, tasks.TaskExecutionMetricsPatch{
		TaskID:           task.TaskID,
		CurrentStage:     tasks.StringPtr(tasks.ExecutionStageQueued),
		BottleneckSource: tasks.StringPtr(tasks.BottleneckSourceProvider),
		FailureStage:     tasks.StringPtr(failureStage),
		FailureCategory:  tasks.StringPtr(failureCategory),
	})
	if requeueErr := o.requeueTask(ctx, task, taskqueue.TaskAssignment{}, reason); requeueErr != nil {
		o.finishTaskAttempt(ctx, attemptRef, tasks.TaskAttemptStatusFailed, reason, failureStage, failureCategory, startTime, providerOutput.Timeline, time.Now())
		return requeueErr
	}
	return nil
}

func (o *taskExecutionOrchestrator) requeueTask(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment, reason string) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	cancelled, err := o.taskRuntime.IsTaskCancelled(cleanupCtx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", err)
	} else if cancelled {
		o.skipCancelledTask(cleanupCtx, task, nil)
		return nil
	}
	if assignment.AccountID != 0 && assignment.LeaseToken != "" {
		if err := o.taskRuntime.ResetRunningTask(cleanupCtx, taskruntime.ResetRunningTaskInput{
			Assignment: assignment,
			Reason:     reason,
		}); err != nil && !isTaskConvergedError(err) {
			slog.Warn("reset task running to pending failed", "task_id", task.TaskID, "account_id", assignment.AccountID, "err", err)
		}
	}
	cancelled, err = o.taskRuntime.IsTaskCancelled(cleanupCtx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", err)
	} else if cancelled {
		o.skipCancelledTask(cleanupCtx, task, nil)
		return nil
	}
	if err := o.taskRuntime.RefreshTaskAccounts(ctx, task); err != nil {
		return o.failTask(cleanupCtx, task, reason)
	}
	if len(task.AccountIDs) == 0 {
		return o.failTask(cleanupCtx, task, reason)
	}
	if err := o.enqueuer.Enqueue(ctx, task); err != nil {
		return o.failTask(cleanupCtx, task, reason)
	}
	return nil
}

func (o *taskExecutionOrchestrator) completeTask(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment, result string, outputs []tasks.TaskOutput, elapsedSec float64, attemptFinish *tasks.TaskAttemptFinish) (bool, error) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	var err error
	err = o.taskRuntime.CompleteTask(cleanupCtx, taskruntime.CompleteTaskInput{
		Assignment:    assignment,
		Result:        result,
		Outputs:       outputs,
		AttemptFinish: attemptFinish,
	})
	if err != nil {
		if isTaskConvergedError(err) {
			slog.Info("task converged before completion writeback", "task_id", task.TaskID, "err", err)
			return false, nil
		}
		slog.Warn("mark task completed failed", "task_id", task.TaskID, "err", err)
		if failErr := o.taskRuntime.FailTask(cleanupCtx, taskruntime.FailTaskInput{
			Assignment: assignment,
			Reason:     "任务结果保存失败，请稍后重试",
		}); failErr != nil {
			slog.Warn("mark task failed after completion writeback failed", "task_id", task.TaskID, "err", failErr)
		}
		return false, err
	}
	slog.Info("task completed", "task_id", task.TaskID, "elapsed_sec", elapsedSec)
	return true, nil
}

func (o *taskExecutionOrchestrator) completeFailedTask(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment, reason string, elapsedSec float64) error {
	return o.completeFailedTaskWithMetrics(ctx, task, assignment, reason, elapsedSec, tasks.ExecutionStageAssigned, tasks.FailureCategoryUnknown)
}

func (o *taskExecutionOrchestrator) completeFailedTaskWithMetrics(
	ctx context.Context,
	task *tasks.TaskPayload,
	assignment taskqueue.TaskAssignment,
	reason string,
	elapsedSec float64,
	failureStage string,
	failureCategory string,
) error {
	return o.completeFailedTaskWithMetricsAndAttempt(ctx, task, assignment, reason, elapsedSec, failureStage, failureCategory, nil)
}

func (o *taskExecutionOrchestrator) completeFailedTaskWithMetricsAndAttempt(
	ctx context.Context,
	task *tasks.TaskPayload,
	assignment taskqueue.TaskAssignment,
	reason string,
	elapsedSec float64,
	failureStage string,
	failureCategory string,
	attemptFinish *tasks.TaskAttemptFinish,
) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	err := o.taskRuntime.FailTask(cleanupCtx, taskruntime.FailTaskInput{
		TaskID:        task.TaskID,
		Assignment:    assignment,
		Reason:        reason,
		AttemptFinish: attemptFinish,
	})
	if err != nil {
		if isTaskConvergedError(err) {
			slog.Info("task converged before failure writeback", "task_id", task.TaskID, "err", err)
			return nil
		}
		slog.Warn("mark task failed failed", "task_id", task.TaskID, "err", err)
		return err
	}
	terminalAt := time.Now()
	if attemptFinish != nil && !attemptFinish.CompletedAt.IsZero() {
		terminalAt = attemptFinish.CompletedAt
	}
	o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageFailed, failureStage, failureCategory, terminalAt)
	slog.Warn("task failed", "task_id", task.TaskID, "elapsed_sec", elapsedSec, "reason", reason)
	return nil
}

func (o *taskExecutionOrchestrator) failTask(ctx context.Context, task *tasks.TaskPayload, reason string) error {
	return o.completeFailedTask(ctx, task, taskqueue.TaskAssignment{}, reason, 0)
}

func (o *taskExecutionOrchestrator) skipCancelledTask(ctx context.Context, task *tasks.TaskPayload, attemptFinish *tasks.TaskAttemptFinish) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	cancelledAt := time.Now()
	if attemptFinish != nil && !attemptFinish.CompletedAt.IsZero() {
		cancelledAt = attemptFinish.CompletedAt
	}
	snapshot := tasks.TaskCancellationSnapshot{
		TaskID:      task.TaskID,
		CancelledAt: cancelledAt,
		Reason:      "任务已取消",
	}
	var err error
	if attemptFinish != nil {
		err = o.taskRuntime.RecordTaskCancelledWithAttempt(cleanupCtx, snapshot, *attemptFinish)
	} else {
		err = o.taskRuntime.RecordTaskCancelled(cleanupCtx, snapshot)
	}
	if err != nil {
		slog.Warn("record task cancellation snapshot failed", "task_id", task.TaskID, "err", err)
	}
	if err := o.taskRuntime.ClearTaskCancelled(cleanupCtx, task.TaskID); err != nil {
		slog.Warn("clear task cancellation marker failed", "task_id", task.TaskID, "err", err)
	}
	slog.Info("task cancelled; stop execution", "task_id", task.TaskID)
}

func (o *taskExecutionOrchestrator) dropTask(ctx context.Context, task *tasks.TaskPayload, reason string) {
	slog.Info("task dropped", "task_id", task.TaskID, "reason", reason)
}

func (o *taskExecutionOrchestrator) handleExecutionInterrupted(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment, execErr error, attemptFinish *tasks.TaskAttemptFinish) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	err := o.taskRuntime.ResetRunningTask(cleanupCtx, taskruntime.ResetRunningTaskInput{
		Assignment:    assignment,
		Reason:        "worker context interrupted, waiting retry",
		AttemptFinish: attemptFinish,
	})
	if err != nil {
		if errors.Is(err, taskruntime.ErrTaskNotFound) || errors.Is(err, taskruntime.ErrTaskTerminal) || errors.Is(err, taskruntime.ErrTaskStateConflict) {
			slog.Info("interrupted task already converged", "task_id", task.TaskID, "err", err)
		} else {
			slog.Warn("interrupted task reset failed", "task_id", task.TaskID, "err", err)
		}
	}
	if execErr == nil {
		return context.Canceled
	}
	return execErr
}

func (o *taskExecutionOrchestrator) recordProviderTimeline(ctx context.Context, taskID string, executionStartedAt time.Time, timeline providerCallTimeline) {
	if timeline.ProviderStartedAt != nil {
		o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
			TaskID:            taskID,
			CurrentStage:      tasks.StringPtr(tasks.ExecutionStageProvider),
			ProviderStartedAt: timeline.ProviderStartedAt,
			WorkerOverheadMS:  int64Ptr(nonNegativeMilliseconds(timeline.ProviderStartedAt.Sub(executionStartedAt))),
		})
	}
	if timeline.ProviderStartedAt != nil && timeline.ProviderFinishedAt != nil {
		o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
			TaskID:             taskID,
			ProviderFinishedAt: timeline.ProviderFinishedAt,
			ProviderLatencyMS:  int64Ptr(nonNegativeMilliseconds(timeline.ProviderFinishedAt.Sub(*timeline.ProviderStartedAt))),
		})
	}
	if timeline.FinalizeStartedAt != nil {
		o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
			TaskID:            taskID,
			CurrentStage:      tasks.StringPtr(tasks.ExecutionStageFinalizing),
			FinalizeStartedAt: timeline.FinalizeStartedAt,
		})
	}
	if timeline.FinalizeStartedAt != nil && timeline.FinalizeFinishedAt != nil {
		o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
			TaskID:             taskID,
			FinalizeFinishedAt: timeline.FinalizeFinishedAt,
			FinalizeLatencyMS:  int64Ptr(nonNegativeMilliseconds(timeline.FinalizeFinishedAt.Sub(*timeline.FinalizeStartedAt))),
		})
	}
}

func (o *taskExecutionOrchestrator) recordFinalizeFinished(ctx context.Context, taskID string, timeline providerCallTimeline, finishedAt time.Time) {
	if timeline.FinalizeStartedAt == nil {
		return
	}
	o.recordExecutionMetrics(ctx, taskID, tasks.TaskExecutionMetricsPatch{
		TaskID:             taskID,
		FinalizeFinishedAt: &finishedAt,
		FinalizeLatencyMS:  int64Ptr(nonNegativeMilliseconds(finishedAt.Sub(*timeline.FinalizeStartedAt))),
	})
}

func (o *taskExecutionOrchestrator) recordExecutionMetrics(ctx context.Context, taskID string, patch tasks.TaskExecutionMetricsPatch) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()
	if err := o.taskRuntime.RecordExecutionMetrics(cleanupCtx, patch); err != nil {
		slog.Warn("record task execution metrics failed", "task_id", taskID, "err", err)
	}
}

func (o *taskExecutionOrchestrator) recordExecutionTerminal(ctx context.Context, taskID string, currentStage string, failureStage string, failureCategory string, finishedAt time.Time) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()
	if err := o.taskRuntime.RecordExecutionTerminal(cleanupCtx, taskID, currentStage, failureStage, failureCategory, finishedAt); err != nil {
		slog.Warn("record terminal task execution metrics failed", "task_id", taskID, "err", err)
	}
}

func (o *taskExecutionOrchestrator) recordExecutionSnapshot(ctx context.Context, taskID string, snapshot tasks.TaskExecutionSnapshot) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()
	if err := o.taskRuntime.RecordExecutionSnapshot(cleanupCtx, snapshot); err != nil {
		slog.Warn("record task execution snapshot failed", "task_id", taskID, "err", err)
	}
}

func (o *taskExecutionOrchestrator) startTaskAttempt(
	ctx context.Context,
	task *tasks.TaskPayload,
	assignment taskqueue.TaskAssignment,
	snapshot tasks.TaskExecutionSnapshot,
	providerStartedAt time.Time,
) (tasks.TaskAttemptRef, error) {
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return tasks.TaskAttemptRef{}, nil
	}
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()
	ref, err := o.taskRuntime.StartTaskAttempt(cleanupCtx, tasks.TaskAttemptStart{
		TaskID:            task.TaskID,
		RiverJobID:        task.RiverJobID,
		LeaseToken:        assignment.LeaseToken,
		AccountID:         assignment.AccountID,
		ChannelID:         assignment.ChannelID,
		FeatureSlug:       snapshot.FeatureSlug,
		RequestedModel:    snapshot.RequestedModel,
		ResolvedModel:     snapshot.ResolvedModel,
		APIType:           snapshot.APIType,
		ProviderStartedAt: providerStartedAt,
	})
	if err != nil {
		return tasks.TaskAttemptRef{}, err
	}
	return ref, nil
}

func buildTaskAttemptFinish(
	ref tasks.TaskAttemptRef,
	status string,
	reason string,
	failureStage string,
	failureCategory string,
	executionStartedAt time.Time,
	timeline providerCallTimeline,
	completedAt time.Time,
) (tasks.TaskAttemptFinish, bool) {
	if ref.ID == 0 {
		return tasks.TaskAttemptFinish{}, false
	}
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	providerLatencyMS := int64(0)
	if timeline.ProviderStartedAt != nil && timeline.ProviderFinishedAt != nil {
		providerLatencyMS = nonNegativeMilliseconds(timeline.ProviderFinishedAt.Sub(*timeline.ProviderStartedAt))
	}
	finalizeLatencyMS := int64(0)
	if timeline.FinalizeStartedAt != nil && timeline.FinalizeFinishedAt != nil {
		finalizeLatencyMS = nonNegativeMilliseconds(timeline.FinalizeFinishedAt.Sub(*timeline.FinalizeStartedAt))
	}
	totalLatencyMS := int64(0)
	if !executionStartedAt.IsZero() {
		totalLatencyMS = nonNegativeMilliseconds(completedAt.Sub(executionStartedAt))
	}
	return tasks.TaskAttemptFinish{
		ID:                 ref.ID,
		Status:             status,
		FailureStage:       failureStage,
		FailureCategory:    failureCategory,
		ErrorMsg:           truncateErrorBody(reason),
		ProviderFinishedAt: timeline.ProviderFinishedAt,
		FinalizeStartedAt:  timeline.FinalizeStartedAt,
		FinalizeFinishedAt: timeline.FinalizeFinishedAt,
		ProviderLatencyMS:  providerLatencyMS,
		FinalizeLatencyMS:  finalizeLatencyMS,
		TotalLatencyMS:     totalLatencyMS,
		CompletedAt:        completedAt,
	}, true
}

func (o *taskExecutionOrchestrator) finishTaskAttempt(
	ctx context.Context,
	ref tasks.TaskAttemptRef,
	status string,
	reason string,
	failureStage string,
	failureCategory string,
	executionStartedAt time.Time,
	timeline providerCallTimeline,
	completedAt time.Time,
) {
	finish, ok := buildTaskAttemptFinish(ref, status, reason, failureStage, failureCategory, executionStartedAt, timeline, completedAt)
	if !ok {
		return
	}
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()
	if err := o.taskRuntime.FinishTaskAttempt(cleanupCtx, finish); err != nil {
		if isTaskConvergedError(err) {
			slog.Info("task attempt already converged before finish", "attempt_id", ref.ID, "err", err)
			return
		}
		slog.Warn("finish task attempt failed", "attempt_id", ref.ID, "err", err)
	}
}

func taskExecutionSnapshot(task *tasks.TaskPayload, account *accounts.ApiAccount, assignment taskqueue.TaskAssignment) tasks.TaskExecutionSnapshot {
	snapshot := tasks.TaskExecutionSnapshot{}
	if task == nil {
		return snapshot
	}
	snapshot.TaskID = task.TaskID
	snapshot.FeatureSlug = accounts.ParseFeatureSlugFromParamsJSON(task.Params)
	params := taskParamsMap(task.Params)
	if requestedModel, ok := workflow.ReadWorkflowStringParam(params, "model"); ok {
		snapshot.RequestedModel = strings.TrimSpace(requestedModel)
	}
	if account == nil {
		return snapshot
	}
	if apiType, ok := workflow.ValidateProviderAPIType(account.ApiType, account.ProxyURL); ok {
		snapshot.APIType = apiType
	}
	if resolvedModel := strings.TrimSpace(assignment.ResolvedModel); resolvedModel != "" {
		snapshot.ResolvedModel = resolvedModel
		return snapshot
	}
	if model, ok := accounts.ResolveAccountModelForParams(account, snapshot.FeatureSlug, params); ok {
		snapshot.ResolvedModel = strings.TrimSpace(model.Name)
	}
	return snapshot
}

func taskParamsMap(paramsJSON string) map[string]interface{} {
	paramsJSON = strings.TrimSpace(paramsJSON)
	if paramsJSON == "" {
		return nil
	}
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return nil
	}
	return params
}

func classifyProviderFailure(err error, timeline providerCallTimeline) (string, string) {
	if errors.Is(err, context.Canceled) {
		return tasks.ExecutionStageProvider, tasks.FailureCategoryCancelled
	}
	if stage, category, ok := executionFailureDetails(err); ok {
		return stage, category
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return tasks.ExecutionStageProvider, tasks.FailureCategoryTimeout
	}
	if timeline.FinalizeStartedAt != nil {
		return tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError
	}
	if status, ok := providerHTTPStatus(err); ok {
		switch status {
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return tasks.ExecutionStageProvider, tasks.FailureCategoryInputInvalid
		case http.StatusUnauthorized, http.StatusForbidden:
			return tasks.ExecutionStageProvider, tasks.FailureCategoryAuth
		case http.StatusPaymentRequired:
			return tasks.ExecutionStageProvider, tasks.FailureCategoryQuota
		case http.StatusRequestTimeout, http.StatusTooManyRequests:
			return tasks.ExecutionStageProvider, tasks.FailureCategoryTimeout
		}
	}
	if timeline.ProviderStartedAt != nil {
		return tasks.ExecutionStageProvider, tasks.FailureCategoryProviderError
	}
	return tasks.ExecutionStageAssigned, tasks.FailureCategoryUnknown
}

func taskCleanupContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), taskCleanupTimeout)
}

func nonNegativeMilliseconds(duration time.Duration) int64 {
	ms := duration.Milliseconds()
	if ms < 0 {
		return 0
	}
	return ms
}

func int64Ptr(value int64) *int64 {
	return &value
}

func isContextInterrupted(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func isTaskConvergedError(err error) bool {
	return errors.Is(err, taskruntime.ErrTaskNotFound) || errors.Is(err, taskruntime.ErrTaskTerminal) || errors.Is(err, taskruntime.ErrTaskStateConflict)
}

func userFacingProviderError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "上游图像服务响应超时"
	}
	if status, ok := providerHTTPStatus(err); ok {
		switch status {
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return "请求参数不合法，请检查输入后重试"
		case http.StatusUnauthorized, http.StatusForbidden:
			return "上游服务鉴权失败，请联系管理员"
		case http.StatusPaymentRequired:
			return "上游服务额度不足或未开通，请联系管理员"
		case http.StatusNotFound:
			return "上游服务接口或模型不可用，请联系管理员"
		case http.StatusTooManyRequests:
			return "上游服务请求过于频繁，请稍后重试"
		}
	}
	if _, category, ok := executionFailureDetails(err); ok {
		switch category {
		case tasks.FailureCategoryStorageError, tasks.FailureCategoryFinalizeError:
			return "图片结果保存失败，请检查存储配置"
		case tasks.FailureCategoryAuth:
			return "上游服务鉴权失败，请联系管理员"
		case tasks.FailureCategoryInputInvalid:
			return "请求参数不合法，请检查输入后重试"
		case tasks.FailureCategoryQuota:
			return "上游服务额度不足或未开通，请联系管理员"
		case tasks.FailureCategoryTimeout:
			return "上游图像服务响应超时"
		}
	}
	return "上游图像服务调用失败"
}

func removeAccountCandidate(accountIDs []int64, accountID int64) []int64 {
	if len(accountIDs) == 0 {
		return accountIDs
	}
	next := make([]int64, 0, len(accountIDs))
	for _, id := range accountIDs {
		if id == accountID {
			continue
		}
		next = append(next, id)
	}
	return next
}
