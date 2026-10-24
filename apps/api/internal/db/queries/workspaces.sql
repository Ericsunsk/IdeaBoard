-- Workspace tenancy queries (#801). Public references use public_id;
-- internal_id stays database-internal.
--
-- Deletion is an immediate irreversible hard delete: there is no
-- archive/restore lifecycle and no workspaces.archived_at column.
--
-- Default workspace rule (Bug2 root fix): every user owns exactly one
-- Default personal workspace (is_default). Plain personal workspaces created
-- without an organization are always non-Default and unlimited; only
-- EnsureDefaultWorkspace mints the Default row.

-- name: CreateWorkspace :one
INSERT INTO workspaces (public_id, organization_id, owner_user_id, name, is_default)
VALUES (sqlc.arg(public_id), sqlc.narg(organization_id), sqlc.narg(owner_user_id), sqlc.arg(name), sqlc.arg(is_default))
RETURNING internal_id, public_id, organization_id, owner_user_id, name, is_default, created_at, updated_at;

-- name: GetWorkspaceByPublicID :one
SELECT internal_id, public_id, organization_id, owner_user_id, name, is_default, created_at, updated_at
FROM workspaces
WHERE public_id = sqlc.arg(public_id)
LIMIT 1;

-- name: LockWorkspaceMemberships :one
SELECT internal_id
FROM workspaces
WHERE public_id = sqlc.arg(public_id)
FOR UPDATE;

-- name: CountWorkspaceMembers :one
SELECT COUNT(*)
FROM workspace_memberships
WHERE workspace_id = sqlc.arg(workspace_id);

-- name: ListWorkspacesByUserID :many
SELECT w.internal_id, w.public_id, w.organization_id, w.owner_user_id, w.name, w.is_default, w.created_at, w.updated_at
FROM workspaces AS w
INNER JOIN workspace_memberships AS m ON m.workspace_id = w.internal_id
WHERE m.user_id = sqlc.arg(user_id)
ORDER BY w.internal_id ASC
LIMIT sqlc.arg(row_limit);

-- name: ListWorkspacesByOrganizationID :many
SELECT internal_id, public_id, organization_id, owner_user_id, name, is_default, created_at, updated_at
FROM workspaces
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY internal_id ASC
LIMIT sqlc.arg(row_limit);

-- name: UpdateWorkspaceName :execrows
UPDATE workspaces
SET name = sqlc.arg(name), updated_at = now()
WHERE public_id = sqlc.arg(public_id);

-- name: UpsertWorkspaceMembership :exec
INSERT INTO workspace_memberships (workspace_id, user_id, role)
VALUES (sqlc.arg(workspace_id), sqlc.arg(user_id), sqlc.arg(role))
ON CONFLICT (workspace_id, user_id) DO UPDATE
SET role = EXCLUDED.role;

-- name: InsertWorkspaceMembershipIgnore :execrows
INSERT INTO workspace_memberships (workspace_id, user_id, role)
VALUES (sqlc.arg(workspace_id), sqlc.arg(user_id), sqlc.arg(role))
ON CONFLICT (workspace_id, user_id) DO NOTHING;

-- name: GetWorkspaceMembership :one
SELECT workspace_id, user_id, role, created_at
FROM workspace_memberships
WHERE workspace_id = sqlc.arg(workspace_id) AND user_id = sqlc.arg(user_id);

-- name: ListWorkspaceMembers :many
SELECT m.workspace_id, m.user_id, m.role, m.created_at
FROM workspace_memberships AS m
WHERE m.workspace_id = sqlc.arg(workspace_id)
ORDER BY m.created_at ASC
LIMIT sqlc.arg(row_limit);

-- name: UpdateWorkspaceMemberRole :execrows
UPDATE workspace_memberships
SET role = sqlc.arg(role)
WHERE workspace_id = sqlc.arg(workspace_id) AND user_id = sqlc.arg(user_id);

-- name: DeleteWorkspaceMembership :execrows
DELETE FROM workspace_memberships
WHERE workspace_id = sqlc.arg(workspace_id) AND user_id = sqlc.arg(user_id);

-- name: CountWorkspaceOwners :one
SELECT COUNT(*)
FROM workspace_memberships
WHERE workspace_id = sqlc.arg(workspace_id) AND role = 'owner';

-- Default workspace provisioning is idempotent on the partial unique index
-- ux_workspaces_default_owner: concurrent inserts for the same owner collapse
-- to a single Default row, and the follow-up select always reads the winner.
-- name: InsertDefaultWorkspaceIgnore :execrows
INSERT INTO workspaces (public_id, owner_user_id, name, is_default)
VALUES (sqlc.arg(public_id), sqlc.arg(owner_user_id), sqlc.arg(name), TRUE)
ON CONFLICT (owner_user_id) WHERE organization_id IS NULL AND is_default DO NOTHING;

-- name: GetDefaultWorkspaceByOwner :one
SELECT internal_id, public_id, organization_id, owner_user_id, name, is_default, created_at, updated_at
FROM workspaces
WHERE owner_user_id = sqlc.arg(owner_user_id) AND organization_id IS NULL AND is_default
LIMIT 1;

-- Batch workspace counts per organization (organization list
-- expand=workspace_counts): a single GROUP BY replaces per-organization
-- ListWorkspacesByOrganizationID round trips. Closed organizations report
-- zero: their workspaces stay hidden from listing, matching
-- ListWorkspacesForUser visibility.
-- name: CountWorkspacesByOrganizationPublicIDs :many
SELECT o.public_id AS organization_public_id, COUNT(w.internal_id) AS workspace_count
FROM organizations AS o
LEFT JOIN workspaces AS w ON w.organization_id = o.internal_id AND o.closed_at IS NULL
WHERE o.public_id = ANY(sqlc.arg(organization_public_ids)::text[])
GROUP BY o.public_id;

-- Billing Account owner stub (#801). Full lifecycle belongs to #804.
-- name: CreateBillingAccountStub :one
INSERT INTO billing_accounts (public_id, workspace_id, organization_id)
VALUES (sqlc.arg(public_id), sqlc.narg(workspace_id), sqlc.narg(organization_id))
RETURNING internal_id, public_id, workspace_id, organization_id, created_at, updated_at;

-- name: GetBillingAccountByWorkspaceID :one
SELECT internal_id, public_id, workspace_id, organization_id, created_at, updated_at
FROM billing_accounts
WHERE workspace_id = sqlc.arg(workspace_id)
LIMIT 1;
