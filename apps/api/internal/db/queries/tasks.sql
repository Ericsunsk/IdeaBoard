-- name: CreateTask :one
INSERT INTO tasks (
  task_id, user_id, assigned_channel_id, account_id, prompt, user_prompt, params,
  feature_slug, requested_model, resolved_model, api_type,
  status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at,
  cancelled_at, cancelled_by, cancel_reason
) VALUES (
  sqlc.arg(task_id), sqlc.arg(user_id), sqlc.narg(assigned_channel_id), sqlc.narg(account_id),
  sqlc.arg(prompt), sqlc.arg(user_prompt), sqlc.arg(params),
  sqlc.arg(feature_slug), sqlc.arg(requested_model), sqlc.arg(resolved_model), sqlc.arg(api_type),
  sqlc.arg(status),
  sqlc.arg(lease_token), sqlc.arg(retry_count), sqlc.arg(error_msg), sqlc.arg(result),
  sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(completed_at),
  sqlc.arg(cancelled_at), sqlc.narg(cancelled_by), sqlc.arg(cancel_reason)
)
RETURNING id;

-- name: GetTaskByTaskID :one
SELECT id, task_id, user_id, assigned_channel_id, account_id, prompt, user_prompt, params,
       feature_slug, requested_model, resolved_model, api_type,
       status, lease_token, retry_count, attempt_seq, error_msg, result, created_at, updated_at, completed_at,
       cancelled_at, cancelled_by, cancel_reason
FROM tasks
WHERE task_id = sqlc.arg(task_id);

-- name: GetTaskByTaskIDAndUserID :one
SELECT id, task_id, user_id, assigned_channel_id, account_id, prompt, user_prompt, params,
       feature_slug, requested_model, resolved_model, api_type,
       status, lease_token, retry_count, attempt_seq, error_msg, result, created_at, updated_at, completed_at,
       cancelled_at, cancelled_by, cancel_reason
FROM tasks
WHERE task_id = sqlc.arg(task_id) AND user_id = sqlc.arg(user_id);

-- name: SummarizeTasks :one
SELECT
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE status = 'pending') AS pending_count,
  COUNT(*) FILTER (WHERE status = 'running') AS running_count,
  COUNT(*) FILTER (WHERE status = 'completed') AS completed_count,
  COUNT(*) FILTER (WHERE status = 'failed') AS failed_count,
  COUNT(*) FILTER (WHERE status = 'cancelled') AS cancelled_count,
  COUNT(*) FILTER (WHERE status NOT IN ('pending', 'running', 'completed', 'failed', 'cancelled')) AS other_count
FROM tasks
WHERE (NOT sqlc.arg(has_user_id)::boolean OR user_id = sqlc.arg(user_id))
  AND (NOT sqlc.arg(has_account_id)::boolean OR account_id = sqlc.arg(account_id))
  AND (NOT sqlc.arg(has_status)::boolean OR status = sqlc.arg(status))
  AND (NOT sqlc.arg(has_keyword)::boolean OR prompt ILIKE sqlc.arg(keyword_like));

-- name: ListTasks :many
SELECT id, task_id, user_id, assigned_channel_id, account_id, prompt, user_prompt, params,
       feature_slug, requested_model, resolved_model, api_type,
       status, lease_token, retry_count, attempt_seq, error_msg, result, created_at, updated_at, completed_at,
       cancelled_at, cancelled_by, cancel_reason
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
       COALESCE(AVG(COALESCE(m.total_latency_ms, 0)), 0)::double precision AS avg_latency_ms
FROM tasks t
LEFT JOIN task_execution_metrics m ON m.task_id = t.task_id
WHERE t.created_at >= sqlc.arg(started_at) AND t.created_at < sqlc.arg(ended_at)
  AND t.status IN ('completed', 'failed')
GROUP BY COALESCE(NULLIF(m.bottleneck_source, ''), 'unknown')
ORDER BY total DESC, bottleneck_source ASC;

-- name: ListTaskExecutionFailureCategoryStatsByCreatedRange :many
SELECT COALESCE(NULLIF(m.failure_category, ''), 'unknown')::text AS failure_category,
       COUNT(*)::bigint AS total,
       COALESCE(AVG(COALESCE(m.total_latency_ms, 0)), 0)::double precision AS avg_latency_ms
FROM tasks t
LEFT JOIN task_execution_metrics m ON m.task_id = t.task_id
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
SET current_stage = CASE
      WHEN task_execution_metrics.current_stage IN ('completed', 'failed', 'cancelled') THEN task_execution_metrics.current_stage
      ELSE COALESCE(sqlc.narg(current_stage)::text, task_execution_metrics.current_stage)
    END,
    bottleneck_source = CASE
      WHEN task_execution_metrics.current_stage IN ('completed', 'failed', 'cancelled') THEN task_execution_metrics.bottleneck_source
      ELSE COALESCE(sqlc.narg(bottleneck_source)::text, task_execution_metrics.bottleneck_source)
    END,
    failure_stage = CASE
      WHEN task_execution_metrics.current_stage IN ('completed', 'failed', 'cancelled') THEN task_execution_metrics.failure_stage
      ELSE COALESCE(sqlc.narg(failure_stage)::text, task_execution_metrics.failure_stage)
    END,
    failure_category = CASE
      WHEN task_execution_metrics.current_stage IN ('completed', 'failed', 'cancelled') THEN task_execution_metrics.failure_category
      ELSE COALESCE(sqlc.narg(failure_category)::text, task_execution_metrics.failure_category)
    END,
    assigned_at = COALESCE(sqlc.narg(assigned_at), task_execution_metrics.assigned_at),
    provider_started_at = COALESCE(sqlc.narg(provider_started_at), task_execution_metrics.provider_started_at),
    provider_finished_at = COALESCE(sqlc.narg(provider_finished_at), task_execution_metrics.provider_finished_at),
    finalize_started_at = COALESCE(sqlc.narg(finalize_started_at), task_execution_metrics.finalize_started_at),
    finalize_finished_at = COALESCE(sqlc.narg(finalize_finished_at), task_execution_metrics.finalize_finished_at),
    queue_wait_ms = COALESCE(sqlc.narg(queue_wait_ms)::bigint, task_execution_metrics.queue_wait_ms),
    worker_overhead_ms = COALESCE(sqlc.narg(worker_overhead_ms)::bigint, task_execution_metrics.worker_overhead_ms),
    provider_latency_ms = COALESCE(sqlc.narg(provider_latency_ms)::bigint, task_execution_metrics.provider_latency_ms),
    finalize_latency_ms = COALESCE(sqlc.narg(finalize_latency_ms)::bigint, task_execution_metrics.finalize_latency_ms),
    total_latency_ms = CASE
      WHEN task_execution_metrics.current_stage IN ('completed', 'failed', 'cancelled') THEN task_execution_metrics.total_latency_ms
      ELSE COALESCE(sqlc.narg(total_latency_ms)::bigint, task_execution_metrics.total_latency_ms)
    END,
    updated_at = now();

-- name: PatchTaskExecutionSnapshot :execrows
UPDATE tasks
SET feature_slug = sqlc.arg(feature_slug),
    requested_model = sqlc.arg(requested_model),
    resolved_model = sqlc.arg(resolved_model),
    api_type = sqlc.arg(api_type)
WHERE task_id = sqlc.arg(task_id);

-- name: MarkTaskCancelled :execrows
UPDATE tasks
SET status = 'cancelled',
    lease_token = '',
    completed_at = sqlc.arg(cancelled_at),
    cancelled_at = sqlc.arg(cancelled_at),
    cancelled_by = sqlc.narg(cancelled_by),
    cancel_reason = sqlc.arg(cancel_reason),
    updated_at = now()
WHERE task_id = sqlc.arg(task_id)
  AND status IN ('pending', 'running');

-- name: CreateTaskAttempt :one
WITH next_attempt AS (
  UPDATE tasks
  SET attempt_seq = attempt_seq + 1,
      updated_at = now()
  WHERE task_id = sqlc.arg(task_id)
    AND status = 'running'
    AND account_id = sqlc.arg(account_id)
    AND assigned_channel_id = sqlc.arg(channel_id)
    AND lease_token = sqlc.arg(lease_token)
  RETURNING attempt_seq AS attempt_no
)
INSERT INTO task_attempts (
  task_id, attempt_no, river_job_id, lease_token, account_id, channel_id,
  feature_slug, requested_model, resolved_model, api_type, status,
  provider_started_at
)
SELECT
  sqlc.arg(task_id), next_attempt.attempt_no, sqlc.arg(river_job_id), sqlc.arg(lease_token),
  sqlc.arg(account_id), sqlc.arg(channel_id), sqlc.arg(feature_slug), sqlc.arg(requested_model),
  sqlc.arg(resolved_model), sqlc.arg(api_type), 'running', sqlc.arg(provider_started_at)
FROM next_attempt
RETURNING id, attempt_no;

-- name: FinishTaskAttempt :execrows
UPDATE task_attempts
SET status = sqlc.arg(status),
    failure_stage = sqlc.arg(failure_stage),
    failure_category = sqlc.arg(failure_category),
    error_msg = sqlc.arg(error_msg),
    provider_finished_at = sqlc.arg(provider_finished_at),
    finalize_started_at = sqlc.arg(finalize_started_at),
    finalize_finished_at = sqlc.arg(finalize_finished_at),
    provider_latency_ms = sqlc.arg(provider_latency_ms),
    finalize_latency_ms = sqlc.arg(finalize_latency_ms),
    total_latency_ms = sqlc.arg(total_latency_ms),
    completed_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(id)
  AND status = 'running';

-- name: ListTaskAttemptsByTaskID :many
SELECT id, task_id, attempt_no, river_job_id, lease_token, account_id, channel_id,
       feature_slug, requested_model, resolved_model, api_type, status,
       failure_stage, failure_category, error_msg,
       provider_started_at, provider_finished_at, finalize_started_at, finalize_finished_at,
       provider_latency_ms, finalize_latency_ms, total_latency_ms,
       completed_at, created_at, updated_at
FROM task_attempts
WHERE task_id = sqlc.arg(task_id)
ORDER BY attempt_no ASC;

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
      WHEN ta.total_latency_ms > 0
      THEN (ta.total_latency_ms::double precision / 1000)
      WHEN ta.completed_at IS NOT NULL AND ta.completed_at > ta.created_at
      THEN EXTRACT(EPOCH FROM (ta.completed_at - ta.created_at))::double precision
      ELSE 0::double precision
    END AS duration_seconds
  FROM task_attempts ta
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

-- name: CountTasksByUserID :one
SELECT COUNT(*)
FROM tasks
WHERE user_id = sqlc.arg(user_id);

-- name: ListTasksByUserID :many
SELECT id, task_id, user_id, assigned_channel_id, account_id, prompt, user_prompt, params,
       feature_slug, requested_model, resolved_model, api_type,
       status, lease_token, retry_count, attempt_seq, error_msg, result, created_at, updated_at, completed_at,
       cancelled_at, cancelled_by, cancel_reason
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
    account_id = NULL,
    assigned_channel_id = NULL,
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
  AND account_id = sqlc.arg(account_id)
  AND assigned_channel_id = sqlc.arg(assigned_channel_id)
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
  AND account_id = sqlc.arg(account_id)
  AND assigned_channel_id = sqlc.arg(assigned_channel_id)
  AND lease_token = sqlc.arg(lease_token);

-- name: ResetTaskRunningToPendingWithLease :execrows
UPDATE tasks
SET status = 'pending',
    account_id = NULL,
    assigned_channel_id = NULL,
    lease_token = '',
    error_msg = sqlc.arg(error_msg)
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND account_id = sqlc.arg(account_id)
  AND assigned_channel_id = sqlc.arg(assigned_channel_id)
  AND lease_token = sqlc.arg(lease_token);

-- name: RequeueTaskRunningWithLease :execrows
UPDATE tasks
SET status = 'pending',
    account_id = NULL,
    assigned_channel_id = NULL,
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    retry_count = retry_count + 1
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND account_id = sqlc.arg(account_id)
  AND assigned_channel_id = sqlc.arg(assigned_channel_id)
  AND lease_token = sqlc.arg(lease_token);

-- name: FinalFailTaskRunningWithLease :execrows
UPDATE tasks
SET status = 'failed',
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    retry_count = retry_count + 1,
    completed_at = sqlc.arg(completed_at)
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND account_id = sqlc.arg(account_id)
  AND assigned_channel_id = sqlc.arg(assigned_channel_id)
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
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    completed_at = sqlc.arg(completed_at)
WHERE account_id = sqlc.arg(account_id) AND status = 'running'
RETURNING task_id, user_id, assigned_channel_id, account_id, result,
          feature_slug, requested_model, resolved_model, api_type,
          created_at, completed_at;

-- name: FailRunningTaskAttemptsByAccountID :execrows
UPDATE task_attempts
SET status = 'failed',
    failure_stage = sqlc.arg(failure_stage),
    failure_category = sqlc.arg(failure_category),
    error_msg = sqlc.arg(error_msg),
    completed_at = sqlc.arg(completed_at)
WHERE account_id = sqlc.arg(account_id)
  AND status = 'running'
  AND task_id = ANY(sqlc.arg(task_ids)::text[]);

-- name: CountTasksByStatus :one
SELECT COUNT(*)
FROM tasks
WHERE status = sqlc.arg(status);

-- name: CountAllTasks :one
SELECT COUNT(*)
FROM tasks;

-- name: ListTaskStatisticsByCreatedRange :many
SELECT task_id, user_id, account_id, status, result,
       feature_slug, requested_model, resolved_model, api_type,
       created_at, completed_at
FROM tasks
WHERE created_at >= sqlc.arg(started_at) AND created_at < sqlc.arg(ended_at)
ORDER BY created_at ASC;
