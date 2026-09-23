package postgres

import (
	"context"
	"fmt"
	"strings"

	dbsqlc "ideaboard/api/internal/db/sqlc"

	tasks "ideaboard/api/internal/tasking/domain"
	runtimecontracts "ideaboard/api/internal/tasking/ports/runtime"

	"github.com/jackc/pgx/v5/pgtype"
)

// ClaimAndStartExecutionAttempt fences one pending task for the execution
// token and opens its running attempt row in the same statement. It returns
// the server-assigned attempt number. A not-found error means another worker
// won the race or the task already converged.
func (r *Repository) ClaimAndStartExecutionAttempt(ctx context.Context, start tasks.ExecutionAttemptStart) (attemptNo int32, err error) {
	if r == nil || r.q == nil {
		return 0, fmt.Errorf("execution attempt repository is unavailable")
	}
	if strings.TrimSpace(start.TaskID) == "" || strings.TrimSpace(start.ExecutionToken) == "" {
		return 0, fmt.Errorf("execution attempt requires task id and token")
	}
	row, err := r.q.ClaimAndStartExecutionAttempt(ctx, dbsqlc.ClaimAndStartExecutionAttemptParams{
		TaskID:            strings.TrimSpace(start.TaskID),
		ExecutionToken:    strings.TrimSpace(start.ExecutionToken),
		RiverJobID:        start.RiverJobID,
		FeatureSlug:       strings.TrimSpace(start.FeatureSlug),
		RequestedModel:    strings.TrimSpace(start.RequestedModel),
		Provider:          strings.TrimSpace(start.Provider),
		ProviderStartedAt: pgtype.Timestamptz{Time: start.StartedAt, Valid: !start.StartedAt.IsZero()},
	})
	if err != nil {
		return 0, fmt.Errorf("claim and start execution attempt: %w", err)
	}
	return row.AttemptNo, nil
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
		ProviderFinishedAt: pgtype.Timestamptz{Time: finish.FinishedAt, Valid: !finish.FinishedAt.IsZero()},
		CompletedAt:        pgtype.Timestamptz{Time: finish.CompletedAt, Valid: !finish.CompletedAt.IsZero()},
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
