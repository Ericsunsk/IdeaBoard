-- Organization member invitation queries (#957, ADR-0041).
--
-- The invite row is the audit fact: created/responded timestamps plus decider.
-- All writes join the caller-owned transaction via the passed Queries,
-- so acceptance stays atomic with the organization_memberships insertion
-- and preserves the single-org invariant per user.

-- name: CreateOrganizationMemberInvite :one
INSERT INTO organization_member_invites (
  public_id, organization_id, inviter_user_id,
  invitee_email, invitee_user_id, role,
  status, token_hash, expires_at
)
VALUES (
  sqlc.arg(public_id), sqlc.arg(organization_id), sqlc.arg(inviter_user_id),
  sqlc.arg(invitee_email), sqlc.narg(invitee_user_id), sqlc.arg(role),
  'pending', sqlc.arg(token_hash), sqlc.arg(expires_at)
)
RETURNING internal_id, public_id, organization_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id;

-- name: GetOrganizationMemberInviteByPublicID :one
SELECT internal_id, public_id, organization_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM organization_member_invites
WHERE public_id = sqlc.arg(public_id)
  AND (sqlc.narg(organization_id)::BIGINT IS NULL OR organization_id = sqlc.narg(organization_id))
LIMIT 1;

-- name: GetOrganizationMemberInviteByPublicIDForUpdate :one
SELECT internal_id, public_id, organization_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM organization_member_invites
WHERE public_id = sqlc.arg(public_id)
  AND (sqlc.narg(organization_id)::BIGINT IS NULL OR organization_id = sqlc.narg(organization_id))
FOR UPDATE;

-- name: GetOrganizationMemberInviteByTokenHash :one
SELECT internal_id, public_id, organization_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM organization_member_invites
WHERE token_hash = sqlc.arg(token_hash)
  AND (sqlc.narg(organization_id)::BIGINT IS NULL OR organization_id = sqlc.narg(organization_id))
LIMIT 1;

-- name: GetPendingOrganizationMemberInviteByEmail :one
SELECT internal_id, public_id, organization_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM organization_member_invites
WHERE organization_id = sqlc.arg(organization_id)
  AND invitee_email = sqlc.arg(invitee_email)
  AND status = 'pending'
LIMIT 1;

-- name: CountPendingOrganizationMemberInvites :one
SELECT COUNT(*)
FROM organization_member_invites
WHERE organization_id = sqlc.arg(organization_id)
  AND status = 'pending'
  AND expires_at > now();

-- name: ListOrganizationMemberInvitesByOrganizationID :many
SELECT internal_id, public_id, organization_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM organization_member_invites
WHERE organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::TEXT IS NULL OR status = sqlc.narg(status))
ORDER BY created_at DESC;

-- name: ListPendingOrganizationMemberInvitesByEmail :many
SELECT internal_id, public_id, organization_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM organization_member_invites
WHERE invitee_email = sqlc.arg(invitee_email)
  AND status = 'pending'
  AND expires_at > now()
ORDER BY created_at DESC;

-- name: ListPendingOrganizationMemberInvitesByInviteeUserID :many
SELECT internal_id, public_id, organization_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM organization_member_invites
WHERE invitee_user_id = sqlc.arg(invitee_user_id)
  AND status = 'pending'
  AND expires_at > now()
ORDER BY created_at DESC;

-- name: UpdateOrganizationMemberInviteStatus :one
UPDATE organization_member_invites
SET status = sqlc.arg(status),
    responded_at = now(),
    responded_by_user_id = sqlc.narg(responded_by_user_id),
    updated_at = now()
WHERE public_id = sqlc.arg(public_id)
  AND organization_id = sqlc.arg(organization_id)
  AND status = 'pending'
RETURNING internal_id, public_id, organization_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id;

-- name: CancelOrganizationMemberInvite :one
UPDATE organization_member_invites
SET status = 'cancelled',
    responded_at = now(),
    responded_by_user_id = sqlc.narg(caller_user_id),
    updated_at = now()
WHERE public_id = sqlc.arg(public_id)
  AND organization_id = sqlc.arg(organization_id)
  AND status = 'pending'
RETURNING internal_id, public_id, organization_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id;
