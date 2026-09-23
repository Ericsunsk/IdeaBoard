package execution

import (
	"context"
	"strings"

	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"

	tasks "ideaboard/api/internal/tasking/domain"
)

type CompleteTaskInput struct {
	TaskID         string
	ExecutionToken string
	Result         string
	Outputs        []tasks.TaskOutput
	ResolvedModel  string
	GatewayStep    string
	ReceiptLogID   string
	ReceiptEventID string
	CacheHit       bool
	AttemptFinish  *tasks.TaskAttemptFinish
}

func (s *Service) CompleteTask(ctx context.Context, input CompleteTaskInput) error {
	token := input.ExecutionToken
	if err := requireTaskTransitionToken(taskTransitionComplete, token); err != nil {
		return err
	}
	completedAt := taskTerminalTime(input.AttemptFinish)
	if err := validateTaskOutputs(input.TaskID, input.Outputs); err != nil {
		return err
	}
	rows, err := s.taskRepo.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind:           runtimecontracts.TaskConvergenceCompleted,
		TaskID:         input.TaskID,
		ExecutionToken: token,
		Result:         input.Result,
		OccurredAt:     completedAt,
		Outputs:        input.Outputs,
		AttemptFinish:  input.AttemptFinish,
		ResolvedModel:  strings.TrimSpace(input.ResolvedModel),
		GatewayStep:    strings.TrimSpace(input.GatewayStep),
		ReceiptLogID:   strings.TrimSpace(input.ReceiptLogID),
		ReceiptEventID: strings.TrimSpace(input.ReceiptEventID),
		CacheHit:       input.CacheHit,
	})
	if err := normalizeTaskTransitionWriteError(err); err != nil {
		return err
	}
	applied, err := s.resolveTaskTransitionResult(ctx, taskTransitionComplete, input.TaskID, rows)
	if err != nil {
		return err
	}
	if applied {
		s.publishTaskStatusEvent(ctx, input.TaskID, tasks.StatusCompleted)
	}
	return nil
}
