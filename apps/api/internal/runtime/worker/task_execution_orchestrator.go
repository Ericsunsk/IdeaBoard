package worker

import (
	"context"
	"log"
	"time"

	"api/internal/accounts"
	"api/internal/platform/redis"
)

type providerCallFunc func(account *accounts.ApiAccount, task *redis.GlobalQueueTask) (string, error)

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

	if err := o.taskRuntime.MarkTaskRunning(task.TaskID, accountID); err != nil {
		log.Printf("[Consumer] 标记任务运行中失败: %v", err)
	}

	result, err := o.providerCall(account, task)
	if err != nil {
		elapsed := time.Since(startTime).Seconds()
		o.completeFailedTask(task, accountID, err.Error(), elapsed)
		disabled, failCount, statErr := o.taskRuntime.RecordAccountFailure(accountID)
		if statErr != nil {
			log.Printf("[Consumer] 记录账号失败状态异常: account=%d err=%v", accountID, statErr)
		}
		if disabled {
			log.Printf("[Consumer] 账号 %d 失败次数过多(%d)，已自动禁用", accountID, failCount)
		}
		return
	}

	elapsed := time.Since(startTime).Seconds()
	o.completeTask(task, accountID, result, elapsed)
	if err := o.taskRuntime.ResetAccountFailure(accountID); err != nil {
		log.Printf("[Consumer] 重置账号失败计数异常: account=%d err=%v", accountID, err)
	}
}

func (o *taskExecutionOrchestrator) requeueTask(task *redis.GlobalQueueTask, accountID uint64, reason string) {
	ctx := context.Background()
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Consumer] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
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

func (o *taskExecutionOrchestrator) completeTask(task *redis.GlobalQueueTask, accountID uint64, result string, elapsedSec float64) {
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Consumer] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	if err := o.taskRuntime.MarkTaskCompleted(task.TaskID, result); err != nil {
		log.Printf("[Consumer] 标记任务完成失败: task=%s err=%v", task.TaskID, err)
	}
	log.Printf("[Consumer] 任务 %s 完成, 状态: completed, 耗时: %.1fs", task.TaskID, elapsedSec)
}

func (o *taskExecutionOrchestrator) completeFailedTask(task *redis.GlobalQueueTask, accountID uint64, reason string, elapsedSec float64) {
	if err := o.taskRuntime.ReleaseTask(accountID, task.TaskID); err != nil {
		log.Printf("[Consumer] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	if err := o.taskRuntime.MarkTaskFailed(task.TaskID, reason); err != nil {
		log.Printf("[Consumer] 标记任务失败失败: task=%s err=%v", task.TaskID, err)
	}
	log.Printf("[Consumer] 任务 %s 失败, 耗时: %.1fs, 原因: %s", task.TaskID, elapsedSec, reason)
}

func (o *taskExecutionOrchestrator) failTask(task *redis.GlobalQueueTask, reason string) {
	o.completeFailedTask(task, 0, reason, 0)
}
