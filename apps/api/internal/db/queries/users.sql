-- name: CreateUser :execlastid
INSERT INTO users (
  username, auth_user_id, storage_uid, avatar, role, status
) VALUES (?, ?, ?, ?, ?, ?);

-- name: GetUserByID :one
SELECT id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE id = ?;

-- name: GetUserByUsername :one
SELECT id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE username = ?;

-- name: GetUserByAuthUserID :one
SELECT id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE auth_user_id = ?;

-- name: GetUserByStorageUID :one
SELECT id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE storage_uid = ?;

-- name: UpdateUser :exec
UPDATE users
SET username = ?,
    auth_user_id = ?,
    storage_uid = ?,
    avatar = ?,
    role = ?,
    status = ?
WHERE id = ?;

-- name: DeleteUserByID :exec
DELETE FROM users
WHERE id = ?;

-- name: CountUsers :one
SELECT COUNT(*)
FROM users
WHERE CAST(sqlc.arg(has_keyword) AS SIGNED) = 0 OR username LIKE sqlc.arg(keyword_like);

-- name: ListUsers :many
SELECT id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE CAST(sqlc.arg(has_keyword) AS SIGNED) = 0 OR username LIKE sqlc.arg(keyword_like)
ORDER BY id DESC
LIMIT ? OFFSET ?;

-- name: UpdateUserStatus :exec
UPDATE users
SET status = ?
WHERE id = ?;

-- name: UpdateUserRole :exec
UPDATE users
SET role = ?
WHERE id = ?;

-- name: UpdateUserAvatar :exec
UPDATE users
SET avatar = ?
WHERE id = ?;

-- name: UpdateUserStorageUID :exec
UPDATE users
SET storage_uid = ?
WHERE id = ?;

-- name: ListUserStorageUIDs :many
SELECT id, storage_uid
FROM users
ORDER BY id ASC;

-- name: GetUserIDByStorageUIDForUpdate :one
SELECT id
FROM users
WHERE storage_uid = ? AND id <> ?
LIMIT 1
FOR UPDATE;

-- name: GetUsedUIDForUpdate :one
SELECT storage_uid
FROM users_used_uids
WHERE storage_uid = ?
LIMIT 1
FOR UPDATE;

-- name: InsertUsedUIDIgnore :execrows
INSERT IGNORE INTO users_used_uids (storage_uid)
VALUES (?);

-- name: ListOwnedBoardIDsByUserID :many
SELECT board_id
FROM user_boards
WHERE user_id = ?;

-- name: DeleteBoardShareTokensByBoardIDs :exec
DELETE FROM board_share_tokens
WHERE board_id IN (sqlc.slice('board_ids'));

-- name: DeleteBoardMembersByBoardIDs :exec
DELETE FROM board_members
WHERE board_id IN (sqlc.slice('board_ids'));

-- name: DeleteUserBoardsByBoardIDs :exec
DELETE FROM user_boards
WHERE board_id IN (sqlc.slice('board_ids'));

-- name: DeleteBoardShareTokensByCreatedBy :exec
DELETE FROM board_share_tokens
WHERE created_by = ?;

-- name: DeleteBoardMembersByUserID :exec
DELETE FROM board_members
WHERE user_id = ?;
