-- name: ListAccessibleBoardsByUserID :many
SELECT
  b.id, b.board_id, b.user_id, b.name, b.state,
  b.revision, b.state_schema_version, b.last_saved_by,
  b.created_at, b.updated_at,
  m.role AS member_role
FROM user_boards AS b
LEFT JOIN board_members AS m ON m.owner_user_id = b.user_id AND m.board_id = b.board_id AND m.user_id = sqlc.arg(user_id)
WHERE b.user_id = sqlc.arg(user_id) OR m.user_id = sqlc.arg(user_id)
ORDER BY b.updated_at DESC;

-- name: GetBoardByOwnerAndBoardID :one
SELECT id, board_id, user_id, name, state, revision, state_schema_version, last_saved_by, created_at, updated_at
FROM user_boards
WHERE user_id = sqlc.arg(owner_user_id) AND board_id = sqlc.arg(board_id)
LIMIT 1;

-- name: GetAccessibleBoardByBoardID :one
SELECT
  b.id, b.board_id, b.user_id, b.name, b.state,
  b.revision, b.state_schema_version, b.last_saved_by,
  b.created_at, b.updated_at,
  m.role AS member_role
FROM user_boards AS b
LEFT JOIN board_members AS m ON m.owner_user_id = b.user_id AND m.board_id = b.board_id AND m.user_id = sqlc.arg(user_id)
WHERE b.board_id = sqlc.arg(board_id)
  AND (b.user_id = sqlc.arg(user_id) OR m.user_id = sqlc.arg(user_id))
ORDER BY CASE WHEN m.user_id IS NOT NULL THEN 0 ELSE 1 END, b.updated_at DESC
LIMIT 1;

-- name: UpsertBoard :exec
INSERT INTO user_boards (
  board_id, user_id, name, state,
  revision, state_schema_version, last_saved_by,
  created_at, updated_at
)
VALUES (
  sqlc.arg(board_id), sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(state),
  COALESCE(sqlc.narg(revision)::bigint, 1),
  COALESCE(sqlc.narg(state_schema_version)::integer, 1),
  sqlc.narg(last_saved_by),
  sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (user_id, board_id) DO UPDATE
SET name = EXCLUDED.name,
    state = EXCLUDED.state,
    revision = user_boards.revision + 1,
    state_schema_version = COALESCE(sqlc.narg(state_schema_version)::integer, user_boards.state_schema_version),
    last_saved_by = COALESCE(sqlc.narg(last_saved_by)::bigint, user_boards.last_saved_by),
    updated_at = EXCLUDED.updated_at;

-- name: CountBoardNameConflicts :one
SELECT COUNT(*)
FROM user_boards
WHERE user_id = sqlc.arg(user_id)
  AND LOWER(name) = LOWER(sqlc.arg(name))
  AND (NOT sqlc.arg(has_board_id)::boolean OR board_id <> sqlc.arg(board_id));

-- name: GetBoardMemberRole :one
SELECT role
FROM board_members
WHERE owner_user_id = sqlc.arg(owner_user_id) AND board_id = sqlc.arg(board_id) AND user_id = sqlc.arg(user_id);

-- name: DeleteBoardByOwnerAndBoardID :execrows
DELETE FROM user_boards
WHERE user_id = sqlc.arg(owner_user_id) AND board_id = sqlc.arg(board_id);

-- name: DeleteBoardMembersByOwnerAndBoardID :exec
DELETE FROM board_members
WHERE owner_user_id = sqlc.arg(owner_user_id) AND board_id = sqlc.arg(board_id);

-- name: DeleteBoardShareTokensByOwnerAndBoardID :exec
DELETE FROM board_share_tokens
WHERE created_by = sqlc.arg(owner_user_id) AND board_id = sqlc.arg(board_id);

-- name: ListBoardMembers :many
SELECT m.user_id, u.storage_uid AS member_uid, u.username, m.role
FROM board_members AS m
INNER JOIN users AS u ON u.id = m.user_id
WHERE m.owner_user_id = sqlc.arg(owner_user_id) AND m.board_id = sqlc.arg(board_id)
ORDER BY m.created_at ASC;

-- name: UpsertBoardMember :exec
INSERT INTO board_members (owner_user_id, board_id, user_id, role)
VALUES (sqlc.arg(owner_user_id), sqlc.arg(board_id), sqlc.arg(user_id), sqlc.arg(role))
ON CONFLICT (owner_user_id, board_id, user_id) DO UPDATE
SET role = EXCLUDED.role;

-- name: DeleteBoardMember :execrows
DELETE FROM board_members
WHERE owner_user_id = sqlc.arg(owner_user_id) AND board_id = sqlc.arg(board_id) AND user_id = sqlc.arg(user_id);

-- name: GetBoardMemberByUserID :one
SELECT id, owner_user_id, board_id, user_id, role, created_at, updated_at
FROM board_members
WHERE owner_user_id = sqlc.arg(owner_user_id) AND board_id = sqlc.arg(board_id) AND user_id = sqlc.arg(user_id);

-- name: CreateBoardShareToken :one
INSERT INTO board_share_tokens (
  board_id, token_hash, token_prefix, role, created_by,
  revoked, expires_at, max_uses
)
VALUES (
  sqlc.arg(board_id), sqlc.arg(token_hash), sqlc.arg(token_prefix), sqlc.arg(role), sqlc.arg(created_by),
  sqlc.arg(revoked), sqlc.arg(expires_at), sqlc.narg(max_uses)
)
RETURNING id;

-- name: ListBoardShareTokens :many
SELECT
  id, board_id, token_hash, token_prefix, role, created_by, revoked,
  usage_count, max_uses, last_used_at, revoked_at, revoked_by,
  expires_at, created_at, updated_at
FROM board_share_tokens
WHERE created_by = sqlc.arg(owner_user_id) AND board_id = sqlc.arg(board_id)
ORDER BY created_at DESC;

-- name: GetBoardShareTokenByHash :one
SELECT
  id, board_id, token_hash, token_prefix, role, created_by, revoked,
  usage_count, max_uses, last_used_at, revoked_at, revoked_by,
  expires_at, created_at, updated_at
FROM board_share_tokens
WHERE token_hash = sqlc.arg(token_hash);

-- name: RevokeBoardShareTokenByID :execrows
UPDATE board_share_tokens
SET revoked = TRUE,
    revoked_at = now(),
    revoked_by = sqlc.narg(revoked_by)
WHERE created_by = sqlc.arg(owner_user_id) AND board_id = sqlc.arg(board_id) AND id = sqlc.arg(token_id);

-- name: ConsumeBoardShareToken :one
UPDATE board_share_tokens
SET usage_count = usage_count + 1,
    last_used_at = now()
WHERE token_hash = sqlc.arg(token_hash)
  AND NOT revoked
  AND (expires_at IS NULL OR expires_at > now())
  AND (max_uses IS NULL OR usage_count < max_uses)
RETURNING
  id, board_id, token_hash, token_prefix, role, created_by, revoked,
  usage_count, max_uses, last_used_at, revoked_at, revoked_by,
  expires_at, created_at, updated_at;

-- name: CountBoardShareTokenByHash :one
SELECT COUNT(*)
FROM board_share_tokens
WHERE token_hash = sqlc.arg(token_hash);
