-- Workspace-scoped task queries (#819, Phase 4-02). The tasks table is the
-- workspace-scoped async job resource (public_id LIKE 'task_%'):
-- lease_token is required when status='running' and empty otherwise
-- (chk_tasks_lease_status); terminal timestamps follow
-- chk_tasks_terminal_consistency. Every query scopes by workspace public_id
-- so cross-workspace ids stay invisible (ErrNotFound), never 403.
-- Workspace membership gating stays at the v1 handler layer
-- (taskWorkspaceContext); the repository enforces scoping + transitions.

-- name: CreateWorkspaceTask :one
INSERT INTO tasks (public_id, workspace_id, submitter_user_id, billing_account_id, workflow_type, workflow_input, status, priority, retry_count, lease_token, error_message)
VALUES (sqlc.arg(public_id), sqlc.arg(workspace_id), sqlc.arg(submitter_user_id), sqlc.arg(billing_account_id), sqlc.arg(workflow_type), sqlc.arg(workflow_input), 'pending', sqlc.arg(priority), 0, '', '')
RETURNING internal_id, public_id, workspace_id, submitter_user_id, billing_account_id, workflow_type, workflow_input, status, priority, retry_count, lease_token, error_message, created_at, updated_at, started_at, completed_at, cancelled_at, cancelled_by_user_id;

-- name: GetWorkspaceTaskByPublicID :one
SELECT t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.workflow_type, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id
FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
WHERE t.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
LIMIT 1;

-- name: ListWorkspaceTasks :many
SELECT t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.workflow_type, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id
FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
ORDER BY t.created_at DESC, t.internal_id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountWorkspaceTasks :one
SELECT COUNT(*)
FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id);

-- ClaimTask fences one pending task for the worker: pending -> running with
-- the lease token. Zero rows means already claimed/converged or invisible;
-- the caller distinguishes via GetWorkspaceTaskByPublicID.
-- name: ClaimWorkspaceTask :one
UPDATE tasks AS t
SET status = 'running', lease_token = sqlc.arg(lease_token), started_at = now(), updated_at = now()
FROM workspaces AS w
WHERE t.workspace_id = w.internal_id
AND t.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
AND t.status = 'pending'
AND t.lease_token = ''
RETURNING t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.workflow_type, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id;

-- CompleteWorkspaceTask flips a live task to completed and releases the
-- lease. Outputs are inserted separately in the same caller-owned
-- transaction (see InsertWorkspaceTaskOutput).
-- name: CompleteWorkspaceTask :one
UPDATE tasks AS t
SET status = 'completed', lease_token = '', error_message = '', completed_at = now(), updated_at = now()
FROM workspaces AS w
WHERE t.workspace_id = w.internal_id
AND t.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
AND t.status IN ('pending', 'running')
RETURNING t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.workflow_type, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id;

-- FailWorkspaceTask flips a live task to failed with the terminal error
-- (chk_tasks_terminal_consistency requires a non-blank error_message) and
-- releases the lease.
-- name: FailWorkspaceTask :one
UPDATE tasks AS t
SET status = 'failed', lease_token = '', error_message = sqlc.arg(error_message), completed_at = now(), updated_at = now()
FROM workspaces AS w
WHERE t.workspace_id = w.internal_id
AND t.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
AND t.status IN ('pending', 'running')
RETURNING t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.workflow_type, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id;

-- RetryWorkspaceTask consumes one retry budget entry: a live task returns
-- to pending with retry_count incremented, the lease released (pending rows
-- must carry an empty lease_token), and the fresh execution token recorded
-- in the workflow_input envelope.
-- name: RetryWorkspaceTask :one
UPDATE tasks AS t
SET status = 'pending', lease_token = '', retry_count = t.retry_count + 1,
    workflow_input = jsonb_set(t.workflow_input, '{execution_token}', to_jsonb(sqlc.arg(execution_token)::text)),
    updated_at = now()
FROM workspaces AS w
WHERE t.workspace_id = w.internal_id
AND t.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
AND t.status IN ('pending', 'running')
RETURNING t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.workflow_type, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id;

-- CancelWorkspaceTask flips a live task to cancelled (chk constraint
-- requires cancelled_at) and releases the lease. The tasks table carries
-- no cancel_reason column, so the reason is recorded in the
-- workflow_input envelope for read-back parity.
-- name: CancelWorkspaceTask :one
UPDATE tasks AS t
SET status = 'cancelled', lease_token = '', cancelled_at = now(), cancelled_by_user_id = sqlc.narg(cancelled_by_user_id),
    workflow_input = jsonb_set(t.workflow_input, '{cancel_reason}', to_jsonb(sqlc.arg(cancel_reason)::text)),
    updated_at = now()
FROM workspaces AS w
WHERE t.workspace_id = w.internal_id
AND t.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
AND t.status IN ('pending', 'running')
RETURNING t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.workflow_type, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id;

-- DeleteWorkspaceTask removes a task row (settlement compensation rollback
-- on enqueue failure). Outputs cascade.
-- name: DeleteWorkspaceTask :execrows
DELETE FROM tasks AS t
USING workspaces AS w
WHERE t.workspace_id = w.internal_id
AND t.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id);

-- InsertWorkspaceTaskOutput appends one output ref to a workspace-scoped
-- task. Scoping resolves atomically in the SELECT so outputs can never
-- attach to a cross-workspace task id.
-- name: InsertWorkspaceTaskOutput :one
INSERT INTO task_outputs (task_id, output_index, output_kind, media_asset_id, remote_url, metadata)
SELECT t.internal_id, sqlc.arg(output_index), sqlc.arg(output_kind), sqlc.narg(media_asset_id), sqlc.narg(remote_url), sqlc.arg(metadata)
FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
WHERE t.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
RETURNING internal_id, task_id, output_index, output_kind, media_asset_id, remote_url, metadata, created_at;

-- name: ListWorkspaceTaskOutputs :many
SELECT o.internal_id, o.task_id, o.output_index, o.output_kind, o.media_asset_id, o.remote_url, o.metadata, o.created_at
FROM task_outputs AS o
INNER JOIN tasks AS t ON t.internal_id = o.task_id
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
WHERE t.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
ORDER BY o.output_index ASC;

-- ListWorkspaceTaskOutputsByTaskIDs batches output reads for list pages
-- (internal task ids resolved from the same page).
-- name: ListWorkspaceTaskOutputsByTaskIDs :many
SELECT internal_id, task_id, output_index, output_kind, media_asset_id, remote_url, metadata, created_at
FROM task_outputs
WHERE task_id = ANY(sqlc.arg(task_ids)::bigint[])
ORDER BY task_id ASC, output_index ASC;

-- GetWorkspaceMediaAssetID resolves a workspace-scoped media asset public
-- id for output association. Minimal lookup only; the full media surface
-- belongs to #820.
-- name: GetWorkspaceMediaAssetID :one
SELECT m.internal_id
FROM media_assets AS m
INNER JOIN workspaces AS w ON w.internal_id = m.workspace_id
WHERE m.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id) AND m.archived_at IS NULL
LIMIT 1;

-- Control Plane global task read model (#809-close). Operators see every
-- workspace (no membership gating at the repository; the control guard
-- owns capability auth and every call is audited). Unknown ids read as
-- zero rows (the adapter converges to ErrNotFound, never 403).
-- Settlement joins stay LEFT: tasks without billing facts (free or
-- pre-settlement rows) still list.

-- name: GetControlTaskByPublicID :one
SELECT t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.workflow_type, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id,
  w.public_id AS workspace_public_id
FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
WHERE t.public_id = sqlc.arg(public_id)
LIMIT 1;

-- name: ListControlTasks :many
SELECT t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.workflow_type, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id,
  w.public_id AS workspace_public_id,
  s.total_credits AS billing_total_credits,
  st.state AS settlement_state,
  st.captured_credits AS settlement_captured_credits
FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
LEFT JOIN task_billing_snapshots AS s ON s.task_id = t.public_id
LEFT JOIN task_billing_settlements AS st ON st.task_id = t.public_id
WHERE (sqlc.arg(status) = '' OR t.status = sqlc.arg(status))
  AND (sqlc.arg(submitter_user_id) = 0 OR t.submitter_user_id = sqlc.arg(submitter_user_id))
  AND (sqlc.arg(keyword) = ''
    OR t.public_id ILIKE '%' || sqlc.arg(keyword) || '%'
    OR t.workflow_type ILIKE '%' || sqlc.arg(keyword) || '%'
    OR t.error_message ILIKE '%' || sqlc.arg(keyword) || '%')
ORDER BY t.created_at DESC, t.internal_id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- Counts honor the user/keyword filters but never the status filter, so
-- the status pills keep their global breakdown while a status is active.
-- name: CountControlTasks :many
SELECT t.status AS status, COUNT(*) AS count
FROM tasks AS t
WHERE (sqlc.arg(submitter_user_id) = 0 OR t.submitter_user_id = sqlc.arg(submitter_user_id))
  AND (sqlc.arg(keyword) = ''
    OR t.public_id ILIKE '%' || sqlc.arg(keyword) || '%'
    OR t.workflow_type ILIKE '%' || sqlc.arg(keyword) || '%'
    OR t.error_message ILIKE '%' || sqlc.arg(keyword) || '%')
GROUP BY t.status;

-- name: GetControlTaskBilling :one
SELECT s.sku, s.display_name, s.price_version_id, s.price_version, s.output_resolution, s.unit_credits, s.quantity, s.total_credits, s.quoted_at, s.hold_transaction_id,
  st.state AS settlement_state, st.captured_credits AS settlement_captured_credits, st.settled_at AS settlement_settled_at
FROM task_billing_snapshots AS s
LEFT JOIN task_billing_settlements AS st ON st.task_id = s.task_id
WHERE s.task_id = sqlc.arg(task_id)
ORDER BY s.created_at DESC
LIMIT 1;
