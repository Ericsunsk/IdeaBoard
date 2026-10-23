-- Organization tenancy queries (#801). Public references use public_id;
-- internal_id stays database-internal.

-- name: CreateOrganization :one
INSERT INTO organizations (public_id, name)
VALUES (sqlc.arg(public_id), sqlc.arg(name))
RETURNING internal_id, public_id, name, created_at, updated_at, closed_at, closed_by;


-- name: GetOrganizationByPublicID :one
SELECT internal_id, public_id, name, created_at, updated_at, closed_at, closed_by
FROM organizations
WHERE public_id = sqlc.arg(public_id)
LIMIT 1;


-- name: GetOrganizationByInternalID :one
SELECT internal_id, public_id, name, created_at, updated_at, closed_at, closed_by
FROM organizations
WHERE internal_id = sqlc.arg(internal_id)
LIMIT 1;


-- name: ListOrganizationsByUserID :many
SELECT o.internal_id, o.public_id, o.name, o.created_at, o.updated_at, o.closed_at, o.closed_by
FROM organizations AS o
INNER JOIN organization_memberships AS m ON m.organization_id = o.internal_id
WHERE m.user_id = sqlc.arg(user_id) AND o.closed_at IS NULL
ORDER BY o.internal_id ASC
LIMIT sqlc.arg(row_limit);


-- name: UpdateOrganizationName :execrows
UPDATE organizations
SET name = sqlc.arg(name), updated_at = now()
WHERE public_id = sqlc.arg(public_id);


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
WHERE m.user_id = sqlc.arg(user_id) AND o.closed_at IS NULL;


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

-- Permanent Organization closure. Row locks serialize closure with mutations;
-- the unique closure audit prevents a second close.
