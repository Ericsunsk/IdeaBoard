package execution

import (
	"context"
	"errors"
	"fmt"

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
	taskExecution consumerTaskExecution
	executeTask   func(ctx context.Context, task *tasks.TaskPayload) error
}

type TaskProcessor = Processor

func NewProcessor(
	taskExecution consumerTaskExecution,
	executeTask func(ctx context.Context, task *tasks.TaskPayload) error,
) *Processor {
	return &Processor{
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
	if err := p.runBeforeExecute(ctx, task.TaskID, beforeExecute); err != nil {
		return err
	}

	return p.executeTask(ctx, task)
}

func (p *Processor) runBeforeExecute(ctx context.Context, taskID string, beforeExecute func() error) error {
	if beforeExecute == nil {
		return nil
	}
	if err := beforeExecute(); err != nil {
		resetErr := p.taskExecution.ResetRunningTask(context.WithoutCancel(ctx), taskexecution.ResetRunningTaskInput{
			TaskID: taskID,
			Reason: "final attempt marker unavailable; waiting retry",
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
