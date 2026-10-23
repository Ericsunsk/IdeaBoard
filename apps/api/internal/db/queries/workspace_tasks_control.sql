-- Workspace task control-plane reads (split from workspace_tasks.sql).
-- Operators see every workspace (no membership gating at the repository;
-- the control guard owns capability auth and every call is audited).
-- Scope guard: these names stay allowlisted in scripts/sql-workspace-scope-check.sh.

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
