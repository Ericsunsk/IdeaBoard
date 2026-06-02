-- name: InsertTaskStatEventIgnore :execrows
INSERT IGNORE INTO task_stat_events (
  task_id, stat_date, model_name, user_key, status, latency_seconds
) VALUES (?, ?, ?, ?, ?, ?);

-- name: UpsertTaskDailyStatDelta :exec
INSERT INTO task_daily_stats (
  stat_date, model_name, user_key, total, completed, failed, latency_seconds_sum, latency_count
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  total = total + VALUES(total),
  completed = completed + VALUES(completed),
  failed = failed + VALUES(failed),
  latency_seconds_sum = latency_seconds_sum + VALUES(latency_seconds_sum),
  latency_count = latency_count + VALUES(latency_count);

-- name: ListDailyStatsByDateRange :many
SELECT stat_date, model_name, user_key, total, completed, failed, latency_seconds_sum, latency_count
FROM task_daily_stats
WHERE stat_date >= ? AND stat_date <= ?
ORDER BY stat_date ASC;

-- name: ListStatEventLatencySamplesByDateRange :many
SELECT model_name, latency_seconds
FROM task_stat_events
WHERE stat_date >= ? AND stat_date <= ? AND latency_seconds > 0
ORDER BY model_name ASC, latency_seconds ASC;

-- name: ListBackfilledDatesByRange :many
SELECT stat_date
FROM task_daily_stat_backfills
WHERE stat_date >= ? AND stat_date <= ?
ORDER BY stat_date ASC;

-- name: MarkDailyBackfillDone :exec
INSERT IGNORE INTO task_daily_stat_backfills (stat_date)
VALUES (?);
