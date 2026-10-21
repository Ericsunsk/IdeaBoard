-- Workspace shared consumption budgets (#945). One row per workspace;
-- absent means unlimited (the default). The budget is a cumulative cap on
-- settled + pending consumption in the current UTC period, not a wallet,
-- grant, or balance reservation. All queries are typed and join the
-- caller-owned transaction via the passed Queries, so Task/Vision
-- settlement keeps Check/Quote/Hold/Task/River atomic.

-- name: GetWorkspaceBudget :one
SELECT workspace_id, period, limit_credits, updated_by_user_id, created_at, updated_at
FROM workspace_budgets
WHERE workspace_id = sqlc.arg(workspace_id)
LIMIT 1;

-- name: GetWorkspaceBudgetForUpdate :one
SELECT workspace_id, period, limit_credits, updated_by_user_id, created_at, updated_at
FROM workspace_budgets
WHERE workspace_id = sqlc.arg(workspace_id)
FOR UPDATE
LIMIT 1;

-- name: UpsertWorkspaceBudget :one
INSERT INTO workspace_budgets (workspace_id, period, limit_credits, updated_by_user_id)
VALUES (sqlc.arg(workspace_id), sqlc.arg(period), sqlc.arg(limit_credits), sqlc.arg(updated_by_user_id))
ON CONFLICT (workspace_id) DO UPDATE SET
  period = EXCLUDED.period,
  limit_credits = EXCLUDED.limit_credits,
  updated_by_user_id = EXCLUDED.updated_by_user_id,
  updated_at = now()
RETURNING workspace_id, period, limit_credits, updated_by_user_id, created_at, updated_at;
