package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	tasks "ideaboard/api/internal/tasking/domain"
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
			AccountID:            row.AccountID,
			ProviderEndpointID:   row.ProviderEndpointID,
			ProviderEndpointName: row.ProviderEndpointName,
			ResolvedModel:        row.ResolvedModel,
			APIType:              row.ApiType,
		}
	}
	return result, nil
}

func adminTaskAttemptFromSQLC(row dbsqlc.ListAdminTaskAttemptsByTaskIDsRow) tasks.AdminTaskAttempt {
	return tasks.AdminTaskAttempt{
		AttemptNo:            int(row.AttemptNo),
		AccountID:            row.AccountID,
		Status:               row.Status,
		Stage:                adminTaskAttemptStage(row),
		FailureCategory:      row.FailureCategory,
		ProviderEndpointID:   row.ProviderEndpointID,
		ProviderEndpointName: row.ProviderEndpointName,
		ResolvedModel:        row.ResolvedModel,
		APIType:              row.ApiType,
		ProviderStartedAt:    nullTimePointer(row.ProviderStartedAt),
		ProviderFinishedAt:   nullTimePointer(row.ProviderFinishedAt),
		FinalizeStartedAt:    nullTimePointer(row.FinalizeStartedAt),
		FinalizeFinishedAt:   nullTimePointer(row.FinalizeFinishedAt),
		CompletedAt:          nullTimePointer(row.CompletedAt),
		CancelledAt:          nullTimePointer(row.CancelledAt),
		ProviderLatencyMS:    row.ProviderLatencyMs,
		FinalizeLatencyMS:    row.FinalizeLatencyMs,
		TotalLatencyMS:       row.TotalLatencyMs,
	}
}

func adminTaskAttemptStage(row dbsqlc.ListAdminTaskAttemptsByTaskIDsRow) string {
	switch row.Status {
	case tasks.TaskAttemptStatusRunning:
		if row.FinalizeStartedAt.Valid {
			return tasks.ExecutionStageFinalizing
		}
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

func nullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
