package taskstore

import (
	"context"
	"database/sql"
	"fmt"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/tasking/tasks"
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
		CurrentStage:       dbutil.NullString(patch.CurrentStage),
		BottleneckSource:   dbutil.NullString(patch.BottleneckSource),
		FailureStage:       optionalString(patch.FailureStage),
		FailureCategory:    optionalString(patch.FailureCategory),
		AssignedAt:         dbutil.NullTime(patch.AssignedAt),
		ProviderStartedAt:  dbutil.NullTime(patch.ProviderStartedAt),
		ProviderFinishedAt: dbutil.NullTime(patch.ProviderFinishedAt),
		FinalizeStartedAt:  dbutil.NullTime(patch.FinalizeStartedAt),
		FinalizeFinishedAt: dbutil.NullTime(patch.FinalizeFinishedAt),
		QueueWaitMs:        dbutil.NullInt64(patch.QueueWaitMS),
		WorkerOverheadMs:   dbutil.NullInt64(patch.WorkerOverheadMS),
		ProviderLatencyMs:  dbutil.NullInt64(patch.ProviderLatencyMS),
		FinalizeLatencyMs:  dbutil.NullInt64(patch.FinalizeLatencyMS),
		TotalLatencyMs:     dbutil.NullInt64(patch.TotalLatencyMS),
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
		CurrentStage:     dbutil.NullString(&currentStage),
		BottleneckSource: dbutil.NullString(&bottleneckSource),
		FailureStage:     optionalString(&failureStage),
		FailureCategory:  optionalString(&failureCategory),
		TotalLatencyMs:   dbutil.NullInt64(&totalLatencyMS),
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
	})
}

func (r *Repository) CreateTaskAttempt(ctx context.Context, start tasks.TaskAttemptStart) (tasks.TaskAttemptRef, error) {
	taskID := strings.TrimSpace(start.TaskID)
	if taskID == "" {
		return tasks.TaskAttemptRef{}, fmt.Errorf("task_id is required")
	}
	if start.CredentialID <= 0 {
		return tasks.TaskAttemptRef{}, fmt.Errorf("credential_id is required")
	}
	providerStartedAt := start.ProviderStartedAt
	if providerStartedAt.IsZero() {
		providerStartedAt = time.Now()
	}
	row, err := r.q.CreateTaskAttempt(ctx, dbsqlc.CreateTaskAttemptParams{
		TaskID:             taskID,
		RiverJobID:         start.RiverJobID,
		LeaseToken:         strings.TrimSpace(start.LeaseToken),
		AccountID:          start.AccountID,
		ProviderEndpointID: start.ProviderEndpointID,
		CredentialID:       start.CredentialID,
		ProviderStartedAt:  sql.NullTime{Time: providerStartedAt, Valid: true},
	})
	if err != nil {
		return tasks.TaskAttemptRef{}, err
	}
	return tasks.TaskAttemptRef{ID: row.ID, LeaseID: row.LeaseID, AttemptNo: int(row.AttemptNo)}, nil
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
	failureStage := strings.TrimSpace(finish.FailureStage)
	failureCategory := strings.TrimSpace(finish.FailureCategory)
	errorMsg := strings.TrimSpace(finish.ErrorMsg)
	switch status {
	case tasks.TaskAttemptStatusCompleted:
		failureStage = ""
		failureCategory = ""
		errorMsg = ""
	case tasks.TaskAttemptStatusCancelled:
		if failureStage == "" {
			failureStage = tasks.ExecutionStageQueued
		}
		failureCategory = tasks.FailureCategoryCancelled
		if errorMsg == "" {
			errorMsg = "task cancelled"
		}
	case tasks.TaskAttemptStatusFailed, tasks.TaskAttemptStatusRequeued:
		if failureStage == "" {
			failureStage = tasks.ExecutionStageAssigned
		}
		if failureCategory == "" {
			failureCategory = tasks.FailureCategoryUnknown
		}
		if errorMsg == "" {
			errorMsg = "task attempt finished"
		}
	}
	completedAt := finish.CompletedAt
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	return dbsqlc.FinishTaskAttemptParams{
		Status:             status,
		FailureStage:       failureStage,
		FailureCategory:    failureCategory,
		ErrorMsg:           errorMsg,
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

func (r *Repository) ListMonitorGoldenSignalSamplesByRange(ctx context.Context, start, now time.Time) ([]MonitorGoldenSignalSample, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("monitor golden signal store not configured")
	}
	if start.IsZero() || now.IsZero() {
		return nil, fmt.Errorf("monitor golden signal date range is required")
	}
	if !now.After(start) {
		return nil, fmt.Errorf("monitor golden signal now_at must be after started_at")
	}
	rows, err := r.db.QueryContext(ctx, `
WITH task_samples AS (
  SELECT
    t.task_id,
    t.created_at,
    t.status,
    COALESCE(m.failure_stage, '') AS failure_stage,
    COALESCE(m.failure_category, '') AS failure_category,
    COALESCE(m.queue_wait_ms, 0) AS queue_wait_ms,
    COALESCE(m.provider_latency_ms, 0) AS provider_latency_ms,
    COALESCE(m.finalize_latency_ms, 0) AS finalize_latency_ms,
    (
      m.provider_started_at IS NOT NULL
      OR m.provider_finished_at IS NOT NULL
      OR COALESCE(m.provider_latency_ms, 0) > 0
      OR m.failure_stage = 'provider'
      OR m.failure_category IN ('auth', 'quota', 'timeout', 'provider_error')
    ) AS provider_observed,
    (
      t.status = 'failed'
      AND (
        m.failure_stage = 'provider'
        OR m.failure_category IN ('auth', 'quota', 'timeout', 'provider_error')
      )
    ) AS provider_error
  FROM tasks t
  JOIN task_execution_metrics m ON m.task_id = t.task_id
  WHERE t.created_at >= $1 AND t.created_at < $2
),
attempt_samples AS (
  SELECT
    ta.task_id,
    COUNT(*) FILTER (
      WHERE ta.provider_started_at IS NOT NULL
        OR ta.provider_finished_at IS NOT NULL
        OR ta.provider_latency_ms > 0
        OR ta.status IN ('completed', 'failed', 'requeued')
    ) AS provider_attempts,
    COUNT(*) FILTER (
      WHERE ta.status IN ('failed', 'requeued')
        AND (
          ta.failure_stage = 'provider'
          OR ta.failure_category IN ('auth', 'quota', 'timeout', 'provider_error')
        )
    ) AS provider_errors
  FROM task_attempts ta
  JOIN task_samples s ON s.task_id = ta.task_id
  GROUP BY ta.task_id
)
SELECT
  s.status,
  s.failure_stage,
  s.failure_category,
  s.queue_wait_ms,
  s.provider_latency_ms,
  s.finalize_latency_ms,
  s.provider_observed,
  s.provider_error,
  COALESCE(a.provider_attempts, CASE WHEN s.provider_observed THEN 1 ELSE 0 END) AS provider_attempts,
  COALESCE(a.provider_errors, CASE WHEN s.provider_error THEN 1 ELSE 0 END) AS provider_errors
FROM task_samples s
LEFT JOIN attempt_samples a ON a.task_id = s.task_id
ORDER BY s.created_at DESC
LIMIT 500`, start, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	samples := []MonitorGoldenSignalSample{}
	for rows.Next() {
		var sample MonitorGoldenSignalSample
		if err := rows.Scan(
			&sample.Status,
			&sample.FailureStage,
			&sample.FailureCategory,
			&sample.QueueWaitMS,
			&sample.ProviderLatencyMS,
			&sample.FinalizeLatencyMS,
			&sample.ProviderObserved,
			&sample.ProviderError,
			&sample.ProviderAttempts,
			&sample.ProviderErrors,
		); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return samples, nil
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
