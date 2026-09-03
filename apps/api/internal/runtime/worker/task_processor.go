package worker

import (
	"context"
	"errors"
	"fmt"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"log/slog"

	taskruntime "ideaboard/api/internal/tasking/application/runtime"
	tasks "ideaboard/api/internal/tasking/domain"
)

type consumerTaskRuntime interface {
	FailTask(ctx context.Context, input taskruntime.FailTaskInput) error
	ResetRunningTask(ctx context.Context, input taskruntime.ResetRunningTaskInput) error
}

var (
	errFinalAttemptMarkerUnavailable = errors.New("final attempt marker unavailable")
	errFinalAttemptMarkerResetFailed = errors.New("reset after final attempt marker failure failed")
)

type taskProcessor struct {
	assigner    taskqueueports.TaskAssigner
	taskRuntime consumerTaskRuntime
	executeTask func(ctx context.Context, task *tasks.TaskPayload, assignment taskqueueports.TaskAssignment) error
}

func newTaskProcessor(
	assigner taskqueueports.TaskAssigner,
	taskRuntime consumerTaskRuntime,
	executeTask func(ctx context.Context, task *tasks.TaskPayload, assignment taskqueueports.TaskAssignment) error,
) *taskProcessor {
	return &taskProcessor{
		assigner:    assigner,
		taskRuntime: taskRuntime,
		executeTask: executeTask,
	}
}

func (p *taskProcessor) Process(ctx context.Context, task *tasks.TaskPayload) error {
	return p.process(ctx, task, nil)
}

func (p *taskProcessor) ProcessBeforeExecute(ctx context.Context, task *tasks.TaskPayload, beforeExecute func() error) error {
	return p.process(ctx, task, beforeExecute)
}

func (p *taskProcessor) process(ctx context.Context, task *tasks.TaskPayload, beforeExecute func() error) error {
	if task == nil {
		return nil
	}
	correlationID := task.EnsureCorrelationID()

	assignment, err := p.assigner.Assign(ctx, task)
	if err != nil {
		if isContextInterrupted(ctx.Err()) {
			return ctx.Err()
		}
		if isContextInterrupted(err) {
			return err
		}
		slog.Warn("assign task failed", "task_id", task.TaskID, "correlation_id", correlationID, "err", err)
		return err
	}

	switch assignment.Status {
	case taskqueueports.AssignStatusDiscard:
		slog.Info("task converged; discard queued task", "task_id", task.TaskID, "correlation_id", correlationID)
		return nil
	case taskqueueports.AssignStatusNoCapacity:
		return fmt.Errorf("%w: task=%s", taskqueueports.ErrNoAccountCapacity, task.TaskID)
	case taskqueueports.AssignStatusAssigned:
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
	if beforeExecute != nil {
		if err := beforeExecute(); err != nil {
			resetErr := p.taskRuntime.ResetRunningTask(context.WithoutCancel(ctx), taskruntime.ResetRunningTaskInput{
				Assignment: assignment,
				Reason:     "final attempt marker unavailable; waiting retry",
			})
			if resetErr != nil {
				return errors.Join(
					fmt.Errorf("%w: %v", errFinalAttemptMarkerResetFailed, err),
					fmt.Errorf("reset task after marker failure: %w", resetErr),
				)
			}
			return fmt.Errorf("%w: %v", errFinalAttemptMarkerUnavailable, err)
		}
	}

	return p.executeTask(ctx, task, assignment)
}
