-- Organization tenancy queries (#801). Public references use public_id;
-- internal_id stays database-internal.

-- name: CreateOrganization :one
INSERT INTO organizations (public_id, name)
VALUES (sqlc.arg(public_id), sqlc.arg(name))
RETURNING internal_id, public_id, name, created_at, updated_at, archived_at;

-- name: GetOrganizationByPublicID :one
SELECT internal_id, public_id, name, created_at, updated_at, archived_at
FROM organizations
WHERE public_id = sqlc.arg(public_id)
LIMIT 1;

-- name: GetOrganizationByInternalID :one
SELECT internal_id, public_id, name, created_at, updated_at, archived_at
FROM organizations
WHERE internal_id = sqlc.arg(internal_id)
LIMIT 1;

-- name: ListOrganizationsByUserID :many
SELECT o.internal_id, o.public_id, o.name, o.created_at, o.updated_at, o.archived_at
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
