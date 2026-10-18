-- Workspace-scoped task queries (#819, Phase 4-02). The tasks table is the
-- workspace-scoped async job resource (public_id LIKE 'task_%'):
-- lease_token is required when status='running' and empty otherwise
-- (chk_tasks_lease_status); terminal timestamps follow
-- chk_tasks_terminal_consistency. Every query scopes by workspace public_id
-- so cross-workspace ids stay invisible (ErrNotFound), never 403.
-- Workspace membership gating stays at the v1 handler layer
-- (taskWorkspaceContext); the repository enforces scoping + transitions.
-- Scope guard: scripts/sql-workspace-scope-check.sh enforces one scope predicate per query.

-- name: CreateWorkspaceTask :one
INSERT INTO tasks (public_id, workspace_id, submitter_user_id, billing_account_id, service, workflow_input, status, priority, retry_count, lease_token, error_message)
VALUES (sqlc.arg(public_id), sqlc.arg(workspace_id), sqlc.arg(submitter_user_id), sqlc.arg(billing_account_id), sqlc.arg(service), sqlc.arg(workflow_input), 'pending', sqlc.arg(priority), 0, '', '')
RETURNING internal_id, public_id, workspace_id, submitter_user_id, billing_account_id, service, workflow_input, status, priority, retry_count, lease_token, error_message, created_at, updated_at, started_at, completed_at, cancelled_at, cancelled_by_user_id, routing_snapshot;

-- name: GetWorkspaceTaskByPublicID :one
SELECT t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.service, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id, t.routing_snapshot
FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
WHERE t.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
LIMIT 1;

-- name: ListWorkspaceTasks :many
SELECT t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.service, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id, t.routing_snapshot
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

-- ClaimTask fences one task execution with the lease token. A pending task is
-- claimed normally; a River redelivery of the same job may reacquire its
-- interrupted running lease because it carries the same execution token.
-- A different token still gets zero rows, preserving the single-worker
-- fence. The caller distinguishes already-converged/invisible rows via
-- GetWorkspaceTaskByPublicID.
-- name: ClaimWorkspaceTask :one
UPDATE tasks AS t
SET status = 'running', lease_token = sqlc.arg(lease_token), started_at = now(), updated_at = now()
FROM workspaces AS w
WHERE t.workspace_id = w.internal_id
AND t.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
AND (
  (t.status = 'pending' AND t.lease_token = '')
  OR (t.status = 'running' AND t.lease_token = sqlc.arg(lease_token))
)
RETURNING t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.service, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id, t.routing_snapshot;

-- ClaimWorkspaceTaskWithAttempt fences one execution AND opens its durable
-- attempt fact in ONE statement (DESIGN.md contract 4: "claim 与 attempt 行在
-- 同一 SQL 语句原子提交，不存在有 token 无 attempt 行的中间态").
--
-- Both writes are data-modifying CTEs, so PostgreSQL runs them in a single
-- implicit transaction: the lease flip and the attempt row commit together or
-- not at all. A worker crash, a rejected pool connection, or a failed attempt
-- insert can therefore no longer leave a running lease with zero attempt rows
-- (which stranded Admin monitor / ListExecutionBottleneckStats buckets with no
-- attributable send).
--
-- execution_token IS the lease token (one token per execution, not two): the
-- row that fences FinishAttempt is exactly the row the claim wrote, so the
-- claim and the attempt fact can never disagree about which execution they
-- describe.
--
-- ON CONFLICT (execution_token) DO NOTHING keeps the same-token redelivery
-- branch of the lease fence legal: a River redelivery of an interrupted lease
-- resumes the attempt row the earlier attempt opened instead of colliding on
-- uq_task_execution_attempts_token. attempt_no is only consumed when a new
-- row is inserted, so one lease token owns at most one attempt number.
--
-- Zero rows means the lease fence rejected the claim exactly as
-- ClaimWorkspaceTask does, so the caller converges through the same
-- GetWorkspaceTaskByPublicID path.
-- name: ClaimWorkspaceTaskWithAttempt :one
WITH claimed AS (
  UPDATE tasks AS t
  SET status = 'running', lease_token = sqlc.arg(lease_token), started_at = now(), updated_at = now()
  FROM workspaces AS w
  WHERE t.workspace_id = w.internal_id
  AND t.public_id = sqlc.arg(public_id)
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND (
    (t.status = 'pending' AND t.lease_token = '')
    OR (t.status = 'running' AND t.lease_token = sqlc.arg(lease_token))
  )
  RETURNING t.*
),
attempt AS (
  INSERT INTO task_execution_attempts (
    task_id, river_job_id, attempt_no, execution_token, service,
    requested_model, resolved_model, provider, retry_layer,
    gateway_retry_count, status
  )
  SELECT c.public_id,
    sqlc.arg(river_job_id)::bigint,
    COALESCE((SELECT MAX(a.attempt_no) FROM task_execution_attempts AS a WHERE a.task_id = c.public_id), 0) + 1,
    c.lease_token,
    sqlc.arg(service)::varchar(64),
    sqlc.arg(requested_model)::varchar(128),
    sqlc.arg(resolved_model)::varchar(128),
    sqlc.arg(provider)::varchar(64),
    sqlc.arg(retry_layer)::text,
    sqlc.arg(gateway_retry_count)::integer,
    'running'
  FROM claimed AS c
  ON CONFLICT (execution_token) DO NOTHING
  RETURNING attempt_no
)
SELECT c.*,
  -- attempt_no is 0 on the resume path (the insert conflicted), where the
  -- caller keeps the attempt_no the earlier attempt already owns.
  COALESCE((SELECT attempt_no FROM attempt), 0) AS attempt_no,
  EXISTS (SELECT 1 FROM attempt) AS attempt_started
FROM claimed AS c;

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
RETURNING t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.service, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id, t.routing_snapshot;

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
RETURNING t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.service, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id, t.routing_snapshot;

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
RETURNING t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.service, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id, t.routing_snapshot;

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
RETURNING t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.service, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id, t.routing_snapshot;

-- DeleteWorkspaceTask removes a task row (settlement compensation rollback
-- on enqueue failure). Outputs cascade.
-- name: DeleteWorkspaceTask :execrows
DELETE FROM tasks AS t
USING workspaces AS w
WHERE t.workspace_id = w.internal_id
AND t.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id);

-- InsertWorkspaceTaskOutput accepts the output reference and its immutable
-- Media origin from the persisted Task in the same statement.
-- name: InsertWorkspaceTaskOutput :one
WITH source AS (
  SELECT t.*
  FROM tasks AS t
  INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
  WHERE t.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
), inserted AS (
  INSERT INTO task_outputs (task_id, output_index, output_kind, media_asset_id, remote_url, metadata)
  SELECT t.internal_id, sqlc.arg(output_index), sqlc.arg(output_kind), sqlc.narg(media_asset_id), sqlc.narg(remote_url), sqlc.arg(metadata)
  FROM source AS t
  WHERE sqlc.narg(media_asset_id)::bigint IS NULL OR EXISTS (
    SELECT 1 FROM media_assets AS m
    WHERE m.internal_id = sqlc.narg(media_asset_id) AND m.workspace_id = t.workspace_id
  )
  RETURNING *
), origins AS (
  INSERT INTO media_asset_task_origins (workspace_id, task_public_id, output_index, media_asset_id, submitter_user_id, board_public_id)
  SELECT t.workspace_id, t.public_id, o.output_index, o.media_asset_id, t.submitter_user_id,
         NULLIF(btrim(t.workflow_input ->> 'board_id'), '')
  FROM inserted AS o
  INNER JOIN source AS t ON t.internal_id = o.task_id
  WHERE o.media_asset_id IS NOT NULL
  RETURNING media_asset_id
)
SELECT internal_id, task_id, output_index, output_kind, media_asset_id, remote_url, metadata, created_at
FROM inserted;

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
SELECT t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.service, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id, t.routing_snapshot,
  w.public_id AS workspace_public_id
FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
WHERE t.public_id = sqlc.arg(public_id)
LIMIT 1;

-- name: ListControlTasks :many
SELECT t.internal_id, t.public_id, t.workspace_id, t.submitter_user_id, t.billing_account_id, t.service, t.workflow_input, t.status, t.priority, t.retry_count, t.lease_token, t.error_message, t.created_at, t.updated_at, t.started_at, t.completed_at, t.cancelled_at, t.cancelled_by_user_id, t.routing_snapshot,
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
    OR t.service ILIKE '%' || sqlc.arg(keyword) || '%'
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
    OR t.service ILIKE '%' || sqlc.arg(keyword) || '%'
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

-- Durable submit idempotency fact (task_submission_idempotency), written
-- inside the settlement transaction so a committed claim always has its task
-- row and a rolled-back settlement leaves no claim behind.
--
-- The workspace is resolved by public id inside the statement, so the scope
-- predicate rides on the same query as the insert and the workspace scope
-- guard covers it. A caller that cannot see the workspace inserts nothing and
-- reads no claim, so it fails closed rather than settling outside its
-- workspace.
--
-- ON CONFLICT DO NOTHING is the concurrency fence: a concurrent submit of the
-- same key blocks on the primary key until the first transaction commits or
-- rolls back, then observes the winner. The claim must be written BEFORE the
-- Hold so a replay never reaches the ledger at all.
-- name: ClaimTaskSubmissionIdempotency :one
INSERT INTO task_submission_idempotency (workspace_id, operation, idempotency_key, request_hash, task_public_id)
SELECT w.internal_id, sqlc.arg(operation), sqlc.arg(idempotency_key), sqlc.arg(request_hash), sqlc.arg(task_public_id)
FROM workspaces AS w
WHERE w.public_id = sqlc.arg(workspace_public_id)
ON CONFLICT (workspace_id, operation, idempotency_key) DO NOTHING
RETURNING task_public_id, request_hash;

-- name: GetTaskSubmissionIdempotency :one
SELECT c.task_public_id, c.request_hash
FROM task_submission_idempotency AS c
INNER JOIN workspaces AS w ON w.internal_id = c.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND c.operation = sqlc.arg(operation)
  AND c.idempotency_key = sqlc.arg(idempotency_key)
LIMIT 1;

-- ListStalledHeldWorkspaceTasks is the DISCOVERY half of the final-timeout
-- convergence (stalled settlement sweep). It names workspace-scoped tasks
-- that are still live, still owe a Credit Hold (snapshot total > 0 with a hold
-- transaction), and whose settlement projection has not reached
-- captured/released — AND whose row no worker has touched since stalled_before.
--
-- Both live statuses are candidates, and both leak. A 'running' row is the
-- claim-committed-then-worker-died shape (SIGKILL, then River discards the job
-- after MaxAttempts). A 'pending' row with an open hold is the same leak one
-- phase earlier: submit committed Hold + task + job atomically, and every
-- attempt died before the claim. 24h with no progress means neither will
-- converge on its own.
--
-- stalled_before is measured from tasks.updated_at, which is the liveness
-- clock, not the submission clock. The set_tasks_updated_at BEFORE UPDATE
-- trigger stamps now() on every write, so no writer can forge a fresh stamp
-- for an abandoned lease; and every reclaim path does write it (a River
-- redelivery of the SAME execution token hits the running+same-token branch
-- of ClaimWorkspaceTask; Retry re-arms pending with a fresh write). So a row
-- older than the threshold is not "slow", it is "no worker has held this
-- lease in a full day".
--
-- Ordering is oldest-first with a bound: the sweep converges the longest
-- abandoned holds first and never scans the whole table in one run.
--
-- Scope: the sweep is cross-workspace BY DESIGN (an abandoned hold is money
-- locked in SOME workspace, and leaving any workspace out would leave its
-- users' credits locked forever), so it cannot carry the per-workspace
-- predicate. It still INNER JOINs workspaces: only rows whose workspace
-- resolves are visible, and each returned pair is converged through the
-- workspace-scoped FailWorkspaceTask (see ListStalledHeldWorkspaceTasks in
-- scripts/sql-workspace-scope-check.sh). A mismatched pair fails closed there
-- instead of settling cross-workspace.
--
-- The settlement projection is the task-side convergence fact, never the
-- ledger: credit_holds is the Credits context's table and Tasking does not
-- read it (CONTEXT.md). The Release itself still passes the ledger's own
-- held-status CAS plus the task-release:<task_id> idempotency key, so a
-- double sweep cannot double-release.
-- name: ListStalledHeldWorkspaceTasks :many
SELECT w.public_id AS workspace_public_id, t.public_id
FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
INNER JOIN task_billing_snapshots AS s ON s.task_id = t.public_id
LEFT JOIN task_billing_settlements AS st ON st.task_id = t.public_id
WHERE t.status IN ('pending', 'running')
  AND t.updated_at < sqlc.arg(stalled_before)::TIMESTAMPTZ
  AND s.total_credits > 0
  AND s.hold_transaction_id IS NOT NULL
  AND (st.state IS NULL OR st.state NOT IN ('captured', 'released'))
ORDER BY t.updated_at ASC, t.internal_id ASC
LIMIT sqlc.arg(row_limit);
