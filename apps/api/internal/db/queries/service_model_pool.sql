-- Service Model Pool catalog queries (#848, ADR-0012).
--
-- The pool repository (internal/generation/catalog) drives every write
-- inside one transaction: per-service advisory lock, version
-- compare-and-swap, then full entry/resolution replacement. No query
-- here bumps the version on its own; the repository computes the next
-- version (1 on first write, current+1 on effective change) and upserts
-- it with the replacement. Reads never take the lock.
-- Snapshot helpers persist the immutable RoutingSnapshot JSONB on the
-- task row; wave 2 submit calls them inside the submit transaction.

-- name: LockServiceModelPool :exec
SELECT pg_advisory_xact_lock(
  hashtextextended('service_model_pool:' || sqlc.arg(service)::text, 0)
);

-- name: GetServiceModelPoolVersion :one
SELECT version
FROM service_model_pool_version
WHERE service = sqlc.arg(service);

-- name: UpsertServiceModelPoolVersion :one
INSERT INTO service_model_pool_version (service, version)
VALUES (sqlc.arg(service), sqlc.arg(version))
ON CONFLICT (service) DO UPDATE
SET version = EXCLUDED.version,
    updated_at = now()
RETURNING version;

-- name: ListServiceModelPoolEntries :many
SELECT id, service, provider, model_id, enabled, created_at, updated_at
FROM service_model_pool_entry
WHERE service = sqlc.arg(service)
ORDER BY provider ASC, model_id ASC, id ASC;

-- name: ListServiceModelPoolResolutions :many
SELECT r.pool_entry_id, r.resolution
FROM service_model_pool_resolution AS r
INNER JOIN service_model_pool_entry AS e ON e.id = r.pool_entry_id
WHERE e.service = sqlc.arg(service)
ORDER BY r.pool_entry_id ASC, r.resolution ASC;

-- name: InsertServiceModelPoolEntry :one
INSERT INTO service_model_pool_entry (service, provider, model_id, enabled)
VALUES (sqlc.arg(service), sqlc.arg(provider), sqlc.arg(model_id), sqlc.arg(enabled))
RETURNING id, service, provider, model_id, enabled, created_at, updated_at;

-- name: InsertServiceModelPoolResolution :exec
INSERT INTO service_model_pool_resolution (pool_entry_id, resolution)
VALUES (sqlc.arg(pool_entry_id), sqlc.arg(resolution))
ON CONFLICT (pool_entry_id, resolution) DO NOTHING;

-- name: DeleteServiceModelPoolResolutions :exec
DELETE FROM service_model_pool_resolution AS r
USING service_model_pool_entry AS e
WHERE r.pool_entry_id = e.id
AND e.service = sqlc.arg(service);

-- name: DeleteServiceModelPoolEntries :exec
DELETE FROM service_model_pool_entry
WHERE service = sqlc.arg(service);

-- name: SetTaskRoutingSnapshot :execrows
UPDATE tasks
SET routing_snapshot = sqlc.arg(routing_snapshot)::jsonb,
    updated_at = now()
WHERE public_id = sqlc.arg(public_id);

-- name: GetTaskRoutingSnapshot :one
SELECT routing_snapshot
FROM tasks
WHERE public_id = sqlc.arg(public_id);
