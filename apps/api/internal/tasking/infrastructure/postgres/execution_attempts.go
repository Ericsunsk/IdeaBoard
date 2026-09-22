package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	tasks "ideaboard/api/internal/tasking/domain"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"
)

// ClaimTaskForExecution fences one pending task for the given execution
// token and bumps the attempt sequence. A not-found error means another
// worker won the race or the task already converged.
func (r *Repository) ClaimTaskForExecution(ctx context.Context, taskID, executionToken string) (int32, error) {
	if r == nil || r.q == nil {
		return 0, fmt.Errorf("execution attempt repository is unavailable")
	}
	attemptSeq, err := r.q.ClaimTaskForExecution(ctx, dbsqlc.ClaimTaskForExecutionParams{
		TaskID:         strings.TrimSpace(taskID),
		ExecutionToken: strings.TrimSpace(executionToken),
	})
	if err != nil {
		return 0, fmt.Errorf("claim task for execution: %w", err)
	}
	return attemptSeq, nil
}

// GetActiveExecutionAttemptByTaskID reads the running attempt for final
// failure triage. Absence means no worker currently fences the task.
func (r *Repository) GetActiveExecutionAttemptByTaskID(ctx context.Context, taskID string) (*runtimecontracts.ActiveTaskExecution, error) {
	if r == nil || r.q == nil {
		return nil, fmt.Errorf("execution attempt repository is unavailable")
	}
	row, err := r.q.GetActiveExecutionAttemptByTaskID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return nil, fmt.Errorf("get active execution attempt: %w", err)
	}
	return &runtimecontracts.ActiveTaskExecution{
		TaskID:         row.TaskID,
		ExecutionToken: row.ExecutionToken,
		RiverJobID:     row.RiverJobID,
		FeatureSlug:    row.FeatureSlug,
		ResolvedModel:  row.ResolvedModel,
	}, nil
}

// StartExecutionAttempt inserts a running gateway execution attempt.
func (r *Repository) StartExecutionAttempt(ctx context.Context, start tasks.ExecutionAttemptStart) (int64, error) {
	if r == nil || r.q == nil {
		return 0, fmt.Errorf("execution attempt repository is unavailable")
	}
	if strings.TrimSpace(start.TaskID) == "" || strings.TrimSpace(start.ExecutionToken) == "" {
		return 0, fmt.Errorf("execution attempt requires task id and token")
	}
	if start.AttemptNo <= 0 {
		return 0, fmt.Errorf("execution attempt number must be positive")
	}
	row, err := r.q.InsertExecutionAttempt(ctx, dbsqlc.InsertExecutionAttemptParams{
		TaskID:            strings.TrimSpace(start.TaskID),
		RiverJobID:        start.RiverJobID,
		AttemptNo:         start.AttemptNo,
		ExecutionToken:    strings.TrimSpace(start.ExecutionToken),
		FeatureSlug:       strings.TrimSpace(start.FeatureSlug),
		RequestedModel:    strings.TrimSpace(start.RequestedModel),
		Provider:          strings.TrimSpace(start.Provider),
		ProviderStartedAt: sql.NullTime{Time: start.StartedAt, Valid: !start.StartedAt.IsZero()},
	})
	if err != nil {
		return 0, fmt.Errorf("insert execution attempt: %w", err)
	}
	return row.ID, nil
}

// FinalizeExecutionAttempt closes the running attempt matching token and
// task. It reports affected rows: 0 means another worker already converged
// the attempt and the caller must back off, not retry the write.
func (r *Repository) FinalizeExecutionAttempt(ctx context.Context, finish tasks.ExecutionAttemptFinish) (int64, error) {
	if r == nil || r.q == nil {
		return 0, fmt.Errorf("execution attempt repository is unavailable")
	}
	if strings.TrimSpace(finish.ExecutionToken) == "" || strings.TrimSpace(finish.TaskID) == "" {
		return 0, fmt.Errorf("execution attempt finalize requires token and task id")
	}
	if finish.Status != "completed" && finish.Status != "failed" && finish.Status != "requeued" && finish.Status != "cancelled" {
		return 0, fmt.Errorf("execution attempt status must be completed, failed, requeued, or cancelled")
	}
	affected, err := r.q.FinalizeExecutionAttempt(ctx, dbsqlc.FinalizeExecutionAttemptParams{
		Status:             finish.Status,
		ResolvedModel:      strings.TrimSpace(finish.ResolvedModel),
		GatewayStep:        strings.TrimSpace(finish.GatewayStep),
		ReceiptLogID:       strings.TrimSpace(finish.ReceiptLogID),
		ReceiptEventID:     strings.TrimSpace(finish.ReceiptEventID),
		CacheHit:           finish.CacheHit,
		FailureStage:       strings.TrimSpace(finish.FailureStage),
		FailureCategory:    strings.TrimSpace(finish.FailureCategory),
		ErrorMsg:           finish.ErrorMsg,
		ProviderFinishedAt: sql.NullTime{Time: finish.FinishedAt, Valid: !finish.FinishedAt.IsZero()},
		CompletedAt:        sql.NullTime{Time: finish.CompletedAt, Valid: !finish.CompletedAt.IsZero()},
		ExecutionToken:     strings.TrimSpace(finish.ExecutionToken),
		TaskID:             strings.TrimSpace(finish.TaskID),
	})
	if err != nil {
		return 0, fmt.Errorf("finalize execution attempt: %w", err)
	}
	return affected, nil
}

// GetExecutionAttemptByToken reads one attempt for triage and tests.
func (r *Repository) GetExecutionAttemptByToken(ctx context.Context, token string) (dbsqlc.TaskExecutionAttempt, error) {
	if r == nil || r.q == nil {
		return dbsqlc.TaskExecutionAttempt{}, fmt.Errorf("execution attempt repository is unavailable")
	}
	attempt, err := r.q.GetExecutionAttemptByToken(ctx, strings.TrimSpace(token))
	if err != nil {
		return dbsqlc.TaskExecutionAttempt{}, fmt.Errorf("get execution attempt: %w", err)
	}
	return attempt, nil
}
