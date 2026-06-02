-- name: CreateChannel :one
INSERT INTO channels (type, name, api_url, status)
VALUES (sqlc.arg(type), sqlc.arg(name), sqlc.arg(api_url), sqlc.arg(status))
RETURNING id;

-- name: GetChannelByID :one
SELECT id, type, name, api_url, status, created_at, updated_at
FROM channels
WHERE id = sqlc.arg(id);

-- name: ListChannels :many
SELECT id, type, name, api_url, status, created_at, updated_at
FROM channels
ORDER BY id ASC;

-- name: UpdateChannel :exec
UPDATE channels
SET type = sqlc.arg(type),
    name = sqlc.arg(name),
    api_url = sqlc.arg(api_url),
    status = sqlc.arg(status)
WHERE id = sqlc.arg(id);

-- name: DeleteChannel :exec
DELETE FROM channels
WHERE id = sqlc.arg(id);
