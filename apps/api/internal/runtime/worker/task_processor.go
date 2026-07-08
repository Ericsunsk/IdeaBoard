package worker

import (
	"context"
	"fmt"
	"log/slog"

	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/taskruntime"
	"ideaboard/api/internal/tasking/tasks"
)

type consumerTaskRuntime interface {
	RefreshTaskAccounts(ctx context.Context, task *tasks.TaskPayload) error
	FailTask(ctx context.Context, input taskruntime.FailTaskInput) error
}

type taskProcessor struct {
	assigner    taskqueue.TaskAssigner
	taskRuntime consumerTaskRuntime
	executeTask func(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment) error
}

func newTaskProcessor(
	assigner taskqueue.TaskAssigner,
	taskRuntime consumerTaskRuntime,
	executeTask func(ctx context.Context, task *tasks.TaskPayload, assignment taskqueue.TaskAssignment) error,
) *taskProcessor {
	return &taskProcessor{
		assigner:    assigner,
		taskRuntime: taskRuntime,
		executeTask: executeTask,
	}
}

func (p *taskProcessor) Process(ctx context.Context, task *tasks.TaskPayload) error {
	if task == nil {
		return nil
	}

	if err := p.taskRuntime.RefreshTaskAccounts(ctx, task); err != nil {
		slog.Warn("refresh task accounts failed", "task_id", task.TaskID, "err", err)
		return err
	}

	if len(task.AccountIDs) == 0 {
		slog.Info("no available account for task", "task_id", task.TaskID)
		p.failTask(ctx, task, "没有可用账号")
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
		slog.Warn("assign task failed", "task_id", task.TaskID, "err", err)
		return err
	}

	switch assignment.Status {
	case taskqueue.AssignStatusDiscard:
		slog.Info("task converged; discard queued task", "task_id", task.TaskID)
		return nil
	case taskqueue.AssignStatusNoCapacity:
		return fmt.Errorf("%w: task=%s", taskqueue.ErrNoAccountCapacity, task.TaskID)
	case taskqueue.AssignStatusAssigned:
		if !assignment.HasRuntimeSnapshot() {
			return fmt.Errorf("invalid assignment snapshot for task=%s account=%d provider_endpoint=%d credential=%d has_token=%v",
				task.TaskID,
				assignment.AccountID,
				assignment.ProviderEndpointID,
				assignment.CredentialID,
				assignment.LeaseToken != "",
			)
		}
	default:
		return fmt.Errorf("unexpected assign status %q for task=%s", assignment.Status, task.TaskID)
	}

	return p.executeTask(ctx, task, assignment)
}

func (p *taskProcessor) failTask(ctx context.Context, task *tasks.TaskPayload, reason string) {
	if err := p.taskRuntime.FailTask(ctx, taskruntime.FailTaskInput{TaskID: task.TaskID, Reason: reason}); err != nil {
		slog.Warn("mark task failed failed", "task_id", task.TaskID, "err", err)
	}
	slog.Warn("task failed", "task_id", task.TaskID, "reason", reason)
}
