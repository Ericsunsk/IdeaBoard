-- Legacy media derivative queries (#809 schema repair).
--
-- The generated media_derivatives.sql.go output survived Phase 3b without its
-- query source, which breaks the schema-release sqlc drift check. This file
-- restores the source for the queries that still validate against the
-- workspace schema: they touch only the retained legacy shells
-- (media_derivatives, media_asset_sources) and never the replaced
-- user-scoped media_assets columns. Queries that JOINed the old
-- media_assets(id, owner_user_id, status, object_generation, deleted_at)
-- shape (GetProcessingInputManifestObject,
-- GetReadyMediaDerivativeStorageLocationByAssetID, ListMediaDerivativesByAsset,
-- MarkMediaDerivativeReady, UpsertMediaDerivative) cannot validate against
-- the workspace model and stay removed; their Go call sites fail closed.

-- name: DeleteMediaDerivativesByAsset :exec
DELETE FROM media_derivatives
WHERE media_asset_id = $1
  AND owner_user_id = $2;

-- name: GetMediaDerivativeByAssetTier :one
SELECT id, derivative_id, media_asset_id, owner_user_id, tier, format, status, storage_profile_id,
  storage_key, mime_type, width, height, size_bytes, checksum_sha256, source_object_generation, object_generation, attempt_count,
  error_code, error_message, created_at, updated_at, ready_at, verified_at, last_attempt_at
FROM media_derivatives
WHERE media_asset_id = $1
  AND owner_user_id = $2
  AND tier = $3;

-- name: ListMediaAssetSources :many
SELECT id, media_asset_id, source_asset_id, owner_user_id, source_role, source_index, metadata, created_at
FROM media_asset_sources
WHERE media_asset_id = $1
  AND owner_user_id = $2
ORDER BY source_index ASC, id ASC;

-- name: UpsertMediaAssetSource :one
INSERT INTO media_asset_sources (
  media_asset_id, source_asset_id, owner_user_id, source_role, source_index, metadata
)
VALUES (
  $1, $2, $3,
  $4, $5, $6
)
ON CONFLICT (media_asset_id, source_asset_id, source_role, source_index) DO UPDATE
SET metadata = EXCLUDED.metadata
WHERE media_asset_sources.owner_user_id = EXCLUDED.owner_user_id
RETURNING id;
