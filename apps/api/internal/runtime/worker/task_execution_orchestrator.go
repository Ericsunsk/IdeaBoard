package worker

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"api/internal/accounts"
	"api/internal/platform/redis"
)

type providerCallFunc func(ctx context.Context, account *accounts.ApiAccount, task *redis.GlobalQueueTask) (string, error)

type taskExecutionOrchestrator struct {
	accountRepo  *accounts.AccountRepository
	redisMgr     *redis.RedisManager
	taskRuntime  *accounts.TaskRuntimeService
	providerCall providerCallFunc
}

func newTaskExecutionOrchestrator(
	accountRepo *accounts.AccountRepository,
	redisMgr *redis.RedisManager,
	taskRuntime *accounts.TaskRuntimeService,
	providerCall providerCallFunc,
) *taskExecutionOrchestrator {
	return &taskExecutionOrchestrator{
		accountRepo:  accountRepo,
		redisMgr:     redisMgr,
		taskRuntime:  taskRuntime,
		providerCall: providerCall,
	}
}

func (o *taskExecutionOrchestrator) execute(task *redis.GlobalQueueTask, accountID uint64) {
	log.Printf("[Consumer] 执行任务: %s, 账号: %d", task.TaskID, accountID)
	startTime := time.Now()

	account, err := o.accountRepo.GetByID(accountID)
	if err != nil {
		o.requeueTask(task, accountID, "账号不存在，重新调度")
		return
	}
	if account.Status != 1 {
		o.requeueTask(task, accountID, "账号已停用，重新调度")
		return
	}

	cancelled, err := o.taskRuntime.IsTaskCancelled(task.TaskID)
	if err != nil {
		log.Printf("[Consumer] 检查任务取消状态失败: task=%s err=%v", task.TaskID, err)
		o.requeueTask(task, accountID, "检查任务取消状态失败")
		return
	}
	if cancelled {
		o.skipCancelledTask(task, accountID)
		return
	}

	taskCtx, stop := o.taskRuntime.WatchTaskCancellation(context.Background(), task.TaskID)
	defer stop()

	if err := o.taskRuntime.MarkTaskRunning(task.TaskID, accountID); err != nil {
		if errors.Is(err, accounts.ErrTaskNotFound) || errors.Is(err, accounts.ErrTaskTerminal) || errors.Is(err, accounts.ErrTaskStateConflict) {
			o.dropTask(task, accountID, "任务状态已收敛，丢弃脏队列任务")
			return
		}
		cancelled, checkErr := o.taskRuntime.IsTaskCancelled(task.TaskID)
		if checkErr != nil {
			log.Printf("[Consumer] 检查任务取消状态失败: task=%s err=%v", task.TaskID, checkErr)
		}
		if cancelled {
			o.skipCancelledTask(task, accountID)
			return
		}
		log.Printf("[Consumer] 标记任务运行中失败: %v", err)
		o.requeueTask(task, accountID, "标记任务运行中失败")
		return
	}

	result, err := o.providerCall(taskCtx, account, task)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(taskCtx.Err(), context.Canceled) {
			o.skipCancelledTask(task, accountID)
			return
		}
		log.Printf("[Consumer] 上游调用失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
		elapsed := time.Since(startTime).Seconds()
		reason := userFacingProviderError(err)
		if !shouldFailoverProviderError(err) {
			o.completeFailedTask(task, accountID, reason, elapsed)
			return
		}
		if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
			log.Printf("[Consumer] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
		}
		requeued, requeueErr := o.taskRuntime.RequeueTaskForFailover(task.TaskID, accountID, reason)
		if requeueErr != nil {
			if errors.Is(requeueErr, accounts.ErrTaskNotFound) || errors.Is(requeueErr, accounts.ErrTaskTerminal) || errors.Is(requeueErr, accounts.ErrTaskStateConflict) {
				o.dropTask(task, 0, "任务状态已收敛，丢弃脏队列任务")
				return
			}
			log.Printf("[Consumer] 重置任务为待重试失败: task=%s err=%v", task.TaskID, requeueErr)
			o.completeFailedTask(task, 0, reason, elapsed)
			return
		}
		if !requeued {
			log.Printf("[Consumer] 任务达到重试上限，停止 failover: task=%s", task.TaskID)
			return
		}
		disabled, failCount, statErr := o.taskRuntime.RecordAccountFailure(accountID)
		if statErr != nil {
			log.Printf("[Consumer] 记录账号失败状态异常: account=%d err=%v", accountID, statErr)
		}
		if disabled {
			log.Printf("[Consumer] 账号 %d 失败次数过多(%d)，已自动禁用", accountID, failCount)
		}
		task.AccountIDs = removeAccountCandidate(task.AccountIDs, accountID)
		if len(task.AccountIDs) == 0 {
			o.completeFailedTask(task, 0, reason, elapsed)
			return
		}
		o.requeueTask(task, 0, reason)
		return
	}

	cancelled, err = o.taskRuntime.IsTaskCancelled(task.TaskID)
	if err != nil {
		log.Printf("[Consumer] 检查任务取消状态失败: task=%s err=%v", task.TaskID, err)
		o.requeueTask(task, accountID, "检查任务取消状态失败")
		return
	}
	if cancelled {
		o.skipCancelledTask(task, accountID)
		return
	}

	elapsed := time.Since(startTime).Seconds()
	if o.completeTask(task, accountID, result, elapsed) {
		if err := o.taskRuntime.ResetAccountFailure(accountID); err != nil {
			log.Printf("[Consumer] 重置账号失败计数异常: account=%d err=%v", accountID, err)
		}
	}
}

func (o *taskExecutionOrchestrator) requeueTask(task *redis.GlobalQueueTask, accountID uint64, reason string) {
	ctx := context.Background()
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Consumer] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	cancelled, err := o.taskRuntime.IsTaskCancelled(task.TaskID)
	if err != nil {
		log.Printf("[Consumer] 检查任务取消状态失败: task=%s err=%v", task.TaskID, err)
	} else if cancelled {
		o.skipCancelledTask(task, 0)
		return
	}
	if err := o.taskRuntime.RefreshTaskAccounts(task); err != nil {
		o.failTask(task, reason)
		return
	}
	if len(task.AccountIDs) == 0 {
		o.failTask(task, reason)
		return
	}
	if err := o.redisMgr.PushGlobalQueueFront(ctx, task); err != nil {
		o.failTask(task, reason)
	}
}

func (o *taskExecutionOrchestrator) completeTask(task *redis.GlobalQueueTask, accountID uint64, result string, elapsedSec float64) bool {
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Consumer] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	if err := o.taskRuntime.MarkTaskCompleted(task.TaskID, result); err != nil {
		if errors.Is(err, accounts.ErrTaskNotFound) || errors.Is(err, accounts.ErrTaskTerminal) || errors.Is(err, accounts.ErrTaskStateConflict) {
			log.Printf("[Consumer] 任务状态已收敛，跳过完成回写: task=%s err=%v", task.TaskID, err)
			return false
		}
		log.Printf("[Consumer] 标记任务完成失败: task=%s err=%v", task.TaskID, err)
		return false
	}
	log.Printf("[Consumer] 任务 %s 完成, 状态: completed, 耗时: %.1fs", task.TaskID, elapsedSec)
	return true
}

func (o *taskExecutionOrchestrator) completeFailedTask(task *redis.GlobalQueueTask, accountID uint64, reason string, elapsedSec float64) {
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Consumer] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	if err := o.taskRuntime.MarkTaskFailed(task.TaskID, reason); err != nil {
		if errors.Is(err, accounts.ErrTaskNotFound) || errors.Is(err, accounts.ErrTaskTerminal) || errors.Is(err, accounts.ErrTaskStateConflict) {
			log.Printf("[Consumer] 任务状态已收敛，跳过失败回写: task=%s err=%v", task.TaskID, err)
			return
		}
		log.Printf("[Consumer] 标记任务失败失败: task=%s err=%v", task.TaskID, err)
		return
	}
	log.Printf("[Consumer] 任务 %s 失败, 耗时: %.1fs, 原因: %s", task.TaskID, elapsedSec, reason)
}

func (o *taskExecutionOrchestrator) failTask(task *redis.GlobalQueueTask, reason string) {
	o.completeFailedTask(task, 0, reason, 0)
}

func (o *taskExecutionOrchestrator) skipCancelledTask(task *redis.GlobalQueueTask, accountID uint64) {
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Consumer] 释放已取消任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	if err := o.taskRuntime.ClearTaskCancelled(task.TaskID); err != nil {
		log.Printf("[Consumer] 清理任务取消标记失败: task=%s err=%v", task.TaskID, err)
	}
	log.Printf("[Consumer] 任务 %s 已取消，停止执行", task.TaskID)
}

func (o *taskExecutionOrchestrator) dropTask(task *redis.GlobalQueueTask, accountID uint64, reason string) {
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Consumer] 释放脏任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	log.Printf("[Consumer] 任务 %s 已丢弃: %s", task.TaskID, reason)
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
