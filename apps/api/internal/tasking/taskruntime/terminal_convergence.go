package taskruntime

import (
	"context"

	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
)

type CompleteTaskInput struct {
	Assignment    taskqueue.TaskAssignment
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
	rows, err := s.taskRepo.ConvergeCompletedTask(ctx, taskstore.CompletedTaskConvergenceIntent{
		TaskID:             assignment.TaskID,
		AccountID:          assignment.AccountID,
		ProviderEndpointID: assignment.ProviderEndpointID,
		CredentialID:       assignment.CredentialID,
		LeaseToken:         assignment.LeaseToken,
		Result:             input.Result,
		CompletedAt:        completedAt,
		Outputs:            input.Outputs,
		AttemptFinish:      input.AttemptFinish,
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
