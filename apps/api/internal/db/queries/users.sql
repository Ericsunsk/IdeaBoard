-- name: CreateUser :one
INSERT INTO users (
  display_name, auth_user_id, storage_uid, avatar, role, status
) VALUES (
  sqlc.arg(display_name), sqlc.arg(auth_user_id), sqlc.arg(storage_uid),
  sqlc.arg(avatar), sqlc.arg(role), sqlc.arg(status)
)
RETURNING id, public_id, display_name;

-- name: GetUserByID :one
SELECT id, public_id, display_name, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE id = sqlc.arg(id);

-- name: GetUsersByIDs :many
SELECT id, public_id, display_name, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE id = ANY(sqlc.arg(ids)::BIGINT[])
ORDER BY id;

-- name: GetUserByPublicID :one
SELECT id, public_id, display_name, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE public_id = sqlc.arg(public_id);

-- name: GetUserByAuthUserID :one
SELECT id, public_id, display_name, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE auth_user_id = sqlc.arg(auth_user_id);

-- name: GetUserByStorageUID :one
SELECT id, public_id, display_name, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE storage_uid = sqlc.arg(storage_uid);

-- name: UpdateUser :execrows
UPDATE users
SET display_name = sqlc.arg(display_name),
    auth_user_id = sqlc.arg(auth_user_id),
    storage_uid = sqlc.arg(storage_uid),
    avatar = sqlc.arg(avatar),
    role = sqlc.arg(role),
    status = sqlc.arg(status)
WHERE id = sqlc.arg(id);

-- name: DeleteUserByID :execrows
DELETE FROM users
WHERE id = sqlc.arg(id);

-- name: SummarizeUsers :one
SELECT
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE status = 1) AS active_count,
  COUNT(*) FILTER (WHERE status = 0) AS banned_count,
  COUNT(*) FILTER (WHERE role = 'admin') AS admin_count
FROM users
WHERE (NOT sqlc.arg(has_keyword)::boolean OR display_name ILIKE sqlc.arg(keyword_like))
  AND (NOT sqlc.arg(has_role)::boolean OR role = sqlc.arg(role))
  AND (NOT sqlc.arg(has_status)::boolean OR status = sqlc.arg(status));

-- name: ListUsers :many
SELECT id, public_id, display_name, auth_user_id, storage_uid, avatar, role, status, created_at, updated_at
FROM users
WHERE (NOT sqlc.arg(has_keyword)::boolean OR display_name ILIKE sqlc.arg(keyword_like))
  AND (NOT sqlc.arg(has_role)::boolean OR role = sqlc.arg(role))
  AND (NOT sqlc.arg(has_status)::boolean OR status = sqlc.arg(status))
ORDER BY id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpdateUserStatus :execrows
UPDATE users
SET status = sqlc.arg(status)
WHERE id = sqlc.arg(id);

-- name: UpdateUserRole :execrows
UPDATE users
SET role = sqlc.arg(role)
WHERE id = sqlc.arg(id);

-- name: UpdateUserAvatar :execrows
UPDATE users
SET avatar = sqlc.arg(avatar)
WHERE id = sqlc.arg(id);

-- name: UpdateUserStorageUID :execrows
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

-- name: DeleteBoardMembersByUserID :exec
DELETE FROM board_members
WHERE user_id = sqlc.arg(user_id);

-- name: DeleteUserBoardsByUserID :exec
DELETE FROM boards
WHERE owner_user_id = sqlc.arg(user_id);
