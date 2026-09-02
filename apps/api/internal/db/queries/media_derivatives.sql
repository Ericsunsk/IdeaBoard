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

-- name: EnqueueMediaDerivativeOutbox :one
INSERT INTO media_derivative_outbox (derivative_id)
VALUES (sqlc.arg(derivative_id))
ON CONFLICT (derivative_id) DO UPDATE
SET available_at = LEAST(media_derivative_outbox.available_at, now()),
    claimed_at = NULL,
    last_error = ''
RETURNING id, derivative_id, available_at, claimed_at, attempt_count, last_error, created_at;

-- name: GetMediaDerivativeByAssetTier :one
SELECT id, derivative_id, media_asset_id, owner_user_id, tier, format, status, storage_profile_id,
  storage_key, mime_type, width, height, size_bytes, checksum_sha256, source_object_generation, object_generation, attempt_count,
  error_code, error_message, created_at, updated_at, ready_at, verified_at, last_attempt_at
FROM media_derivatives
WHERE media_asset_id = sqlc.arg(media_asset_id)
  AND owner_user_id = sqlc.arg(owner_user_id)
  AND tier = sqlc.arg(tier);

-- name: GetMediaDerivativeByID :one
SELECT id, derivative_id, media_asset_id, owner_user_id, tier, format, status, storage_profile_id,
  storage_key, mime_type, width, height, size_bytes, checksum_sha256, source_object_generation, object_generation, attempt_count,
  error_code, error_message, created_at, updated_at, ready_at, verified_at, last_attempt_at
FROM media_derivatives
WHERE id = sqlc.arg(id);

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
ORDER BY CASE tier WHEN 'thumb' THEN 1 WHEN 'preview' THEN 2 WHEN 'large-preview' THEN 3 WHEN 'processing' THEN 4 ELSE 5 END,
  format ASC;

-- name: MarkMediaDerivativeProcessing :one
UPDATE media_derivatives
SET status = 'processing',
    attempt_count = attempt_count + 1,
    last_attempt_at = now(),
    error_code = '',
    error_message = ''
WHERE media_derivatives.id = sqlc.arg(id)
  AND (
    status = 'pending'
    OR (status = 'failed' AND attempt_count < sqlc.arg(max_attempt_count))
  )
RETURNING id, derivative_id, media_asset_id, owner_user_id, tier, format, status, storage_profile_id,
  storage_key, mime_type, width, height, size_bytes, checksum_sha256, source_object_generation, object_generation, attempt_count,
  error_code, error_message, created_at, updated_at, ready_at, verified_at, last_attempt_at;

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

-- name: MarkMediaDerivativeFailed :one
UPDATE media_derivatives
SET status = 'failed',
    error_code = sqlc.arg(error_code),
    error_message = sqlc.arg(error_message),
    attempt_count = GREATEST(attempt_count, sqlc.arg(min_attempt_count))
WHERE id = sqlc.arg(id)
RETURNING id, derivative_id, media_asset_id, owner_user_id, tier, format, status, storage_profile_id,
  storage_key, mime_type, width, height, size_bytes, checksum_sha256, source_object_generation, object_generation, attempt_count,
  error_code, error_message, created_at, updated_at, ready_at, verified_at, last_attempt_at;

-- name: RescheduleMediaDerivativeOutbox :exec
UPDATE media_derivative_outbox
SET available_at = sqlc.arg(available_at),
    claimed_at = NULL,
    last_error = sqlc.arg(last_error)
WHERE derivative_id = sqlc.arg(derivative_id);

-- name: GetMediaDerivativeForDelivery :one
SELECT d.id, d.derivative_id, d.media_asset_id, d.owner_user_id, d.tier, d.format, d.status,
  d.storage_profile_id, d.storage_key, d.mime_type, d.width, d.height, d.size_bytes,
  d.checksum_sha256, d.source_object_generation, d.object_generation, d.attempt_count, d.error_code, d.error_message,
  d.created_at, d.updated_at, d.ready_at, d.verified_at, d.last_attempt_at
FROM media_derivatives AS d
JOIN media_assets AS a ON a.id = d.media_asset_id AND a.owner_user_id = d.owner_user_id
WHERE d.media_asset_id = sqlc.arg(media_asset_id)
  AND d.owner_user_id = sqlc.arg(owner_user_id)
  AND d.tier = sqlc.arg(tier)
  AND d.status = 'ready'
  AND d.source_object_generation = a.object_generation
  AND a.status = 'ready'
  AND a.deleted_at IS NULL;

-- name: ListPendingMediaDerivativeIDs :many
SELECT d.id
FROM media_derivatives AS d
JOIN media_derivative_outbox AS o ON o.derivative_id = d.id
WHERE o.claimed_at IS NULL
  AND o.available_at <= now()
  AND (
    d.status = 'pending'
    OR (d.status = 'failed' AND d.attempt_count < sqlc.arg(max_attempt_count))
  )
ORDER BY o.available_at ASC, d.id ASC
LIMIT sqlc.arg(limit_count);

-- name: ListStaleProcessingMediaDerivativeIDs :many
SELECT id
FROM media_derivatives
WHERE status = 'processing'
  AND updated_at < sqlc.arg(stale_before)
ORDER BY updated_at ASC, id ASC
LIMIT sqlc.arg(row_limit);

-- name: ResetStaleProcessingMediaDerivatives :execrows
UPDATE media_derivatives
SET status = CASE
      WHEN attempt_count >= sqlc.arg(max_attempt_count) THEN 'failed'
      ELSE 'pending'
    END,
    error_code = 'stale_processing_recovered',
    error_message = 'processing lease expired; reconciler requeued the derivative'
WHERE id = ANY(sqlc.arg(ids)::bigint[])
  AND status = 'processing'
  AND updated_at < sqlc.arg(stale_before);

-- name: ListMediaDerivativeStorageKeys :many
SELECT storage_profile_id, storage_key
FROM media_derivatives
WHERE btrim(storage_key) <> '';

-- name: ListReadyMediaAssetIDsMissingDerivatives :many
SELECT a.id, a.owner_user_id
FROM media_assets AS a
WHERE a.status = 'ready'
  AND a.deleted_at IS NULL
  AND a.media_kind = 'image'
  AND (
    NOT EXISTS (SELECT 1 FROM media_derivatives AS d WHERE d.media_asset_id = a.id AND d.tier = 'thumb')
    OR NOT EXISTS (SELECT 1 FROM media_derivatives AS d WHERE d.media_asset_id = a.id AND d.tier = 'preview')
    OR NOT EXISTS (SELECT 1 FROM media_derivatives AS d WHERE d.media_asset_id = a.id AND d.tier = 'processing-input')
  )
ORDER BY a.updated_at ASC, a.id ASC
LIMIT sqlc.arg(row_limit);

-- name: ListMediaDerivativeIDsMissingOutbox :many
SELECT d.id
FROM media_derivatives AS d
LEFT JOIN media_derivative_outbox AS o ON o.derivative_id = d.id
WHERE o.id IS NULL
  AND (
    d.status = 'pending'
    OR (d.status = 'failed' AND d.attempt_count < sqlc.arg(max_attempt_count))
  )
ORDER BY d.updated_at ASC, d.id ASC
LIMIT sqlc.arg(row_limit);

-- name: DeleteMediaDerivativeOutbox :exec
DELETE FROM media_derivative_outbox
WHERE derivative_id = sqlc.arg(derivative_id);

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

-- name: ListReadyDerivativesForReconcile :many
SELECT id, derivative_id, media_asset_id, owner_user_id, tier, format, status, storage_profile_id,
  storage_key, mime_type, width, height, size_bytes, checksum_sha256, source_object_generation, object_generation, attempt_count,
  error_code, error_message, created_at, updated_at, ready_at, verified_at, last_attempt_at
FROM media_derivatives
WHERE status = 'ready'
  AND btrim(storage_key) <> ''
  AND COALESCE(verified_at, ready_at, updated_at) < sqlc.arg(reconcile_before)
ORDER BY COALESCE(verified_at, ready_at, updated_at) ASC, id ASC
LIMIT sqlc.arg(row_limit);

-- name: MarkMediaDerivativeVerified :execrows
UPDATE media_derivatives
SET verified_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'ready'
  AND object_generation = sqlc.arg(object_generation);

-- name: MarkMediaDerivativeMissing :execrows
UPDATE media_derivatives
SET status = 'missing', error_code = 'object_missing', error_message = sqlc.arg(error_message), verified_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'ready'
  AND object_generation = sqlc.arg(object_generation);

-- name: MarkMediaDerivativeInvalid :execrows
UPDATE media_derivatives
SET status = 'invalid', error_code = 'object_invalid', error_message = sqlc.arg(error_message), verified_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'ready'
  AND object_generation = sqlc.arg(object_generation);

-- name: ListMediaDerivativeRepairCandidates :many
SELECT id, attempt_count
FROM media_derivatives
WHERE status IN ('missing', 'invalid')
ORDER BY updated_at ASC, id ASC
LIMIT sqlc.arg(row_limit);

-- name: RequeueMediaDerivativeRepair :execrows
UPDATE media_derivatives
SET status = 'pending', error_code = '', error_message = ''
WHERE id = sqlc.arg(id)
  AND status IN ('missing', 'invalid')
  AND attempt_count < sqlc.arg(max_attempt_count);

-- name: MarkMediaDerivativeRepairTerminal :execrows
UPDATE media_derivatives
SET status = 'failed', error_code = 'repair_not_possible', error_message = sqlc.arg(error_message)
WHERE id = sqlc.arg(id)
  AND status IN ('missing', 'invalid')
  AND attempt_count >= sqlc.arg(max_attempt_count);

-- name: CountMediaDerivativesByStatus :many
SELECT status, COUNT(*)::bigint AS count
FROM media_derivatives
GROUP BY status
ORDER BY status;
