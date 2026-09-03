package execution

import (
	"context"
	taskqueueports "ideaboard/api/internal/tasking/ports/queue"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"

	tasks "ideaboard/api/internal/tasking/domain"
)

type CompleteTaskInput struct {
	Assignment    taskqueueports.TaskAssignment
	Result        string
	Outputs       []tasks.TaskOutput
	AttemptFinish *tasks.TaskAttemptFinish
}

func (s *Service) CompleteTask(ctx context.Context, input CompleteTaskInput) error {
	assignment := input.Assignment
	if err := requireTaskTransitionLease(taskTransitionComplete, assignment); err != nil {
		return err
	}
	completedAt := taskTerminalTime(input.AttemptFinish)
	if err := validateTaskOutputs(assignment.TaskID, input.Outputs); err != nil {
		return err
	}
	rows, err := s.taskRepo.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind:          runtimecontracts.TaskConvergenceCompleted,
		TaskID:        assignment.TaskID,
		Lease:         taskConvergenceLease(assignment),
		Result:        input.Result,
		OccurredAt:    completedAt,
		Outputs:       input.Outputs,
		AttemptFinish: input.AttemptFinish,
	})
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return err
	}
	applied, err := s.resolveTaskTransitionResult(ctx, taskTransitionComplete, assignment.TaskID, assignment, rows)
	if err != nil {
		return err
	}
	if applied {
		s.publishTaskStatusEvent(ctx, assignment.TaskID, tasks.StatusCompleted)
	}
	return nil
}
