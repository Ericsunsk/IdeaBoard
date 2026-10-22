-- Organization tenancy queries (#801). Public references use public_id;
-- internal_id stays database-internal.

-- name: CreateOrganization :one
INSERT INTO organizations (public_id, name)
VALUES (sqlc.arg(public_id), sqlc.arg(name))
RETURNING internal_id, public_id, name, created_at, updated_at, archived_at, closed_at, closed_by;

-- name: GetOrganizationByPublicID :one
SELECT internal_id, public_id, name, created_at, updated_at, archived_at, closed_at, closed_by
FROM organizations
WHERE public_id = sqlc.arg(public_id)
LIMIT 1;

-- name: GetOrganizationByInternalID :one
SELECT internal_id, public_id, name, created_at, updated_at, archived_at, closed_at, closed_by
FROM organizations
WHERE internal_id = sqlc.arg(internal_id)
LIMIT 1;

-- name: ListOrganizationsByUserID :many
SELECT o.internal_id, o.public_id, o.name, o.created_at, o.updated_at, o.archived_at, o.closed_at, o.closed_by
FROM organizations AS o
INNER JOIN organization_memberships AS m ON m.organization_id = o.internal_id
WHERE m.user_id = sqlc.arg(user_id) AND o.archived_at IS NULL
ORDER BY o.internal_id ASC
LIMIT sqlc.arg(row_limit);

-- name: UpdateOrganizationName :execrows
UPDATE organizations
SET name = sqlc.arg(name), updated_at = now()
WHERE public_id = sqlc.arg(public_id) AND archived_at IS NULL;

-- name: UpsertOrganizationMembership :exec
INSERT INTO organization_memberships (organization_id, user_id, role)
VALUES (sqlc.arg(organization_id), sqlc.arg(user_id), sqlc.arg(role))
ON CONFLICT (organization_id, user_id) DO UPDATE
SET role = EXCLUDED.role;

-- name: GetOrganizationMembership :one
SELECT organization_id, user_id, role, created_at
FROM organization_memberships
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id);

-- name: ListOrganizationMembers :many
SELECT m.organization_id, m.user_id, m.role, m.created_at
FROM organization_memberships AS m
WHERE m.organization_id = sqlc.arg(organization_id)
ORDER BY m.user_id ASC
LIMIT sqlc.arg(row_limit);

-- name: CountOrganizationOwners :one
SELECT COUNT(*)
FROM organization_memberships
WHERE organization_id = sqlc.arg(organization_id) AND role = 'owner';

-- name: CountOrganizationsByUserID :one
SELECT COUNT(*)
FROM organizations AS o
INNER JOIN organization_memberships AS m ON m.organization_id = o.internal_id
WHERE m.user_id = sqlc.arg(user_id) AND o.archived_at IS NULL;

-- name: DeleteOrganizationMembership :execrows
DELETE FROM organization_memberships
WHERE organization_id = sqlc.arg(organization_id) AND user_id = sqlc.arg(user_id);

-- name: LockUserRow :one
SELECT id
FROM users
WHERE id = sqlc.arg(user_id)
FOR UPDATE;

-- name: DeleteOrgWorkspaceMembershipsForUser :exec
DELETE FROM workspace_memberships AS m
USING workspaces AS w
WHERE m.workspace_id = w.internal_id
  AND w.organization_id = sqlc.arg(organization_id)
  AND m.user_id = sqlc.arg(user_id);

-- Organization closure queries (#955). Close/Restore pin the organization
-- row FOR UPDATE so concurrent closes serialize; the open-closure unique
-- index collapses a lost race to the winner via the insert conflict plus
-- the SetOrganizationClosed row count below.

-- name: LockOrganizationByPublicID :one
SELECT internal_id, public_id, name, created_at, updated_at, archived_at, closed_at, closed_by
FROM organizations
WHERE public_id = sqlc.arg(public_id)
FOR UPDATE;

-- name: SetOrganizationClosed :execrows
UPDATE organizations
SET closed_at = now(), closed_by = sqlc.arg(closed_by), updated_at = now()
WHERE public_id = sqlc.arg(public_id) AND closed_at IS NULL;

-- name: ClearOrganizationClosed :execrows
UPDATE organizations
SET closed_at = NULL, closed_by = NULL, updated_at = now()
WHERE public_id = sqlc.arg(public_id) AND closed_at IS NOT NULL;

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
  disposition, disposition_detail, restored_at, restored_by;

-- name: MarkOrganizationClosureRestored :execrows
UPDATE organization_closures
SET restored_at = now(), restored_by = sqlc.arg(restored_by)
WHERE organization_id = sqlc.arg(organization_id)
  AND restored_at IS NULL;

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
