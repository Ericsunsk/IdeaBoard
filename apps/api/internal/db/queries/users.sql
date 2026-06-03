-- name: CreateUser :one
INSERT INTO users (
  username, auth_user_id, storage_uid, avatar, role, status
) VALUES (
  sqlc.arg(username), sqlc.arg(auth_user_id), sqlc.arg(storage_uid),
  sqlc.arg(avatar), sqlc.arg(role), sqlc.arg(status)
)
RETURNING id;

-- name: GetUserByID :one
SELECT id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE id = sqlc.arg(id);

-- name: GetUserByUsername :one
SELECT id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE username = sqlc.arg(username);

-- name: GetUserByAuthUserID :one
SELECT id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE auth_user_id = sqlc.arg(auth_user_id);

-- name: GetUserByStorageUID :one
SELECT id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE storage_uid = sqlc.arg(storage_uid);

-- name: UpdateUser :exec
UPDATE users
SET username = sqlc.arg(username),
    auth_user_id = sqlc.arg(auth_user_id),
    storage_uid = sqlc.arg(storage_uid),
    avatar = sqlc.arg(avatar),
    role = sqlc.arg(role),
    status = sqlc.arg(status)
WHERE id = sqlc.arg(id);

-- name: DeleteUserByID :exec
DELETE FROM users
WHERE id = sqlc.arg(id);

-- name: CountUsers :one
SELECT COUNT(*)
FROM users
WHERE NOT sqlc.arg(has_keyword)::boolean OR username ILIKE sqlc.arg(keyword_like);

-- name: ListUsers :many
SELECT id, username, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE NOT sqlc.arg(has_keyword)::boolean OR username ILIKE sqlc.arg(keyword_like)
ORDER BY id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpdateUserStatus :exec
UPDATE users
SET status = sqlc.arg(status)
WHERE id = sqlc.arg(id);

-- name: UpdateUserRole :exec
UPDATE users
SET role = sqlc.arg(role)
WHERE id = sqlc.arg(id);

-- name: UpdateUserAvatar :exec
UPDATE users
SET avatar = sqlc.arg(avatar)
WHERE id = sqlc.arg(id);

-- name: UpdateUserStorageUID :exec
UPDATE users
SET storage_uid = sqlc.arg(storage_uid)
WHERE id = sqlc.arg(id);

-- name: ListUserStorageUIDs :many
SELECT id, storage_uid
FROM users
ORDER BY id ASC;

-- name: GetUserIDByStorageUIDForUpdate :one
SELECT id
FROM users
WHERE storage_uid = sqlc.arg(storage_uid) AND id <> sqlc.arg(id)
LIMIT 1
FOR UPDATE;

-- name: GetUsedUIDForUpdate :one
SELECT storage_uid
FROM users_used_uids
WHERE storage_uid = sqlc.arg(storage_uid)
LIMIT 1
FOR UPDATE;

-- name: InsertUsedUIDIgnore :execrows
INSERT INTO users_used_uids (storage_uid)
VALUES (sqlc.arg(storage_uid))
ON CONFLICT (storage_uid) DO NOTHING;

-- name: DeleteBoardShareTokensByCreatedBy :exec
DELETE FROM board_share_tokens
WHERE created_by = sqlc.arg(created_by);

-- name: DeleteBoardMembersByOwnerUserID :exec
DELETE FROM board_members
WHERE owner_user_id = sqlc.arg(owner_user_id);

-- name: DeleteBoardMembersByUserID :exec
DELETE FROM board_members
WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserBoardsByUserID :exec
DELETE FROM user_boards
WHERE user_id = sqlc.arg(user_id);
