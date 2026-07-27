-- name: InsertTaskStatEventIgnore :execrows
INSERT INTO task_stat_events (
  task_id, occurred_at, stat_date, user_id, account_id, provider_endpoint_id, credential_id,
  feature_slug, api_type, model_name, user_key, status, latency_seconds, cost_micros
) VALUES (
  sqlc.arg(task_id), sqlc.arg(occurred_at), sqlc.arg(stat_date), sqlc.arg(user_id), sqlc.arg(account_id),
  sqlc.arg(provider_endpoint_id), sqlc.arg(credential_id), sqlc.arg(feature_slug), sqlc.arg(api_type),
  sqlc.arg(model_name), sqlc.arg(user_key), sqlc.arg(status), sqlc.arg(latency_seconds),
  sqlc.arg(cost_micros)
)
ON CONFLICT (task_id) DO NOTHING;

-- name: UpsertTaskStatEventCorrection :execrows
INSERT INTO task_stat_events (
  task_id, occurred_at, stat_date, user_id, account_id, provider_endpoint_id, credential_id,
  feature_slug, api_type, model_name, user_key, status, latency_seconds, cost_micros
) VALUES (
  sqlc.arg(task_id), sqlc.arg(occurred_at), sqlc.arg(stat_date), sqlc.arg(user_id), sqlc.arg(account_id),
  sqlc.arg(provider_endpoint_id), sqlc.arg(credential_id), sqlc.arg(feature_slug), sqlc.arg(api_type),
  sqlc.arg(model_name), sqlc.arg(user_key), sqlc.arg(status), sqlc.arg(latency_seconds),
  sqlc.arg(cost_micros)
)
ON CONFLICT (task_id) DO UPDATE
SET occurred_at = EXCLUDED.occurred_at,
    stat_date = EXCLUDED.stat_date,
    user_id = EXCLUDED.user_id,
    account_id = EXCLUDED.account_id,
    provider_endpoint_id = EXCLUDED.provider_endpoint_id,
    credential_id = EXCLUDED.credential_id,
    feature_slug = EXCLUDED.feature_slug,
    api_type = EXCLUDED.api_type,
    model_name = EXCLUDED.model_name,
    user_key = EXCLUDED.user_key,
    status = EXCLUDED.status,
    latency_seconds = EXCLUDED.latency_seconds,
    cost_micros = EXCLUDED.cost_micros;

-- name: UpsertTaskDailyStatDelta :exec
INSERT INTO task_daily_stats (
  stat_date, user_id, account_id, provider_endpoint_id, credential_id, feature_slug, api_type,
  model_name, user_key, total, completed, failed, cancelled, latency_seconds_sum, latency_count,
  cost_micros_sum
) VALUES (
  sqlc.arg(stat_date), sqlc.arg(user_id), sqlc.arg(account_id), sqlc.arg(provider_endpoint_id),
  sqlc.arg(credential_id), sqlc.arg(feature_slug), sqlc.arg(api_type), sqlc.arg(model_name), sqlc.arg(user_key),
  sqlc.arg(total), sqlc.arg(completed), sqlc.arg(failed), sqlc.arg(cancelled),
  sqlc.arg(latency_seconds_sum), sqlc.arg(latency_count), sqlc.arg(cost_micros_sum)
)
ON CONFLICT (stat_date, user_id, account_id, provider_endpoint_id, credential_id, feature_slug, api_type, model_name, user_key) DO UPDATE
SET total = task_daily_stats.total + EXCLUDED.total,
    completed = task_daily_stats.completed + EXCLUDED.completed,
    failed = task_daily_stats.failed + EXCLUDED.failed,
    cancelled = task_daily_stats.cancelled + EXCLUDED.cancelled,
    latency_seconds_sum = task_daily_stats.latency_seconds_sum + EXCLUDED.latency_seconds_sum,
    latency_count = task_daily_stats.latency_count + EXCLUDED.latency_count,
    cost_micros_sum = task_daily_stats.cost_micros_sum + EXCLUDED.cost_micros_sum;

-- name: ListDailyStatsByDateRange :many
SELECT stat_date, model_name, user_key, total, completed, failed, cancelled, latency_seconds_sum, latency_count, cost_micros_sum
FROM task_daily_stats
WHERE stat_date >= sqlc.arg(start_date) AND stat_date <= sqlc.arg(end_date)
ORDER BY stat_date ASC;

-- name: ListStatEventsByOccurredRange :many
SELECT occurred_at, stat_date, model_name, user_key, status, latency_seconds, cost_micros
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

-- name: RebuildTaskDailyStatsByEventDateRange :one
WITH deleted AS (
  DELETE FROM task_daily_stats
  WHERE task_daily_stats.stat_date >= sqlc.arg(start_date) AND task_daily_stats.stat_date <= sqlc.arg(end_date)
  RETURNING 1
),
daily_delta AS (
  SELECT
    e.stat_date,
    e.user_id,
    e.account_id,
    e.provider_endpoint_id,
    e.credential_id,
    e.feature_slug,
    e.api_type,
    e.model_name,
    e.user_key,
    COUNT(*)::bigint AS total,
    COUNT(*) FILTER (WHERE e.status = 'completed')::bigint AS completed,
    COUNT(*) FILTER (WHERE e.status = 'failed')::bigint AS failed,
    COUNT(*) FILTER (WHERE e.status = 'cancelled')::bigint AS cancelled,
    COALESCE(SUM(e.latency_seconds), 0)::double precision AS latency_seconds_sum,
    COUNT(*) FILTER (WHERE e.status = 'completed' AND e.latency_seconds > 0)::bigint AS latency_count,
    COALESCE(SUM(e.cost_micros), 0)::bigint AS cost_micros_sum
  FROM task_stat_events AS e
  WHERE e.stat_date >= sqlc.arg(start_date) AND e.stat_date <= sqlc.arg(end_date)
  GROUP BY e.stat_date, e.user_id, e.account_id, e.provider_endpoint_id, e.credential_id, e.feature_slug, e.api_type, e.model_name, e.user_key
),
inserted AS (
  INSERT INTO task_daily_stats (
    stat_date, user_id, account_id, provider_endpoint_id, credential_id, feature_slug, api_type,
    model_name, user_key, total, completed, failed, cancelled,
    latency_seconds_sum, latency_count, cost_micros_sum
  )
  SELECT stat_date, user_id, account_id, provider_endpoint_id, credential_id, feature_slug, api_type,
         model_name, user_key, total, completed, failed, cancelled,
         latency_seconds_sum, latency_count, cost_micros_sum
  FROM daily_delta
  RETURNING 1
)
SELECT
  (SELECT COUNT(*) FROM deleted)::bigint AS deleted_groups,
  (SELECT COUNT(*) FROM inserted)::bigint AS rebuilt_groups;

-- name: BackfillDailyStatsByCreatedRange :one
WITH candidate_tasks AS (
  SELECT t.task_id, t.user_id, t.account_id, t.assigned_provider_endpoint_id, t.assigned_credential_id, t.status,
         t.feature_slug,
         COALESCE(latest_lease.resolved_model, '') AS resolved_model,
         COALESCE(latest_lease.api_type, '') AS api_type,
         t.created_at, t.completed_at, t.cancelled_at,
         COALESCE(t.completed_at, t.cancelled_at, t.updated_at) AS occurred_at
  FROM tasks AS t
  LEFT JOIN LATERAL (
    SELECT resolved_model, api_type
    FROM task_leases
    WHERE task_id = t.task_id
    ORDER BY (released_at IS NULL) DESC, COALESCE(released_at, acquired_at) DESC, id DESC
    LIMIT 1
  ) AS latest_lease ON TRUE
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
    COALESCE(t.assigned_provider_endpoint_id, 0)::bigint AS provider_endpoint_id,
    COALESCE(t.assigned_credential_id, 0)::bigint AS credential_id,
    LEFT(COALESCE(NULLIF(trim(t.feature_slug), ''), ''), 64)::varchar(64) AS feature_slug,
    LEFT(COALESCE(NULLIF(trim(t.api_type), ''), ''), 50)::varchar(50) AS api_type,
    LEFT(COALESCE(
      NULLIF(trim(t.resolved_model), ''),
      '未识别模型'
    ), 128)::varchar(128) AS model_name,
    LEFT(COALESCE(NULLIF(t.user_id::text, '0'), 'unknown'), 128)::varchar(128) AS user_key,
    t.status,
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
    feature_slug, api_type, model_name, user_key, status, latency_seconds, cost_micros
  )
  SELECT task_id, occurred_at, stat_date, user_id, account_id, provider_endpoint_id, credential_id,
         feature_slug, api_type, model_name, user_key, status, latency_seconds, cost_micros
  FROM normalized_events
  ON CONFLICT (task_id) DO NOTHING
  RETURNING stat_date, user_id, account_id, provider_endpoint_id, credential_id, feature_slug, api_type,
            model_name, user_key, status, latency_seconds, cost_micros
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
    COUNT(*)::bigint AS total,
    COUNT(*) FILTER (WHERE status = 'completed')::bigint AS completed,
    COUNT(*) FILTER (WHERE status = 'failed')::bigint AS failed,
    COUNT(*) FILTER (WHERE status = 'cancelled')::bigint AS cancelled,
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
