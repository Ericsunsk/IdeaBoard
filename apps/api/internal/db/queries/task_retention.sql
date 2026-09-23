-- Task retention: terminal tasks older than the retention window are purged
-- after their statistics have been frozen into task_daily_stats. Financial
-- facts (task_billing_snapshots/settlements/claims, credit ledger) and
-- statistics aggregates are never touched here; task_execution_metrics,
-- task_execution_attempts and task_outputs follow tasks via ON DELETE CASCADE,
-- task_cancellations has no FK and is deleted explicitly first.

-- name: ListExpiredTerminalTaskDates :many
SELECT DISTINCT (COALESCE(completed_at, cancelled_at, updated_at)::date) AS stat_date
FROM tasks
WHERE status IN ('completed', 'failed', 'cancelled', 'cancelled_with_results')
  AND COALESCE(completed_at, cancelled_at, updated_at) < sqlc.arg(cutoff)
ORDER BY stat_date
LIMIT sqlc.arg(row_limit);

-- name: DeleteExpiredTaskCancellations :execrows
DELETE FROM task_cancellations
WHERE task_id IN (
  SELECT task_id
  FROM tasks
  WHERE status IN ('completed', 'failed', 'cancelled', 'cancelled_with_results')
    AND COALESCE(completed_at, cancelled_at, updated_at) < sqlc.arg(cutoff)
  ORDER BY COALESCE(completed_at, cancelled_at, updated_at)
  LIMIT sqlc.arg(batch_size)
);

-- name: DeleteExpiredTasks :execrows
DELETE FROM tasks
WHERE task_id IN (
  SELECT expired.task_id
  FROM tasks AS expired
  WHERE expired.status IN ('completed', 'failed', 'cancelled', 'cancelled_with_results')
    AND COALESCE(expired.completed_at, expired.cancelled_at, expired.updated_at) < sqlc.arg(cutoff)
  ORDER BY COALESCE(expired.completed_at, expired.cancelled_at, expired.updated_at)
  LIMIT sqlc.arg(batch_size)
);
