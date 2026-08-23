-- name: CreateProviderEndpoint :one
INSERT INTO provider_endpoints (api_type, protocol, name, base_url, status)
VALUES (sqlc.arg(api_type), sqlc.arg(protocol), sqlc.arg(name), sqlc.arg(base_url), sqlc.arg(status))
RETURNING id;

-- name: GetProviderEndpointByID :one
SELECT id, api_type, protocol, name, base_url, status, created_at, updated_at,
       EXISTS (
         SELECT 1
         FROM api_accounts AS managed_account
         INNER JOIN api_account_secrets AS active_secret
           ON active_secret.account_id = managed_account.id
          AND active_secret.status = 'active'
          AND active_secret.secret_material_kind = 'encrypted'
          AND btrim(active_secret.secret_payload) <> ''
         WHERE managed_account.provider_endpoint_id = provider_endpoints.id
           AND managed_account.deleted_at IS NULL
           AND managed_account.extra_config = '{"managed_by":"image-processing-endpoint"}'
       ) AS api_key_configured
FROM provider_endpoints
WHERE provider_endpoints.id = sqlc.arg(id);

-- name: ListProviderEndpoints :many
SELECT id, api_type, protocol, name, base_url, status, created_at, updated_at,
       EXISTS (
         SELECT 1
         FROM api_accounts AS managed_account
         INNER JOIN api_account_secrets AS active_secret
           ON active_secret.account_id = managed_account.id
          AND active_secret.status = 'active'
          AND active_secret.secret_material_kind = 'encrypted'
          AND btrim(active_secret.secret_payload) <> ''
         WHERE managed_account.provider_endpoint_id = provider_endpoints.id
           AND managed_account.deleted_at IS NULL
           AND managed_account.extra_config = '{"managed_by":"image-processing-endpoint"}'
       ) AS api_key_configured
FROM provider_endpoints
ORDER BY id ASC;

-- name: GetManagedProviderEndpointAccountID :one
SELECT id
FROM api_accounts
WHERE provider_endpoint_id = sqlc.arg(provider_endpoint_id)
  AND deleted_at IS NULL
  AND extra_config = '{"managed_by":"image-processing-endpoint"}'
LIMIT 1;

-- name: UpdateManagedProviderEndpointAccount :exec
UPDATE api_accounts
SET name = sqlc.arg(name),
    status = sqlc.arg(status),
    max_concurrent = sqlc.arg(max_concurrent),
    timeout = sqlc.arg(timeout),
    extra_config = '{"managed_by":"image-processing-endpoint"}',
    fail_count = 0,
    weight = 1
WHERE id = sqlc.arg(id)
  AND provider_endpoint_id = sqlc.arg(provider_endpoint_id)
  AND deleted_at IS NULL;

-- name: DeleteManagedProviderEndpointAccounts :execrows
DELETE FROM api_accounts
WHERE provider_endpoint_id = sqlc.arg(provider_endpoint_id)
  AND extra_config = '{"managed_by":"image-processing-endpoint"}';

-- name: ListProviderEndpointCapabilities :many
SELECT provider_endpoint_id, capability
FROM provider_endpoint_capabilities
WHERE provider_endpoint_id = ANY(sqlc.arg(provider_endpoint_ids)::bigint[])
ORDER BY provider_endpoint_id ASC, capability ASC;

-- name: ReplaceProviderEndpointCapabilities :exec
DELETE FROM provider_endpoint_capabilities
WHERE provider_endpoint_id = sqlc.arg(provider_endpoint_id);

-- name: AddProviderEndpointCapability :exec
INSERT INTO provider_endpoint_capabilities (provider_endpoint_id, capability)
VALUES (sqlc.arg(provider_endpoint_id), sqlc.arg(capability));

-- name: UpdateProviderEndpoint :execrows
UPDATE provider_endpoints
SET api_type = sqlc.arg(api_type),
    protocol = sqlc.arg(protocol),
    name = sqlc.arg(name),
    base_url = sqlc.arg(base_url),
    status = sqlc.arg(status)
WHERE id = sqlc.arg(id);

-- name: UpdateProviderEndpointStatus :execrows
UPDATE provider_endpoints
SET status = sqlc.arg(status)
WHERE id = sqlc.arg(id);

-- name: UpdateManagedProviderEndpointAccountStatus :exec
UPDATE api_accounts
SET status = sqlc.arg(status)
WHERE provider_endpoint_id = sqlc.arg(provider_endpoint_id)
  AND extra_config = '{"managed_by":"image-processing-endpoint"}'
  AND deleted_at IS NULL;

-- name: DeleteProviderEndpoint :execrows
DELETE FROM provider_endpoints
WHERE id = sqlc.arg(id);
