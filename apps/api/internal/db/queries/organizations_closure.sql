-- Organization closure guards (split from organizations.sql).
-- Permanent Organization closure (#955). Row locks serialize closure with mutations;
-- the unique closure audit prevents a second close.

-- name: LockOrganizationByPublicID :one
SELECT internal_id, public_id, name, created_at, updated_at, closed_at, closed_by
FROM organizations
WHERE public_id = sqlc.arg(public_id)
FOR UPDATE;


-- name: SetOrganizationClosed :execrows
UPDATE organizations
SET closed_at = now(), closed_by = sqlc.arg(closed_by), updated_at = now()
WHERE public_id = sqlc.arg(public_id) AND closed_at IS NULL;


-- name: InsertOrganizationClosure :one
INSERT INTO organization_closures (
  organization_id, billing_account_id, closed_by, reason,
  membership_snapshot, workspace_snapshot,
  available_balance, reserved_balance, credited_order_refs,
  disposition, disposition_detail
)
VALUES (
  sqlc.arg(organization_id), sqlc.arg(billing_account_id), sqlc.arg(closed_by), sqlc.arg(reason),
  sqlc.arg(membership_snapshot), sqlc.arg(workspace_snapshot),
  sqlc.arg(available_balance), sqlc.arg(reserved_balance), sqlc.arg(credited_order_refs),
  sqlc.arg(disposition), sqlc.arg(disposition_detail)
)
RETURNING internal_id, organization_id, billing_account_id,
  closed_at, closed_by, reason,
  membership_snapshot, workspace_snapshot,
  available_balance, reserved_balance, credited_order_refs,
  disposition, disposition_detail;


-- name: ListOrganizationMembersForSnapshot :many
SELECT m.user_id, m.role
FROM organization_memberships AS m
WHERE m.organization_id = sqlc.arg(organization_id)
ORDER BY m.user_id ASC;


-- name: ListOrganizationWorkspacePublicIDs :many
SELECT w.public_id
FROM workspaces AS w
WHERE w.organization_id = sqlc.arg(organization_id)
ORDER BY w.internal_id ASC;


-- name: CountActiveOrgTasks :one
SELECT COUNT(*)
FROM tasks AS t
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
WHERE w.organization_id = sqlc.arg(organization_id)
  AND t.status IN ('pending', 'running');


-- name: CountActiveOrgSearchOperations :one
SELECT COUNT(*)
FROM ai_search_operations AS op
INNER JOIN workspaces AS w ON w.internal_id = op.workspace_id
WHERE w.organization_id = sqlc.arg(organization_id)
  AND op.status IN ('pending', 'reconciliation_pending');


-- name: CountUnsettledOrgPaymentOrders :one
SELECT COUNT(*)
FROM payment_orders AS orders
WHERE orders.billing_account_id = sqlc.arg(billing_account_id)
  AND orders.status IN ('creating', 'checkout_open', 'async_payment_pending', 'paid_pending_credit');


-- name: CountPendingOrgPaymentRefunds :one
SELECT COUNT(*)
FROM payment_refunds AS refunds
INNER JOIN payment_orders AS orders ON orders.id = refunds.payment_order_id
WHERE orders.billing_account_id = sqlc.arg(billing_account_id)
  AND refunds.status = 'pending';


-- name: CountOpenOrgPaymentDisputes :one
SELECT COUNT(*)
FROM payment_disputes AS disputes
INNER JOIN payment_orders AS orders ON orders.id = disputes.payment_order_id
WHERE orders.billing_account_id = sqlc.arg(billing_account_id)
  AND disputes.status = 'open';


-- name: CountPendingOrgTransferInvites :one
SELECT COUNT(*)
FROM billing_account_transfer_invites AS invites
WHERE invites.billing_account_id = sqlc.arg(billing_account_id)
  AND invites.status = 'pending'
  AND invites.expires_at > now();


-- name: GetWalletBalancesForAccount :one
SELECT
  MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
  MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
FROM credit_accounts AS accounts
WHERE accounts.billing_account_id = sqlc.arg(billing_account_id)
  AND accounts.account_role IN ('user_available', 'user_reserved');


-- name: ListCreditedOrgPaymentOrders :many
SELECT orders.public_order_number, orders.terms_version, orders.refund_policy_version
FROM payment_orders AS orders
WHERE orders.billing_account_id = sqlc.arg(billing_account_id)
  AND orders.status = 'credited'
ORDER BY orders.created_at ASC
LIMIT sqlc.arg(row_limit);


-- name: FreezeOrganizationWallet :exec
SELECT set_billing_account_wallet_status(sqlc.arg(billing_account_id)::BIGINT, 'frozen'::TEXT);
