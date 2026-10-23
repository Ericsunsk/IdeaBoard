-- Member cross-workspace consumption quotas (#946). One row per
-- (Billing Account, User); absent means unlimited (the default). The quota
-- is a cumulative cap on one member's settled + pending consumption across
-- every Workspace sharing the account, not a wallet, grant, or balance
-- reservation. All queries are typed and join the caller-owned transaction
-- via the passed Queries, so Task/Vision settlement keeps Check/Quote/
-- Hold/Task/River atomic at the same boundary (one fixed lock order).

-- name: GetMemberQuota :one
SELECT billing_account_id, user_id, period, limit_credits, updated_by_user_id, created_at, updated_at
FROM billing_account_member_quotas
WHERE billing_account_id = sqlc.arg(billing_account_id) AND user_id = sqlc.arg(user_id)
LIMIT 1;

-- name: GetMemberQuotaForUpdate :one
SELECT billing_account_id, user_id, period, limit_credits, updated_by_user_id, created_at, updated_at
FROM billing_account_member_quotas
WHERE billing_account_id = sqlc.arg(billing_account_id) AND user_id = sqlc.arg(user_id)
FOR UPDATE
LIMIT 1;

-- name: UpsertMemberQuota :one
INSERT INTO billing_account_member_quotas (billing_account_id, user_id, period, limit_credits, updated_by_user_id)
VALUES (sqlc.arg(billing_account_id), sqlc.arg(user_id), sqlc.arg(period), sqlc.arg(limit_credits), sqlc.arg(updated_by_user_id))
ON CONFLICT (billing_account_id, user_id) DO UPDATE SET
  period = EXCLUDED.period,
  limit_credits = EXCLUDED.limit_credits,
  updated_by_user_id = EXCLUDED.updated_by_user_id,
  updated_at = now()
RETURNING billing_account_id, user_id, period, limit_credits, updated_by_user_id, created_at, updated_at;

-- name: ListMemberQuotasByAccount :many
SELECT billing_account_id, user_id, period, limit_credits, updated_by_user_id, created_at, updated_at
FROM billing_account_member_quotas
WHERE billing_account_id = sqlc.arg(billing_account_id)
ORDER BY user_id ASC
LIMIT sqlc.arg(row_limit);
