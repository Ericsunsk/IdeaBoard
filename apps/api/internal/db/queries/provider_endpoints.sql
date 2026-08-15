-- name: CreateProviderEndpoint :one
INSERT INTO provider_endpoints (api_type, name, base_url, status)
VALUES (sqlc.arg(api_type), sqlc.arg(name), sqlc.arg(base_url), sqlc.arg(status))
RETURNING id;

-- name: GetProviderEndpointByID :one
SELECT id, api_type, name, base_url, status, created_at, updated_at
FROM provider_endpoints
WHERE id = sqlc.arg(id);

-- name: ListProviderEndpoints :many
SELECT id, api_type, name, base_url, status, created_at, updated_at
FROM provider_endpoints
ORDER BY id ASC;

-- name: UpdateProviderEndpoint :execrows
UPDATE provider_endpoints
SET api_type = sqlc.arg(api_type),
    name = sqlc.arg(name),
    base_url = sqlc.arg(base_url),
    status = sqlc.arg(status)
WHERE id = sqlc.arg(id);

-- name: DeleteProviderEndpoint :execrows
DELETE FROM provider_endpoints
WHERE id = sqlc.arg(id);
