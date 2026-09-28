package postgres

// Workspace-backed control monitor/statistics store (#809-close).
//
// The legacy statistics store was deleted in phase 3b because its SQL
// targeted dropped tables. This store re-implements the 12-method
// application statistics.Store seam over the CURRENT schema only:
//   - task facts read from the workspace-scoped tasks table
//     (public_id, submitter_user_id, billing_account_id, service,
//     created/completed/cancelled timestamps);
//   - aggregates read from task_daily_stats / task_stat_events /
//     task_daily_stat_backfills (populated by BackfillDailyStatsByCreatedRange);
//   - execution attribution reads from task_execution_attempts /
//     task_execution_metrics (populated by the worker once it records
//     attempts; until then attribution groups under 'unknown' and golden
//     signals sample zero rows — rendered as-is, never fabricated);
//   - captured-credits reads from credit_transactions (capture/refund),
//     whose schema is unchanged.
//
// No legacy tables or queries are referenced. Backfill dimension mapping
// is documented in internal/db/queries/task_statistics.sql.

import (
	"context"
	"fmt"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	statisticscontracts "ideaboard/api/internal/tasking/ports/statistics"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StatisticsStore serves the control monitor/statistics/backfill reads.
// Repository methods take context.Context first and use the dbWithContext
// pattern, matching the settlement Store in this package.
type StatisticsStore struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// NewStatisticsStore builds a statistics Store over pool. A nil pool keeps
// every method fail-closed with an error (composition leaves the control
// Monitor handler nil in that case, so the routes stay 501).
func NewStatisticsStore(db *pgxpool.Pool) *StatisticsStore {
	if db == nil {
		return &StatisticsStore{}
	}
	return &StatisticsStore{db: db, q: dbsqlc.New(db)}
}

func (s *StatisticsStore) dbWithContext(ctx context.Context) repositoryQueryer {
	return repositoryQueryer{ctx: ctx, db: s.db, q: s.q}
}

func (s *StatisticsStore) ready() error {
	if s == nil || s.db == nil || s.q == nil {
		return fmt.Errorf("tasking statistics: database pool not configured")
	}
	return nil
}

// CountByStatus counts workspace tasks in one status. The workspace schema
// has no cancelled_with_results status, so that count honestly reads 0.
func (s *StatisticsStore) CountByStatus(ctx context.Context, status string) (int64, error) {
	if err := s.ready(); err != nil {
		return 0, err
	}
	return s.dbWithContext(ctx).q.CountWorkspaceTasksByStatus(ctx, status)
}

// CountAllTaskCalls sums quoted billing quantities (the workspace
// equivalent of the legacy requested-output-count sum). Tasks without a
// billing snapshot contribute nothing.
func (s *StatisticsStore) CountAllTaskCalls(ctx context.Context) (int64, error) {
	if err := s.ready(); err != nil {
		return 0, err
	}
	return s.dbWithContext(ctx).q.SumWorkspaceTaskBillingQuantity(ctx)
}

// ListBackfilledDatesByRange lists frozen day markers in [startDate, endDate].
func (s *StatisticsStore) ListBackfilledDatesByRange(ctx context.Context, startDate, endDate time.Time) ([]time.Time, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if startDate.IsZero() || endDate.IsZero() {
		return nil, fmt.Errorf("tasking statistics: stat date range is required")
	}
	rows, err := s.dbWithContext(ctx).q.ListBackfilledDatesByRange(ctx, dbsqlc.ListBackfilledDatesByRangeParams{
		StartDate: dbutil.DateFromTime(startDate),
		EndDate:   dbutil.DateFromTime(endDate),
	})
	if err != nil {
		return nil, err
	}
	result := make([]time.Time, 0, len(rows))
	for _, row := range rows {
		result = append(result, dbutil.DateValue(row))
	}
	return result, nil
}

// ListDailyStatsByDateRange reads frozen daily aggregates in [startDate, endDate].
func (s *StatisticsStore) ListDailyStatsByDateRange(ctx context.Context, startDate, endDate time.Time) ([]statisticscontracts.DailyStatAggregate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if startDate.IsZero() || endDate.IsZero() {
		return nil, fmt.Errorf("tasking statistics: stat date range is required")
	}
	rows, err := s.dbWithContext(ctx).q.ListDailyStatsByDateRange(ctx, dbsqlc.ListDailyStatsByDateRangeParams{
		StartDate: dbutil.DateFromTime(startDate),
		EndDate:   dbutil.DateFromTime(endDate),
	})
	if err != nil {
		return nil, err
	}
	result := make([]statisticscontracts.DailyStatAggregate, 0, len(rows))
	for _, row := range rows {
		result = append(result, statisticscontracts.DailyStatAggregate{
			StatDate:          dbutil.DateValue(row.StatDate),
			ModelName:         row.ModelName,
			UserKey:           row.UserKey,
			Total:             row.Total,
			Completed:         row.Completed,
			Failed:            row.Failed,
			Cancelled:         row.Cancelled,
			LatencySecondsSum: row.LatencySecondsSum,
			LatencyCount:      row.LatencyCount,
			CostMicrosSum:     row.CostMicrosSum,
		})
	}
	return result, nil
}

// ListDailyCaptureCreditsByRange sums capture minus refund credits per day,
// bucketed in the caller's location. The credit_transactions schema is
// unchanged, so this keeps the legacy day-bucketing semantics verbatim.
func (s *StatisticsStore) ListDailyCaptureCreditsByRange(ctx context.Context, start, end time.Time, location *time.Location) (map[string]int64, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil, fmt.Errorf("tasking statistics: valid daily capture range is required")
	}
	if location == nil {
		return nil, fmt.Errorf("tasking statistics: daily capture location is required")
	}
	timezoneName := location.String()
	if timezoneName == "" || timezoneName == "Local" {
		// Go reports unnamed local zones as "Local", which PostgreSQL
		// cannot resolve. Fall back to the numeric UTC offset at the range
		// start so day buckets stay aligned; named zones keep DST-aware
		// grouping. PostgreSQL reads numeric AT TIME ZONE offsets with
		// POSIX sign convention (positive = west), opposite of Go/ISO
		// offsets, so the sign is flipped: "+08:00" must pass as "-08:00".
		_, offset := start.In(location).Zone()
		sign := "-"
		if offset < 0 {
			sign = "+"
			offset = -offset
		}
		timezoneName = fmt.Sprintf("%s%02d:%02d", sign, offset/3600, (offset%3600)/60)
	}
	rows, err := s.db.Query(ctx, `
SELECT ((created_at AT TIME ZONE $3)::date)::text AS capture_day,
       SUM(CASE WHEN command_type = 'capture' THEN command_amount
                WHEN command_type = 'refund' THEN -command_amount
                ELSE 0 END)::bigint AS amount
FROM credit_transactions
WHERE command_type IN ('capture', 'refund')
  AND created_at >= $1
  AND created_at < $2
GROUP BY (created_at AT TIME ZONE $3)::date
ORDER BY capture_day`, start, end, timezoneName)
	if err != nil {
		return nil, fmt.Errorf("tasking statistics: query daily capture credits: %w", err)
	}
	defer rows.Close()
	dailyCredits := make(map[string]int64)
	for rows.Next() {
		var date string
		var amount int64
		if err := rows.Scan(&date, &amount); err != nil {
			return nil, fmt.Errorf("tasking statistics: scan daily capture credits: %w", err)
		}
		dailyCredits[date] = amount
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasking statistics: read daily capture credits: %w", err)
	}
	return dailyCredits, nil
}

// ListExecutionBottleneckStatsByCreatedRange attributes terminal tasks
// created in [start, end) by bottleneck source. Tasks without a metrics
// row group under 'unknown'.
func (s *StatisticsStore) ListExecutionBottleneckStatsByCreatedRange(ctx context.Context, start, end time.Time) ([]statisticscontracts.ExecutionBottleneckAggregate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if start.IsZero() || end.IsZero() {
		return nil, fmt.Errorf("tasking statistics: execution range is required")
	}
	rows, err := s.dbWithContext(ctx).q.ListTaskExecutionBottleneckStatsByCreatedRange(ctx, dbsqlc.ListTaskExecutionBottleneckStatsByCreatedRangeParams{
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

// ListExecutionFailureCategoryStatsByCreatedRange attributes failed tasks
// created in [start, end) by failure category.
func (s *StatisticsStore) ListExecutionFailureCategoryStatsByCreatedRange(ctx context.Context, start, end time.Time) ([]statisticscontracts.ExecutionFailureCategoryAggregate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if start.IsZero() || end.IsZero() {
		return nil, fmt.Errorf("tasking statistics: execution range is required")
	}
	rows, err := s.dbWithContext(ctx).q.ListTaskExecutionFailureCategoryStatsByCreatedRange(ctx, dbsqlc.ListTaskExecutionFailureCategoryStatsByCreatedRangeParams{
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

// ListStatEventsByOccurredRange reads raw stat events with
// occurred_at in [start, end).
func (s *StatisticsStore) ListStatEventsByOccurredRange(ctx context.Context, start, end time.Time) ([]statisticscontracts.StatEventTrendRow, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if start.IsZero() || end.IsZero() {
		return nil, fmt.Errorf("tasking statistics: stat event occurred range is required")
	}
	rows, err := s.dbWithContext(ctx).q.ListStatEventsByOccurredRange(ctx, dbsqlc.ListStatEventsByOccurredRangeParams{
		StartedAt: dbutil.TimestamptzFromTime(start),
		EndedAt:   dbutil.TimestamptzFromTime(end),
	})
	if err != nil {
		return nil, err
	}
	result := make([]statisticscontracts.StatEventTrendRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, statisticscontracts.StatEventTrendRow{
			OccurredAt:     dbutil.TimeValue(row.OccurredAt),
			StatDate:       dbutil.DateValue(row.StatDate),
			ModelName:      row.ModelName,
			UserKey:        row.UserKey,
			Status:         row.Status,
			OutputCount:    int64(row.OutputCount),
			LatencySeconds: row.LatencySeconds,
			CostMicros:     row.CostMicros,
		})
	}
	return result, nil
}

// ListStatEventLatencySamplesByDateRange reads positive latency samples
// for stat dates in [startDate, endDate].
func (s *StatisticsStore) ListStatEventLatencySamplesByDateRange(ctx context.Context, startDate, endDate time.Time) ([]statisticscontracts.StatEventLatencySample, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if startDate.IsZero() || endDate.IsZero() {
		return nil, fmt.Errorf("tasking statistics: stat date range is required")
	}
	rows, err := s.dbWithContext(ctx).q.ListStatEventLatencySamplesByDateRange(ctx, dbsqlc.ListStatEventLatencySamplesByDateRangeParams{
		StartDate: dbutil.DateFromTime(startDate),
		EndDate:   dbutil.DateFromTime(endDate),
	})
	if err != nil {
		return nil, err
	}
	result := make([]statisticscontracts.StatEventLatencySample, 0, len(rows))
	for _, row := range rows {
		result = append(result, statisticscontracts.StatEventLatencySample{
			ModelName:      row.ModelName,
			LatencySeconds: row.LatencySeconds,
		})
	}
	return result, nil
}

// ListMonitorExecutionPressureByRange buckets live pending workspace tasks
// plus recent execution attempts in (start, now]. Attempt buckets stay
// empty until the worker records attempts.
func (s *StatisticsStore) ListMonitorExecutionPressureByRange(ctx context.Context, start, now time.Time) ([]statisticscontracts.MonitorExecutionPressureAggregate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if start.IsZero() || now.IsZero() {
		return nil, fmt.Errorf("tasking statistics: monitor execution pressure date range is required")
	}
	if !now.After(start) {
		return nil, fmt.Errorf("tasking statistics: monitor execution pressure now_at must be after started_at")
	}
	rows, err := s.dbWithContext(ctx).q.ListMonitorExecutionPressureByRange(ctx, dbsqlc.ListMonitorExecutionPressureByRangeParams{
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

// ListMonitorGoldenSignalSamplesByRange samples tasks created in
// [start, now) that carry an execution metrics row (INNER JOIN). Until
// the worker records metrics this honestly returns zero samples and the
// service renders empty golden signals.
func (s *StatisticsStore) ListMonitorGoldenSignalSamplesByRange(ctx context.Context, start, now time.Time) ([]statisticscontracts.MonitorGoldenSignalSample, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if start.IsZero() || now.IsZero() {
		return nil, fmt.Errorf("tasking statistics: monitor golden signal date range is required")
	}
	if !now.After(start) {
		return nil, fmt.Errorf("tasking statistics: monitor golden signal now_at must be after started_at")
	}
	rows, err := s.db.Query(ctx, `
WITH task_samples AS (
  SELECT
    t.public_id AS task_id,
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
  JOIN task_execution_metrics m ON m.task_id = t.public_id
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

// BackfillDailyStatsByCreatedRange freezes terminal workspace tasks whose
// terminal timestamp falls in [startedAt, endedAt) into task_stat_events
// (idempotent per task_id) plus their daily aggregate deltas, and records
// the statDate marker when markDone. Re-running a day inserts nothing new.
func (s *StatisticsStore) BackfillDailyStatsByCreatedRange(ctx context.Context, startedAt, endedAt, statDate time.Time, markDone bool) (statisticscontracts.BackfillDailyStatsResult, error) {
	if err := s.ready(); err != nil {
		return statisticscontracts.BackfillDailyStatsResult{}, err
	}
	row, err := s.dbWithContext(ctx).q.BackfillDailyStatsByCreatedRange(ctx, dbsqlc.BackfillDailyStatsByCreatedRangeParams{
		StartedAt: dbutil.TimestamptzFromTime(startedAt),
		EndedAt:   dbutil.TimestamptzFromTime(endedAt),
		StatDate:  dbutil.DateFromTime(statDate),
		MarkDone:  markDone,
	})
	if err != nil {
		return statisticscontracts.BackfillDailyStatsResult{}, err
	}
	return statisticscontracts.BackfillDailyStatsResult{
		InsertedEvents: row.InsertedEvents,
		TouchedGroups:  row.TouchedGroups,
		MarkedDays:     row.MarkedDays,
	}, nil
}
