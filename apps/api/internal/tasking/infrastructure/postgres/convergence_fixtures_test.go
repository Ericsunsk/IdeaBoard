package postgres

import (
	"context"
	"time"

	tasks "ideaboard/api/internal/tasking/domain"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
)

func (r *Repository) convergePendingFailureForTest(ctx context.Context, taskID, errorMessage, result string, occurredAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	return r.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind: runtimecontracts.TaskConvergenceFailed, TaskID: taskID, ErrorMessage: errorMessage, Result: result, OccurredAt: occurredAt, AttemptFinish: attemptFinish,
	})
}

func (r *Repository) convergeFailedWithLeaseForTest(ctx context.Context, taskID string, executionToken, errorMessage, result string, occurredAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	return r.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind: runtimecontracts.TaskConvergenceFailed, TaskID: taskID, ExecutionToken: executionToken, ErrorMessage: errorMessage, Result: result, OccurredAt: occurredAt, AttemptFinish: attemptFinish,
	})
}

func (r *Repository) convergeFinalFailureForTest(ctx context.Context, taskID string, executionToken, errorMessage, result string, occurredAt time.Time, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	return r.ConvergeTask(ctx, runtimecontracts.TaskConvergenceIntent{
		Kind: runtimecontracts.TaskConvergenceFinalFailed, TaskID: taskID, ExecutionToken: executionToken, ErrorMessage: errorMessage, Result: result, OccurredAt: occurredAt, AttemptFinish: attemptFinish,
	})
}
