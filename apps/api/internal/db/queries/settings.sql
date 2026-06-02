-- name: GetSystemConfig :one
SELECT id, `key`, value, created_at, updated_at
FROM system_configs
WHERE `key` = ?;

-- name: ListSystemConfigsByPrefix :many
SELECT id, `key`, value, created_at, updated_at
FROM system_configs
WHERE `key` LIKE ?
ORDER BY `key` ASC;

-- name: UpsertSystemConfig :exec
INSERT INTO system_configs (`key`, value)
VALUES (?, ?)
ON DUPLICATE KEY UPDATE value = VALUES(value);
