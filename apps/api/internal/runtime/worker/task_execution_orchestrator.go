package worker

import (
	"context"
	"errors"
	"ideaboard/api/internal/runtime/queuetypes"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/accounts"
	"ideaboard/api/internal/runtime/taskqueue"
	"ideaboard/api/internal/runtime/taskruntime"
	"ideaboard/api/internal/tasks"
)

type providerCallFunc func(ctx context.Context, account *accounts.ApiAccount, task *queuetypes.TaskPayload) (providerCallOutput, error)

type accountLookupStore interface {
	GetByID(ctx context.Context, id int64) (*accounts.ApiAccount, error)
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

func (o *taskExecutionOrchestrator) execute(ctx context.Context, task *queuetypes.TaskPayload, assignment taskqueue.TaskAssignment) error {
	accountID := assignment.AccountID
	slog.Info("worker executing task", "task_id", task.TaskID, "account_id", accountID)
	startTime := time.Now()

	account, err := o.accountRepo.GetByID(ctx, accountID)
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

	cancelled, err := o.taskRuntime.IsTaskCancelled(ctx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", err)
		if requeueErr := o.requeueTask(ctx, task, assignment, "检查任务取消状态失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if cancelled {
		o.skipCancelledTask(ctx, task, assignment)
		return nil
	}

	taskCtx, stop := o.taskRuntime.WatchTaskCancellation(ctx, task.TaskID)
	defer stop()

	if err := o.taskRuntime.ConfirmTaskLease(ctx, assignment); err != nil {
		slog.Warn("confirm task running lease failed", "task_id", task.TaskID, "account_id", accountID, "err", err)
		cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
		defer cleanupCancel()
		if resetErr := o.taskRuntime.ResetTaskRunningToPending(cleanupCtx, task.TaskID, accountID, assignment.LeaseToken, "确认任务运行租约失败"); resetErr != nil {
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

	providerStartAt := time.Now()
	o.recordExecutionMetrics(ctx, task.TaskID, tasks.TaskExecutionMetricsPatch{
		TaskID:            task.TaskID,
		CurrentStage:      stringPtr(tasks.ExecutionStageProvider),
		ProviderStartedAt: &providerStartAt,
		WorkerOverheadMS:  int64Ptr(nonNegativeMilliseconds(providerStartAt.Sub(startTime))),
	})

	providerOutput, err := o.providerCall(taskCtx, account, task)
	o.recordProviderTimeline(ctx, task.TaskID, startTime, providerOutput.Timeline)
	if err != nil {
		if isContextInterrupted(taskCtx.Err()) || isContextInterrupted(ctx.Err()) {
			cancelled, checkErr := o.taskRuntime.IsTaskCancelled(ctx, task.TaskID)
			if checkErr != nil {
				slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", checkErr)
			}
			if cancelled {
				o.recordExecutionMetrics(ctx, task.TaskID, tasks.TaskExecutionMetricsPatch{
					TaskID:           task.TaskID,
					CurrentStage:     stringPtr(tasks.ExecutionStageQueued),
					FailureStage:     stringPtr(tasks.ExecutionStageProvider),
					FailureCategory:  stringPtr(tasks.FailureCategoryCancelled),
					BottleneckSource: stringPtr(tasks.BottleneckSourceProvider),
				})
				o.skipCancelledTask(ctx, task, assignment)
				return nil
			}
			return o.handleExecutionInterrupted(ctx, task, assignment, err)
		}
		slog.Warn("upstream provider call failed", "task_id", task.TaskID, "account_id", accountID, "err", err)
		elapsed := time.Since(startTime).Seconds()
		reason := userFacingProviderError(err)
		failureStage, failureCategory := classifyProviderFailure(err, providerOutput.Timeline)
		if !shouldFailoverProviderError(err) {
			if markErr := o.completeFailedTaskWithMetrics(ctx, task, assignment, reason, elapsed, failureStage, failureCategory); markErr != nil {
				return markErr
			}
			return nil
		}
		requeued, requeueErr := o.taskRuntime.RequeueTaskForFailover(ctx, task.TaskID, accountID, assignment.LeaseToken, reason)
		if requeueErr != nil {
			if errors.Is(requeueErr, taskruntime.ErrTaskNotFound) || errors.Is(requeueErr, taskruntime.ErrTaskTerminal) || errors.Is(requeueErr, taskruntime.ErrTaskStateConflict) {
				o.dropTask(ctx, task, "任务状态已收敛，丢弃脏队列任务")
				return nil
			}
			slog.Warn("reset task for failover retry failed", "task_id", task.TaskID, "err", requeueErr)
			if markErr := o.completeFailedTaskWithMetrics(ctx, task, assignment, reason, elapsed, failureStage, failureCategory); markErr != nil {
				return markErr
			}
			return nil
		}
		if !requeued {
			slog.Info("task reached failover retry limit", "task_id", task.TaskID)
			o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageFailed, failureStage, failureCategory, time.Now())
			return nil
		}
		o.recordExecutionMetrics(ctx, task.TaskID, tasks.TaskExecutionMetricsPatch{
			TaskID:           task.TaskID,
			CurrentStage:     stringPtr(tasks.ExecutionStageQueued),
			BottleneckSource: stringPtr(tasks.BottleneckSourceProvider),
			FailureStage:     stringPtr(failureStage),
			FailureCategory:  stringPtr(failureCategory),
		})
		disabled, failCount, statErr := o.taskRuntime.RecordAccountFailure(ctx, accountID)
		if statErr != nil {
			slog.Warn("record account failure state failed", "account_id", accountID, "err", statErr)
		}
		if disabled {
			slog.Warn("account disabled after too many failures", "account_id", accountID, "fail_count", failCount)
		}
		task.AccountIDs = removeAccountCandidate(task.AccountIDs, accountID)
		if len(task.AccountIDs) == 0 {
			if markErr := o.completeFailedTaskWithMetrics(ctx, task, taskqueue.TaskAssignment{}, reason, elapsed, failureStage, failureCategory); markErr != nil {
				return markErr
			}
			return nil
		}
		if requeueErr := o.requeueTask(ctx, task, taskqueue.TaskAssignment{}, reason); requeueErr != nil {
			return requeueErr
		}
		return nil
	}

	cancelled, err = o.taskRuntime.IsTaskCancelled(ctx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", err)
		if requeueErr := o.requeueTask(ctx, task, assignment, "检查任务取消状态失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if cancelled {
		o.skipCancelledTask(ctx, task, assignment)
		return nil
	}

	elapsed := time.Since(startTime).Seconds()
	completed, completeErr := o.completeTask(ctx, task, assignment, providerOutput.Result, elapsed)
	if completeErr != nil {
		o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageFailed, tasks.ExecutionStageFinalizing, tasks.FailureCategoryFinalizeError, time.Now())
		return completeErr
	}
	if completed {
		finishedAt := time.Now()
		o.recordFinalizeFinished(ctx, task.TaskID, providerOutput.Timeline, finishedAt)
		o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageCompleted, "", "", finishedAt)
		if err := o.taskRuntime.ResetAccountFailure(ctx, accountID); err != nil {
			slog.Warn("reset account failure counter failed", "account_id", accountID, "err", err)
		}
	}
	return nil
}

func (o *taskExecutionOrchestrator) requeueTask(ctx context.Context, task *queuetypes.TaskPayload, assignment taskqueue.TaskAssignment, reason string) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	if assignment.AccountID != 0 && assignment.LeaseToken != "" {
		if err := o.taskRuntime.ResetTaskRunningToPending(cleanupCtx, task.TaskID, assignment.AccountID, assignment.LeaseToken, reason); err != nil && !isTaskConvergedError(err) {
			slog.Warn("reset task running to pending failed", "task_id", task.TaskID, "account_id", assignment.AccountID, "err", err)
		}
	}
	cancelled, err := o.taskRuntime.IsTaskCancelled(cleanupCtx, task.TaskID)
	if err != nil {
		slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", err)
	} else if cancelled {
		o.skipCancelledTask(cleanupCtx, task, assignment)
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

func (o *taskExecutionOrchestrator) completeTask(ctx context.Context, task *queuetypes.TaskPayload, assignment taskqueue.TaskAssignment, result string, elapsedSec float64) (bool, error) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	if err := o.taskRuntime.MarkTaskCompleted(cleanupCtx, task.TaskID, assignment.LeaseToken, result); err != nil {
		if isTaskConvergedError(err) {
			slog.Info("task converged before completion writeback", "task_id", task.TaskID, "err", err)
			return false, nil
		}
		slog.Warn("mark task completed failed", "task_id", task.TaskID, "err", err)
		if failErr := o.taskRuntime.MarkTaskFailedWithLease(cleanupCtx, task.TaskID, assignment.LeaseToken, "任务结果保存失败，请稍后重试"); failErr != nil {
			slog.Warn("mark task failed after completion writeback failed", "task_id", task.TaskID, "err", failErr)
		}
		return false, err
	}
	slog.Info("task completed", "task_id", task.TaskID, "elapsed_sec", elapsedSec)
	return true, nil
}

func (o *taskExecutionOrchestrator) completeFailedTask(ctx context.Context, task *queuetypes.TaskPayload, assignment taskqueue.TaskAssignment, reason string, elapsedSec float64) error {
	return o.completeFailedTaskWithMetrics(ctx, task, assignment, reason, elapsedSec, tasks.ExecutionStageAssigned, tasks.FailureCategoryUnknown)
}

func (o *taskExecutionOrchestrator) completeFailedTaskWithMetrics(
	ctx context.Context,
	task *queuetypes.TaskPayload,
	assignment taskqueue.TaskAssignment,
	reason string,
	elapsedSec float64,
	failureStage string,
	failureCategory string,
) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	var err error
	if assignment.LeaseToken != "" {
		err = o.taskRuntime.MarkTaskFailedWithLease(cleanupCtx, task.TaskID, assignment.LeaseToken, reason)
	} else {
		err = o.taskRuntime.MarkTaskFailed(cleanupCtx, task.TaskID, reason)
	}
	if err != nil {
		if isTaskConvergedError(err) {
			slog.Info("task converged before failure writeback", "task_id", task.TaskID, "err", err)
			return nil
		}
		slog.Warn("mark task failed failed", "task_id", task.TaskID, "err", err)
		return err
	}
	o.recordExecutionTerminal(ctx, task.TaskID, tasks.ExecutionStageFailed, failureStage, failureCategory, time.Now())
	slog.Warn("task failed", "task_id", task.TaskID, "elapsed_sec", elapsedSec, "reason", reason)
	return nil
}

func (o *taskExecutionOrchestrator) failTask(ctx context.Context, task *queuetypes.TaskPayload, reason string) error {
	return o.completeFailedTask(ctx, task, taskqueue.TaskAssignment{}, reason, 0)
}

func (o *taskExecutionOrchestrator) skipCancelledTask(ctx context.Context, task *queuetypes.TaskPayload, assignment taskqueue.TaskAssignment) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	if assignment.AccountID != 0 && strings.TrimSpace(assignment.LeaseToken) != "" {
		if err := o.taskRuntime.ResetTaskRunningToPending(cleanupCtx, task.TaskID, assignment.AccountID, assignment.LeaseToken, "任务已取消，释放运行租约"); err != nil && !isTaskConvergedError(err) {
			slog.Warn("reset cancelled task running lease failed", "task_id", task.TaskID, "account_id", assignment.AccountID, "err", err)
		}
	}
	if err := o.taskRuntime.ClearTaskCancelled(cleanupCtx, task.TaskID); err != nil {
		slog.Warn("clear task cancellation marker failed", "task_id", task.TaskID, "err", err)
	}
	slog.Info("task cancelled; stop execution", "task_id", task.TaskID)
}

func (o *taskExecutionOrchestrator) dropTask(ctx context.Context, task *queuetypes.TaskPayload, reason string) {
	slog.Info("task dropped", "task_id", task.TaskID, "reason", reason)
}

func (o *taskExecutionOrchestrator) handleExecutionInterrupted(ctx context.Context, task *queuetypes.TaskPayload, assignment taskqueue.TaskAssignment, execErr error) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	if err := o.taskRuntime.ResetTaskRunningToPending(cleanupCtx, task.TaskID, assignment.AccountID, assignment.LeaseToken, "worker context interrupted, waiting retry"); err != nil {
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
			CurrentStage:      stringPtr(tasks.ExecutionStageProvider),
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
			CurrentStage:      stringPtr(tasks.ExecutionStageFinalizing),
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

func stringPtr(value string) *string {
	return &value
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
