-- name: CreateTask :one
INSERT INTO tasks (
  task_id, user_id, client_action_id, assigned_provider_endpoint_id, assigned_credential_id, account_id, prompt, user_prompt, params,
  feature_slug, priority,
  status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at,
  cancelled_at, cancelled_by, cancel_reason
) VALUES (
  sqlc.arg(task_id), sqlc.arg(user_id), sqlc.arg(client_action_id), sqlc.narg(assigned_provider_endpoint_id), sqlc.narg(assigned_credential_id), sqlc.narg(account_id),
  sqlc.arg(prompt), sqlc.arg(user_prompt), sqlc.arg(params),
  sqlc.arg(feature_slug), COALESCE(NULLIF(sqlc.arg(priority)::integer, 0), 2),
  sqlc.arg(status),
  sqlc.arg(lease_token), sqlc.arg(retry_count), sqlc.arg(error_msg), sqlc.arg(result),
  sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(completed_at),
  sqlc.arg(cancelled_at), sqlc.narg(cancelled_by), sqlc.arg(cancel_reason)
)
RETURNING id;

-- name: CreateTaskIdempotent :one
INSERT INTO tasks (
  task_id, user_id, client_action_id, assigned_provider_endpoint_id, assigned_credential_id, account_id, prompt, user_prompt, params,
  feature_slug, priority,
  status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at,
  cancelled_at, cancelled_by, cancel_reason
) VALUES (
  sqlc.arg(task_id), sqlc.arg(user_id), sqlc.arg(client_action_id), sqlc.narg(assigned_provider_endpoint_id), sqlc.narg(assigned_credential_id), sqlc.narg(account_id),
  sqlc.arg(prompt), sqlc.arg(user_prompt), sqlc.arg(params),
  sqlc.arg(feature_slug), COALESCE(NULLIF(sqlc.arg(priority)::integer, 0), 2),
  sqlc.arg(status),
  sqlc.arg(lease_token), sqlc.arg(retry_count), sqlc.arg(error_msg), sqlc.arg(result),
  sqlc.arg(created_at), sqlc.arg(updated_at), sqlc.arg(completed_at),
  sqlc.arg(cancelled_at), sqlc.narg(cancelled_by), sqlc.arg(cancel_reason)
)
ON CONFLICT (user_id, client_action_id) WHERE client_action_id <> '' DO NOTHING
RETURNING id, task_id;

-- name: GetTaskByUserClientActionForUpdate :one
SELECT id, task_id
FROM tasks
WHERE user_id = sqlc.arg(user_id) AND client_action_id = sqlc.arg(client_action_id)
FOR UPDATE;

-- name: GetTaskByTaskID :one
SELECT sqlc.embed(t),
       COALESCE(latest_attempt.resolved_model, '') AS resolved_model
FROM tasks t
LEFT JOIN LATERAL (
    SELECT resolved_model
    FROM task_execution_attempts
    WHERE task_id = t.task_id
    ORDER BY (status = 'running') DESC, attempt_no DESC, id DESC
    LIMIT 1
  ) AS latest_attempt ON TRUE
WHERE t.task_id = sqlc.arg(task_id);

-- name: GetTaskByTaskIDAndUserID :one
SELECT sqlc.embed(t),
       COALESCE(latest_attempt.resolved_model, '') AS resolved_model
FROM tasks t
LEFT JOIN LATERAL (
    SELECT resolved_model
    FROM task_execution_attempts
    WHERE task_id = t.task_id
    ORDER BY (status = 'running') DESC, attempt_no DESC, id DESC
    LIMIT 1
  ) AS latest_attempt ON TRUE
WHERE t.task_id = sqlc.arg(task_id) AND t.user_id = sqlc.arg(user_id);

-- name: SummarizeTasks :one
SELECT
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE status = 'pending') AS pending_count,
  COUNT(*) FILTER (WHERE status = 'running') AS running_count,
  COUNT(*) FILTER (WHERE status = 'completed') AS completed_count,
  COUNT(*) FILTER (WHERE status = 'failed') AS failed_count,
  COUNT(*) FILTER (WHERE status IN ('cancelled', 'cancelled_with_results')) AS cancelled_count,
  COUNT(*) FILTER (WHERE status NOT IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'cancelled_with_results')) AS other_count
FROM tasks
WHERE (NOT sqlc.arg(has_user_id)::boolean OR user_id = sqlc.arg(user_id))
  AND (NOT sqlc.arg(has_account_id)::boolean OR account_id = sqlc.arg(account_id))
  AND (NOT sqlc.arg(has_status)::boolean OR status = sqlc.arg(status))
  AND (NOT sqlc.arg(has_keyword)::boolean OR prompt ILIKE sqlc.arg(keyword_like));

-- name: ListTasks :many
SELECT sqlc.embed(t),
       COALESCE(latest_attempt.resolved_model, '') AS resolved_model
FROM tasks t
LEFT JOIN LATERAL (
    SELECT resolved_model
    FROM task_execution_attempts
    WHERE task_id = t.task_id
    ORDER BY (status = 'running') DESC, attempt_no DESC, id DESC
    LIMIT 1
  ) AS latest_attempt ON TRUE
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
    WHEN t.status = 'cancelled_with_results' THEN 'cancelled'
    WHEN t.status IN ('completed', 'failed', 'cancelled') THEN t.status
    ELSE COALESCE(sqlc.narg(current_stage)::text, 'queued')
  END,
  COALESCE(sqlc.narg(bottleneck_source)::text, 'unknown'),
  CASE
    WHEN t.status = 'completed' THEN ''
    WHEN t.status = 'failed' THEN COALESCE(NULLIF(sqlc.narg(failure_stage)::text, ''), 'assigned')
    WHEN t.status IN ('cancelled', 'cancelled_with_results') THEN COALESCE(NULLIF(sqlc.narg(failure_stage)::text, ''), 'queued')
    ELSE COALESCE(sqlc.narg(failure_stage)::text, '')
  END,
  CASE
    WHEN t.status = 'completed' THEN ''
    WHEN t.status = 'failed' THEN COALESCE(NULLIF(sqlc.narg(failure_category)::text, ''), 'unknown')
    WHEN t.status IN ('cancelled', 'cancelled_with_results') THEN 'cancelled'
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
SET feature_slug = sqlc.arg(feature_slug)
WHERE task_id = sqlc.arg(task_id);

-- name: MarkTaskCancelled :execrows
UPDATE tasks
SET status = CASE WHEN EXISTS (SELECT 1 FROM task_outputs outputs WHERE outputs.task_id = sqlc.arg(task_id)) THEN 'cancelled_with_results' ELSE 'cancelled' END,
    lease_token = '',
    completed_at = sqlc.narg(cancelled_at)::timestamptz,
    cancelled_at = sqlc.narg(cancelled_at)::timestamptz,
    cancelled_by = sqlc.narg(cancelled_by),
    cancel_reason = sqlc.arg(cancel_reason),
    updated_at = now()
WHERE task_id = sqlc.arg(task_id)
  AND status IN ('pending', 'running');

-- name: ListRunningTasksForStartupRecovery :many
SELECT t.task_id,
       COALESCE(active_attempt.river_job_id, 0)::bigint AS river_job_id
FROM tasks t
LEFT JOIN LATERAL (
  SELECT river_job_id
  FROM task_execution_attempts
  WHERE task_id = t.task_id
    AND status = 'running'
  ORDER BY attempt_no DESC, id DESC
  LIMIT 1
) AS active_attempt ON TRUE
WHERE t.status = 'running'
ORDER BY t.created_at ASC, t.task_id ASC;

-- name: ListAdminTaskAttemptsByTaskIDs :many
SELECT
  task_id,
  attempt_no,
  status,
  failure_stage,
  failure_category,
  resolved_model,
  provider,
  gateway_step,
  provider_started_at,
  provider_finished_at,
  completed_at,
  created_at
FROM task_execution_attempts
WHERE task_id = ANY(sqlc.arg(task_ids)::text[])
ORDER BY task_id ASC, attempt_no ASC, id ASC;

-- name: ListAdminTaskLatestExecutionsByTaskIDs :many
SELECT DISTINCT ON (task_id)
  task_id,
  resolved_model,
  provider,
  gateway_step
FROM task_execution_attempts
WHERE task_id = ANY(sqlc.arg(task_ids)::text[])
ORDER BY task_id ASC, attempt_no DESC, id DESC;

-- name: MarkUnleasedTaskFailed :execrows
UPDATE tasks t
SET status = 'failed',
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    completed_at = sqlc.narg(completed_at)::timestamptz
WHERE t.task_id = sqlc.arg(task_id)
  AND t.status = ANY(sqlc.arg(statuses)::text[])
  AND NOT EXISTS (
    SELECT 1
    FROM task_execution_attempts a
    WHERE a.task_id = t.task_id
      AND a.status = 'running'
  );

-- name: CountTasksByStatus :one
SELECT COUNT(*)
FROM tasks
WHERE status = sqlc.arg(status);

-- name: CountAllTaskCalls :one
SELECT COALESCE(SUM(
  CASE
    WHEN pg_input_is_valid(requested_count, 'bigint'::regtype)
    THEN requested_count::bigint
    ELSE 1
  END
), 0)::bigint
FROM (
  SELECT CASE
    WHEN pg_input_is_valid(NULLIF(btrim(params), ''), 'jsonb'::regtype)
    THEN NULLIF(btrim(params), '')::jsonb #>> '{generation,requested_output_count}'
  END AS requested_count
  FROM tasks
) parsed;

-- ai-gateway T4: execution-attempt seam for the gateway cutover (T5).

-- name: ClaimAndStartExecutionAttempt :one
-- Atomic claim-plus-insert: fences one pending task for the execution token,
-- bumps the attempt sequence, and opens the running attempt row in the same
-- statement. Zero rows means another worker won the race or the task already
-- converged (sql.ErrNoRows → task state conflict upstream). The old split
-- (claim, then insert) could strand a claimed token with no attempt row when
-- the worker died between the two writes; that state is unrepresentable here.
WITH claimed AS (
  UPDATE tasks
  SET status = 'running',
      lease_token = sqlc.arg(execution_token),
      attempt_seq = attempt_seq + 1,
      updated_at = now()
  WHERE tasks.task_id = sqlc.arg(task_id)
    AND status = 'pending'
  RETURNING task_id, attempt_seq
)
INSERT INTO task_execution_attempts (
  task_id, river_job_id, attempt_no, execution_token, feature_slug,
  requested_model, provider, provider_started_at
)
SELECT claimed.task_id, sqlc.arg(river_job_id), claimed.attempt_seq,
  sqlc.arg(execution_token), sqlc.arg(feature_slug), sqlc.arg(requested_model),
  sqlc.arg(provider), sqlc.narg(provider_started_at)::timestamptz
FROM claimed
RETURNING task_execution_attempts.id, task_execution_attempts.task_id, task_execution_attempts.attempt_no;

-- name: FinalizeExecutionAttempt :execrows
UPDATE task_execution_attempts
SET status = sqlc.arg(status),
    resolved_model = sqlc.arg(resolved_model),
    gateway_step = sqlc.arg(gateway_step),
    receipt_log_id = sqlc.arg(receipt_log_id),
    receipt_event_id = sqlc.arg(receipt_event_id),
    cache_hit = sqlc.arg(cache_hit),
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
    completed_at = sqlc.narg(completed_at)::timestamptz,
    updated_at = now()
WHERE execution_token = sqlc.arg(execution_token)
  AND task_id = sqlc.arg(task_id)
  AND status = 'running'
  AND sqlc.arg(status) IN ('completed', 'failed', 'requeued', 'cancelled');

-- name: GetExecutionAttemptByToken :one
SELECT id, task_id, river_job_id, attempt_no, execution_token, feature_slug,
  requested_model, resolved_model, provider, gateway_step, receipt_log_id,
  receipt_event_id, cache_hit, status, failure_stage, failure_category,
  error_msg, provider_started_at, provider_finished_at, completed_at,
  created_at, updated_at
FROM task_execution_attempts
WHERE execution_token = sqlc.arg(execution_token);

-- ai-gateway T5: token-fenced transitions. The tasks.lease_token column now
-- carries the execution token; account columns are no longer matched.

-- name: ResetTaskExecutionToken :execrows
UPDATE tasks
SET status = 'pending',
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    updated_at = now()
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND lease_token = sqlc.arg(execution_token);

-- name: CompleteTaskWithExecutionToken :execrows
UPDATE tasks
SET status = 'completed',
    lease_token = '',
    result = sqlc.arg(result),
    error_msg = '',
    completed_at = sqlc.narg(completed_at)::timestamptz
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND lease_token = sqlc.arg(execution_token);

-- name: MarkTaskFailedWithExecutionToken :execrows
-- Single-attempt failure: terminal write without retry accounting. Kept
-- separate from FinalFail below (which bumps retry_count) on purpose.
UPDATE tasks
SET status = 'failed',
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    completed_at = sqlc.narg(completed_at)::timestamptz
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND lease_token = sqlc.arg(execution_token);

-- name: FinalFailTaskRunningWithExecutionToken :execrows
-- Exhausted failure: same terminal write plus retry accounting.
UPDATE tasks
SET status = 'failed',
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    retry_count = retry_count + 1,
    completed_at = sqlc.narg(completed_at)::timestamptz
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND lease_token = sqlc.arg(execution_token);

-- name: RequeueTaskRunningWithExecutionToken :execrows
UPDATE tasks
SET status = 'pending',
    lease_token = '',
    error_msg = sqlc.arg(error_msg),
    result = sqlc.arg(result),
    retry_count = retry_count + 1
WHERE task_id = sqlc.arg(task_id)
  AND status = ANY(sqlc.arg(statuses)::text[])
  AND lease_token = sqlc.arg(execution_token);

-- name: FinishRunningExecutionAttemptsByTaskIDs :execrows
UPDATE task_execution_attempts
SET status = sqlc.arg(status)::varchar,
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
    completed_at = sqlc.narg(finished_at)::timestamptz
WHERE task_id = ANY(sqlc.arg(task_ids)::text[])
  AND status = 'running';

-- name: GetActiveExecutionAttemptByTaskID :one
SELECT id, task_id, river_job_id, attempt_no, execution_token, feature_slug,
  requested_model, resolved_model, provider, gateway_step, receipt_log_id,
  receipt_event_id, cache_hit, status, created_at
FROM task_execution_attempts
WHERE task_id = sqlc.arg(task_id)
  AND status = 'running'
ORDER BY attempt_no DESC
LIMIT 1;

-- name: CountTasksByUserID :one
SELECT COUNT(*)
FROM tasks
WHERE user_id = sqlc.arg(user_id);

-- name: ListTasksByUserID :many
SELECT sqlc.embed(t),
       COALESCE(latest_attempt.resolved_model, '') AS resolved_model
FROM tasks t
LEFT JOIN LATERAL (
  SELECT resolved_model
  FROM task_execution_attempts
  WHERE task_id = t.task_id
  ORDER BY (status = 'running') DESC, attempt_no DESC, id DESC
  LIMIT 1
) AS latest_attempt ON TRUE
WHERE t.user_id = sqlc.arg(user_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

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
