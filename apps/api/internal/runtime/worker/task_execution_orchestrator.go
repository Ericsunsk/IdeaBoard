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
)

type providerCallFunc func(ctx context.Context, account *accounts.ApiAccount, task *queuetypes.TaskPayload) (string, error)

type taskExecutionOrchestrator struct {
	accountRepo  *accounts.AccountRepository
	enqueuer     taskqueue.TaskEnqueuer
	taskRuntime  *accounts.TaskRuntimeService
	providerCall providerCallFunc
}

func newTaskExecutionOrchestrator(
	accountRepo *accounts.AccountRepository,
	enqueuer taskqueue.TaskEnqueuer,
	taskRuntime *accounts.TaskRuntimeService,
	providerCall providerCallFunc,
) *taskExecutionOrchestrator {
	return &taskExecutionOrchestrator{
		accountRepo:  accountRepo,
		enqueuer:     enqueuer,
		taskRuntime:  taskRuntime,
		providerCall: providerCall,
	}
}

func (o *taskExecutionOrchestrator) execute(ctx context.Context, task *queuetypes.TaskPayload, accountID uint64) error {
	log.Printf("[Worker] 执行任务: %s, 账号: %d", task.TaskID, accountID)
	startTime := time.Now()

	account, err := o.accountRepo.GetByID(accountID)
	if err != nil {
		if requeueErr := o.requeueTask(task, accountID, "账号不存在，重新调度"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if account.Status != 1 {
		if requeueErr := o.requeueTask(task, accountID, "账号已停用，重新调度"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}

	cancelled, err := o.taskRuntime.IsTaskCancelled(task.TaskID)
	if err != nil {
		log.Printf("[Worker] 检查任务取消状态失败: task=%s err=%v", task.TaskID, err)
		if requeueErr := o.requeueTask(task, accountID, "检查任务取消状态失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if cancelled {
		o.skipCancelledTask(task, accountID)
		return nil
	}

	taskCtx, stop := o.taskRuntime.WatchTaskCancellation(ctx, task.TaskID)
	defer stop()

	if err := o.taskRuntime.MarkTaskRunning(task.TaskID, accountID); err != nil {
		if errors.Is(err, accounts.ErrTaskAlreadyRunning) {
			o.dropTask(task, accountID, "任务已由同账号执行，丢弃重复队列任务")
			return nil
		}
		if errors.Is(err, accounts.ErrTaskNotFound) || errors.Is(err, accounts.ErrTaskTerminal) || errors.Is(err, accounts.ErrTaskStateConflict) {
			o.dropTask(task, accountID, "任务状态已收敛，丢弃脏队列任务")
			return nil
		}
		cancelled, checkErr := o.taskRuntime.IsTaskCancelled(task.TaskID)
		if checkErr != nil {
			log.Printf("[Worker] 检查任务取消状态失败: task=%s err=%v", task.TaskID, checkErr)
		}
		if cancelled {
			o.skipCancelledTask(task, accountID)
			return nil
		}
		log.Printf("[Worker] 标记任务运行中失败: %v", err)
		if requeueErr := o.requeueTask(task, accountID, "标记任务运行中失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}

	result, err := o.providerCall(taskCtx, account, task)
	if err != nil {
		if isContextInterrupted(taskCtx.Err()) || isContextInterrupted(ctx.Err()) {
			cancelled, checkErr := o.taskRuntime.IsTaskCancelled(task.TaskID)
			if checkErr != nil {
				log.Printf("[Worker] 检查任务取消状态失败: task=%s err=%v", task.TaskID, checkErr)
			}
			if cancelled {
				o.skipCancelledTask(task, accountID)
				return nil
			}
			return o.handleExecutionInterrupted(task, accountID, err)
		}
		log.Printf("[Worker] 上游调用失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
		elapsed := time.Since(startTime).Seconds()
		reason := userFacingProviderError(err)
		if !shouldFailoverProviderError(err) {
			if markErr := o.completeFailedTask(task, accountID, reason, elapsed); markErr != nil {
				return markErr
			}
			return nil
		}
		if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
			log.Printf("[Worker] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
		}
		requeued, requeueErr := o.taskRuntime.RequeueTaskForFailover(task.TaskID, accountID, reason)
		if requeueErr != nil {
			if errors.Is(requeueErr, accounts.ErrTaskNotFound) || errors.Is(requeueErr, accounts.ErrTaskTerminal) || errors.Is(requeueErr, accounts.ErrTaskStateConflict) {
				o.dropTask(task, 0, "任务状态已收敛，丢弃脏队列任务")
				return nil
			}
			log.Printf("[Worker] 重置任务为待重试失败: task=%s err=%v", task.TaskID, requeueErr)
			if markErr := o.completeFailedTask(task, 0, reason, elapsed); markErr != nil {
				return markErr
			}
			return nil
		}
		if !requeued {
			log.Printf("[Worker] 任务达到重试上限，停止 failover: task=%s", task.TaskID)
			return nil
		}
		disabled, failCount, statErr := o.taskRuntime.RecordAccountFailure(accountID)
		if statErr != nil {
			log.Printf("[Worker] 记录账号失败状态异常: account=%d err=%v", accountID, statErr)
		}
		if disabled {
			log.Printf("[Worker] 账号 %d 失败次数过多(%d)，已自动禁用", accountID, failCount)
		}
		task.AccountIDs = removeAccountCandidate(task.AccountIDs, accountID)
		if len(task.AccountIDs) == 0 {
			if markErr := o.completeFailedTask(task, 0, reason, elapsed); markErr != nil {
				return markErr
			}
			return nil
		}
		if requeueErr := o.requeueTask(task, 0, reason); requeueErr != nil {
			return requeueErr
		}
		return nil
	}

	cancelled, err = o.taskRuntime.IsTaskCancelled(task.TaskID)
	if err != nil {
		log.Printf("[Worker] 检查任务取消状态失败: task=%s err=%v", task.TaskID, err)
		if requeueErr := o.requeueTask(task, accountID, "检查任务取消状态失败"); requeueErr != nil {
			return requeueErr
		}
		return nil
	}
	if cancelled {
		o.skipCancelledTask(task, accountID)
		return nil
	}

	elapsed := time.Since(startTime).Seconds()
	completed, completeErr := o.completeTask(task, accountID, result, elapsed)
	if completeErr != nil {
		return completeErr
	}
	if completed {
		if err := o.taskRuntime.ResetAccountFailure(accountID); err != nil {
			log.Printf("[Worker] 重置账号失败计数异常: account=%d err=%v", accountID, err)
		}
	}
	return nil
}

func (o *taskExecutionOrchestrator) requeueTask(task *queuetypes.TaskPayload, accountID uint64, reason string) error {
	ctx := context.Background()
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Worker] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	cancelled, err := o.taskRuntime.IsTaskCancelled(task.TaskID)
	if err != nil {
		log.Printf("[Worker] 检查任务取消状态失败: task=%s err=%v", task.TaskID, err)
	} else if cancelled {
		o.skipCancelledTask(task, 0)
		return nil
	}
	if err := o.taskRuntime.RefreshTaskAccounts(task); err != nil {
		return o.failTask(task, reason)
	}
	if len(task.AccountIDs) == 0 {
		return o.failTask(task, reason)
	}
	if err := o.enqueuer.Enqueue(ctx, task); err != nil {
		return o.failTask(task, reason)
	}
	return nil
}

func (o *taskExecutionOrchestrator) completeTask(task *queuetypes.TaskPayload, accountID uint64, result string, elapsedSec float64) (bool, error) {
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Worker] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	if err := o.taskRuntime.MarkTaskCompleted(task.TaskID, result); err != nil {
		if isTaskConvergedError(err) {
			log.Printf("[Worker] 任务状态已收敛，跳过完成回写: task=%s err=%v", task.TaskID, err)
			return false, nil
		}
		log.Printf("[Worker] 标记任务完成失败: task=%s err=%v", task.TaskID, err)
		if failErr := o.taskRuntime.MarkTaskFailed(task.TaskID, "任务结果保存失败，请稍后重试"); failErr != nil {
			log.Printf("[Worker] 完成回写失败后标记任务失败也失败: task=%s err=%v", task.TaskID, failErr)
		}
		return false, err
	}
	log.Printf("[Worker] 任务 %s 完成, 状态: completed, 耗时: %.1fs", task.TaskID, elapsedSec)
	return true, nil
}

func (o *taskExecutionOrchestrator) completeFailedTask(task *queuetypes.TaskPayload, accountID uint64, reason string, elapsedSec float64) error {
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Worker] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	if err := o.taskRuntime.MarkTaskFailed(task.TaskID, reason); err != nil {
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

func (o *taskExecutionOrchestrator) failTask(task *queuetypes.TaskPayload, reason string) error {
	return o.completeFailedTask(task, 0, reason, 0)
}

func (o *taskExecutionOrchestrator) skipCancelledTask(task *queuetypes.TaskPayload, accountID uint64) {
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Worker] 释放已取消任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	if err := o.taskRuntime.ClearTaskCancelled(task.TaskID); err != nil {
		log.Printf("[Worker] 清理任务取消标记失败: task=%s err=%v", task.TaskID, err)
	}
	log.Printf("[Worker] 任务 %s 已取消，停止执行", task.TaskID)
}

func (o *taskExecutionOrchestrator) dropTask(task *queuetypes.TaskPayload, accountID uint64, reason string) {
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Worker] 释放脏任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	log.Printf("[Worker] 任务 %s 已丢弃: %s", task.TaskID, reason)
}

func (o *taskExecutionOrchestrator) handleExecutionInterrupted(task *queuetypes.TaskPayload, accountID uint64, execErr error) error {
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Worker] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	if err := o.taskRuntime.ResetTaskRunningToPending(task.TaskID, accountID, "worker context interrupted, waiting retry"); err != nil {
		if errors.Is(err, accounts.ErrTaskNotFound) || errors.Is(err, accounts.ErrTaskTerminal) || errors.Is(err, accounts.ErrTaskStateConflict) {
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

func isContextInterrupted(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func isTaskConvergedError(err error) bool {
	return errors.Is(err, accounts.ErrTaskNotFound) || errors.Is(err, accounts.ErrTaskTerminal) || errors.Is(err, accounts.ErrTaskStateConflict)
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

func removeAccountCandidate(accountIDs []uint64, accountID uint64) []uint64 {
	if len(accountIDs) == 0 {
		return accountIDs
	}
	next := make([]uint64, 0, len(accountIDs))
	for _, id := range accountIDs {
		if id == accountID {
			continue
		}
		next = append(next, id)
	}
	return next
}
