-- name: CreateAccount :one
INSERT INTO api_accounts (
  name, channel_id, platform, api_type, proxy_url,
  max_concurrent, timeout, extra_config, status, fail_count, weight
) VALUES (
  sqlc.arg(name), sqlc.arg(channel_id), sqlc.arg(platform), sqlc.arg(api_type),
  sqlc.arg(proxy_url),
  sqlc.arg(max_concurrent), sqlc.arg(timeout), sqlc.arg(extra_config),
  sqlc.arg(status), sqlc.arg(fail_count), sqlc.arg(weight)
)
RETURNING id;

-- name: GetAccountByID :one
SELECT a.id, a.name, a.channel_id, a.platform, a.api_type,
       ''::text AS api_key,
       EXISTS (
         SELECT 1 FROM api_account_secrets AS active_secret
         WHERE active_secret.account_id = a.id AND active_secret.status = 'active'
       ) AS secret_configured,
       a.proxy_url, a.max_concurrent, a.timeout, a.extra_config, a.status,
       a.fail_count, a.weight, a.created_at, a.updated_at
FROM api_accounts AS a
WHERE a.id = sqlc.arg(id)
  AND a.deleted_at IS NULL;

-- name: GetRuntimeAccountByIDWithSecret :one
SELECT a.id, a.name, a.channel_id, a.platform, a.api_type,
       secret.secret_payload::text AS api_key,
       TRUE::boolean AS secret_configured,
       a.proxy_url, a.max_concurrent, a.timeout, a.extra_config, a.status,
       a.fail_count, a.weight, a.created_at, a.updated_at
FROM api_accounts AS a
INNER JOIN LATERAL (
  SELECT secret_payload
  FROM api_account_secrets
  WHERE account_id = a.id
    AND status = 'active'
    AND btrim(secret_payload) <> ''
  ORDER BY rotated_at DESC, id DESC
  LIMIT 1
) AS secret ON TRUE
WHERE a.id = sqlc.arg(id)
  AND a.status = 1
  AND a.deleted_at IS NULL;

-- name: CountAccounts :one
SELECT COUNT(*)
FROM api_accounts
WHERE deleted_at IS NULL
  AND (NOT sqlc.arg(has_keyword)::boolean OR name ILIKE sqlc.arg(keyword_like) OR platform ILIKE sqlc.arg(keyword_like) OR api_type ILIKE sqlc.arg(keyword_like))
  AND (NOT sqlc.arg(has_platform)::boolean OR platform = sqlc.arg(platform));

-- name: ListAccounts :many
SELECT a.id, a.name, a.channel_id, a.platform, a.api_type,
       ''::text AS api_key,
       EXISTS (
         SELECT 1 FROM api_account_secrets AS active_secret
         WHERE active_secret.account_id = a.id AND active_secret.status = 'active'
       ) AS secret_configured,
       a.proxy_url, a.max_concurrent, a.timeout, a.extra_config, a.status,
       a.fail_count, a.weight, a.created_at, a.updated_at
FROM api_accounts AS a
WHERE a.deleted_at IS NULL
  AND (NOT sqlc.arg(has_keyword)::boolean OR a.name ILIKE sqlc.arg(keyword_like) OR a.platform ILIKE sqlc.arg(keyword_like) OR a.api_type ILIKE sqlc.arg(keyword_like))
  AND (NOT sqlc.arg(has_platform)::boolean OR a.platform = sqlc.arg(platform))
ORDER BY a.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListEnabledAccounts :many
SELECT a.id, a.name, a.channel_id, a.platform, a.api_type,
       ''::text AS api_key,
       TRUE::boolean AS secret_configured,
       a.proxy_url, a.max_concurrent, a.timeout, a.extra_config, a.status,
       a.fail_count, a.weight, a.created_at, a.updated_at
FROM api_accounts AS a
WHERE a.status = 1
  AND a.deleted_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM api_account_secrets AS active_secret
    WHERE active_secret.account_id = a.id
      AND active_secret.status = 'active'
      AND btrim(active_secret.secret_payload) <> ''
  )
ORDER BY a.id ASC;

-- name: ListAccountIDs :many
SELECT id
FROM api_accounts
WHERE deleted_at IS NULL
ORDER BY id ASC;

-- name: ListSingleEnabledModelsByAccountIDs :many
SELECT
  m.account_id AS id,
  (array_agg(m.model_name ORDER BY m.sort_order, m.id))[1]::text AS model_name
FROM api_account_models AS m
INNER JOIN api_accounts AS a ON a.id = m.account_id AND a.deleted_at IS NULL
WHERE m.account_id = ANY(sqlc.arg(ids)::bigint[])
  AND m.enabled = TRUE
GROUP BY m.account_id
HAVING COUNT(*) = 1
ORDER BY m.account_id ASC;

-- name: GetSingleEnabledModelByAccountID :one
SELECT (array_agg(m.model_name ORDER BY m.sort_order, m.id))[1]::text AS model_name
FROM api_account_models AS m
INNER JOIN api_accounts AS a ON a.id = m.account_id AND a.deleted_at IS NULL
WHERE m.account_id = sqlc.arg(id)
  AND m.enabled = TRUE
HAVING COUNT(*) = 1;

-- name: DeleteAccountModelsByAccountID :exec
DELETE FROM api_account_models
WHERE account_id = sqlc.arg(account_id)
  AND EXISTS (
    SELECT 1 FROM api_accounts
    WHERE id = sqlc.arg(account_id)
      AND deleted_at IS NULL
  );

-- name: ListAccountModelsByAccountID :many
SELECT id, account_id, model_name, enabled, weight, max_concurrent, feature_slugs, metadata, sort_order, created_at, updated_at
FROM api_account_models AS m
WHERE m.account_id = sqlc.arg(account_id)
  AND EXISTS (
    SELECT 1
    FROM api_accounts AS a
    WHERE a.id = m.account_id
      AND a.deleted_at IS NULL
  )
ORDER BY m.sort_order ASC, m.id ASC;

-- name: ListAccountModelsByAccountIDs :many
SELECT id, account_id, model_name, enabled, weight, max_concurrent, feature_slugs, metadata, sort_order, created_at, updated_at
FROM api_account_models AS m
WHERE m.account_id = ANY(sqlc.arg(account_ids)::bigint[])
  AND EXISTS (
    SELECT 1
    FROM api_accounts AS a
    WHERE a.id = m.account_id
      AND a.deleted_at IS NULL
  )
ORDER BY m.account_id ASC, m.sort_order ASC, m.id ASC;

-- name: UpsertAccountModel :one
INSERT INTO api_account_models (
  account_id, model_name, enabled, weight, max_concurrent, feature_slugs, metadata, sort_order
) SELECT
  sqlc.arg(account_id), sqlc.arg(model_name), sqlc.arg(enabled), sqlc.arg(weight),
  sqlc.narg(max_concurrent), sqlc.arg(feature_slugs)::jsonb, sqlc.arg(metadata)::jsonb,
  sqlc.arg(sort_order)
WHERE EXISTS (
  SELECT 1 FROM api_accounts
  WHERE id = sqlc.arg(account_id)
    AND deleted_at IS NULL
)
ON CONFLICT (account_id, lower(model_name)) DO UPDATE
SET enabled = EXCLUDED.enabled,
    weight = EXCLUDED.weight,
    max_concurrent = EXCLUDED.max_concurrent,
    feature_slugs = EXCLUDED.feature_slugs,
    metadata = EXCLUDED.metadata,
    sort_order = EXCLUDED.sort_order
RETURNING id;

-- name: UpdateAccountModelEnabled :execrows
UPDATE api_account_models
SET enabled = sqlc.arg(enabled)
WHERE account_id = sqlc.arg(account_id)
  AND lower(model_name) = lower(sqlc.arg(model_name))
  AND EXISTS (
    SELECT 1 FROM api_accounts
    WHERE id = sqlc.arg(account_id)
      AND deleted_at IS NULL
  );

-- name: DeleteAccountModel :execrows
DELETE FROM api_account_models
WHERE account_id = sqlc.arg(account_id)
  AND lower(model_name) = lower(sqlc.arg(model_name))
  AND EXISTS (
    SELECT 1 FROM api_accounts
    WHERE id = sqlc.arg(account_id)
      AND deleted_at IS NULL
  );

-- name: RetireActiveAccountSecrets :exec
UPDATE api_account_secrets
SET status = 'retired',
    retired_at = now()
WHERE account_id = sqlc.arg(account_id)
  AND status = 'active'
  AND EXISTS (
    SELECT 1 FROM api_accounts
    WHERE id = sqlc.arg(account_id)
      AND deleted_at IS NULL
  );

-- name: RevokeAccountSecretsForArchive :exec
UPDATE api_account_secrets
SET status = 'revoked',
    secret_ref = '',
    secret_payload = '',
    retired_at = COALESCE(retired_at, now())
WHERE account_id = sqlc.arg(account_id)
  AND status = 'active';

-- name: CreateAccountSecret :one
INSERT INTO api_account_secrets (
  account_id, secret_material_kind, secret_ref, secret_payload, key_version, fingerprint, status, rotated_at, expires_at
) SELECT
  sqlc.arg(account_id), sqlc.arg(secret_material_kind), sqlc.arg(secret_ref), sqlc.arg(secret_payload),
  sqlc.arg(key_version), sqlc.arg(fingerprint), sqlc.arg(status), now(), sqlc.narg(expires_at)
WHERE EXISTS (
  SELECT 1 FROM api_accounts
  WHERE id = sqlc.arg(account_id)
    AND deleted_at IS NULL
)
RETURNING id;

-- name: GetActiveAccountSecretByAccountID :one
SELECT id, account_id, secret_material_kind, secret_ref, secret_payload, key_version, fingerprint,
       status, rotated_at, expires_at, retired_at, created_at, updated_at
FROM api_account_secrets
WHERE account_id = sqlc.arg(account_id)
  AND status = 'active'
ORDER BY rotated_at DESC, id DESC
LIMIT 1;

-- name: UpdateAccount :execrows
UPDATE api_accounts
SET name = sqlc.arg(name),
    platform = sqlc.arg(platform),
    api_type = sqlc.arg(api_type),
    proxy_url = sqlc.arg(proxy_url),
    max_concurrent = sqlc.arg(max_concurrent),
    timeout = sqlc.arg(timeout),
    extra_config = sqlc.arg(extra_config),
    fail_count = sqlc.arg(fail_count),
    weight = sqlc.arg(weight)
WHERE id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: ArchiveAccount :execrows
UPDATE api_accounts
SET status = 0,
    fail_count = 0,
    deleted_at = COALESCE(deleted_at, now())
WHERE id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: UpdateAccountStatus :execrows
UPDATE api_accounts
SET status = sqlc.arg(status)
WHERE id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: IncrementAccountFailCount :execrows
UPDATE api_accounts
SET fail_count = fail_count + 1
WHERE id = sqlc.arg(id)
  AND deleted_at IS NULL;

-- name: ResetAccountFailCount :execrows
UPDATE api_accounts
SET fail_count = 0
WHERE id = sqlc.arg(id)
  AND deleted_at IS NULL;
