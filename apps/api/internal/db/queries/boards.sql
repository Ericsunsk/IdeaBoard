-- name: ListAccessibleBoardsByUserID :many
SELECT b.id, b.board_id, b.user_id, b.name, b.state, b.created_at, b.updated_at, m.role AS member_role
FROM user_boards AS b
LEFT JOIN board_members AS m ON m.board_id = b.board_id AND m.user_id = sqlc.arg(user_id)
WHERE b.user_id = sqlc.arg(user_id) OR m.user_id = sqlc.arg(user_id)
ORDER BY b.updated_at DESC;

-- name: GetBoardByBoardID :one
SELECT id, board_id, user_id, name, state, created_at, updated_at
FROM user_boards
WHERE board_id = sqlc.arg(board_id)
LIMIT 1;

-- name: UpsertBoard :exec
INSERT INTO user_boards (board_id, user_id, name, state, created_at, updated_at)
VALUES (
  sqlc.arg(board_id), sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(state),
  sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (board_id) DO UPDATE
SET name = EXCLUDED.name,
    state = EXCLUDED.state,
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
WHERE board_id = sqlc.arg(board_id) AND user_id = sqlc.arg(user_id);

-- name: DeleteBoardByBoardID :exec
DELETE FROM user_boards
WHERE board_id = sqlc.arg(board_id);

-- name: DeleteBoardMembersByBoardID :exec
DELETE FROM board_members
WHERE board_id = sqlc.arg(board_id);

-- name: DeleteBoardShareTokensByBoardID :exec
DELETE FROM board_share_tokens
WHERE board_id = sqlc.arg(board_id);

-- name: ListBoardMembers :many
SELECT m.user_id, u.storage_uid AS member_uid, u.username, m.role
FROM board_members AS m
INNER JOIN users AS u ON u.id = m.user_id
WHERE m.board_id = sqlc.arg(board_id)
ORDER BY m.created_at ASC;

-- name: UpsertBoardMember :exec
INSERT INTO board_members (board_id, user_id, role)
VALUES (sqlc.arg(board_id), sqlc.arg(user_id), sqlc.arg(role))
ON CONFLICT (board_id, user_id) DO UPDATE
SET role = EXCLUDED.role;

-- name: DeleteBoardMember :exec
DELETE FROM board_members
WHERE board_id = sqlc.arg(board_id) AND user_id = sqlc.arg(user_id);

-- name: GetBoardMemberByUserID :one
SELECT id, board_id, user_id, role, created_at, updated_at
FROM board_members
WHERE board_id = sqlc.arg(board_id) AND user_id = sqlc.arg(user_id);

-- name: CreateBoardShareToken :one
INSERT INTO board_share_tokens (board_id, token, role, created_by, revoked, expires_at)
VALUES (
  sqlc.arg(board_id), sqlc.arg(token), sqlc.arg(role), sqlc.arg(created_by),
  sqlc.arg(revoked), sqlc.arg(expires_at)
)
RETURNING id;

-- name: ListBoardShareTokens :many
SELECT id, board_id, token, role, created_by, revoked, expires_at, created_at, updated_at
FROM board_share_tokens
WHERE board_id = sqlc.arg(board_id)
ORDER BY created_at DESC;

-- name: GetBoardShareTokenByToken :one
SELECT id, board_id, token, role, created_by, revoked, expires_at, created_at, updated_at
FROM board_share_tokens
WHERE token = sqlc.arg(token);

-- name: RevokeBoardShareToken :exec
UPDATE board_share_tokens
SET revoked = TRUE
WHERE board_id = sqlc.arg(board_id) AND token = sqlc.arg(token);

-- name: CountBoardShareTokenByToken :one
SELECT COUNT(*)
FROM board_share_tokens
WHERE token = sqlc.arg(token);
