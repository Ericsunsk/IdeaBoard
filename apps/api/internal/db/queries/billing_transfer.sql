-- Studio handover invitation queries (#948). The invite row is the audit
-- fact: created/decided timestamps plus decider survive scope hard deletes
-- (billing_account_id is a plain BIGINT with no FK, same precedent as
-- payment_orders.billing_account_id). All writes join the caller-owned
-- transaction via the passed Queries, so acceptance stays atomic with the
-- billing-member rewrite plus non-Default Workspace grants.

-- name: CreateBillingTransferInvite :one
INSERT INTO billing_account_transfer_invites (
  public_id, billing_account_id, billing_account_public_id,
  from_user_id, to_user_id, status, idempotency_key, expires_at
)
VALUES (
  sqlc.arg(public_id), sqlc.arg(billing_account_id),
  sqlc.arg(billing_account_public_id), sqlc.arg(from_user_id),
  sqlc.arg(to_user_id), 'pending', sqlc.arg(idempotency_key),
  now() + INTERVAL '7 days'
)
RETURNING internal_id, public_id, billing_account_id, billing_account_public_id,
  from_user_id, to_user_id, status, idempotency_key,
  scope_workspace_public_ids, created_at, expires_at, decided_at, decided_by;

-- name: GetBillingTransferInviteByAccountIdem :one
SELECT internal_id, public_id, billing_account_id, billing_account_public_id,
  from_user_id, to_user_id, status, idempotency_key,
  scope_workspace_public_ids, created_at, expires_at, decided_at, decided_by
FROM billing_account_transfer_invites
WHERE billing_account_id = sqlc.arg(billing_account_id)
  AND idempotency_key = sqlc.arg(idempotency_key)
LIMIT 1;

-- name: GetBillingTransferInviteByPublicID :one
SELECT internal_id, public_id, billing_account_id, billing_account_public_id,
  from_user_id, to_user_id, status, idempotency_key,
  scope_workspace_public_ids, created_at, expires_at, decided_at, decided_by
FROM billing_account_transfer_invites
WHERE public_id = sqlc.arg(public_id)
LIMIT 1;

-- name: GetBillingTransferInviteForUpdate :one
SELECT internal_id, public_id, billing_account_id, billing_account_public_id,
  from_user_id, to_user_id, status, idempotency_key,
  scope_workspace_public_ids, created_at, expires_at, decided_at, decided_by
FROM billing_account_transfer_invites
WHERE public_id = sqlc.arg(public_id)
FOR UPDATE;

-- name: GetPendingBillingTransferForAccount :one
SELECT internal_id, public_id, billing_account_id, billing_account_public_id,
  from_user_id, to_user_id, status, idempotency_key,
  scope_workspace_public_ids, created_at, expires_at, decided_at, decided_by
FROM billing_account_transfer_invites
WHERE billing_account_id = sqlc.arg(billing_account_id)
  AND status = 'pending'
LIMIT 1;

-- name: ListBillingTransferInvitesForAccount :many
SELECT internal_id, public_id, billing_account_id, billing_account_public_id,
  from_user_id, to_user_id, status, idempotency_key,
  scope_workspace_public_ids, created_at, expires_at, decided_at, decided_by
FROM billing_account_transfer_invites
WHERE billing_account_id = sqlc.arg(billing_account_id)
  AND (sqlc.arg(recipient_user_id)::BIGINT = 0 OR to_user_id = sqlc.arg(recipient_user_id))
  AND (sqlc.arg(before_id)::BIGINT = 0 OR internal_id < sqlc.arg(before_id)::BIGINT)
ORDER BY internal_id DESC
LIMIT sqlc.arg(row_limit);

-- name: DecideBillingTransferInvite :one
UPDATE billing_account_transfer_invites
SET status = sqlc.arg(status),
    decided_at = now(),
    decided_by = sqlc.arg(decided_by),
    scope_workspace_public_ids = sqlc.arg(scope_workspace_public_ids)
WHERE public_id = sqlc.arg(public_id)
  AND status = 'pending'
RETURNING internal_id, public_id, billing_account_id, billing_account_public_id,
  from_user_id, to_user_id, status, idempotency_key,
  scope_workspace_public_ids, created_at, expires_at, decided_at, decided_by;

-- name: ReissueBillingTransferInvite :one
-- Re-issue after lapse (#948 follow-up): when the account's single pending
-- invite has expired, the owner may send again. The pending row refreshes in
-- place (new parties, key, and 7-day expiry under the same bti_ id), so the
-- at-most-one-pending-per-account invariant needs no index change. Matches
-- zero rows unless a lapsed pending invite exists; callers fall back to the
-- pending guard on no match (lost race or still-live invite).
UPDATE billing_account_transfer_invites
SET from_user_id = sqlc.arg(from_user_id),
    to_user_id = sqlc.arg(to_user_id),
    idempotency_key = sqlc.arg(idempotency_key),
    created_at = now(),
    expires_at = now() + INTERVAL '7 days'
WHERE billing_account_id = sqlc.arg(billing_account_id)
  AND status = 'pending'
  AND expires_at <= now()
RETURNING internal_id, public_id, billing_account_id, billing_account_public_id,
  from_user_id, to_user_id, status, idempotency_key,
  scope_workspace_public_ids, created_at, expires_at, decided_at, decided_by;

-- name: ListHandoverWorkspacesForAccount :many
-- Non-Default business Workspace scope for handover acceptance (#948):
-- the anchor workspace plus explicitly linked workspaces plus org-pool
-- workspaces, excluding every Default personal workspace (never transferred).
SELECT w.internal_id, w.public_id, w.organization_id, w.owner_user_id, w.name, w.is_default, w.created_at, w.updated_at
FROM workspaces AS w
WHERE w.is_default = FALSE
  AND (
    w.internal_id IN (
      SELECT b.workspace_id FROM billing_accounts AS b
      WHERE b.internal_id = sqlc.arg(billing_account_id) AND b.workspace_id IS NOT NULL
    )
    OR w.internal_id IN (
      SELECT l.workspace_id FROM workspace_billing_links AS l
      WHERE l.billing_account_id = sqlc.arg(billing_account_id)
    )
    OR (
      w.organization_id IS NOT NULL
      AND w.organization_id IN (
        SELECT b.organization_id FROM billing_accounts AS b
        WHERE b.internal_id = sqlc.arg(billing_account_id) AND b.organization_id IS NOT NULL
      )
    )
  )
ORDER BY w.internal_id ASC;
