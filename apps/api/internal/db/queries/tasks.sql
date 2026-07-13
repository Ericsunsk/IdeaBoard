-- name: CreateTask :one
INSERT INTO tasks (
  task_id, user_id, client_action_id, assigned_provider_endpoint_id, assigned_credential_id, account_id, prompt, user_prompt, params,
  feature_slug, requested_model, priority,
  status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at,
  cancelled_at, cancelled_by, cancel_reason
) VALUES (
  sqlc.arg(task_id), sqlc.arg(user_id), sqlc.arg(client_action_id), sqlc.narg(assigned_provider_endpoint_id), sqlc.narg(assigned_credential_id), sqlc.narg(account_id),
  sqlc.arg(prompt), sqlc.arg(user_prompt), sqlc.arg(params),
  sqlc.arg(feature_slug), sqlc.arg(requested_model), COALESCE(NULLIF(sqlc.arg(priority)::integer, 0), 2),
  sqlc.arg(status),
  sqlc.arg(lease_token), sqlc.arg(retry_count), sqlc.arg(error_msg), sqlc.arg(result),
  sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(completed_at),
  sqlc.arg(cancelled_at), sqlc.narg(cancelled_by), sqlc.arg(cancel_reason)
)
RETURNING id;

-- name: GetTaskByTaskID :one
SELECT sqlc.embed(t),
       COALESCE(latest_lease.resolved_model, '') AS resolved_model,
       COALESCE(latest_lease.api_type, '') AS api_type,
       COALESCE(latest_lease.provider_endpoint_name, '') AS provider_endpoint_name,
       COALESCE(latest_lease.provider_endpoint_base_url, '') AS provider_endpoint_base_url,
       COALESCE(latest_lease.credential_fingerprint, '') AS credential_fingerprint,
       COALESCE(latest_lease.credential_key_version, '') AS credential_key_version
FROM tasks t
LEFT JOIN LATERAL (
  SELECT resolved_model, api_type, provider_endpoint_name, provider_endpoint_base_url,
         credential_fingerprint, credential_key_version
  FROM task_leases
  WHERE task_id = t.task_id
  ORDER BY (released_at IS NULL) DESC, COALESCE(released_at, acquired_at) DESC, id DESC
  LIMIT 1
) AS latest_lease ON TRUE
WHERE t.task_id = sqlc.arg(task_id);

-- name: GetTaskByTaskIDAndUserID :one
SELECT sqlc.embed(t),
       COALESCE(latest_lease.resolved_model, '') AS resolved_model,
       COALESCE(latest_lease.api_type, '') AS api_type,
       COALESCE(latest_lease.provider_endpoint_name, '') AS provider_endpoint_name,
       COALESCE(latest_lease.provider_endpoint_base_url, '') AS provider_endpoint_base_url,
       COALESCE(latest_lease.credential_fingerprint, '') AS credential_fingerprint,
       COALESCE(latest_lease.credential_key_version, '') AS credential_key_version
FROM tasks t
LEFT JOIN LATERAL (
  SELECT resolved_model, api_type, provider_endpoint_name, provider_endpoint_base_url,
         credential_fingerprint, credential_key_version
  FROM task_leases
  WHERE task_id = t.task_id
  ORDER BY (released_at IS NULL) DESC, COALESCE(released_at, acquired_at) DESC, id DESC
  LIMIT 1
) AS latest_lease ON TRUE
WHERE t.task_id = sqlc.arg(task_id) AND t.user_id = sqlc.arg(user_id);

-- name: ListTasksByTaskIDs :many
SELECT sqlc.embed(t),
       COALESCE(latest_lease.resolved_model, '') AS resolved_model,
       COALESCE(latest_lease.api_type, '') AS api_type,
       COALESCE(latest_lease.provider_endpoint_name, '') AS provider_endpoint_name,
       COALESCE(latest_lease.provider_endpoint_base_url, '') AS provider_endpoint_base_url,
       COALESCE(latest_lease.credential_fingerprint, '') AS credential_fingerprint,
       COALESCE(latest_lease.credential_key_version, '') AS credential_key_version
FROM tasks t
LEFT JOIN LATERAL (
  SELECT resolved_model, api_type, provider_endpoint_name, provider_endpoint_base_url,
         credential_fingerprint, credential_key_version
  FROM task_leases
  WHERE task_id = t.task_id
  ORDER BY (released_at IS NULL) DESC, COALESCE(released_at, acquired_at) DESC, id DESC
  LIMIT 1
) AS latest_lease ON TRUE
WHERE t.task_id = ANY(sqlc.arg(task_ids)::text[])
ORDER BY array_position(sqlc.arg(task_ids)::text[], t.task_id);

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
SELECT sqlc.embed(t),
       COALESCE(latest_lease.resolved_model, '') AS resolved_model,
       COALESCE(latest_lease.api_type, '') AS api_type,
       COALESCE(latest_lease.provider_endpoint_name, '') AS provider_endpoint_name,
       COALESCE(latest_lease.provider_endpoint_base_url, '') AS provider_endpoint_base_url,
       COALESCE(latest_lease.credential_fingerprint, '') AS credential_fingerprint,
       COALESCE(latest_lease.credential_key_version, '') AS credential_key_version
FROM tasks t
LEFT JOIN LATERAL (
  SELECT resolved_model, api_type, provider_endpoint_name, provider_endpoint_base_url,
         credential_fingerprint, credential_key_version
  FROM task_leases
  WHERE task_id = t.task_id
  ORDER BY (released_at IS NULL) DESC, COALESCE(released_at, acquired_at) DESC, id DESC
  LIMIT 1
) AS latest_lease ON TRUE
WHERE (NOT sqlc.arg(has_user_id)::boolean OR t.user_id = sqlc.arg(user_id))
  AND (NOT sqlc.arg(has_account_id)::boolean OR t.account_id = sqlc.arg(account_id))
  AND (NOT sqlc.arg(has_status)::boolean OR t.status = sqlc.arg(status))
  AND (NOT sqlc.arg(has_keyword)::boolean OR t.prompt ILIKE sqlc.arg(keyword_like))
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
)
SELECT
  t.task_id,
  CASE
    WHEN t.status IN ('completed', 'failed', 'cancelled') THEN t.status
    ELSE COALESCE(sqlc.narg(current_stage)::text, 'queued')
  END,
  COALESCE(sqlc.narg(bottleneck_source)::text, 'unknown'),
  CASE
    WHEN t.status = 'completed' THEN ''
    WHEN t.status = 'failed' THEN COALESCE(NULLIF(sqlc.narg(failure_stage)::text, ''), 'assigned')
    WHEN t.status = 'cancelled' THEN COALESCE(NULLIF(sqlc.narg(failure_stage)::text, ''), 'queued')
    ELSE COALESCE(sqlc.narg(failure_stage)::text, '')
  END,
  CASE
    WHEN t.status = 'completed' THEN ''
    WHEN t.status = 'failed' THEN COALESCE(NULLIF(sqlc.narg(failure_category)::text, ''), 'unknown')
    WHEN t.status = 'cancelled' THEN 'cancelled'
    ELSE COALESCE(sqlc.narg(failure_category)::text, '')
  END,
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
FROM tasks t
WHERE t.task_id = sqlc.arg(task_id)
ON CONFLICT (task_id) DO UPDATE
SET current_stage = CASE
      WHEN task_execution_metrics.current_stage IN ('completed', 'failed', 'cancelled') THEN task_execution_metrics.current_stage
      ELSE EXCLUDED.current_stage
    END,
    bottleneck_source = CASE
      WHEN task_execution_metrics.current_stage IN ('completed', 'failed', 'cancelled') THEN task_execution_metrics.bottleneck_source
      ELSE EXCLUDED.bottleneck_source
    END,
    failure_stage = CASE
      WHEN task_execution_metrics.current_stage IN ('completed', 'failed', 'cancelled') THEN task_execution_metrics.failure_stage
      ELSE EXCLUDED.failure_stage
    END,
    failure_category = CASE
      WHEN task_execution_metrics.current_stage IN ('completed', 'failed', 'cancelled') THEN task_execution_metrics.failure_category
      ELSE EXCLUDED.failure_category
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
    requested_model = sqlc.arg(requested_model)
WHERE task_id = sqlc.arg(task_id);

-- name: ReleaseTaskLease :execrows
UPDATE task_leases
SET released_at = sqlc.arg(released_at),
    release_reason = sqlc.arg(release_reason),
    updated_at = now()
WHERE task_id = sqlc.arg(task_id)
  AND lease_token = sqlc.arg(lease_token)
  AND account_id = sqlc.arg(account_id)
  AND provider_endpoint_id = sqlc.arg(provider_endpoint_id)
  AND credential_id = sqlc.arg(credential_id)
  AND released_at IS NULL;

-- name: ReleaseActiveTaskLeaseByTaskID :execrows
UPDATE task_leases
SET released_at = sqlc.arg(released_at),
    release_reason = sqlc.arg(release_reason),
    updated_at = now()
WHERE task_id = sqlc.arg(task_id)
  AND released_at IS NULL;

-- name: ReleaseActiveTaskLeasesByTaskIDs :execrows
UPDATE task_leases
SET released_at = sqlc.arg(released_at),
    release_reason = sqlc.arg(release_reason),
    updated_at = now()
WHERE task_id = ANY(sqlc.arg(task_ids)::text[])
  AND released_at IS NULL;

-- name: GetActiveTaskLeaseByTaskID :one
SELECT id, task_id, lease_token, river_job_id, account_id, provider_endpoint_id, credential_id,
       feature_slug, requested_model, resolved_model, api_type,
       provider_endpoint_name, provider_endpoint_base_url, credential_fingerprint, credential_key_version,
       acquired_at, released_at, release_reason, created_at, updated_at
FROM task_leases
WHERE task_id = sqlc.arg(task_id)
  AND released_at IS NULL;

-- name: MarkTaskCancelled :execrows
UPDATE tasks
SET status = 'cancelled',
    lease_token = '',
    completed_at = sqlc.narg(cancelled_at)::timestamptz,
    cancelled_at = sqlc.narg(cancelled_at)::timestamptz,
    cancelled_by = sqlc.narg(cancelled_by),
    cancel_reason = sqlc.arg(cancel_reason),
    updated_at = now()
WHERE task_id = sqlc.arg(task_id)
  AND status IN ('pending', 'running');

-- name: CancelRunningTasksForStartupRecovery :many
WITH updated_tasks AS (
  UPDATE tasks t
  SET status = 'cancelled',
      lease_token = '',
      completed_at = sqlc.narg(cancelled_at)::timestamptz,
      cancelled_at = sqlc.narg(cancelled_at)::timestamptz,
      cancelled_by = NULL,
      cancel_reason = sqlc.arg(cancel_reason),
      updated_at = now()
  WHERE t.status = 'running'
  RETURNING t.task_id, t.user_id, t.assigned_provider_endpoint_id, t.assigned_credential_id, t.account_id,
            t.feature_slug, t.requested_model, t.created_at, t.completed_at, t.cancelled_at
)
SELECT t.task_id, t.user_id, t.assigned_provider_endpoint_id, t.assigned_credential_id, t.account_id,
       t.feature_slug, t.requested_model,
       COALESCE(latest_lease.river_job_id, 0)::bigint AS river_job_id,
       COALESCE(latest_lease.resolved_model, '') AS resolved_model,
       COALESCE(latest_lease.api_type, '') AS api_type,
       t.created_at, t.completed_at, t.cancelled_at
FROM updated_tasks t
LEFT JOIN LATERAL (
  SELECT river_job_id, resolved_model, api_type
  FROM task_leases
  WHERE task_id = t.task_id
  ORDER BY (released_at IS NULL) DESC, COALESCE(released_at, acquired_at) DESC, id DESC
  LIMIT 1
) AS latest_lease ON TRUE
ORDER BY t.created_at ASC, t.task_id ASC;

-- name: CreateTaskAttempt :one
WITH active_lease AS (
  SELECT tl.id
  FROM task_leases tl
  WHERE tl.task_id = sqlc.arg(task_id)
    AND tl.account_id = sqlc.arg(account_id)
    AND tl.provider_endpoint_id = sqlc.arg(provider_endpoint_id)
    AND tl.credential_id = sqlc.arg(credential_id)
    AND tl.lease_token = sqlc.arg(lease_token)
    AND tl.released_at IS NULL
),
next_attempt AS (
  UPDATE tasks
  SET attempt_seq = attempt_seq + 1,
      updated_at = now()
  WHERE task_id = sqlc.arg(task_id)
    AND status = 'running'
    AND account_id = sqlc.arg(account_id)
    AND assigned_provider_endpoint_id = sqlc.arg(provider_endpoint_id)
    AND assigned_credential_id = sqlc.arg(credential_id)
    AND lease_token = sqlc.arg(lease_token)
    AND EXISTS (SELECT 1 FROM active_lease)
  RETURNING attempt_seq AS attempt_no
)
INSERT INTO task_attempts (
  task_id, lease_id, attempt_no, river_job_id, status, provider_started_at
)
SELECT
  sqlc.arg(task_id), active_lease.id, next_attempt.attempt_no, sqlc.arg(river_job_id),
  'running', sqlc.arg(provider_started_at)
FROM next_attempt
CROSS JOIN active_lease
RETURNING id, lease_id, attempt_no;

-- name: FinishTaskAttempt :execrows
UPDATE task_attempts
SET status = sqlc.arg(status),
    failure_stage = CASE
      WHEN sqlc.arg(status) = 'completed' THEN ''
      ELSE sqlc.arg(failure_stage)
    END,
    failure_category = CASE
      WHEN sqlc.arg(status) = 'completed' THEN ''
      ELSE sqlc.arg(failure_category)
    END,
    error_msg = CASE
      WHEN sqlc.arg(status) = 'completed' THEN ''
      ELSE sqlc.arg(error_msg)
    END,
    provider_finished_at = sqlc.narg(provider_finished_at)::timestamptz,
    finalize_started_at = sqlc.narg(finalize_started_at)::timestamptz,
    finalize_finished_at = sqlc.narg(finalize_finished_at)::timestamptz,
    provider_latency_ms = sqlc.arg(provider_latency_ms),
    finalize_latency_ms = sqlc.arg(finalize_latency_ms),
    total_latency_ms = sqlc.arg(total_latency_ms),
    completed_at = CASE
      WHEN sqlc.arg(status) = 'cancelled' THEN NULL
      ELSE sqlc.narg(completed_at)::timestamptz
    END,
    cancelled_at = CASE
      WHEN sqlc.arg(status) = 'cancelled' THEN sqlc.narg(completed_at)::timestamptz
      ELSE NULL
    END
WHERE id = sqlc.arg(id)
  AND status = 'running'
  AND sqlc.arg(status) IN ('completed', 'failed', 'requeued', 'cancelled');

-- name: CancelRunningTaskAttemptsByTaskIDs :execrows
UPDATE task_attempts
SET status = 'cancelled',
    failure_stage = sqlc.arg(failure_stage),
    failure_category = sqlc.arg(failure_category),
    error_msg = sqlc.arg(error_msg),
    completed_at = NULL,
    cancelled_at = sqlc.narg(cancelled_at)::timestamptz,
    total_latency_ms = GREATEST(
      EXTRACT(EPOCH FROM (sqlc.narg(cancelled_at)::timestamptz - created_at)) * 1000,
      0
    )::bigint,
    updated_at = now()
WHERE task_id = ANY(sqlc.arg(task_ids)::text[])
  AND status = 'running';

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
SELECT sqlc.embed(t),
       COALESCE(latest_lease.resolved_model, '') AS resolved_model,
       COALESCE(latest_lease.api_type, '') AS api_type,
       COALESCE(latest_lease.provider_endpoint_name, '') AS provider_endpoint_name,
       COALESCE(latest_lease.provider_endpoint_base_url, '') AS provider_endpoint_base_url,
       COALESCE(latest_lease.credential_fingerprint, '') AS credential_fingerprint,
       COALESCE(latest_lease.credential_key_version, '') AS credential_key_version
FROM tasks t
LEFT JOIN LATERAL (
  SELECT resolved_model, api_type, provider_endpoint_name, provider_endpoint_base_url,
         credential_fingerprint, credential_key_version
  FROM task_leases
  WHERE task_id = t.task_id
  ORDER BY (released_at IS NULL) DESC, COALESCE(released_at, acquired_at) DESC, id DESC
  LIMIT 1
) AS latest_lease ON TRUE
WHERE t.user_id = sqlc.arg(user_id)
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
    assigned_provider_endpoint_id = NULL,
    assigned_credential_id = NULL,
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
    completed_at = sqlc.narg(completed_at)::timestamptz
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND account_id = sqlc.arg(account_id)
  AND assigned_provider_endpoint_id = sqlc.arg(assigned_provider_endpoint_id)
  AND assigned_credential_id = sqlc.arg(assigned_credential_id)
  AND lease_token = sqlc.arg(lease_token);

-- name: MarkTaskFailed :execrows
UPDATE tasks
SET status = 'failed',
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    completed_at = sqlc.narg(completed_at)::timestamptz
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[]);

-- name: MarkTaskFailedWithLease :execrows
UPDATE tasks
SET status = 'failed',
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    completed_at = sqlc.narg(completed_at)::timestamptz
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND account_id = sqlc.arg(account_id)
  AND assigned_provider_endpoint_id = sqlc.arg(assigned_provider_endpoint_id)
  AND assigned_credential_id = sqlc.arg(assigned_credential_id)
  AND lease_token = sqlc.arg(lease_token);

-- name: ResetTaskRunningToPendingWithLease :execrows
UPDATE tasks
SET status = 'pending',
    account_id = NULL,
    assigned_provider_endpoint_id = NULL,
    assigned_credential_id = NULL,
    lease_token = '',
    error_msg = sqlc.arg(error_msg)
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND account_id = sqlc.arg(account_id)
  AND assigned_provider_endpoint_id = sqlc.arg(assigned_provider_endpoint_id)
  AND assigned_credential_id = sqlc.arg(assigned_credential_id)
  AND lease_token = sqlc.arg(lease_token);

-- name: RequeueTaskRunningWithLease :execrows
UPDATE tasks
SET status = 'pending',
    account_id = NULL,
    assigned_provider_endpoint_id = NULL,
    assigned_credential_id = NULL,
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    retry_count = retry_count + 1
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND account_id = sqlc.arg(account_id)
  AND assigned_provider_endpoint_id = sqlc.arg(assigned_provider_endpoint_id)
  AND assigned_credential_id = sqlc.arg(assigned_credential_id)
  AND lease_token = sqlc.arg(lease_token);

-- name: FinalFailTaskRunningWithLease :execrows
UPDATE tasks
SET status = 'failed',
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    retry_count = retry_count + 1,
    completed_at = sqlc.narg(completed_at)::timestamptz
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND account_id = sqlc.arg(account_id)
  AND assigned_provider_endpoint_id = sqlc.arg(assigned_provider_endpoint_id)
  AND assigned_credential_id = sqlc.arg(assigned_credential_id)
  AND lease_token = sqlc.arg(lease_token);

-- name: DeleteTaskByTaskIDAndUserID :exec
DELETE FROM tasks
WHERE task_id = sqlc.arg(task_id) AND user_id = sqlc.arg(user_id);

-- name: DeleteTasksByTaskIDs :execrows
DELETE FROM tasks
WHERE task_id = ANY(sqlc.arg(task_ids)::text[]);

-- name: DeleteTasksByUserID :exec
DELETE FROM tasks
WHERE user_id = sqlc.arg(user_id);

-- name: FailRunningTasksByAccountID :many
WITH updated_tasks AS (
  UPDATE tasks t
  SET status = 'failed',
      lease_token = '',
      error_msg = sqlc.arg(error_msg),
      completed_at = sqlc.narg(completed_at)::timestamptz
  WHERE t.account_id = sqlc.arg(account_id) AND t.status = 'running'
  RETURNING t.task_id, t.user_id, t.assigned_provider_endpoint_id, t.assigned_credential_id, t.account_id, t.result,
            t.feature_slug, t.requested_model, t.created_at, t.completed_at
)
SELECT t.task_id, t.user_id, t.assigned_provider_endpoint_id, t.assigned_credential_id, t.account_id, t.result,
       t.feature_slug, t.requested_model,
       COALESCE(latest_lease.resolved_model, '') AS resolved_model,
       COALESCE(latest_lease.api_type, '') AS api_type,
       t.created_at, t.completed_at
FROM updated_tasks t
LEFT JOIN LATERAL (
  SELECT resolved_model, api_type
  FROM task_leases
  WHERE task_id = t.task_id
  ORDER BY (released_at IS NULL) DESC, COALESCE(released_at, acquired_at) DESC, id DESC
  LIMIT 1
) AS latest_lease ON TRUE;

-- name: FailRunningTaskAttemptsByAccountID :execrows
UPDATE task_attempts
SET status = 'failed',
    failure_stage = sqlc.arg(failure_stage),
    failure_category = sqlc.arg(failure_category),
    error_msg = sqlc.arg(error_msg),
    completed_at = sqlc.narg(completed_at)::timestamptz
WHERE lease_id IN (
    SELECT id
    FROM task_leases
    WHERE account_id = sqlc.arg(account_id)
  )
  AND status = 'running'
  AND task_id = ANY(sqlc.arg(task_ids)::text[]);

-- name: CountTasksByStatus :one
SELECT COUNT(*)
FROM tasks
WHERE status = sqlc.arg(status);

-- name: CountAllTasks :one
SELECT COUNT(*)
FROM tasks;
