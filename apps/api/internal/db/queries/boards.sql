-- name: ListAccessibleBoardsByUserID :many
SELECT b.id, b.board_id, b.user_id, b.name, b.state, b.created_at, b.updated_at, m.role AS member_role
FROM user_boards AS b
LEFT JOIN board_members AS m ON m.board_id = b.board_id AND m.user_id = sqlc.arg(user_id)
WHERE b.user_id = sqlc.arg(user_id) OR m.user_id = sqlc.arg(user_id)
ORDER BY b.updated_at DESC;

-- name: GetBoardByBoardID :one
SELECT id, board_id, user_id, name, state, created_at, updated_at
FROM user_boards
WHERE board_id = ?
LIMIT 1;

-- name: UpsertBoard :exec
INSERT INTO user_boards (board_id, user_id, name, state, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  name = VALUES(name),
  state = VALUES(state),
  updated_at = VALUES(updated_at);

-- name: CountBoardNameConflicts :one
SELECT COUNT(*)
FROM user_boards
WHERE user_id = ?
  AND LOWER(name) = LOWER(sqlc.arg(name))
  AND (CAST(sqlc.arg(has_board_id) AS SIGNED) = 0 OR board_id <> sqlc.arg(board_id));

-- name: GetBoardMemberRole :one
SELECT role
FROM board_members
WHERE board_id = ? AND user_id = ?;

-- name: DeleteBoardByBoardID :exec
DELETE FROM user_boards
WHERE board_id = ?;

-- name: DeleteBoardMembersByBoardID :exec
DELETE FROM board_members
WHERE board_id = ?;

-- name: DeleteBoardShareTokensByBoardID :exec
DELETE FROM board_share_tokens
WHERE board_id = ?;

-- name: ListBoardMembers :many
SELECT m.user_id, u.storage_uid AS member_uid, u.username, m.role
FROM board_members AS m
INNER JOIN users AS u ON u.id = m.user_id
WHERE m.board_id = ?
ORDER BY m.created_at ASC;

-- name: UpsertBoardMember :exec
INSERT INTO board_members (board_id, user_id, role)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE
  role = VALUES(role);

-- name: DeleteBoardMember :exec
DELETE FROM board_members
WHERE board_id = ? AND user_id = ?;

-- name: GetBoardMemberByUserID :one
SELECT id, board_id, user_id, role, created_at, updated_at
FROM board_members
WHERE board_id = ? AND user_id = ?;

-- name: CreateBoardShareToken :execlastid
INSERT INTO board_share_tokens (board_id, token, role, created_by, revoked, expires_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListBoardShareTokens :many
SELECT id, board_id, token, role, created_by, revoked, expires_at, created_at, updated_at
FROM board_share_tokens
WHERE board_id = ?
ORDER BY created_at DESC;

-- name: GetBoardShareTokenByToken :one
SELECT id, board_id, token, role, created_by, revoked, expires_at, created_at, updated_at
FROM board_share_tokens
WHERE token = ?;

-- name: RevokeBoardShareToken :exec
UPDATE board_share_tokens
SET revoked = 1
WHERE board_id = ? AND token = ?;

-- name: CountBoardShareTokenByToken :one
SELECT COUNT(*)
FROM board_share_tokens
WHERE token = ?;
