package taskstore

import (
	"context"
	"database/sql"
	"fmt"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasks"
	"strings"
	"time"
)

func (r *Repository) ListOutputsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]tasks.TaskOutput, error) {
	result := make(map[string][]tasks.TaskOutput, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	rows, err := r.q.ListOutputsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		output := taskOutputFromSQLC(row)
		result[output.TaskID] = append(result[output.TaskID], output)
	}
	return result, nil
}

func (r *Repository) PatchExecutionMetrics(ctx context.Context, patch tasks.TaskExecutionMetricsPatch) error {
	taskID := strings.TrimSpace(patch.TaskID)
	if taskID == "" {
		return fmt.Errorf("task_id is required")
	}
	return r.q.PatchTaskExecutionMetrics(ctx, dbsqlc.PatchTaskExecutionMetricsParams{
		TaskID:             taskID,
		CurrentStage:       optionalNonEmptyString(patch.CurrentStage),
		BottleneckSource:   optionalNonEmptyString(patch.BottleneckSource),
		FailureStage:       optionalString(patch.FailureStage),
		FailureCategory:    optionalString(patch.FailureCategory),
		AssignedAt:         dbutil.NullTime(patch.AssignedAt),
		ProviderStartedAt:  dbutil.NullTime(patch.ProviderStartedAt),
		ProviderFinishedAt: dbutil.NullTime(patch.ProviderFinishedAt),
		FinalizeStartedAt:  dbutil.NullTime(patch.FinalizeStartedAt),
		FinalizeFinishedAt: dbutil.NullTime(patch.FinalizeFinishedAt),
		QueueWaitMs:        optionalInt64(patch.QueueWaitMS),
		WorkerOverheadMs:   optionalInt64(patch.WorkerOverheadMS),
		ProviderLatencyMs:  optionalInt64(patch.ProviderLatencyMS),
		FinalizeLatencyMs:  optionalInt64(patch.FinalizeLatencyMS),
		TotalLatencyMs:     optionalInt64(patch.TotalLatencyMS),
	})
}

func patchTerminalExecutionMetricsTx(ctx context.Context, q *dbsqlc.Queries, taskID string, currentStage string, failureStage string, failureCategory string, finishedAt time.Time) error {
	taskRow, err := q.GetTaskByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	metricsByTaskID, err := listExecutionMetricsByTaskIDsTx(ctx, q, []string{taskID})
	if err != nil {
		return err
	}
	return patchTerminalExecutionMetricsForTaskTx(ctx, q, taskID, taskRow.CreatedAt, metricsByTaskID[taskID], currentStage, failureStage, failureCategory, finishedAt)
}

func patchTerminalExecutionMetricsForTaskTx(
	ctx context.Context,
	q *dbsqlc.Queries,
	taskID string,
	createdAt time.Time,
	metrics *tasks.TaskExecutionMetrics,
	currentStage string,
	failureStage string,
	failureCategory string,
	finishedAt time.Time,
) error {
	currentStage = strings.TrimSpace(currentStage)
	if taskID == "" || currentStage == "" {
		return nil
	}
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	totalLatencyMS := finishedAt.Sub(createdAt).Milliseconds()
	if totalLatencyMS < 0 {
		totalLatencyMS = 0
	}
	bottleneckSource := tasks.ResolveExecutionBottleneck(metrics, failureStage, failureCategory)
	if currentStage == tasks.ExecutionStageCancelled {
		bottleneckSource = tasks.BottleneckSourceUnknown
	}
	if currentStage != tasks.ExecutionStageFailed && currentStage != tasks.ExecutionStageCancelled {
		failureStage = ""
		failureCategory = ""
	}
	return q.PatchTaskExecutionMetrics(ctx, dbsqlc.PatchTaskExecutionMetricsParams{
		TaskID:           taskID,
		CurrentStage:     optionalNonEmptyString(&currentStage),
		BottleneckSource: optionalNonEmptyString(&bottleneckSource),
		FailureStage:     optionalString(&failureStage),
		FailureCategory:  optionalString(&failureCategory),
		TotalLatencyMs:   optionalInt64(&totalLatencyMS),
	})
}

func listExecutionMetricsByTaskIDsTx(ctx context.Context, q *dbsqlc.Queries, taskIDs []string) (map[string]*tasks.TaskExecutionMetrics, error) {
	result := make(map[string]*tasks.TaskExecutionMetrics, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	rows, err := q.ListTaskExecutionMetricsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		metrics := taskExecutionMetricsFromSQLC(row)
		result[metrics.TaskID] = &metrics
	}
	return result, nil
}

func taskAttemptFailureStage(attemptFinish *tasks.TaskAttemptFinish, fallback string) string {
	if attemptFinish != nil {
		if stage := strings.TrimSpace(attemptFinish.FailureStage); stage != "" {
			return stage
		}
	}
	return fallback
}

func taskAttemptFailureCategory(attemptFinish *tasks.TaskAttemptFinish, fallback string) string {
	if attemptFinish != nil {
		if category := strings.TrimSpace(attemptFinish.FailureCategory); category != "" {
			return category
		}
	}
	return fallback
}

func (r *Repository) PatchExecutionSnapshot(ctx context.Context, snapshot tasks.TaskExecutionSnapshot) (int64, error) {
	taskID := strings.TrimSpace(snapshot.TaskID)
	if taskID == "" {
		return 0, fmt.Errorf("task_id is required")
	}
	return r.q.PatchTaskExecutionSnapshot(ctx, dbsqlc.PatchTaskExecutionSnapshotParams{
		TaskID:         taskID,
		FeatureSlug:    strings.TrimSpace(snapshot.FeatureSlug),
		RequestedModel: strings.TrimSpace(snapshot.RequestedModel),
		ResolvedModel:  strings.TrimSpace(snapshot.ResolvedModel),
		ApiType:        strings.TrimSpace(snapshot.APIType),
	})
}

func (r *Repository) CreateTaskAttempt(ctx context.Context, start tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error) {
	taskID := strings.TrimSpace(start.TaskID)
	if taskID == "" {
		return tasks.TaskAttemptRef{}, fmt.Errorf("task_id is required")
	}
	providerStartedAt := start.ProviderStartedAt
	if providerStartedAt.IsZero() {
		providerStartedAt = time.Now()
	}
	row, err := r.q.CreateTaskAttempt(ctx, dbsqlc.CreateTaskAttemptParams{
		TaskID:            taskID,
		RiverJobID:        start.RiverJobID,
		LeaseToken:        strings.TrimSpace(start.LeaseToken),
		AccountID:         start.AccountID,
		ChannelID:         start.ChannelID,
		FeatureSlug:       strings.TrimSpace(start.FeatureSlug),
		RequestedModel:    strings.TrimSpace(start.RequestedModel),
		ResolvedModel:     strings.TrimSpace(start.ResolvedModel),
		ApiType:           strings.TrimSpace(start.APIType),
		ProviderStartedAt: sql.NullTime{Time: providerStartedAt, Valid: true},
	})
	if err != nil {
		return tasks.TaskAttemptRef{}, err
	}
	return tasks.TaskAttemptRef{ID: row.ID, AttemptNo: int(row.AttemptNo)}, nil
}

func (r *Repository) FinishTaskAttempt(ctx context.Context, finish tasks.TaskAttemptFinish) (int64, error) {
	if finish.ID == 0 {
		return 0, fmt.Errorf("task_attempt id is required")
	}
	return r.q.FinishTaskAttempt(ctx, finishTaskAttemptParams(finish))
}

func finishTaskAttemptTx(ctx context.Context, q *dbsqlc.Queries, finish *tasks.TaskAttemptFinish) error {
	if finish == nil || finish.ID == 0 {
		return nil
	}
	updated, err := q.FinishTaskAttempt(ctx, finishTaskAttemptParams(*finish))
	if err != nil {
		return err
	}
	if updated != 1 {
		return fmt.Errorf("finish task attempt affected %d rows, want 1", updated)
	}
	return nil
}

func finishTaskAttemptParams(finish tasks.TaskAttemptFinish) dbsqlc.FinishTaskAttemptParams {
	status := strings.TrimSpace(finish.Status)
	if status == "" {
		status = tasks.TaskAttemptStatusFailed
	}
	completedAt := finish.CompletedAt
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	return dbsqlc.FinishTaskAttemptParams{
		Status:             status,
		FailureStage:       strings.TrimSpace(finish.FailureStage),
		FailureCategory:    strings.TrimSpace(finish.FailureCategory),
		ErrorMsg:           strings.TrimSpace(finish.ErrorMsg),
		ProviderFinishedAt: dbutil.NullTime(finish.ProviderFinishedAt),
		FinalizeStartedAt:  dbutil.NullTime(finish.FinalizeStartedAt),
		FinalizeFinishedAt: dbutil.NullTime(finish.FinalizeFinishedAt),
		ProviderLatencyMs:  finish.ProviderLatencyMS,
		FinalizeLatencyMs:  finish.FinalizeLatencyMS,
		TotalLatencyMs:     finish.TotalLatencyMS,
		CompletedAt:        sql.NullTime{Time: completedAt, Valid: true},
		ID:                 finish.ID,
	}
}

func (r *Repository) ListMonitorExecutionPressureByRange(ctx context.Context, start, now time.Time) ([]MonitorExecutionPressureAggregate, error) {
	if start.IsZero() || now.IsZero() {
		return nil, fmt.Errorf("monitor execution pressure date range is required")
	}
	if !now.After(start) {
		return nil, fmt.Errorf("monitor execution pressure now_at must be after started_at")
	}
	rows, err := r.q.ListMonitorExecutionPressureByRange(ctx, dbsqlc.ListMonitorExecutionPressureByRangeParams{
		NowAt:     now,
		StartedAt: start,
	})
	if err != nil {
		return nil, err
	}
	result := make([]MonitorExecutionPressureAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, MonitorExecutionPressureAggregate{
			Key:                row.BucketKey,
			Total:              row.Total,
			AvgDurationSeconds: row.AvgDurationSeconds,
			MaxDurationSeconds: row.MaxDurationSeconds,
			Failed:             row.Failed,
			Retry:              row.Retry,
		})
	}
	return result, nil
}

func (r *Repository) ListExecutionMetricsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]*tasks.TaskExecutionMetrics, error) {
	result := make(map[string]*tasks.TaskExecutionMetrics, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	rows, err := r.q.ListTaskExecutionMetricsByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		metrics := taskExecutionMetricsFromSQLC(row)
		result[metrics.TaskID] = &metrics
	}
	return result, nil
}

func (r *Repository) ListExecutionBottleneckStatsByCreatedRange(ctx context.Context, start, end time.Time) ([]ExecutionBottleneckAggregate, error) {
	rows, err := r.q.ListTaskExecutionBottleneckStatsByCreatedRange(ctx, dbsqlc.ListTaskExecutionBottleneckStatsByCreatedRangeParams{
		StartedAt: start,
		EndedAt:   end,
	})
	if err != nil {
		return nil, err
	}
	result := make([]ExecutionBottleneckAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, ExecutionBottleneckAggregate{
			Source:       row.BottleneckSource,
			Total:        row.Total,
			Failed:       row.Failed,
			AvgLatencyMS: row.AvgLatencyMs,
		})
	}
	return result, nil
}

func (r *Repository) ListExecutionFailureCategoryStatsByCreatedRange(ctx context.Context, start, end time.Time) ([]ExecutionFailureCategoryAggregate, error) {
	rows, err := r.q.ListTaskExecutionFailureCategoryStatsByCreatedRange(ctx, dbsqlc.ListTaskExecutionFailureCategoryStatsByCreatedRangeParams{
		StartedAt: start,
		EndedAt:   end,
	})
	if err != nil {
		return nil, err
	}
	result := make([]ExecutionFailureCategoryAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, ExecutionFailureCategoryAggregate{
			Category:     row.FailureCategory,
			Total:        row.Total,
			AvgLatencyMS: row.AvgLatencyMs,
		})
	}
	return result, nil
}
