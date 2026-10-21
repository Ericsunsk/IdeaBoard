-- Billing Account resolution and membership queries (#915). Public references
-- use public_id (ba_); internal_id stays database-internal.
--
-- Resolution is explicit and fail-closed: one Workspace resolves to exactly
-- one Billing Account via (1) explicit workspace_billing_links, else (2) the
-- Organization account for org workspaces, else (3) the per-workspace
-- account. Missing, ambiguous, or conflicting ownership fails closed in the
-- caller (no silent fallback to another wallet). All queries are typed and
-- join the caller-owned transaction via the passed Queries (no ambient pool
-- use here), so Task settlement keeps Quote/Hold/Task/River atomic.

-- name: GetBillingAccountByOrganizationID :one
SELECT internal_id, public_id, workspace_id, organization_id, created_at, updated_at
FROM billing_accounts
WHERE organization_id = sqlc.arg(organization_id)
LIMIT 1;

-- name: GetBillingAccountByPublicID :one
SELECT internal_id, public_id, workspace_id, organization_id, created_at, updated_at
FROM billing_accounts
WHERE public_id = sqlc.arg(public_id)
LIMIT 1;

-- name: GetBillingAccountByInternalID :one
SELECT internal_id, public_id, workspace_id, organization_id, created_at, updated_at
FROM billing_accounts
WHERE internal_id = sqlc.arg(internal_id)
LIMIT 1;

-- name: LockBillingAccount :one
-- Serialize financial authority changes and Workspace scope mutations on the
-- stable account before reading roles or taking Workspace/invitation locks.
SELECT internal_id, public_id, workspace_id, organization_id, created_at, updated_at
FROM billing_accounts
WHERE internal_id = sqlc.arg(internal_id)
FOR NO KEY UPDATE;

-- name: ListBillingAccountsForAdmin :many
SELECT b.internal_id, b.public_id, b.workspace_id, b.organization_id, b.created_at, b.updated_at,
       w.public_id AS workspace_public_id, o.public_id AS organization_public_id
FROM billing_accounts AS b
LEFT JOIN workspaces AS w ON w.internal_id = b.workspace_id
LEFT JOIN organizations AS o ON o.internal_id = b.organization_id
WHERE (sqlc.arg(before_id)::BIGINT = 0 OR b.internal_id < sqlc.arg(before_id)::BIGINT)
  AND (sqlc.arg(owner_scope)::TEXT = 'all'
    OR (sqlc.arg(owner_scope)::TEXT = 'organization' AND b.organization_id IS NOT NULL)
    OR (sqlc.arg(owner_scope)::TEXT = 'workspace' AND b.organization_id IS NULL))
ORDER BY b.internal_id DESC
LIMIT sqlc.arg(row_limit);

-- name: GetWorkspaceBillingLink :one
SELECT workspace_id, billing_account_id, created_at
FROM workspace_billing_links
WHERE workspace_id = sqlc.arg(workspace_id)
LIMIT 1;

-- name: UpsertWorkspaceBillingLink :one
INSERT INTO workspace_billing_links (workspace_id, billing_account_id)
VALUES (sqlc.arg(workspace_id), sqlc.arg(billing_account_id))
ON CONFLICT (workspace_id) DO UPDATE SET billing_account_id = EXCLUDED.billing_account_id
RETURNING workspace_id, billing_account_id, created_at;

-- name: ListWorkspacesByBillingAccountID :many
SELECT w.internal_id, w.public_id, w.organization_id, w.owner_user_id, w.name, w.is_default, w.created_at, w.updated_at
FROM workspaces AS w
INNER JOIN workspace_billing_links AS l ON l.workspace_id = w.internal_id
WHERE l.billing_account_id = sqlc.arg(billing_account_id)
ORDER BY w.internal_id ASC
LIMIT sqlc.arg(row_limit);

-- name: GetBillingAccountMember :one
SELECT billing_account_id, user_id, role, created_at
FROM billing_account_members
WHERE billing_account_id = sqlc.arg(billing_account_id) AND user_id = sqlc.arg(user_id);

-- name: ListBillingAccountMembers :many
SELECT billing_account_id, user_id, role, created_at
FROM billing_account_members
WHERE billing_account_id = sqlc.arg(billing_account_id)
ORDER BY user_id ASC
LIMIT sqlc.arg(row_limit);

-- name: ListBillingAccountsForUser :many
SELECT b.internal_id, b.public_id, b.workspace_id, b.organization_id, b.created_at, b.updated_at
FROM billing_accounts AS b
INNER JOIN billing_account_members AS m ON m.billing_account_id = b.internal_id
WHERE m.user_id = sqlc.arg(user_id)
ORDER BY b.internal_id ASC
LIMIT sqlc.arg(row_limit);

-- name: UpsertBillingAccountMember :one
INSERT INTO billing_account_members (billing_account_id, user_id, role)
VALUES (sqlc.arg(billing_account_id), sqlc.arg(user_id), sqlc.arg(role))
ON CONFLICT (billing_account_id, user_id) DO UPDATE SET role = EXCLUDED.role
RETURNING billing_account_id, user_id, role, created_at;

-- name: DeleteBillingAccountMember :execrows
DELETE FROM billing_account_members
WHERE billing_account_id = sqlc.arg(billing_account_id) AND user_id = sqlc.arg(user_id);

-- name: CountBillingAccountOwners :one
SELECT COUNT(*)
FROM billing_account_members
WHERE billing_account_id = sqlc.arg(billing_account_id) AND role = 'owner';

-- name: GetWorkspaceByInternalID :one
SELECT internal_id, public_id, organization_id, owner_user_id, name, is_default, created_at, updated_at
FROM workspaces
WHERE internal_id = sqlc.arg(internal_id)
LIMIT 1;

-- name: GetWorkspaceMembershipByIDs :one
SELECT workspace_id, user_id, role, created_at
FROM workspace_memberships
WHERE workspace_id = sqlc.arg(workspace_id) AND user_id = sqlc.arg(user_id);

-- name: ListOrganizationOwners :many
SELECT organization_id, user_id, role, created_at
FROM organization_memberships
WHERE organization_id = sqlc.arg(organization_id) AND role = 'owner'
ORDER BY user_id ASC;

-- name: LockWorkspaceBillingAccounts :many
-- Lock the resolved payer and any account anchored to the Workspace before
-- deletion. Stable ID ordering keeps the account -> Workspace lock order even
-- when an explicitly shared Workspace already has an older anchor account.
-- Resolve without provisioning; an unfunded Workspace returns no rows.
SELECT b.internal_id
FROM workspaces AS w
LEFT JOIN workspace_billing_links AS l ON l.workspace_id = w.internal_id
JOIN billing_accounts AS b ON b.workspace_id = w.internal_id
  OR b.internal_id = COALESCE(
    l.billing_account_id,
    (SELECT a.internal_id FROM billing_accounts AS a WHERE a.organization_id = w.organization_id),
    (SELECT a.internal_id FROM billing_accounts AS a WHERE a.workspace_id = w.internal_id)
  )
WHERE w.public_id = sqlc.arg(workspace_public_id)
ORDER BY b.internal_id
FOR NO KEY UPDATE OF b;
