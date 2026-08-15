-- name: CreateProviderEndpoint :one
INSERT INTO provider_endpoints (api_type, protocol, name, base_url, status)
VALUES (sqlc.arg(api_type), sqlc.arg(protocol), sqlc.arg(name), sqlc.arg(base_url), sqlc.arg(status))
RETURNING id;

-- name: GetProviderEndpointByID :one
SELECT id, api_type, protocol, name, base_url, status, created_at, updated_at
FROM provider_endpoints
WHERE id = sqlc.arg(id);

-- name: ListProviderEndpoints :many
SELECT id, api_type, protocol, name, base_url, status, created_at, updated_at
FROM provider_endpoints
ORDER BY id ASC;

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

-- name: DeleteProviderEndpoint :execrows
DELETE FROM provider_endpoints
WHERE id = sqlc.arg(id);
