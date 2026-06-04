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
)

type providerCallFunc func(ctx context.Context, account *accounts.ApiAccount, task *queuetypes.TaskPayload) (string, error)

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

	result, err := o.providerCall(taskCtx, account, task)
	if err != nil {
		if isContextInterrupted(taskCtx.Err()) || isContextInterrupted(ctx.Err()) {
			cancelled, checkErr := o.taskRuntime.IsTaskCancelled(ctx, task.TaskID)
			if checkErr != nil {
				slog.Warn("check task cancellation failed", "task_id", task.TaskID, "err", checkErr)
			}
			if cancelled {
				o.skipCancelledTask(ctx, task, assignment)
				return nil
			}
			return o.handleExecutionInterrupted(ctx, task, assignment, err)
		}
		slog.Warn("upstream provider call failed", "task_id", task.TaskID, "account_id", accountID, "err", err)
		elapsed := time.Since(startTime).Seconds()
		reason := userFacingProviderError(err)
		if !shouldFailoverProviderError(err) {
			if markErr := o.completeFailedTask(ctx, task, assignment, reason, elapsed); markErr != nil {
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
			if markErr := o.completeFailedTask(ctx, task, assignment, reason, elapsed); markErr != nil {
				return markErr
			}
			return nil
		}
		if !requeued {
			slog.Info("task reached failover retry limit", "task_id", task.TaskID)
			return nil
		}
		disabled, failCount, statErr := o.taskRuntime.RecordAccountFailure(ctx, accountID)
		if statErr != nil {
			slog.Warn("record account failure state failed", "account_id", accountID, "err", statErr)
		}
		if disabled {
			slog.Warn("account disabled after too many failures", "account_id", accountID, "fail_count", failCount)
		}
		task.AccountIDs = removeAccountCandidate(task.AccountIDs, accountID)
		if len(task.AccountIDs) == 0 {
			if markErr := o.completeFailedTask(ctx, task, taskqueue.TaskAssignment{}, reason, elapsed); markErr != nil {
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
	completed, completeErr := o.completeTask(ctx, task, assignment, result, elapsed)
	if completeErr != nil {
		return completeErr
	}
	if completed {
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

func taskCleanupContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), taskCleanupTimeout)
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
	if strings.Contains(err.Error(), "持久化上游图片失败") {
		return "图片结果保存失败，请检查存储配置"
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
