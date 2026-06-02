-- name: CreateChannel :execlastid
INSERT INTO channels (type, name, api_url, status)
VALUES (?, ?, ?, ?);

-- name: GetChannelByID :one
SELECT id, type, name, api_url, status, created_at, updated_at
FROM channels
WHERE id = ?;

-- name: ListChannels :many
SELECT id, type, name, api_url, status, created_at, updated_at
FROM channels
ORDER BY id ASC;

-- name: UpdateChannel :exec
UPDATE channels
SET type = ?,
    name = ?,
    api_url = ?,
    status = ?
WHERE id = ?;

-- name: DeleteChannel :exec
DELETE FROM channels
WHERE id = ?;
