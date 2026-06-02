-- name: CreateAccount :one
INSERT INTO api_accounts (
  name, channel_id, platform, api_type, model_set, api_key, proxy_url,
  max_concurrent, timeout, extra_config, status, fail_count, weight
) VALUES (
  sqlc.arg(name), sqlc.arg(channel_id), sqlc.arg(platform), sqlc.arg(api_type),
  sqlc.arg(model_set), sqlc.arg(api_key), sqlc.arg(proxy_url),
  sqlc.arg(max_concurrent), sqlc.arg(timeout), sqlc.arg(extra_config),
  sqlc.arg(status), sqlc.arg(fail_count), sqlc.arg(weight)
)
RETURNING id;

-- name: GetAccountByID :one
SELECT id, name, channel_id, platform, api_type, model_set, api_key, proxy_url,
       max_concurrent, timeout, extra_config, status, fail_count, weight,
       created_at, updated_at
FROM api_accounts
WHERE id = sqlc.arg(id);

-- name: CountAccounts :one
SELECT COUNT(*)
FROM api_accounts
WHERE (NOT sqlc.arg(has_keyword)::boolean OR name ILIKE sqlc.arg(keyword_like) OR platform ILIKE sqlc.arg(keyword_like) OR api_type ILIKE sqlc.arg(keyword_like))
  AND (NOT sqlc.arg(has_platform)::boolean OR platform = sqlc.arg(platform));

-- name: ListAccounts :many
SELECT id, name, channel_id, platform, api_type, model_set, api_key, proxy_url,
       max_concurrent, timeout, extra_config, status, fail_count, weight,
       created_at, updated_at
FROM api_accounts
WHERE (NOT sqlc.arg(has_keyword)::boolean OR name ILIKE sqlc.arg(keyword_like) OR platform ILIKE sqlc.arg(keyword_like) OR api_type ILIKE sqlc.arg(keyword_like))
  AND (NOT sqlc.arg(has_platform)::boolean OR platform = sqlc.arg(platform))
ORDER BY id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

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
WHERE id = ANY(sqlc.arg(ids)::bigint[])
ORDER BY id ASC;

-- name: GetAccountModelSetByID :one
SELECT model_set
FROM api_accounts
WHERE id = sqlc.arg(id);

-- name: UpdateAccount :exec
UPDATE api_accounts
SET name = sqlc.arg(name),
    channel_id = sqlc.arg(channel_id),
    platform = sqlc.arg(platform),
    api_type = sqlc.arg(api_type),
    model_set = sqlc.arg(model_set),
    api_key = sqlc.arg(api_key),
    proxy_url = sqlc.arg(proxy_url),
    max_concurrent = sqlc.arg(max_concurrent),
    timeout = sqlc.arg(timeout),
    extra_config = sqlc.arg(extra_config),
    status = sqlc.arg(status),
    fail_count = sqlc.arg(fail_count),
    weight = sqlc.arg(weight)
WHERE id = sqlc.arg(id);

-- name: DeleteAccount :exec
DELETE FROM api_accounts
WHERE id = sqlc.arg(id);

-- name: UpdateAccountStatus :exec
UPDATE api_accounts
SET status = sqlc.arg(status)
WHERE id = sqlc.arg(id);

-- name: IncrementAccountFailCount :exec
UPDATE api_accounts
SET fail_count = fail_count + 1
WHERE id = sqlc.arg(id);

-- name: ResetAccountFailCount :exec
UPDATE api_accounts
SET fail_count = 0
WHERE id = sqlc.arg(id);
