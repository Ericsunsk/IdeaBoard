-- name: UpsertMediaAssetSource :one
INSERT INTO media_asset_sources (
  media_asset_id, source_asset_id, owner_user_id, source_role, source_index, metadata
)
VALUES (
  sqlc.arg(media_asset_id), sqlc.arg(source_asset_id), sqlc.arg(owner_user_id),
  sqlc.arg(source_role), sqlc.arg(source_index), sqlc.arg(metadata)
)
ON CONFLICT (media_asset_id, source_asset_id, source_role, source_index) DO UPDATE
SET metadata = EXCLUDED.metadata
WHERE media_asset_sources.owner_user_id = EXCLUDED.owner_user_id
RETURNING id;

-- name: ListMediaAssetSources :many
SELECT id, media_asset_id, source_asset_id, owner_user_id, source_role, source_index, metadata, created_at
FROM media_asset_sources
WHERE media_asset_id = sqlc.arg(media_asset_id)
  AND owner_user_id = sqlc.arg(owner_user_id)
ORDER BY source_index ASC, id ASC;

-- name: UpsertMediaDerivative :one
INSERT INTO media_derivatives (
  derivative_id, media_asset_id, owner_user_id, tier, format, status, storage_profile_id, source_object_generation
)
SELECT
  sqlc.arg(derivative_id), sqlc.arg(media_asset_id), sqlc.arg(owner_user_id), sqlc.arg(tier), sqlc.arg(format),
  'pending', a.storage_profile_id, a.object_generation
FROM media_assets AS a
WHERE a.id = sqlc.arg(media_asset_id)
  AND a.owner_user_id = sqlc.arg(owner_user_id)
  AND a.deleted_at IS NULL
ON CONFLICT (media_asset_id, tier) DO UPDATE
SET status = CASE
      WHEN media_derivatives.status IN ('obsolete', 'failed')
        OR media_derivatives.source_object_generation <> EXCLUDED.source_object_generation THEN 'pending'
      ELSE media_derivatives.status
    END,
    storage_profile_id = EXCLUDED.storage_profile_id,
    source_object_generation = EXCLUDED.source_object_generation,
    attempt_count = CASE WHEN media_derivatives.status IN ('obsolete', 'failed')
      OR media_derivatives.source_object_generation <> EXCLUDED.source_object_generation THEN 0 ELSE media_derivatives.attempt_count END,
    error_code = CASE WHEN media_derivatives.status IN ('obsolete', 'failed')
      OR media_derivatives.source_object_generation <> EXCLUDED.source_object_generation THEN '' ELSE media_derivatives.error_code END,
    error_message = CASE WHEN media_derivatives.status IN ('obsolete', 'failed')
      OR media_derivatives.source_object_generation <> EXCLUDED.source_object_generation THEN '' ELSE media_derivatives.error_message END
WHERE media_derivatives.owner_user_id = EXCLUDED.owner_user_id
RETURNING id, derivative_id, media_asset_id, owner_user_id, tier, format, status, storage_profile_id,
  storage_key, mime_type, width, height, size_bytes, checksum_sha256, source_object_generation, object_generation, attempt_count,
  error_code, error_message, created_at, updated_at, ready_at, verified_at, last_attempt_at;

-- name: GetMediaDerivativeByAssetTier :one
SELECT id, derivative_id, media_asset_id, owner_user_id, tier, format, status, storage_profile_id,
  storage_key, mime_type, width, height, size_bytes, checksum_sha256, source_object_generation, object_generation, attempt_count,
  error_code, error_message, created_at, updated_at, ready_at, verified_at, last_attempt_at
FROM media_derivatives
WHERE media_asset_id = sqlc.arg(media_asset_id)
  AND owner_user_id = sqlc.arg(owner_user_id)
  AND tier = sqlc.arg(tier);

-- name: GetReadyMediaDerivativeStorageLocationByAssetID :one
SELECT d.storage_profile_id, d.storage_key
FROM media_derivatives AS d
JOIN media_assets AS a ON a.id = d.media_asset_id AND a.owner_user_id = d.owner_user_id
WHERE d.media_asset_id = sqlc.arg(media_asset_id)
  AND d.tier = sqlc.arg(tier)
  AND d.status = 'ready'
  AND d.source_object_generation = a.object_generation
  AND a.status = 'ready'
  AND a.deleted_at IS NULL;

-- name: ListMediaDerivativesByAsset :many
SELECT media_derivatives.id, media_derivatives.derivative_id, media_derivatives.media_asset_id, media_derivatives.owner_user_id,
  media_derivatives.tier, media_derivatives.format, media_derivatives.status, media_derivatives.storage_profile_id,
  media_derivatives.storage_key, media_derivatives.mime_type, media_derivatives.width, media_derivatives.height,
  media_derivatives.size_bytes, media_derivatives.checksum_sha256, media_derivatives.source_object_generation,
  media_derivatives.object_generation, media_derivatives.attempt_count, media_derivatives.error_code, media_derivatives.error_message,
  media_derivatives.created_at, media_derivatives.updated_at, media_derivatives.ready_at, media_derivatives.verified_at,
  media_derivatives.last_attempt_at
FROM media_derivatives
JOIN media_assets AS a ON a.id = media_derivatives.media_asset_id AND a.owner_user_id = media_derivatives.owner_user_id
WHERE media_derivatives.media_asset_id = sqlc.arg(media_asset_id)
  AND media_derivatives.owner_user_id = sqlc.arg(owner_user_id)
  AND (media_derivatives.status <> 'ready' OR media_derivatives.source_object_generation = a.object_generation)
ORDER BY CASE tier WHEN 'thumb' THEN 1 WHEN 'preview' THEN 2 WHEN 'large-preview' THEN 3 WHEN 'processing-input' THEN 4 ELSE 5 END,
  format ASC;

-- name: MarkMediaDerivativeReady :one
UPDATE media_derivatives
SET status = 'ready', format = sqlc.arg(format), storage_key = sqlc.arg(storage_key), mime_type = sqlc.arg(mime_type),
    width = sqlc.arg(width), height = sqlc.arg(height), size_bytes = sqlc.arg(size_bytes),
    checksum_sha256 = sqlc.arg(checksum_sha256), source_object_generation = sqlc.arg(source_object_generation), object_generation = object_generation + 1,
    ready_at = now(), verified_at = now(), error_code = '', error_message = ''
WHERE media_derivatives.id = sqlc.arg(id)
  AND EXISTS (
    SELECT 1 FROM media_assets AS a
    WHERE a.id = media_derivatives.media_asset_id
      AND a.owner_user_id = media_derivatives.owner_user_id
      AND a.status = 'ready'
      AND a.deleted_at IS NULL
      AND a.object_generation = sqlc.arg(source_object_generation)
  )
RETURNING id, derivative_id, media_asset_id, owner_user_id, tier, format, status, storage_profile_id,
  storage_key, mime_type, width, height, size_bytes, checksum_sha256, source_object_generation, object_generation, attempt_count,
  error_code, error_message, created_at, updated_at, ready_at, verified_at, last_attempt_at;

-- name: DeleteMediaDerivativesByAsset :exec
DELETE FROM media_derivatives
WHERE media_asset_id = sqlc.arg(media_asset_id)
  AND owner_user_id = sqlc.arg(owner_user_id);

-- name: GetProcessingInputManifestObject :one
SELECT a.asset_id, d.id AS media_derivative_id, d.derivative_id, d.tier,
  d.storage_profile_id, d.storage_key, d.object_generation,
  d.checksum_sha256, d.mime_type, d.size_bytes
FROM media_derivatives AS d
JOIN media_assets AS a ON a.id = d.media_asset_id AND a.owner_user_id = d.owner_user_id
WHERE d.media_asset_id = sqlc.arg(media_asset_id)
  AND d.owner_user_id = sqlc.arg(owner_user_id)
  AND d.tier = 'processing-input'
  AND d.status = 'ready'
  AND d.source_object_generation = a.object_generation
  AND a.status = 'ready'
  AND a.deleted_at IS NULL;
