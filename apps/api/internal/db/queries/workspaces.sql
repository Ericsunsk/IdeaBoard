-- Workspace tenancy queries (#801). Public references use public_id;
-- internal_id stays database-internal.

-- name: CreateWorkspace :one
INSERT INTO workspaces (public_id, organization_id, owner_user_id, name)
VALUES (sqlc.arg(public_id), sqlc.narg(organization_id), sqlc.narg(owner_user_id), sqlc.arg(name))
RETURNING internal_id, public_id, organization_id, owner_user_id, name, created_at, updated_at, archived_at;

-- name: GetWorkspaceByPublicID :one
SELECT internal_id, public_id, organization_id, owner_user_id, name, created_at, updated_at, archived_at
FROM workspaces
WHERE public_id = sqlc.arg(public_id)
LIMIT 1;

-- name: GetWorkspaceByInternalID :one
SELECT internal_id, public_id, organization_id, owner_user_id, name, created_at, updated_at, archived_at
FROM workspaces
WHERE internal_id = sqlc.arg(internal_id)
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
SELECT w.internal_id, w.public_id, w.organization_id, w.owner_user_id, w.name, w.created_at, w.updated_at, w.archived_at
FROM workspaces AS w
INNER JOIN workspace_memberships AS m ON m.workspace_id = w.internal_id
WHERE m.user_id = sqlc.arg(user_id) AND w.archived_at IS NULL
ORDER BY w.internal_id ASC
LIMIT sqlc.arg(row_limit);

-- name: ListWorkspacesByOrganizationID :many
SELECT internal_id, public_id, organization_id, owner_user_id, name, created_at, updated_at, archived_at
FROM workspaces
WHERE organization_id = sqlc.arg(organization_id) AND archived_at IS NULL
ORDER BY internal_id ASC
LIMIT sqlc.arg(row_limit);

-- name: UpdateWorkspaceName :execrows
UPDATE workspaces
SET name = sqlc.arg(name), updated_at = now()
WHERE public_id = sqlc.arg(public_id) AND archived_at IS NULL;

-- name: ArchiveWorkspace :execrows
UPDATE workspaces
SET archived_at = now(), updated_at = now()
WHERE public_id = sqlc.arg(public_id) AND archived_at IS NULL;

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

-- Personal workspace provisioning is idempotent on the partial unique index
-- ux_workspaces_personal_owner: concurrent inserts for the same owner collapse
-- to a single row, and the follow-up select always reads the winner.
-- name: InsertPersonalWorkspaceIgnore :execrows
INSERT INTO workspaces (public_id, owner_user_id, name)
VALUES (sqlc.arg(public_id), sqlc.arg(owner_user_id), sqlc.arg(name))
ON CONFLICT (owner_user_id) WHERE organization_id IS NULL AND archived_at IS NULL DO NOTHING;

-- name: GetPersonalWorkspaceByOwner :one
SELECT internal_id, public_id, organization_id, owner_user_id, name, created_at, updated_at, archived_at
FROM workspaces
WHERE owner_user_id = sqlc.arg(owner_user_id) AND organization_id IS NULL AND archived_at IS NULL
LIMIT 1;

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
