-- name: GetSystemConfig :one
SELECT id, config_key, value, created_at, updated_at
FROM system_configs
WHERE config_key = sqlc.arg(config_key);

-- name: ListSystemConfigsByPrefix :many
SELECT id, config_key, value, created_at, updated_at
FROM system_configs
WHERE config_key LIKE sqlc.arg(config_key_like)
ORDER BY config_key ASC;

-- name: UpsertSystemConfig :exec
INSERT INTO system_configs (config_key, value)
VALUES (sqlc.arg(config_key), sqlc.arg(value))
ON CONFLICT (config_key) DO UPDATE
SET value = EXCLUDED.value;
