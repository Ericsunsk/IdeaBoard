-- name: CreateAccount :execlastid
INSERT INTO api_accounts (
  name, channel_id, platform, api_type, model_set, api_key, proxy_url,
  max_concurrent, timeout, extra_config, status, fail_count, weight
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetAccountByID :one
SELECT id, name, channel_id, platform, api_type, model_set, api_key, proxy_url,
       max_concurrent, timeout, extra_config, status, fail_count, weight,
       created_at, updated_at
FROM api_accounts
WHERE id = ?;

-- name: CountAccounts :one
SELECT COUNT(*)
FROM api_accounts
WHERE (CAST(sqlc.arg(has_keyword) AS SIGNED) = 0 OR name LIKE sqlc.arg(keyword_like) OR platform LIKE sqlc.arg(keyword_like) OR api_type LIKE sqlc.arg(keyword_like))
  AND (CAST(sqlc.arg(has_platform) AS SIGNED) = 0 OR platform = sqlc.arg(platform));

-- name: ListAccounts :many
SELECT id, name, channel_id, platform, api_type, model_set, api_key, proxy_url,
       max_concurrent, timeout, extra_config, status, fail_count, weight,
       created_at, updated_at
FROM api_accounts
WHERE (CAST(sqlc.arg(has_keyword) AS SIGNED) = 0 OR name LIKE sqlc.arg(keyword_like) OR platform LIKE sqlc.arg(keyword_like) OR api_type LIKE sqlc.arg(keyword_like))
  AND (CAST(sqlc.arg(has_platform) AS SIGNED) = 0 OR platform = sqlc.arg(platform))
ORDER BY id DESC
LIMIT ? OFFSET ?;

-- name: ListEnabledAccounts :many
SELECT id, name, channel_id, platform, api_type, model_set, api_key, proxy_url,
       max_concurrent, timeout, extra_config, status, fail_count, weight,
       created_at, updated_at
FROM api_accounts
WHERE status = 1
ORDER BY id ASC;

-- name: ListAccountIDs :many
SELECT id
FROM api_accounts
ORDER BY id ASC;

-- name: ListAccountModelSetsByIDs :many
SELECT id, model_set
FROM api_accounts
WHERE id IN (sqlc.slice('ids'))
ORDER BY id ASC;

-- name: GetAccountModelSetByID :one
SELECT model_set
FROM api_accounts
WHERE id = ?;

-- name: UpdateAccount :exec
UPDATE api_accounts
SET name = ?,
    channel_id = ?,
    platform = ?,
    api_type = ?,
    model_set = ?,
    api_key = ?,
    proxy_url = ?,
    max_concurrent = ?,
    timeout = ?,
    extra_config = ?,
    status = ?,
    fail_count = ?,
    weight = ?
WHERE id = ?;

-- name: DeleteAccount :exec
DELETE FROM api_accounts
WHERE id = ?;

-- name: UpdateAccountStatus :exec
UPDATE api_accounts
SET status = ?
WHERE id = ?;

-- name: IncrementAccountFailCount :exec
UPDATE api_accounts
SET fail_count = fail_count + 1
WHERE id = ?;

-- name: ResetAccountFailCount :exec
UPDATE api_accounts
SET fail_count = 0
WHERE id = ?;
