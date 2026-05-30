package worker

import (
	"context"
	"fmt"
	"log"

	"api/internal/runtime/queuetypes"
	"api/internal/runtime/taskqueue"
)

type consumerTaskRuntime interface {
	RefreshTaskAccounts(ctx context.Context, task *queuetypes.TaskPayload) error
	ReleaseTask(ctx context.Context, assignment taskqueue.TaskAssignment) error
	MarkTaskFailed(ctx context.Context, taskID string, reason string) error
}

type taskProcessor struct {
	assigner     taskqueue.TaskAssigner
	taskRuntime  consumerTaskRuntime
	executeTask  func(ctx context.Context, task *queuetypes.TaskPayload, assignment taskqueue.TaskAssignment) error
	asyncExecute bool
}

func newTaskProcessor(
	assigner taskqueue.TaskAssigner,
	taskRuntime consumerTaskRuntime,
	executeTask func(ctx context.Context, task *queuetypes.TaskPayload, assignment taskqueue.TaskAssignment) error,
	asyncExecute bool,
) *taskProcessor {
	return &taskProcessor{
		assigner:     assigner,
		taskRuntime:  taskRuntime,
		executeTask:  executeTask,
		asyncExecute: asyncExecute,
	}
}

func (p *taskProcessor) Process(ctx context.Context, task *queuetypes.TaskPayload) error {
	if task == nil {
		return nil
	}

	if err := p.taskRuntime.RefreshTaskAccounts(ctx, task); err != nil {
		log.Printf("[TaskProcessor] 刷新任务账号失败: task=%s err=%v", task.TaskID, err)
		return err
	}

	if len(task.AccountIDs) == 0 {
		log.Printf("[TaskProcessor] 没有可用账号: task=%s", task.TaskID)
		p.failTask(ctx, task, 0, "没有可用账号")
		return nil
	}

	assignment, err := p.assigner.Assign(ctx, task.AccountIDs, task)
	if err != nil {
		if isContextInterrupted(ctx.Err()) {
			return ctx.Err()
		}
		if isContextInterrupted(err) {
			return err
		}
		log.Printf("[TaskProcessor] 分配任务失败: task=%s err=%v", task.TaskID, err)
		return err
	}

	switch assignment.Status {
	case taskqueue.AssignStatusNoCapacity:
		return fmt.Errorf("%w: task=%s", taskqueue.ErrNoAccountCapacity, task.TaskID)
	case taskqueue.AssignStatusAssigned:
		if assignment.AccountID == 0 || assignment.LeaseToken == "" {
			return fmt.Errorf("invalid assignment for task=%s account=%d has_token=%v", task.TaskID, assignment.AccountID, assignment.LeaseToken != "")
		}
	default:
		return fmt.Errorf("unexpected assign status %q for task=%s", assignment.Status, task.TaskID)
	}

	if p.asyncExecute {
		go func() {
			if execErr := p.executeTask(ctx, task, assignment); execErr != nil {
				log.Printf("[TaskProcessor] 异步执行任务失败: task=%s account=%d err=%v", task.TaskID, assignment.AccountID, execErr)
			}
		}()
		return nil
	}
	return p.executeTask(ctx, task, assignment)
}

func (p *taskProcessor) failTask(ctx context.Context, task *queuetypes.TaskPayload, accountID uint64, reason string) {
	if err := p.taskRuntime.ReleaseTask(ctx, taskqueue.TaskAssignment{TaskID: task.TaskID, AccountID: accountID}); err != nil {
		log.Printf("[TaskProcessor] 释放任务占位失败: task=%s account=%d err=%v", task.TaskID, accountID, err)
	}
	if err := p.taskRuntime.MarkTaskFailed(ctx, task.TaskID, reason); err != nil {
		log.Printf("[TaskProcessor] 标记任务失败失败: task=%s err=%v", task.TaskID, err)
	}
	log.Printf("[TaskProcessor] 任务 %s 失败, 原因: %s", task.TaskID, reason)
}
