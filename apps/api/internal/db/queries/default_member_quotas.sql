-- Account default new-member consumption quotas (#947). One row per
-- Billing Account; absent means NOT CONFIGURED (fail closed: invites are
-- blocked until a Billing Owner/Admin sets the default, never an implicit
-- unlimited). The default seeds the per-member quota row on first join of
-- the account consumption scope (same five periods as #946). All queries
-- are typed; the invite path joins the caller-owned transaction so
-- membership insert + quota seed stay atomic.

-- name: GetDefaultMemberQuota :one
SELECT billing_account_id, period, limit_credits, updated_by_user_id, created_at, updated_at
FROM billing_account_default_member_quotas
WHERE billing_account_id = sqlc.arg(billing_account_id)
LIMIT 1;

-- name: GetDefaultMemberQuotaForUpdate :one
SELECT billing_account_id, period, limit_credits, updated_by_user_id, created_at, updated_at
FROM billing_account_default_member_quotas
WHERE billing_account_id = sqlc.arg(billing_account_id)
FOR UPDATE
LIMIT 1;

-- name: UpsertDefaultMemberQuota :one
INSERT INTO billing_account_default_member_quotas (billing_account_id, period, limit_credits, updated_by_user_id)
VALUES (sqlc.arg(billing_account_id), sqlc.arg(period), sqlc.arg(limit_credits), sqlc.arg(updated_by_user_id))
ON CONFLICT (billing_account_id) DO UPDATE SET
  period = EXCLUDED.period,
  limit_credits = EXCLUDED.limit_credits,
  updated_by_user_id = EXCLUDED.updated_by_user_id,
  updated_at = now()
RETURNING billing_account_id, period, limit_credits, updated_by_user_id, created_at, updated_at;

-- name: InsertMemberQuotaFromDefault :one
INSERT INTO billing_account_member_quotas (billing_account_id, user_id, period, limit_credits, updated_by_user_id)
SELECT sqlc.arg(billing_account_id), sqlc.arg(user_id), period, limit_credits, sqlc.arg(updated_by_user_id)
FROM billing_account_default_member_quotas
WHERE billing_account_id = sqlc.arg(billing_account_id)
ON CONFLICT (billing_account_id, user_id) DO NOTHING
RETURNING billing_account_id, user_id, period, limit_credits, updated_by_user_id, created_at, updated_at;
