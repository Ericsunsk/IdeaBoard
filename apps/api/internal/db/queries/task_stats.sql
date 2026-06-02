-- name: InsertTaskStatEventIgnore :execrows
INSERT INTO task_stat_events (
  task_id, stat_date, model_name, user_key, status, latency_seconds
) VALUES (
  sqlc.arg(task_id), sqlc.arg(stat_date), sqlc.arg(model_name), sqlc.arg(user_key),
  sqlc.arg(status), sqlc.arg(latency_seconds)
)
ON CONFLICT (task_id) DO NOTHING;

-- name: UpsertTaskDailyStatDelta :exec
INSERT INTO task_daily_stats (
  stat_date, model_name, user_key, total, completed, failed, latency_seconds_sum, latency_count
) VALUES (
  sqlc.arg(stat_date), sqlc.arg(model_name), sqlc.arg(user_key), sqlc.arg(total),
  sqlc.arg(completed), sqlc.arg(failed), sqlc.arg(latency_seconds_sum),
  sqlc.arg(latency_count)
)
ON CONFLICT (stat_date, model_name, user_key) DO UPDATE
SET total = task_daily_stats.total + EXCLUDED.total,
    completed = task_daily_stats.completed + EXCLUDED.completed,
    failed = task_daily_stats.failed + EXCLUDED.failed,
    latency_seconds_sum = task_daily_stats.latency_seconds_sum + EXCLUDED.latency_seconds_sum,
    latency_count = task_daily_stats.latency_count + EXCLUDED.latency_count;

-- name: ListDailyStatsByDateRange :many
SELECT stat_date, model_name, user_key, total, completed, failed, latency_seconds_sum, latency_count
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

-- name: MarkDailyBackfillDone :exec
INSERT INTO task_daily_stat_backfills (stat_date)
VALUES (sqlc.arg(stat_date))
ON CONFLICT (stat_date) DO NOTHING;
