package postgres

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"

	tasks "ideaboard/api/internal/tasking/domain"

	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) ListAdminTaskAttempts(
	ctx context.Context,
	taskIDs []string,
) (map[string][]tasks.AdminTaskAttempt, error) {
	result := make(map[string][]tasks.AdminTaskAttempt)
	if len(taskIDs) == 0 {
		return result, nil
	}
	rows, err := r.q.ListAdminTaskAttemptsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("taskstore: list Admin Task attempts: %w", err)
	}
	for _, row := range rows {
		result[row.TaskID] = append(result[row.TaskID], adminTaskAttemptFromSQLC(row))
	}
	return result, nil
}

func (r *Repository) ListAdminTaskLatestExecutions(
	ctx context.Context,
	taskIDs []string,
) (map[string]tasks.AdminTaskExecutionSnapshot, error) {
	result := make(map[string]tasks.AdminTaskExecutionSnapshot)
	if len(taskIDs) == 0 {
		return result, nil
	}
	rows, err := r.q.ListAdminTaskLatestExecutionsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("taskstore: list Admin Task latest executions: %w", err)
	}
	for _, row := range rows {
		result[row.TaskID] = tasks.AdminTaskExecutionSnapshot{
			ResolvedModel: row.ResolvedModel,
			Provider:      row.Provider,
			GatewayStep:   row.GatewayStep,
		}
	}
	return result, nil
}

func adminTaskAttemptFromSQLC(row dbsqlc.ListAdminTaskAttemptsByTaskIDsRow) tasks.AdminTaskAttempt {
	providerLatencyMS := int64(0)
	if row.ProviderStartedAt.Valid && row.ProviderFinishedAt.Valid {
		providerLatencyMS = row.ProviderFinishedAt.Time.Sub(row.ProviderStartedAt.Time).Milliseconds()
		if providerLatencyMS < 0 {
			providerLatencyMS = 0
		}
	}
	var totalLatencyMS int64
	if row.CompletedAt.Valid {
		totalLatencyMS = row.CompletedAt.Time.Sub(dbutil.TimeValue(row.CreatedAt)).Milliseconds()
		if totalLatencyMS < 0 {
			totalLatencyMS = 0
		}
	}
	var cancelledAt *time.Time
	if row.Status == tasks.TaskAttemptStatusCancelled && row.CompletedAt.Valid {
		completed := row.CompletedAt.Time
		cancelledAt = &completed
	}
	return tasks.AdminTaskAttempt{
		AttemptNo:          int(row.AttemptNo),
		Status:             row.Status,
		Stage:              adminTaskAttemptStage(row),
		FailureCategory:    row.FailureCategory,
		ResolvedModel:      row.ResolvedModel,
		Provider:           row.Provider,
		GatewayStep:        row.GatewayStep,
		ProviderStartedAt:  nullTimePointer(row.ProviderStartedAt),
		ProviderFinishedAt: nullTimePointer(row.ProviderFinishedAt),
		CompletedAt:        nullTimePointer(row.CompletedAt),
		CancelledAt:        cancelledAt,
		ProviderLatencyMS:  providerLatencyMS,
		TotalLatencyMS:     totalLatencyMS,
	}
}

func adminTaskAttemptStage(row dbsqlc.ListAdminTaskAttemptsByTaskIDsRow) string {
	switch row.Status {
	case tasks.TaskAttemptStatusRunning:
		if row.ProviderStartedAt.Valid {
			return tasks.ExecutionStageProvider
		}
		return tasks.ExecutionStageAssigned
	case tasks.TaskAttemptStatusCompleted:
		return tasks.ExecutionStageCompleted
	case tasks.TaskAttemptStatusCancelled:
		return tasks.ExecutionStageCancelled
	}
	if row.FailureStage != "" {
		return row.FailureStage
	}
	return tasks.ExecutionStageFailed
}

func nullTimePointer(value pgtype.Timestamptz) *time.Time {
	return dbutil.TimePtr(value)
}
