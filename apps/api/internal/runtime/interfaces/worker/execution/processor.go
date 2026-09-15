package execution

import (
	"context"
	"errors"
	"fmt"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	"log/slog"

	taskexecution "ideaboard/api/internal/tasking/application/execution"
	tasks "ideaboard/api/internal/tasking/domain"
)

type consumerTaskExecution interface {
	FailTask(ctx context.Context, input taskexecution.FailTaskInput) error
	ResetRunningTask(ctx context.Context, input taskexecution.ResetRunningTaskInput) error
}

var (
	ErrFinalAttemptMarkerUnavailable = errors.New("final attempt marker unavailable")
	ErrFinalAttemptMarkerResetFailed = errors.New("reset after final attempt marker failure failed")
)

type Processor struct {
	assigner      taskqueueports.TaskAssigner
	taskExecution consumerTaskExecution
	executeTask   func(ctx context.Context, task *tasks.TaskPayload, assignment taskqueueports.TaskAssignment) error
}

type TaskProcessor = Processor

func NewProcessor(
	assigner taskqueueports.TaskAssigner,
	taskExecution consumerTaskExecution,
	executeTask func(ctx context.Context, task *tasks.TaskPayload, assignment taskqueueports.TaskAssignment) error,
) *Processor {
	return &Processor{
		assigner:      assigner,
		taskExecution: taskExecution,
		executeTask:   executeTask,
	}
}

func (p *Processor) TaskExecution() any {
	if p == nil {
		return nil
	}
	return p.taskExecution
}

func (p *Processor) Process(ctx context.Context, task *tasks.TaskPayload) error {
	return p.process(ctx, task, nil)
}

func (p *Processor) ProcessBeforeExecute(ctx context.Context, task *tasks.TaskPayload, beforeExecute func() error) error {
	return p.process(ctx, task, beforeExecute)
}

func (p *Processor) process(ctx context.Context, task *tasks.TaskPayload, beforeExecute func() error) error {
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

	if shouldDiscard, err := p.validateAssignment(task, assignment, correlationID); err != nil || shouldDiscard {
		return err
	}
	if err := p.runBeforeExecute(ctx, assignment, beforeExecute); err != nil {
		return err
	}

	return p.executeTask(ctx, task, assignment)
}

func (p *Processor) validateAssignment(task *tasks.TaskPayload, assignment taskqueueports.TaskAssignment, correlationID string) (bool, error) {
	switch assignment.Status {
	case taskqueueports.AssignStatusDiscard:
		slog.Info("task converged; discard queued task", "task_id", task.TaskID, "correlation_id", correlationID)
		return true, nil
	case taskqueueports.AssignStatusNoCapacity:
		return false, fmt.Errorf("%w: task=%s", taskqueueports.ErrNoAccountCapacity, task.TaskID)
	case taskqueueports.AssignStatusAssigned:
		if !assignment.HasRuntimeSnapshot() {
			return false, fmt.Errorf("invalid assignment snapshot for task=%s account=%d provider_endpoint=%d credential=%d has_token=%v",
				task.TaskID,
				assignment.AccountID,
				assignment.ProviderEndpointID,
				assignment.CredentialID,
				assignment.LeaseToken != "",
			)
		}
		return false, nil
	default:
		return false, fmt.Errorf("unexpected assign status %q for task=%s", assignment.Status, task.TaskID)
	}
}

func (p *Processor) runBeforeExecute(ctx context.Context, assignment taskqueueports.TaskAssignment, beforeExecute func() error) error {
	if beforeExecute == nil {
		return nil
	}
	if err := beforeExecute(); err != nil {
		resetErr := p.taskExecution.ResetRunningTask(context.WithoutCancel(ctx), taskexecution.ResetRunningTaskInput{
			Assignment: assignment,
			Reason:     "final attempt marker unavailable; waiting retry",
		})
		if resetErr != nil {
			return errors.Join(
				fmt.Errorf("%w: %w", ErrFinalAttemptMarkerResetFailed, err),
				fmt.Errorf("reset task after marker failure: %w", resetErr),
			)
		}
		return fmt.Errorf("%w: %w", ErrFinalAttemptMarkerUnavailable, err)
	}
	return nil
}
