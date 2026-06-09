-- name: InsertTaskStatEventIgnore :execrows
INSERT INTO task_stat_events (
  task_id, stat_date, user_id, account_id, channel_id, feature_slug, api_type,
  model_name, user_key, status, latency_seconds
) VALUES (
  sqlc.arg(task_id), sqlc.arg(stat_date), sqlc.arg(user_id), sqlc.arg(account_id),
  sqlc.arg(channel_id), sqlc.arg(feature_slug), sqlc.arg(api_type), sqlc.arg(model_name),
  sqlc.arg(user_key), sqlc.arg(status), sqlc.arg(latency_seconds)
)
ON CONFLICT (task_id) DO NOTHING;

-- name: UpsertTaskDailyStatDelta :exec
INSERT INTO task_daily_stats (
  stat_date, user_id, account_id, channel_id, feature_slug, api_type, model_name, user_key,
  total, completed, failed, cancelled, latency_seconds_sum, latency_count
) VALUES (
  sqlc.arg(stat_date), sqlc.arg(user_id), sqlc.arg(account_id), sqlc.arg(channel_id),
  sqlc.arg(feature_slug), sqlc.arg(api_type), sqlc.arg(model_name), sqlc.arg(user_key),
  sqlc.arg(total), sqlc.arg(completed), sqlc.arg(failed), sqlc.arg(cancelled),
  sqlc.arg(latency_seconds_sum), sqlc.arg(latency_count)
)
ON CONFLICT (stat_date, user_id, account_id, channel_id, feature_slug, api_type, model_name, user_key) DO UPDATE
SET total = task_daily_stats.total + EXCLUDED.total,
    completed = task_daily_stats.completed + EXCLUDED.completed,
    failed = task_daily_stats.failed + EXCLUDED.failed,
    cancelled = task_daily_stats.cancelled + EXCLUDED.cancelled,
    latency_seconds_sum = task_daily_stats.latency_seconds_sum + EXCLUDED.latency_seconds_sum,
    latency_count = task_daily_stats.latency_count + EXCLUDED.latency_count;

-- name: ListDailyStatsByDateRange :many
SELECT stat_date, model_name, user_key, total, completed, failed, cancelled, latency_seconds_sum, latency_count
FROM task_daily_stats
WHERE stat_date >= sqlc.arg(start_date) AND stat_date <= sqlc.arg(end_date)
ORDER BY stat_date ASC;

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

-- name: BackfillDailyStatsByCreatedRange :one
WITH candidate_tasks AS (
  SELECT t.task_id, t.user_id, t.account_id, t.assigned_channel_id, t.status, t.result,
         t.feature_slug, t.requested_model, t.resolved_model, t.api_type, t.created_at, t.completed_at
  FROM tasks AS t
  WHERE t.created_at >= sqlc.arg(started_at)
    AND t.created_at < sqlc.arg(ended_at)
    AND t.status IN ('completed', 'failed', 'cancelled')
),
single_enabled_models AS (
  SELECT m.account_id,
         (array_agg(trim(m.model_name) ORDER BY m.sort_order, m.id))[1]::text AS model_name
  FROM api_account_models m
  JOIN api_accounts a ON a.id = m.account_id AND a.deleted_at IS NULL
  JOIN (SELECT DISTINCT account_id FROM candidate_tasks WHERE account_id IS NOT NULL) c
    ON c.account_id = m.account_id
  WHERE m.enabled = TRUE
  GROUP BY m.account_id
  HAVING COUNT(*) = 1
),
normalized_events AS (
  SELECT
    t.task_id,
    sqlc.arg(stat_date)::date AS stat_date,
    GREATEST(t.user_id, 0)::bigint AS user_id,
    COALESCE(t.account_id, 0)::bigint AS account_id,
    COALESCE(t.assigned_channel_id, 0)::bigint AS channel_id,
    LEFT(COALESCE(NULLIF(trim(t.feature_slug), ''), ''), 64)::varchar(64) AS feature_slug,
    LEFT(COALESCE(NULLIF(trim(t.api_type), ''), ''), 50)::varchar(50) AS api_type,
    LEFT(COALESCE(
      NULLIF(trim(t.resolved_model), ''),
      NULLIF(trim(t.requested_model), ''),
      CASE
        WHEN jsonb_typeof(result_json.value -> 'model') = 'string'
        THEN NULLIF(trim(result_json.value ->> 'model'), '')
      END,
      NULLIF(trim(sm.model_name), ''),
      '未识别模型'
    ), 128)::varchar(128) AS model_name,
    LEFT(COALESCE(
      CASE
        WHEN jsonb_typeof(result_json.value -> 'username') = 'string'
        THEN NULLIF(trim(result_json.value ->> 'username'), '')
      END,
      CASE
        WHEN jsonb_typeof(result_json.value -> 'email') = 'string'
        THEN NULLIF(trim(result_json.value ->> 'email'), '')
      END,
      CASE
        WHEN jsonb_typeof(result_json.value -> 'user_id') = 'string'
        THEN NULLIF(trim(result_json.value ->> 'user_id'), '')
        WHEN jsonb_typeof(result_json.value -> 'user_id') = 'number'
         AND (result_json.value ->> 'user_id')::numeric > 0
        THEN round((result_json.value ->> 'user_id')::numeric)::text
      END,
      NULLIF(t.user_id::text, '0'),
      'unknown'
    ), 128)::varchar(128) AS user_key,
    t.status,
    CASE
      WHEN t.status = 'completed'
       AND t.completed_at IS NOT NULL
       AND t.completed_at > t.created_at
      THEN EXTRACT(EPOCH FROM (t.completed_at - t.created_at))::double precision
      ELSE 0::double precision
    END AS latency_seconds
  FROM candidate_tasks t
  LEFT JOIN single_enabled_models sm ON sm.account_id = t.account_id
  LEFT JOIN LATERAL safe_jsonb_object(t.result) AS result_json(value) ON TRUE
),
inserted_events AS (
  INSERT INTO task_stat_events (
    task_id, stat_date, user_id, account_id, channel_id, feature_slug, api_type,
    model_name, user_key, status, latency_seconds
  )
  SELECT task_id, stat_date, user_id, account_id, channel_id, feature_slug, api_type,
         model_name, user_key, status, latency_seconds
  FROM normalized_events
  ON CONFLICT (task_id) DO NOTHING
  RETURNING stat_date, user_id, account_id, channel_id, feature_slug, api_type,
            model_name, user_key, status, latency_seconds
),
daily_delta AS (
  SELECT
    stat_date,
    user_id,
    account_id,
    channel_id,
    feature_slug,
    api_type,
    model_name,
    user_key,
    COUNT(*)::bigint AS total,
    COUNT(*) FILTER (WHERE status = 'completed')::bigint AS completed,
    COUNT(*) FILTER (WHERE status = 'failed')::bigint AS failed,
    COUNT(*) FILTER (WHERE status = 'cancelled')::bigint AS cancelled,
    COALESCE(SUM(latency_seconds), 0)::double precision AS latency_seconds_sum,
    COUNT(*) FILTER (WHERE status = 'completed' AND latency_seconds > 0)::bigint AS latency_count
  FROM inserted_events
  GROUP BY stat_date, user_id, account_id, channel_id, feature_slug, api_type, model_name, user_key
),
upserted_daily AS (
  INSERT INTO task_daily_stats (
    stat_date, user_id, account_id, channel_id, feature_slug, api_type, model_name, user_key,
    total, completed, failed, cancelled,
    latency_seconds_sum, latency_count
  )
  SELECT stat_date, user_id, account_id, channel_id, feature_slug, api_type, model_name, user_key,
         total, completed, failed, cancelled,
         latency_seconds_sum, latency_count
  FROM daily_delta
  ON CONFLICT (stat_date, user_id, account_id, channel_id, feature_slug, api_type, model_name, user_key) DO UPDATE
  SET total = task_daily_stats.total + EXCLUDED.total,
      completed = task_daily_stats.completed + EXCLUDED.completed,
      failed = task_daily_stats.failed + EXCLUDED.failed,
      cancelled = task_daily_stats.cancelled + EXCLUDED.cancelled,
      latency_seconds_sum = task_daily_stats.latency_seconds_sum + EXCLUDED.latency_seconds_sum,
      latency_count = task_daily_stats.latency_count + EXCLUDED.latency_count
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
