package postgres

import (
	"context"
	"fmt"
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	tasks "ideaboard/api/internal/tasking/domain"
	statisticscontracts "ideaboard/api/internal/tasking/ports/statistics"
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

func (r *Repository) PatchExecutionSnapshot(ctx context.Context, snapshot tasks.TaskExecutionSnapshot) (int64, error) {
	taskID := strings.TrimSpace(snapshot.TaskID)
	if taskID == "" {
		return 0, fmt.Errorf("task_id is required")
	}
	return r.q.PatchTaskExecutionSnapshot(ctx, dbsqlc.PatchTaskExecutionSnapshotParams{
		TaskID:      taskID,
		FeatureSlug: strings.TrimSpace(snapshot.FeatureSlug),
	})
}

func (r *Repository) ListMonitorExecutionPressureByRange(ctx context.Context, start, now time.Time) ([]statisticscontracts.MonitorExecutionPressureAggregate, error) {
	if start.IsZero() || now.IsZero() {
		return nil, fmt.Errorf("monitor execution pressure date range is required")
	}
	if !now.After(start) {
		return nil, fmt.Errorf("monitor execution pressure now_at must be after started_at")
	}
	rows, err := r.q.ListMonitorExecutionPressureByRange(ctx, dbsqlc.ListMonitorExecutionPressureByRangeParams{
		NowAt:     dbutil.TimestamptzFromTime(now),
		StartedAt: dbutil.TimestamptzFromTime(start),
	})
	if err != nil {
		return nil, err
	}
	result := make([]statisticscontracts.MonitorExecutionPressureAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, statisticscontracts.MonitorExecutionPressureAggregate{
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

func (r *Repository) ListMonitorGoldenSignalSamplesByRange(ctx context.Context, start, now time.Time) ([]statisticscontracts.MonitorGoldenSignalSample, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("monitor golden signal store not configured")
	}
	if start.IsZero() || now.IsZero() {
		return nil, fmt.Errorf("monitor golden signal date range is required")
	}
	if !now.After(start) {
		return nil, fmt.Errorf("monitor golden signal now_at must be after started_at")
	}
	rows, err := r.dbWithContext(ctx).Query(`
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
    ) AS provider_attempts,
    COUNT(*) FILTER (
      WHERE ta.status IN ('failed', 'requeued')
        AND (
          ta.failure_stage = 'provider'
          OR ta.failure_category IN ('auth', 'quota', 'timeout', 'provider_error')
        )
    ) AS provider_errors
  FROM task_execution_attempts ta
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

	samples := []statisticscontracts.MonitorGoldenSignalSample{}
	for rows.Next() {
		var sample statisticscontracts.MonitorGoldenSignalSample
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

func (r *Repository) ListExecutionBottleneckStatsByCreatedRange(ctx context.Context, start, end time.Time) ([]statisticscontracts.ExecutionBottleneckAggregate, error) {
	rows, err := r.q.ListTaskExecutionBottleneckStatsByCreatedRange(ctx, dbsqlc.ListTaskExecutionBottleneckStatsByCreatedRangeParams{
		StartedAt: dbutil.TimestamptzFromTime(start),
		EndedAt:   dbutil.TimestamptzFromTime(end),
	})
	if err != nil {
		return nil, err
	}
	result := make([]statisticscontracts.ExecutionBottleneckAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, statisticscontracts.ExecutionBottleneckAggregate{
			Source:       row.BottleneckSource,
			Total:        row.Total,
			Failed:       row.Failed,
			AvgLatencyMS: row.AvgLatencyMs,
		})
	}
	return result, nil
}

func (r *Repository) ListExecutionFailureCategoryStatsByCreatedRange(ctx context.Context, start, end time.Time) ([]statisticscontracts.ExecutionFailureCategoryAggregate, error) {
	rows, err := r.q.ListTaskExecutionFailureCategoryStatsByCreatedRange(ctx, dbsqlc.ListTaskExecutionFailureCategoryStatsByCreatedRangeParams{
		StartedAt: dbutil.TimestamptzFromTime(start),
		EndedAt:   dbutil.TimestamptzFromTime(end),
	})
	if err != nil {
		return nil, err
	}
	result := make([]statisticscontracts.ExecutionFailureCategoryAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, statisticscontracts.ExecutionFailureCategoryAggregate{
			Category:     row.FailureCategory,
			Total:        row.Total,
			AvgLatencyMS: row.AvgLatencyMs,
		})
	}
	return result, nil
}
