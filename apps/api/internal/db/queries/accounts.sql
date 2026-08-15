-- name: CreateAccount :one
INSERT INTO api_accounts (
  name, provider_endpoint_id,
  max_concurrent, timeout, extra_config, status, fail_count, weight
) VALUES (
  sqlc.arg(name), sqlc.arg(provider_endpoint_id),
  sqlc.arg(max_concurrent), sqlc.arg(timeout), sqlc.arg(extra_config),
  sqlc.arg(status), sqlc.arg(fail_count), sqlc.arg(weight)
)
RETURNING id;

-- name: GetAccountByID :one
SELECT sqlc.embed(a),
       pe.api_type,
       ''::text AS api_key,
       EXISTS (
         SELECT 1 FROM api_account_secrets AS active_secret
         WHERE active_secret.account_id = a.id AND active_secret.status = 'active'
       ) AS secret_configured,
       pe.base_url AS proxy_url
FROM api_accounts AS a
INNER JOIN provider_endpoints AS pe ON pe.id = a.provider_endpoint_id
WHERE a.id = sqlc.arg(id)
  AND a.deleted_at IS NULL;

-- name: GetRuntimeAccountByIDWithSecret :one
SELECT sqlc.embed(a),
       pe.api_type,
       secret.secret_payload::text AS api_key,
       secret.secret_material_kind AS api_key_material_kind,
       TRUE::boolean AS secret_configured,
       pe.base_url AS proxy_url
FROM api_accounts AS a
INNER JOIN provider_endpoints AS pe ON pe.id = a.provider_endpoint_id
INNER JOIN LATERAL (
  SELECT secret_payload, secret_material_kind
  FROM api_account_secrets
  WHERE account_id = a.id
    AND status = 'active'
    AND secret_material_kind = 'encrypted'
    AND btrim(secret_payload) <> ''
  ORDER BY rotated_at DESC, id DESC
  LIMIT 1
) AS secret ON TRUE
WHERE a.id = sqlc.arg(id)
  AND a.status = 1
  AND pe.status = 1
  AND a.deleted_at IS NULL;

-- name: GetRuntimeAccountForAssignment :one
SELECT sqlc.embed(a),
       pe.api_type,
       secret.secret_payload::text AS api_key,
       secret.secret_material_kind AS api_key_material_kind,
       TRUE::boolean AS secret_configured,
       pe.base_url AS proxy_url
FROM api_accounts AS a
INNER JOIN provider_endpoints AS pe ON pe.id = a.provider_endpoint_id
INNER JOIN api_account_secrets AS secret ON secret.id = sqlc.arg(credential_id)
  AND secret.account_id = a.id
  AND secret.status = 'active'
  AND secret.secret_material_kind = 'encrypted'
  AND btrim(secret.secret_payload) <> ''
WHERE a.id = sqlc.arg(account_id)
  AND a.provider_endpoint_id = sqlc.arg(provider_endpoint_id)
  AND a.status = 1
  AND pe.status = 1
  AND a.deleted_at IS NULL;

-- name: CountAccounts :one
SELECT COUNT(*)
FROM api_accounts AS a
INNER JOIN provider_endpoints AS pe ON pe.id = a.provider_endpoint_id
WHERE a.deleted_at IS NULL
  AND (NOT sqlc.arg(has_keyword)::boolean OR a.name ILIKE sqlc.arg(keyword_like) OR pe.name ILIKE sqlc.arg(keyword_like) OR pe.api_type ILIKE sqlc.arg(keyword_like) OR pe.base_url ILIKE sqlc.arg(keyword_like))
  AND (NOT sqlc.arg(has_api_type)::boolean OR pe.api_type = sqlc.arg(api_type));

-- name: ListAccounts :many
SELECT sqlc.embed(a),
       pe.api_type,
       ''::text AS api_key,
       EXISTS (
         SELECT 1 FROM api_account_secrets AS active_secret
         WHERE active_secret.account_id = a.id AND active_secret.status = 'active'
       ) AS secret_configured,
       pe.base_url AS proxy_url
FROM api_accounts AS a
INNER JOIN provider_endpoints AS pe ON pe.id = a.provider_endpoint_id
WHERE a.deleted_at IS NULL
  AND (NOT sqlc.arg(has_keyword)::boolean OR a.name ILIKE sqlc.arg(keyword_like) OR pe.name ILIKE sqlc.arg(keyword_like) OR pe.api_type ILIKE sqlc.arg(keyword_like) OR pe.base_url ILIKE sqlc.arg(keyword_like))
  AND (NOT sqlc.arg(has_api_type)::boolean OR pe.api_type = sqlc.arg(api_type))
ORDER BY a.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListEnabledAccounts :many
SELECT sqlc.embed(a),
       pe.api_type,
       ''::text AS api_key,
       TRUE::boolean AS secret_configured,
       pe.base_url AS proxy_url
FROM api_accounts AS a
INNER JOIN provider_endpoints AS pe ON pe.id = a.provider_endpoint_id
WHERE a.status = 1
  AND pe.status = 1
  AND a.deleted_at IS NULL
  AND EXISTS (
    SELECT 1
	    FROM api_account_secrets AS active_secret
	    WHERE active_secret.account_id = a.id
	      AND active_secret.status = 'active'
	      AND active_secret.secret_material_kind = 'encrypted'
	      AND btrim(active_secret.secret_payload) <> ''
	  )
ORDER BY a.id ASC;

-- name: ListAccountIDs :many
SELECT id
FROM api_accounts
WHERE deleted_at IS NULL
ORDER BY id ASC;

-- name: DeleteAccountModelsByAccountID :exec
DELETE FROM api_account_models
WHERE account_id = sqlc.arg(account_id)
  AND EXISTS (
    SELECT 1 FROM api_accounts
    WHERE id = sqlc.arg(account_id)
      AND deleted_at IS NULL
  );

-- name: ListAccountModelsByAccountIDs :many
SELECT m.id, m.account_id, m.model_name, m.enabled, m.weight, m.max_concurrent, m.metadata, m.sort_order, m.created_at, m.updated_at,
       COALESCE(pem.supports_2k, FALSE) AS supports_2k,
       COALESCE(pem.supports_4k, FALSE) AS supports_4k
FROM api_account_models AS m
JOIN api_accounts AS a ON a.id = m.account_id
LEFT JOIN provider_endpoint_models AS pem
  ON pem.provider_endpoint_id = a.provider_endpoint_id
 AND lower(pem.model_name) = lower(m.model_name)
WHERE m.account_id = ANY(sqlc.arg(account_ids)::bigint[])
  AND EXISTS (
    SELECT 1
    FROM api_accounts AS a
    WHERE a.id = m.account_id
      AND a.deleted_at IS NULL
  )
ORDER BY m.account_id ASC, m.sort_order ASC, m.id ASC;

-- name: UpsertProviderEndpointModel :exec
INSERT INTO provider_endpoint_models (
  provider_endpoint_id, model_name, supports_2k, supports_4k
) VALUES (
  sqlc.arg(provider_endpoint_id), sqlc.arg(model_name), sqlc.arg(supports_2k), sqlc.arg(supports_4k)
)
ON CONFLICT (provider_endpoint_id, lower(model_name)) DO UPDATE
SET supports_2k = EXCLUDED.supports_2k,
    supports_4k = EXCLUDED.supports_4k;

-- name: UpsertAccountModel :one
INSERT INTO api_account_models (
  account_id, model_name, enabled, weight, max_concurrent, metadata, sort_order
) SELECT
  sqlc.arg(account_id), sqlc.arg(model_name), sqlc.arg(enabled), sqlc.arg(weight),
  sqlc.narg(max_concurrent), sqlc.arg(metadata)::jsonb,
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
    secret_payload = '',
    retired_at = COALESCE(retired_at, now())
WHERE account_id = sqlc.arg(account_id)
  AND status = 'active';

-- name: CreateAccountSecret :one
INSERT INTO api_account_secrets (
  account_id, secret_material_kind, secret_payload, key_version, fingerprint, status, rotated_at, expires_at
) SELECT
  sqlc.arg(account_id), sqlc.arg(secret_material_kind), sqlc.arg(secret_payload),
  sqlc.arg(key_version), sqlc.arg(fingerprint), sqlc.arg(status), now(), sqlc.narg(expires_at)
WHERE EXISTS (
  SELECT 1 FROM api_accounts
  WHERE id = sqlc.arg(account_id)
    AND deleted_at IS NULL
)
RETURNING id;

-- name: GetActiveAccountSecretByAccountID :one
SELECT id, account_id, secret_material_kind, secret_payload, key_version, fingerprint,
       status, rotated_at, expires_at, retired_at, created_at, updated_at
FROM api_account_secrets
WHERE account_id = sqlc.arg(account_id)
  AND status = 'active'
ORDER BY rotated_at DESC, id DESC
LIMIT 1;

-- name: GetRuntimeCredentialByID :one
SELECT s.id, s.account_id, s.secret_material_kind, s.secret_payload, s.key_version, s.fingerprint,
       s.status, s.rotated_at, s.expires_at, s.retired_at, s.created_at, s.updated_at
FROM api_account_secrets AS s
JOIN api_accounts AS a ON a.id = s.account_id
  AND a.status = 1
  AND a.deleted_at IS NULL
JOIN provider_endpoints AS pe ON pe.id = a.provider_endpoint_id
  AND pe.status = 1
WHERE s.id = sqlc.arg(id)
  AND s.account_id = sqlc.arg(account_id)
  AND s.status = 'active'
  AND s.secret_material_kind = 'encrypted'
  AND btrim(s.secret_payload) <> '';

-- name: UpdateAccount :execrows
UPDATE api_accounts
SET name = sqlc.arg(name),
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
