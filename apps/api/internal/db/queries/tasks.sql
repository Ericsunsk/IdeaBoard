-- name: CreateTask :execlastid
INSERT INTO tasks (
  task_id, user_id, channel_id, account_id, prompt, user_prompt, params,
  status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetTaskByTaskID :one
SELECT id, task_id, user_id, channel_id, account_id, prompt, user_prompt, params,
       status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at
FROM tasks
WHERE task_id = ?;

-- name: GetTaskByTaskIDAndUserID :one
SELECT id, task_id, user_id, channel_id, account_id, prompt, user_prompt, params,
       status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at
FROM tasks
WHERE task_id = ? AND user_id = ?;

-- name: GetTaskByID :one
SELECT id, task_id, user_id, channel_id, account_id, prompt, user_prompt, params,
       status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at
FROM tasks
WHERE id = ?;

-- name: UpdateTask :exec
UPDATE tasks
SET task_id = ?,
    user_id = ?,
    channel_id = ?,
    account_id = ?,
    prompt = ?,
    user_prompt = ?,
    params = ?,
    status = ?,
    lease_token = ?,
    retry_count = ?,
    error_msg = ?,
    result = ?,
    completed_at = ?
WHERE id = ?;

-- name: CountTasks :one
SELECT COUNT(*)
FROM tasks
WHERE (CAST(sqlc.arg(has_user_id) AS SIGNED) = 0 OR user_id = sqlc.arg(user_id))
  AND (CAST(sqlc.arg(has_account_id) AS SIGNED) = 0 OR account_id = sqlc.arg(account_id))
  AND (CAST(sqlc.arg(has_status) AS SIGNED) = 0 OR status = sqlc.arg(status))
  AND (CAST(sqlc.arg(has_keyword) AS SIGNED) = 0 OR prompt LIKE sqlc.arg(keyword_like));

-- name: ListTasks :many
SELECT id, task_id, user_id, channel_id, account_id, prompt, user_prompt, params,
       status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at
FROM tasks
WHERE (CAST(sqlc.arg(has_user_id) AS SIGNED) = 0 OR user_id = sqlc.arg(user_id))
  AND (CAST(sqlc.arg(has_account_id) AS SIGNED) = 0 OR account_id = sqlc.arg(account_id))
  AND (CAST(sqlc.arg(has_status) AS SIGNED) = 0 OR status = sqlc.arg(status))
  AND (CAST(sqlc.arg(has_keyword) AS SIGNED) = 0 OR prompt LIKE sqlc.arg(keyword_like))
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CountTasksByUserID :one
SELECT COUNT(*)
FROM tasks
WHERE user_id = ?;

-- name: ListTasksByUserID :many
SELECT id, task_id, user_id, channel_id, account_id, prompt, user_prompt, params,
       status, lease_token, retry_count, error_msg, result, created_at, updated_at, completed_at
FROM tasks
WHERE user_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: ListTaskIDsByAccountID :many
SELECT task_id
FROM tasks
WHERE account_id = ?
ORDER BY id ASC;

-- name: ListTaskIDsByAccountIDAndStatuses :many
SELECT task_id
FROM tasks
WHERE account_id = ? AND status IN (sqlc.slice('statuses'))
ORDER BY id ASC;

-- name: ListRunningTaskLeases :many
SELECT task_id, account_id, lease_token
FROM tasks
WHERE status = 'running'
ORDER BY id ASC
LIMIT ?;

-- name: CountRunningTasksMissingLeaseToken :one
SELECT COUNT(*)
FROM tasks
WHERE status = 'running' AND (lease_token IS NULL OR lease_token = '');

-- name: ListRunningTasksMissingLeaseToken :many
SELECT task_id, account_id, lease_token
FROM tasks
WHERE status = 'running' AND (lease_token IS NULL OR lease_token = '')
ORDER BY id ASC
LIMIT ?;

-- name: ListRunningMissingLeaseTaskIDs :many
SELECT task_id
FROM tasks
WHERE status = 'running' AND (lease_token IS NULL OR lease_token = '')
ORDER BY id ASC
LIMIT ?;

-- name: ResetRunningMissingLeaseTasksByIDs :execrows
UPDATE tasks
SET status = 'pending',
    account_id = 0,
    lease_token = '',
    error_msg = 'lease_token missing; reset to pending by task lease repair',
    completed_at = NULL
WHERE task_id IN (sqlc.slice('task_ids'))
  AND status = 'running'
  AND (lease_token IS NULL OR lease_token = '');

-- name: MarkTaskRunning :execrows
UPDATE tasks
SET status = 'running',
    account_id = ?,
    lease_token = ?
WHERE task_id = ?
  AND status IN (sqlc.slice('statuses'));

-- name: CompleteTaskWithLease :execrows
UPDATE tasks
SET status = 'completed',
    lease_token = '',
    result = ?,
    error_msg = '',
    completed_at = ?
WHERE task_id = ?
  AND status IN (sqlc.slice('statuses'))
  AND lease_token = ?;

-- name: CompleteTask :execrows
UPDATE tasks
SET status = 'completed',
    lease_token = '',
    result = ?,
    error_msg = '',
    completed_at = ?
WHERE task_id = ?
  AND status IN (sqlc.slice('statuses'));

-- name: MarkTaskFailed :execrows
UPDATE tasks
SET status = 'failed',
    lease_token = '',
    error_msg = ?,
    result = ?,
    completed_at = ?
WHERE task_id = ?
  AND status IN (sqlc.slice('statuses'));

-- name: MarkTaskFailedWithLease :execrows
UPDATE tasks
SET status = 'failed',
    lease_token = '',
    error_msg = ?,
    result = ?,
    completed_at = ?
WHERE task_id = ?
  AND status IN (sqlc.slice('statuses'))
  AND lease_token = ?;

-- name: ResetTaskRunningToPendingWithLease :execrows
UPDATE tasks
SET status = 'pending',
    account_id = 0,
    lease_token = '',
    error_msg = ?
WHERE task_id = ?
  AND status IN (sqlc.slice('statuses'))
  AND lease_token = ?;

-- name: RequeueTaskRunningWithLease :execrows
UPDATE tasks
SET status = 'pending',
    account_id = 0,
    lease_token = '',
    error_msg = ?,
    retry_count = retry_count + 1
WHERE task_id = ?
  AND status IN (sqlc.slice('statuses'))
  AND lease_token = ?;

-- name: FinalFailTaskRunningWithLease :execrows
UPDATE tasks
SET status = 'failed',
    account_id = 0,
    lease_token = '',
    error_msg = ?,
    result = ?,
    retry_count = retry_count + 1,
    completed_at = ?
WHERE task_id = ?
  AND status IN (sqlc.slice('statuses'))
  AND lease_token = ?;

-- name: DeleteTaskByTaskIDAndUserID :exec
DELETE FROM tasks
WHERE task_id = ? AND user_id = ?;

-- name: DeleteTasksByUserID :exec
DELETE FROM tasks
WHERE user_id = ?;

-- name: MarkQueueFailed :exec
UPDATE tasks
SET status = 'failed',
    error_msg = ?,
    completed_at = ?
WHERE task_id = ?;

-- name: ListFailRunningCandidatesByAccountID :many
SELECT task_id, user_id, account_id, result, created_at, completed_at
FROM tasks
WHERE account_id = ? AND status IN ('running', 'pending')
ORDER BY id ASC;

-- name: FailRunningTasksByAccountID :execrows
UPDATE tasks
SET status = 'failed',
    error_msg = ?,
    completed_at = ?
WHERE account_id = ? AND status IN ('running', 'pending');

-- name: CountTasksByStatus :one
SELECT COUNT(*)
FROM tasks
WHERE status = ?;

-- name: CountAllTasks :one
SELECT COUNT(*)
FROM tasks;

-- name: ListTaskStatisticsByCreatedRange :many
SELECT task_id, user_id, account_id, status, result, created_at, completed_at
FROM tasks
WHERE created_at >= ? AND created_at < ?
ORDER BY created_at ASC;
