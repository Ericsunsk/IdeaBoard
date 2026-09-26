-- Workspace-backed control monitor/statistics reads (#809-close).
--
-- The workspace schema keeps the legacy stat tables (task_daily_stats,
-- task_stat_events, task_daily_stat_backfills) and the execution
-- attribution tables (task_execution_attempts, task_execution_metrics),
-- but the task fact source is now the workspace-scoped tasks table
-- (public_id LIKE 'task_%', statuses
-- pending/running/completed/failed/cancelled). There is no
-- cancelled_with_results status in this schema: counts for it read as 0.
--
-- Backfill dimension mapping (clean cut, no legacy compat):
--   task_id              <- tasks.public_id (VARCHAR(64) fits task_... ids)
--   user_id              <- tasks.submitter_user_id
--   account_id           <- tasks.billing_account_id (workspace billing
--                           account; preserves the per-account breakdown)
--   provider_endpoint_id <- 0 (the workspace schema records no provider
--                           endpoint assignment on tasks)
--   credential_id        <- 0 (same: no credential assignment on tasks)
--   feature_slug         <- tasks.workflow_type
--   api_type             <- '' (no provider api_type on workspace tasks)
--   model_name           <- latest execution attempt resolved_model, else
--                           '未识别模型' (attempts are empty until the
--                           worker records them; the fallback is honest)
--   output_count         <- latest billing snapshot quantity clamped to
--                           1..4 (immutable quote fact; satisfies the
--                           task_stat_events output_count CHECK). Tasks
--                           without a snapshot backfill with 1.
--   latency_seconds      <- completed_at - created_at for completed tasks
--   cost_micros          <- 0 (no cost attribution in the workspace schema)
--
-- Backfill stays idempotent: task_stat_events inserts use ON CONFLICT
-- (task_id) DO NOTHING and daily deltas aggregate only newly inserted
-- events, so re-running a day never double-counts. The
-- task_daily_stat_backfills marker still gates rescans.

-- name: CountWorkspaceTasksByStatus :one
SELECT COUNT(*)::bigint AS count
FROM tasks
WHERE status = sqlc.arg(status);

-- name: SumWorkspaceTaskBillingQuantity :one
SELECT COALESCE(SUM(quantity), 0)::bigint AS total_calls
FROM task_billing_snapshots;

-- name: ListDailyStatsByDateRange :many
SELECT stat_date, model_name, user_key, total, completed, failed, cancelled, latency_seconds_sum, latency_count, cost_micros_sum
FROM task_daily_stats
WHERE stat_date >= sqlc.arg(start_date) AND stat_date <= sqlc.arg(end_date)
ORDER BY stat_date ASC;

-- name: ListStatEventsByOccurredRange :many
SELECT occurred_at, stat_date, model_name, user_key, status, output_count, latency_seconds, cost_micros
FROM task_stat_events
WHERE occurred_at >= sqlc.arg(started_at) AND occurred_at < sqlc.arg(ended_at)
ORDER BY occurred_at ASC;

-- name: ListStatEventLatencySamplesByDateRange :many
SELECT model_name, latency_seconds
FROM task_stat_events
WHERE stat_date >= sqlc.arg(start_date) AND stat_date <= sqlc.arg(end_date) AND latency_seconds > 0
ORDER BY model_name ASC, latency_seconds ASC;

-- name: ListBackfilledDatesByRange :many
SELECT stat_date
FROM task_daily_stat_backfills
WHERE stat_date >= sqlc.arg(start_date) AND stat_date <= sqlc.arg(end_date)
ORDER BY stat_date ASC;

-- Execution attribution reads join on the workspace public id. Metrics
-- rows only exist once the worker records them; until then every task
-- groups under 'unknown', which the monitor renders as-is (no zero-fill
-- fabrication here).

-- name: ListTaskExecutionBottleneckStatsByCreatedRange :many
SELECT COALESCE(NULLIF(m.bottleneck_source, ''), 'unknown')::text AS bottleneck_source,
       COUNT(*)::bigint AS total,
       COUNT(*) FILTER (WHERE t.status = 'failed')::bigint AS failed,
       COALESCE(AVG(COALESCE(m.total_latency_ms, 0)), 0)::double precision AS avg_latency_ms
FROM tasks t
LEFT JOIN task_execution_metrics m ON m.task_id = t.public_id
WHERE t.created_at >= sqlc.arg(started_at) AND t.created_at < sqlc.arg(ended_at)
  AND t.status IN ('completed', 'failed')
GROUP BY COALESCE(NULLIF(m.bottleneck_source, ''), 'unknown')
ORDER BY total DESC, bottleneck_source ASC;

-- name: ListTaskExecutionFailureCategoryStatsByCreatedRange :many
SELECT COALESCE(NULLIF(m.failure_category, ''), 'unknown')::text AS failure_category,
       COUNT(*)::bigint AS total,
       COALESCE(AVG(COALESCE(m.total_latency_ms, 0)), 0)::double precision AS avg_latency_ms
FROM tasks t
LEFT JOIN task_execution_metrics m ON m.task_id = t.public_id
WHERE t.created_at >= sqlc.arg(started_at) AND t.created_at < sqlc.arg(ended_at)
  AND t.status = 'failed'
GROUP BY COALESCE(NULLIF(m.failure_category, ''), 'unknown')
ORDER BY total DESC, failure_category ASC;

-- Monitor execution pressure: live pending tasks from the workspace table
-- plus recent execution attempts. Attempt buckets stay empty until the
-- worker records attempts; pending buckets are live immediately.

-- name: ListMonitorExecutionPressureByRange :many
WITH pending_tasks AS (
  SELECT
    CASE
      WHEN GREATEST(EXTRACT(EPOCH FROM (sqlc.arg(now_at)::timestamptz - t.updated_at)), 0) >= 60 THEN 'backlog_pending'
      ELSE 'normal_pending'
    END AS bucket_key,
    GREATEST(EXTRACT(EPOCH FROM (sqlc.arg(now_at)::timestamptz - t.updated_at)), 0)::double precision AS duration_seconds,
    0::bigint AS failed,
    0::bigint AS retry
  FROM tasks t
  WHERE t.status = 'pending'
    AND t.updated_at <= sqlc.arg(now_at)
),
timed_attempts AS (
  SELECT
    ta.status,
    ta.attempt_no,
    CASE
      WHEN ta.status = 'running'
      THEN GREATEST(EXTRACT(EPOCH FROM (sqlc.arg(now_at)::timestamptz - ta.created_at)), 0)::double precision
      WHEN ta.completed_at IS NOT NULL AND ta.completed_at > ta.created_at
      THEN EXTRACT(EPOCH FROM (ta.completed_at - ta.created_at))::double precision
      ELSE 0::double precision
    END AS duration_seconds
  FROM task_execution_attempts ta
  WHERE (
      ta.status = 'running'
      AND ta.created_at <= sqlc.arg(now_at)
    )
    OR (
      ta.status IN ('completed', 'failed', 'requeued', 'cancelled')
      AND ta.completed_at IS NOT NULL
      AND ta.completed_at >= sqlc.arg(started_at)::timestamptz
      AND ta.completed_at < sqlc.arg(now_at)
    )
),
attempt_pressure AS (
  SELECT
    CASE
      WHEN status = 'running' AND duration_seconds >= 120 THEN 'stuck_running'
      WHEN status = 'running' THEN 'normal_running'
      WHEN status = 'completed' AND duration_seconds >= 60 THEN 'slow_completed'
      WHEN status = 'completed' THEN 'fast_completed'
      WHEN status = 'failed' AND duration_seconds >= 60 THEN 'slow_failed'
      WHEN status = 'failed' THEN 'fast_failed'
      WHEN status = 'cancelled' THEN 'cancelled'
      WHEN status = 'requeued' THEN 'retrying'
      ELSE 'other'
    END AS bucket_key,
    duration_seconds,
    CASE WHEN status = 'failed' THEN 1 ELSE 0 END::bigint AS failed,
    CASE WHEN status = 'requeued' OR attempt_no > 1 THEN 1 ELSE 0 END::bigint AS retry
  FROM timed_attempts
),
pressure_rows AS (
  SELECT bucket_key, duration_seconds, failed, retry
  FROM pending_tasks
  UNION ALL
  SELECT bucket_key, duration_seconds, failed, retry
  FROM attempt_pressure
)
SELECT
  bucket_key,
  COUNT(*)::bigint AS total,
  COALESCE(AVG(duration_seconds), 0)::double precision AS avg_duration_seconds,
  COALESCE(MAX(duration_seconds), 0)::double precision AS max_duration_seconds,
  COALESCE(SUM(failed), 0)::bigint AS failed,
  COALESCE(SUM(retry), 0)::bigint AS retry
FROM pressure_rows
GROUP BY bucket_key
ORDER BY bucket_key ASC;

-- BackfillDailyStatsByCreatedRange freezes one day of terminal workspace
-- tasks into task_stat_events + task_daily_stats. started_at/ended_at
-- bound the terminal-timestamp window (COALESCE(completed_at,
-- cancelled_at, updated_at)); stat_date stamps the frozen day bucket;
-- mark_done records the day marker (today is never marked so the current
-- day stays re-backfillable).

-- name: BackfillDailyStatsByCreatedRange :one
WITH candidate_tasks AS (
  SELECT t.public_id AS task_id,
         t.submitter_user_id AS user_id,
         t.billing_account_id AS account_id,
         t.status,
         t.workflow_type AS feature_slug,
         COALESCE(latest_snapshot.quantity, 1) AS quantity,
         COALESCE(latest_attempt.resolved_model, '') AS resolved_model,
         t.created_at, t.completed_at, t.cancelled_at,
         COALESCE(t.completed_at, t.cancelled_at, t.updated_at) AS occurred_at
  FROM tasks AS t
  LEFT JOIN LATERAL (
    SELECT s.quantity
    FROM task_billing_snapshots AS s
    WHERE s.task_id = t.public_id
    ORDER BY s.created_at DESC
    LIMIT 1
  ) AS latest_snapshot ON TRUE
  LEFT JOIN LATERAL (
    SELECT resolved_model
    FROM task_execution_attempts
    WHERE task_id = t.public_id
    ORDER BY (status = 'running') DESC, attempt_no DESC, id DESC
    LIMIT 1
  ) AS latest_attempt ON TRUE
  WHERE COALESCE(t.completed_at, t.cancelled_at, t.updated_at) >= sqlc.arg(started_at)
    AND COALESCE(t.completed_at, t.cancelled_at, t.updated_at) < sqlc.arg(ended_at)
    AND t.status IN ('completed', 'failed', 'cancelled')
),
normalized_events AS (
  SELECT
    t.task_id,
    t.occurred_at,
    sqlc.arg(stat_date)::date AS stat_date,
    GREATEST(t.user_id, 0)::bigint AS user_id,
    COALESCE(t.account_id, 0)::bigint AS account_id,
    0::bigint AS provider_endpoint_id,
    0::bigint AS credential_id,
    LEFT(COALESCE(NULLIF(trim(t.feature_slug), ''), ''), 64)::varchar(64) AS feature_slug,
    ''::varchar(50) AS api_type,
    LEFT(COALESCE(
      NULLIF(trim(t.resolved_model), ''),
      '未识别模型'
    ), 128)::varchar(128) AS model_name,
    LEFT(COALESCE(NULLIF(t.user_id::text, '0'), 'unknown'), 128)::varchar(128) AS user_key,
    t.status,
    LEAST(GREATEST(t.quantity, 1), 4)::smallint AS output_count,
    CASE
      WHEN t.status = 'completed'
       AND t.completed_at IS NOT NULL
       AND t.completed_at > t.created_at
      THEN EXTRACT(EPOCH FROM (t.completed_at - t.created_at))::double precision
      ELSE 0::double precision
    END AS latency_seconds,
    0::bigint AS cost_micros
  FROM candidate_tasks t
),
inserted_events AS (
  INSERT INTO task_stat_events (
    task_id, occurred_at, stat_date, user_id, account_id, provider_endpoint_id, credential_id,
    feature_slug, api_type, model_name, user_key, status, output_count, latency_seconds, cost_micros
  )
  SELECT task_id, occurred_at, stat_date, user_id, account_id, provider_endpoint_id, credential_id,
         feature_slug, api_type, model_name, user_key, status, output_count, latency_seconds, cost_micros
  FROM normalized_events
  ON CONFLICT (task_id) DO NOTHING
  RETURNING stat_date, user_id, account_id, provider_endpoint_id, credential_id, feature_slug, api_type,
            model_name, user_key, status, output_count, latency_seconds, cost_micros
),
daily_delta AS (
  SELECT
    stat_date,
    user_id,
    account_id,
    provider_endpoint_id,
    credential_id,
    feature_slug,
    api_type,
    model_name,
    user_key,
    COALESCE(SUM(output_count), 0)::bigint AS total,
    COALESCE(SUM(output_count) FILTER (WHERE status = 'completed'), 0)::bigint AS completed,
    COALESCE(SUM(output_count) FILTER (WHERE status = 'failed'), 0)::bigint AS failed,
    COALESCE(SUM(output_count) FILTER (WHERE status = 'cancelled'), 0)::bigint AS cancelled,
    COALESCE(SUM(latency_seconds), 0)::double precision AS latency_seconds_sum,
    COUNT(*) FILTER (WHERE status = 'completed' AND latency_seconds > 0)::bigint AS latency_count,
    COALESCE(SUM(cost_micros), 0)::bigint AS cost_micros_sum
  FROM inserted_events
  GROUP BY stat_date, user_id, account_id, provider_endpoint_id, credential_id, feature_slug, api_type, model_name, user_key
),
upserted_daily AS (
  INSERT INTO task_daily_stats (
    stat_date, user_id, account_id, provider_endpoint_id, credential_id, feature_slug, api_type, model_name, user_key,
    total, completed, failed, cancelled,
    latency_seconds_sum, latency_count, cost_micros_sum
  )
  SELECT stat_date, user_id, account_id, provider_endpoint_id, credential_id, feature_slug, api_type, model_name, user_key,
         total, completed, failed, cancelled,
         latency_seconds_sum, latency_count, cost_micros_sum
  FROM daily_delta
  ON CONFLICT (stat_date, user_id, account_id, provider_endpoint_id, credential_id, feature_slug, api_type, model_name, user_key) DO UPDATE
  SET total = task_daily_stats.total + EXCLUDED.total,
      completed = task_daily_stats.completed + EXCLUDED.completed,
      failed = task_daily_stats.failed + EXCLUDED.failed,
      cancelled = task_daily_stats.cancelled + EXCLUDED.cancelled,
      latency_seconds_sum = task_daily_stats.latency_seconds_sum + EXCLUDED.latency_seconds_sum,
      latency_count = task_daily_stats.latency_count + EXCLUDED.latency_count,
      cost_micros_sum = task_daily_stats.cost_micros_sum + EXCLUDED.cost_micros_sum
  RETURNING 1
),
marked AS (
  INSERT INTO task_daily_stat_backfills (stat_date)
  SELECT sqlc.arg(stat_date)::date
  WHERE sqlc.arg(mark_done)::boolean
  ON CONFLICT (stat_date) DO NOTHING
  RETURNING 1
)
SELECT
  (SELECT COUNT(*) FROM inserted_events)::bigint AS inserted_events,
  (SELECT COUNT(*) FROM upserted_daily)::bigint AS touched_groups,
  (SELECT COUNT(*) FROM marked)::bigint AS marked_days;
