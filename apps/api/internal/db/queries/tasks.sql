-- name: CreateTask :one
INSERT INTO tasks (
  task_id, user_id, channel_id, account_id, prompt, user_prompt, params,
  status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at
) VALUES (
  sqlc.arg(task_id), sqlc.arg(user_id), sqlc.arg(channel_id), sqlc.arg(account_id),
  sqlc.arg(prompt), sqlc.arg(user_prompt), sqlc.arg(params), sqlc.arg(status),
  sqlc.arg(lease_token), sqlc.arg(retry_count), sqlc.arg(error_msg), sqlc.arg(result),
  sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(completed_at)
)
RETURNING id;

-- name: GetTaskByTaskID :one
SELECT id, task_id, user_id, channel_id, account_id, prompt, user_prompt, params,
       status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at
FROM tasks
WHERE task_id = sqlc.arg(task_id);

-- name: GetTaskByTaskIDAndUserID :one
SELECT id, task_id, user_id, channel_id, account_id, prompt, user_prompt, params,
       status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at
FROM tasks
WHERE task_id = sqlc.arg(task_id) AND user_id = sqlc.arg(user_id);

-- name: CountTasks :one
SELECT COUNT(*)
FROM tasks
WHERE (NOT sqlc.arg(has_user_id)::boolean OR user_id = sqlc.arg(user_id))
  AND (NOT sqlc.arg(has_account_id)::boolean OR account_id = sqlc.arg(account_id))
  AND (NOT sqlc.arg(has_status)::boolean OR status = sqlc.arg(status))
  AND (NOT sqlc.arg(has_keyword)::boolean OR prompt ILIKE sqlc.arg(keyword_like));

-- name: ListTasks :many
SELECT id, task_id, user_id, channel_id, account_id, prompt, user_prompt, params,
       status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at
FROM tasks
WHERE (NOT sqlc.arg(has_user_id)::boolean OR user_id = sqlc.arg(user_id))
  AND (NOT sqlc.arg(has_account_id)::boolean OR account_id = sqlc.arg(account_id))
  AND (NOT sqlc.arg(has_status)::boolean OR status = sqlc.arg(status))
  AND (NOT sqlc.arg(has_keyword)::boolean OR prompt ILIKE sqlc.arg(keyword_like))
ORDER BY created_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListTaskExecutionMetricsByTaskIDs :many
SELECT task_id, current_stage, bottleneck_source, failure_stage, failure_category,
       assigned_at, provider_started_at, provider_finished_at, finalize_started_at, finalize_finished_at,
       queue_wait_ms, worker_overhead_ms, provider_latency_ms, finalize_latency_ms, total_latency_ms,
       created_at, updated_at
FROM task_execution_metrics
WHERE task_id = ANY(sqlc.arg(task_ids)::text[]);

-- name: ListTaskExecutionBottleneckStatsByCreatedRange :many
SELECT COALESCE(NULLIF(m.bottleneck_source, ''), 'unknown')::text AS bottleneck_source,
       COUNT(*)::bigint AS total,
       COUNT(*) FILTER (WHERE t.status = 'failed')::bigint AS failed,
       COALESCE(AVG(m.total_latency_ms), 0)::double precision AS avg_latency_ms
FROM task_execution_metrics m
JOIN tasks t ON t.task_id = m.task_id
WHERE t.created_at >= sqlc.arg(started_at) AND t.created_at < sqlc.arg(ended_at)
  AND t.status IN ('completed', 'failed')
GROUP BY COALESCE(NULLIF(m.bottleneck_source, ''), 'unknown')
ORDER BY total DESC, bottleneck_source ASC;

-- name: ListTaskExecutionFailureCategoryStatsByCreatedRange :many
SELECT COALESCE(NULLIF(m.failure_category, ''), 'unknown')::text AS failure_category,
       COUNT(*)::bigint AS total,
       COALESCE(AVG(m.total_latency_ms), 0)::double precision AS avg_latency_ms
FROM task_execution_metrics m
JOIN tasks t ON t.task_id = m.task_id
WHERE t.created_at >= sqlc.arg(started_at) AND t.created_at < sqlc.arg(ended_at)
  AND t.status = 'failed'
GROUP BY COALESCE(NULLIF(m.failure_category, ''), 'unknown')
ORDER BY total DESC, failure_category ASC;

-- name: PatchTaskExecutionMetrics :exec
INSERT INTO task_execution_metrics (
  task_id, current_stage, bottleneck_source, failure_stage, failure_category,
  assigned_at, provider_started_at, provider_finished_at, finalize_started_at, finalize_finished_at,
  queue_wait_ms, worker_overhead_ms, provider_latency_ms, finalize_latency_ms, total_latency_ms
) VALUES (
  sqlc.arg(task_id),
  COALESCE(sqlc.narg(current_stage)::text, 'queued'),
  COALESCE(sqlc.narg(bottleneck_source)::text, 'unknown'),
  COALESCE(sqlc.narg(failure_stage)::text, ''),
  COALESCE(sqlc.narg(failure_category)::text, ''),
  sqlc.narg(assigned_at),
  sqlc.narg(provider_started_at),
  sqlc.narg(provider_finished_at),
  sqlc.narg(finalize_started_at),
  sqlc.narg(finalize_finished_at),
  COALESCE(sqlc.narg(queue_wait_ms)::bigint, 0),
  COALESCE(sqlc.narg(worker_overhead_ms)::bigint, 0),
  COALESCE(sqlc.narg(provider_latency_ms)::bigint, 0),
  COALESCE(sqlc.narg(finalize_latency_ms)::bigint, 0),
  COALESCE(sqlc.narg(total_latency_ms)::bigint, 0)
)
ON CONFLICT (task_id) DO UPDATE
SET current_stage = COALESCE(sqlc.narg(current_stage)::text, task_execution_metrics.current_stage),
    bottleneck_source = COALESCE(sqlc.narg(bottleneck_source)::text, task_execution_metrics.bottleneck_source),
    failure_stage = COALESCE(sqlc.narg(failure_stage)::text, task_execution_metrics.failure_stage),
    failure_category = COALESCE(sqlc.narg(failure_category)::text, task_execution_metrics.failure_category),
    assigned_at = COALESCE(sqlc.narg(assigned_at), task_execution_metrics.assigned_at),
    provider_started_at = COALESCE(sqlc.narg(provider_started_at), task_execution_metrics.provider_started_at),
    provider_finished_at = COALESCE(sqlc.narg(provider_finished_at), task_execution_metrics.provider_finished_at),
    finalize_started_at = COALESCE(sqlc.narg(finalize_started_at), task_execution_metrics.finalize_started_at),
    finalize_finished_at = COALESCE(sqlc.narg(finalize_finished_at), task_execution_metrics.finalize_finished_at),
    queue_wait_ms = COALESCE(sqlc.narg(queue_wait_ms)::bigint, task_execution_metrics.queue_wait_ms),
    worker_overhead_ms = COALESCE(sqlc.narg(worker_overhead_ms)::bigint, task_execution_metrics.worker_overhead_ms),
    provider_latency_ms = COALESCE(sqlc.narg(provider_latency_ms)::bigint, task_execution_metrics.provider_latency_ms),
    finalize_latency_ms = COALESCE(sqlc.narg(finalize_latency_ms)::bigint, task_execution_metrics.finalize_latency_ms),
    total_latency_ms = COALESCE(sqlc.narg(total_latency_ms)::bigint, task_execution_metrics.total_latency_ms),
    updated_at = now();

-- name: CountTasksByUserID :one
SELECT COUNT(*)
FROM tasks
WHERE user_id = sqlc.arg(user_id);

-- name: ListTasksByUserID :many
SELECT id, task_id, user_id, channel_id, account_id, prompt, user_prompt, params,
       status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at
FROM tasks
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListTaskIDsByAccountID :many
SELECT task_id
FROM tasks
WHERE account_id = sqlc.arg(account_id)
ORDER BY id ASC;

-- name: ListTaskIDsByAccountIDAndStatuses :many
SELECT task_id
FROM tasks
WHERE account_id = sqlc.arg(account_id) AND status = ANY(sqlc.arg(statuses)::text[])
ORDER BY id ASC;

-- name: CountRunningTasksMissingLeaseToken :one
SELECT COUNT(*)
FROM tasks
WHERE status = 'running' AND (lease_token IS NULL OR lease_token = '');

-- name: ListRunningTasksMissingLeaseToken :many
SELECT task_id, account_id, lease_token
FROM tasks
WHERE status = 'running' AND (lease_token IS NULL OR lease_token = '')
ORDER BY id ASC
LIMIT sqlc.arg(row_limit);

-- name: ListRunningMissingLeaseTaskIDs :many
SELECT task_id
FROM tasks
WHERE status = 'running' AND (lease_token IS NULL OR lease_token = '')
ORDER BY id ASC
LIMIT sqlc.arg(row_limit);

-- name: ResetRunningMissingLeaseTasksByIDs :execrows
UPDATE tasks
SET status = 'pending',
    account_id = 0,
    lease_token = '',
    error_msg = 'lease_token missing; reset to pending by task lease repair',
    completed_at = NULL
WHERE task_id = ANY(sqlc.arg(task_ids)::text[])
  AND status = 'running'
  AND (lease_token IS NULL OR lease_token = '');

-- name: CompleteTaskWithLease :execrows
UPDATE tasks
SET status = 'completed',
    lease_token = '',
    result = sqlc.arg(result),
    error_msg = '',
    completed_at = sqlc.arg(completed_at)
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND lease_token = sqlc.arg(lease_token);

-- name: MarkTaskFailed :execrows
UPDATE tasks
SET status = 'failed',
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    completed_at = sqlc.arg(completed_at)
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[]);

-- name: MarkTaskFailedWithLease :execrows
UPDATE tasks
SET status = 'failed',
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    completed_at = sqlc.arg(completed_at)
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND lease_token = sqlc.arg(lease_token);

-- name: ResetTaskRunningToPendingWithLease :execrows
UPDATE tasks
SET status = 'pending',
    account_id = 0,
    lease_token = '',
    error_msg = sqlc.arg(error_msg)
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND lease_token = sqlc.arg(lease_token);

-- name: RequeueTaskRunningWithLease :execrows
UPDATE tasks
SET status = 'pending',
    account_id = 0,
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    retry_count = retry_count + 1
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND lease_token = sqlc.arg(lease_token);

-- name: FinalFailTaskRunningWithLease :execrows
UPDATE tasks
SET status = 'failed',
    account_id = 0,
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    retry_count = retry_count + 1,
    completed_at = sqlc.arg(completed_at)
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND lease_token = sqlc.arg(lease_token);

-- name: DeleteTaskByTaskIDAndUserID :exec
DELETE FROM tasks
WHERE task_id = sqlc.arg(task_id) AND user_id = sqlc.arg(user_id);

-- name: DeleteTasksByUserID :exec
DELETE FROM tasks
WHERE user_id = sqlc.arg(user_id);

-- name: FailRunningTasksByAccountID :many
UPDATE tasks
SET status = 'failed',
    error_msg = sqlc.arg(error_msg),
    completed_at = sqlc.arg(completed_at)
WHERE account_id = sqlc.arg(account_id) AND status IN ('running', 'pending')
RETURNING task_id, user_id, account_id, result, created_at, completed_at;

-- name: CountTasksByStatus :one
SELECT COUNT(*)
FROM tasks
WHERE status = sqlc.arg(status);

-- name: CountAllTasks :one
SELECT COUNT(*)
FROM tasks;

-- name: ListTaskStatisticsByCreatedRange :many
SELECT task_id, user_id, account_id, status, result, created_at, completed_at
FROM tasks
WHERE created_at >= sqlc.arg(started_at) AND created_at < sqlc.arg(ended_at)
ORDER BY created_at ASC;
