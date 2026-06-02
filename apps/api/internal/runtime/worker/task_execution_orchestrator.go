package worker

import (
	"api/internal/runtime/queuetypes"
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"api/internal/accounts"
	"api/internal/runtime/taskqueue"
	"api/internal/runtime/taskruntime"
)

type providerCallFunc func(ctx context.Context, account *accounts.ApiAccount, task *queuetypes.TaskPayload) (string, error)

type taskExecutionOrchestrator struct {
	accountRepo  *accounts.AccountRepository
	enqueuer     taskqueue.TaskEnqueuer
	taskRuntime  *taskruntime.Service
	providerCall providerCallFunc
}

const taskCleanupTimeout = 5 * time.Second

func newTaskExecutionOrchestrator(
	accountRepo *accounts.AccountRepository,
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
	log.Printf("[Worker] 执行任务: %s, 账号: %d", task.TaskID, accountID)
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
		log.Printf("[Worker] 检查任务取消状态失败: task=%s err=%v", task.TaskID, err)
		if requeueErr := o.requeueTask(ctx, task, assignment, "检查任务取消状态失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if cancelled {
		o.skipCancelledTask(ctx, task)
		return nil
	}

	taskCtx, stop := o.taskRuntime.WatchTaskCancellation(ctx, task.TaskID)
	defer stop()

	if err := o.taskRuntime.MarkTaskRunning(ctx, task.TaskID, accountID, assignment.LeaseToken); err != nil {
		if errors.Is(err, taskruntime.ErrTaskAlreadyRunning) {
			o.dropTask(ctx, task, "任务已由同账号执行，丢弃重复队列任务")
			return nil
		}
		if errors.Is(err, taskruntime.ErrTaskNotFound) || errors.Is(err, taskruntime.ErrTaskTerminal) || errors.Is(err, taskruntime.ErrTaskStateConflict) {
			o.dropTask(ctx, task, "任务状态已收敛，丢弃脏队列任务")
			return nil
		}
		cancelled, checkErr := o.taskRuntime.IsTaskCancelled(ctx, task.TaskID)
		if checkErr != nil {
			log.Printf("[Worker] 检查任务取消状态失败: task=%s err=%v", task.TaskID, checkErr)
		}
		if cancelled {
			o.skipCancelledTask(ctx, task)
			return nil
		}
		log.Printf("[Worker] 标记任务运行中失败: %v", err)
		if requeueErr := o.requeueTask(ctx, task, assignment, "标记任务运行中失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if err := o.taskRuntime.ConfirmTaskRunning(ctx, assignment); err != nil {
		log.Printf("[Worker] 确认任务运行租约失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
		cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
		if resetErr := o.taskRuntime.ResetTaskRunningToPending(cleanupCtx, task.TaskID, accountID, assignment.LeaseToken, "确认任务运行租约失败"); resetErr != nil {
			cleanupCancel()
			if isTaskConvergedError(resetErr) {
				o.dropTask(ctx, task, "任务状态已收敛，丢弃脏队列任务")
				return nil
			}
			return resetErr
		}
		cleanupCancel()
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
				log.Printf("[Worker] 检查任务取消状态失败: task=%s err=%v", task.TaskID, checkErr)
			}
			if cancelled {
				o.skipCancelledTask(ctx, task)
				return nil
			}
			return o.handleExecutionInterrupted(ctx, task, assignment, err)
		}
		log.Printf("[Worker] 上游调用失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
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
			log.Printf("[Worker] 重置任务为待重试失败: task=%s err=%v", task.TaskID, requeueErr)
			if markErr := o.completeFailedTask(ctx, task, assignment, reason, elapsed); markErr != nil {
				return markErr
			}
			return nil
		}
		if !requeued {
			log.Printf("[Worker] 任务达到重试上限，停止 failover: task=%s", task.TaskID)
			return nil
		}
		disabled, failCount, statErr := o.taskRuntime.RecordAccountFailure(ctx, accountID)
		if statErr != nil {
			log.Printf("[Worker] 记录账号失败状态异常: account=%d err=%v", accountID, statErr)
		}
		if disabled {
			log.Printf("[Worker] 账号 %d 失败次数过多(%d)，已自动禁用", accountID, failCount)
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
		log.Printf("[Worker] 检查任务取消状态失败: task=%s err=%v", task.TaskID, err)
		if requeueErr := o.requeueTask(ctx, task, assignment, "检查任务取消状态失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if cancelled {
		o.skipCancelledTask(ctx, task)
		return nil
	}

	elapsed := time.Since(startTime).Seconds()
	completed, completeErr := o.completeTask(ctx, task, assignment, result, elapsed)
	if completeErr != nil {
		return completeErr
	}
	if completed {
		if err := o.taskRuntime.ResetAccountFailure(ctx, accountID); err != nil {
			log.Printf("[Worker] 重置账号失败计数异常: account=%d err=%v", accountID, err)
		}
	}
	return nil
}

func (o *taskExecutionOrchestrator) requeueTask(ctx context.Context, task *queuetypes.TaskPayload, assignment taskqueue.TaskAssignment, reason string) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	if assignment.AccountID != 0 && assignment.LeaseToken != "" {
		if err := o.taskRuntime.ResetTaskRunningToPending(cleanupCtx, task.TaskID, assignment.AccountID, assignment.LeaseToken, reason); err != nil && !isTaskConvergedError(err) {
			log.Printf("[Worker] 重置任务为待调度失败: task=%s account=%d err=%v", task.TaskID, assignment.AccountID, err)
		}
	}
	cancelled, err := o.taskRuntime.IsTaskCancelled(cleanupCtx, task.TaskID)
	if err != nil {
		log.Printf("[Worker] 检查任务取消状态失败: task=%s err=%v", task.TaskID, err)
	} else if cancelled {
		o.skipCancelledTask(cleanupCtx, task)
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
			log.Printf("[Worker] 任务状态已收敛，跳过完成回写: task=%s err=%v", task.TaskID, err)
			return false, nil
		}
		log.Printf("[Worker] 标记任务完成失败: task=%s err=%v", task.TaskID, err)
		if failErr := o.taskRuntime.MarkTaskFailedWithLease(cleanupCtx, task.TaskID, assignment.LeaseToken, "任务结果保存失败，请稍后重试"); failErr != nil {
			log.Printf("[Worker] 完成回写失败后标记任务失败也失败: task=%s err=%v", task.TaskID, failErr)
		}
		return false, err
	}
	log.Printf("[Worker] 任务 %s 完成, 状态: completed, 耗时: %.1fs", task.TaskID, elapsedSec)
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
			log.Printf("[Worker] 任务状态已收敛，跳过失败回写: task=%s err=%v", task.TaskID, err)
			return nil
		}
		log.Printf("[Worker] 标记任务失败失败: task=%s err=%v", task.TaskID, err)
		return err
	}
	log.Printf("[Worker] 任务 %s 失败, 耗时: %.1fs, 原因: %s", task.TaskID, elapsedSec, reason)
	return nil
}

func (o *taskExecutionOrchestrator) failTask(ctx context.Context, task *queuetypes.TaskPayload, reason string) error {
	return o.completeFailedTask(ctx, task, taskqueue.TaskAssignment{}, reason, 0)
}

func (o *taskExecutionOrchestrator) skipCancelledTask(ctx context.Context, task *queuetypes.TaskPayload) {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	if err := o.taskRuntime.ClearTaskCancelled(cleanupCtx, task.TaskID); err != nil {
		log.Printf("[Worker] 清理任务取消标记失败: task=%s err=%v", task.TaskID, err)
	}
	log.Printf("[Worker] 任务 %s 已取消，停止执行", task.TaskID)
}

func (o *taskExecutionOrchestrator) dropTask(ctx context.Context, task *queuetypes.TaskPayload, reason string) {
	log.Printf("[Worker] 任务 %s 已丢弃: %s", task.TaskID, reason)
}

func (o *taskExecutionOrchestrator) handleExecutionInterrupted(ctx context.Context, task *queuetypes.TaskPayload, assignment taskqueue.TaskAssignment, execErr error) error {
	cleanupCtx, cleanupCancel := taskCleanupContext(ctx)
	defer cleanupCancel()

	if err := o.taskRuntime.ResetTaskRunningToPending(cleanupCtx, task.TaskID, assignment.AccountID, assignment.LeaseToken, "worker context interrupted, waiting retry"); err != nil {
		if errors.Is(err, taskruntime.ErrTaskNotFound) || errors.Is(err, taskruntime.ErrTaskTerminal) || errors.Is(err, taskruntime.ErrTaskStateConflict) {
			log.Printf("[Worker] 中断任务状态已收敛: task=%s err=%v", task.TaskID, err)
		} else {
			log.Printf("[Worker] 中断任务重置失败: task=%s err=%v", task.TaskID, err)
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
