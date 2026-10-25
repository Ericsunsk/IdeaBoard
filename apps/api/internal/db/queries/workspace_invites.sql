-- Workspace member invitation queries (#957).
--
-- The invite row is the audit fact: created/responded timestamps plus decider.
-- All writes join the caller-owned transaction via the passed Queries,
-- so acceptance stays atomic with the workspace_memberships insertion
-- and preserves the (active_members + pending_invites) <= 5 invariant.

-- name: CreateWorkspaceMemberInvite :one
INSERT INTO workspace_member_invites (
  public_id, workspace_id, inviter_user_id,
  invitee_email, invitee_user_id, role,
  status, token_hash, expires_at
)
VALUES (
  sqlc.arg(public_id), sqlc.arg(workspace_id), sqlc.arg(inviter_user_id),
  sqlc.arg(invitee_email), sqlc.narg(invitee_user_id), sqlc.arg(role),
  'pending', sqlc.arg(token_hash), sqlc.arg(expires_at)
)
RETURNING internal_id, public_id, workspace_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id;

-- name: GetWorkspaceMemberInviteByPublicID :one
SELECT internal_id, public_id, workspace_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM workspace_member_invites
WHERE public_id = sqlc.arg(public_id)
  AND (sqlc.narg(workspace_id)::BIGINT IS NULL OR workspace_id = sqlc.narg(workspace_id))
LIMIT 1;

-- name: GetWorkspaceMemberInviteByPublicIDForUpdate :one
SELECT internal_id, public_id, workspace_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM workspace_member_invites
WHERE public_id = sqlc.arg(public_id)
  AND (sqlc.narg(workspace_id)::BIGINT IS NULL OR workspace_id = sqlc.narg(workspace_id))
FOR UPDATE;

-- name: GetWorkspaceMemberInviteByTokenHash :one
SELECT internal_id, public_id, workspace_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM workspace_member_invites
WHERE token_hash = sqlc.arg(token_hash)
  AND (sqlc.narg(workspace_id)::BIGINT IS NULL OR workspace_id = sqlc.narg(workspace_id))
LIMIT 1;

-- name: GetPendingWorkspaceMemberInviteByEmail :one
SELECT internal_id, public_id, workspace_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM workspace_member_invites
WHERE workspace_id = sqlc.arg(workspace_id)
  AND invitee_email = sqlc.arg(invitee_email)
  AND status = 'pending'
LIMIT 1;

-- name: CountPendingWorkspaceMemberInvites :one
SELECT COUNT(*)
FROM workspace_member_invites
WHERE workspace_id = sqlc.arg(workspace_id)
  AND status = 'pending'
  AND expires_at > now();

-- name: ListWorkspaceMemberInvitesByWorkspaceID :many
SELECT internal_id, public_id, workspace_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM workspace_member_invites
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(status)::TEXT IS NULL OR status = sqlc.narg(status))
ORDER BY created_at DESC;

-- name: ListPendingWorkspaceMemberInvitesByEmail :many
SELECT internal_id, public_id, workspace_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM workspace_member_invites
WHERE invitee_email = sqlc.arg(invitee_email)
  AND status = 'pending'
  AND expires_at > now()
ORDER BY created_at DESC;

-- name: ListPendingWorkspaceMemberInvitesByInviteeUserID :many
SELECT internal_id, public_id, workspace_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id
FROM workspace_member_invites
WHERE invitee_user_id = sqlc.arg(invitee_user_id)
  AND status = 'pending'
  AND expires_at > now()
ORDER BY created_at DESC;

-- name: UpdateWorkspaceMemberInviteStatus :one
UPDATE workspace_member_invites
SET status = sqlc.arg(status),
    responded_at = now(),
    responded_by_user_id = sqlc.narg(responded_by_user_id),
    updated_at = now()
WHERE public_id = sqlc.arg(public_id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND status = 'pending'
RETURNING internal_id, public_id, workspace_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id;

-- name: CancelWorkspaceMemberInvite :one
UPDATE workspace_member_invites
SET status = 'cancelled',
    responded_at = now(),
    responded_by_user_id = sqlc.arg(cancelled_by_user_id),
    updated_at = now()
WHERE public_id = sqlc.arg(public_id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND status = 'pending'
RETURNING internal_id, public_id, workspace_id, inviter_user_id,
  invitee_email, invitee_user_id, role, status, token_hash,
  expires_at, created_at, updated_at, responded_at, responded_by_user_id;

-- name: ExpireWorkspaceMemberInvites :many
UPDATE workspace_member_invites
SET status = 'expired',
    updated_at = now()
WHERE status = 'pending'
  AND expires_at <= now()
RETURNING internal_id, public_id, workspace_id, invitee_email;
