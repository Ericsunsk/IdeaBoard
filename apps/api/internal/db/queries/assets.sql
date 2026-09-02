-- name: UpsertMediaAsset :one
INSERT INTO media_assets (
  asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256,
  status, metadata
)
VALUES (
  sqlc.arg(asset_id), sqlc.arg(owner_user_id), sqlc.arg(storage_profile_id), sqlc.arg(storage_key),
  sqlc.arg(url), sqlc.arg(source_url), sqlc.arg(media_kind), sqlc.arg(mime_type),
  sqlc.arg(width), sqlc.arg(height), sqlc.arg(size_bytes), sqlc.arg(checksum_sha256),
  sqlc.arg(status), sqlc.arg(metadata)
)
ON CONFLICT (storage_profile_id, storage_key) DO UPDATE
SET url = EXCLUDED.url,
    source_url = EXCLUDED.source_url,
    media_kind = EXCLUDED.media_kind,
    mime_type = EXCLUDED.mime_type,
    width = EXCLUDED.width,
    height = EXCLUDED.height,
    size_bytes = EXCLUDED.size_bytes,
    checksum_sha256 = EXCLUDED.checksum_sha256,
    status = EXCLUDED.status,
    metadata = EXCLUDED.metadata,
    deleted_at = NULL,
    reclaim_after = NULL,
    reclaim_started_at = NULL,
    reclaim_claim_token = NULL
WHERE media_assets.owner_user_id = EXCLUDED.owner_user_id
RETURNING id;

-- name: CreateBoardUploadMediaAsset :one
INSERT INTO media_assets (
  asset_id, owner_user_id, storage_profile_id, storage_key,
  url, media_kind, mime_type, width, height, size_bytes,
  checksum_sha256, status, metadata, reclaim_after, reclaim_generation
)
VALUES (
  sqlc.arg(asset_id), sqlc.arg(owner_user_id), sqlc.arg(storage_profile_id), sqlc.arg(storage_key),
  sqlc.arg(url), 'image', sqlc.arg(mime_type), sqlc.arg(width), sqlc.arg(height), sqlc.arg(size_bytes), sqlc.arg(checksum_sha256),
  sqlc.arg(status), sqlc.arg(metadata), sqlc.arg(reclaim_after), 1
)
RETURNING id;

-- name: CreatePreparedMediaAsset :one
INSERT INTO media_assets (
  asset_id, owner_user_id, storage_profile_id, storage_key, url, source_url,
  media_kind, mime_type, width, height, size_bytes, checksum_sha256, status, metadata,
  reclaim_after, reclaim_generation
)
VALUES (
  sqlc.arg(asset_id), sqlc.arg(owner_user_id), sqlc.arg(storage_profile_id), sqlc.arg(storage_key),
  sqlc.arg(url), sqlc.arg(source_url), 'image', sqlc.arg(mime_type), sqlc.arg(width), sqlc.arg(height),
  sqlc.arg(size_bytes), sqlc.arg(checksum_sha256), sqlc.arg(status), sqlc.arg(metadata), sqlc.arg(reclaim_after), 1
)
ON CONFLICT (storage_profile_id, storage_key) DO UPDATE
SET url = EXCLUDED.url,
    source_url = EXCLUDED.source_url,
    mime_type = EXCLUDED.mime_type,
    width = EXCLUDED.width,
    height = EXCLUDED.height,
    size_bytes = EXCLUDED.size_bytes,
    checksum_sha256 = EXCLUDED.checksum_sha256,
    status = CASE WHEN media_assets.status = 'ready' THEN 'ready' ELSE EXCLUDED.status END,
    metadata = EXCLUDED.metadata,
    deleted_at = NULL,
    reclaim_after = NULL,
    reclaim_started_at = NULL,
    reclaim_claim_token = NULL
WHERE media_assets.owner_user_id = EXCLUDED.owner_user_id
RETURNING id;

-- name: BeginTaskResultMediaAsset :one
INSERT INTO media_assets (
  asset_id, owner_user_id, storage_profile_id, storage_key, url, source_url,
  media_kind, status, metadata, reclaim_after, reclaim_generation
)
VALUES (
  sqlc.arg(asset_id), sqlc.arg(owner_user_id), sqlc.arg(storage_profile_id), sqlc.arg(storage_key),
  sqlc.arg(url), sqlc.arg(source_url), 'image', 'uploading', sqlc.arg(metadata),
  sqlc.arg(reclaim_after), 1
)
ON CONFLICT (storage_profile_id, storage_key) DO UPDATE
SET source_url = EXCLUDED.source_url,
    status = CASE WHEN media_assets.status = 'ready' THEN 'ready' ELSE 'uploading' END,
    metadata = CASE WHEN media_assets.status = 'ready' THEN media_assets.metadata ELSE EXCLUDED.metadata END,
    updated_at = now(),
    deleted_at = NULL,
    reclaim_after = CASE WHEN media_assets.status = 'ready' THEN media_assets.reclaim_after ELSE EXCLUDED.reclaim_after END,
    reclaim_started_at = NULL,
    reclaim_claim_token = NULL
WHERE media_assets.owner_user_id = EXCLUDED.owner_user_id
RETURNING id;

-- name: MarkMediaAssetReady :one
UPDATE media_assets
SET status = 'ready', updated_at = now(),
    object_generation = CASE WHEN status = 'ready' THEN object_generation + 1 ELSE object_generation END,
    storage_key = COALESCE(NULLIF(sqlc.arg(storage_key)::text, ''), storage_key),
    url = sqlc.arg(url), source_url = sqlc.arg(source_url),
    mime_type = sqlc.arg(mime_type), width = sqlc.arg(width), height = sqlc.arg(height),
    size_bytes = sqlc.arg(size_bytes), checksum_sha256 = sqlc.arg(checksum_sha256), metadata = sqlc.arg(metadata)
WHERE id = sqlc.arg(id) AND owner_user_id = sqlc.arg(owner_user_id) AND status IN ('pending', 'uploading', 'failed', 'ready')
RETURNING id;

-- name: MarkMediaAssetFailed :one
UPDATE media_assets
SET status = 'failed', updated_at = now(), metadata = jsonb_set(
  metadata, '{ingestion_error}', to_jsonb(sqlc.arg(error_message)::text), true
)
WHERE id = sqlc.arg(id) AND owner_user_id = sqlc.arg(owner_user_id) AND status IN ('pending', 'uploading')
RETURNING id;

-- name: GetReadyLibraryMaterializedMediaAsset :one
SELECT asset.id, asset.asset_id, asset.owner_user_id, asset.storage_profile_id, asset.storage_key,
  asset.url, asset.source_url, asset.media_kind, asset.mime_type,
  asset.width, asset.height, asset.size_bytes, asset.checksum_sha256, asset.object_generation,
  asset.status, asset.metadata, asset.created_at, asset.updated_at, asset.deleted_at, asset.reclaim_after, asset.reclaim_started_at,
  asset.reclaim_claim_token, asset.reclaim_generation
FROM library_asset_media_assets AS link
JOIN media_assets AS asset ON asset.id = link.media_asset_id AND asset.owner_user_id = link.owner_user_id
WHERE link.library_asset_id = sqlc.arg(library_asset_id)
  AND link.owner_user_id = sqlc.arg(owner_user_id)
  AND asset.status = 'ready'
  AND asset.deleted_at IS NULL;

-- name: BeginLibraryMaterializedMediaAsset :one
INSERT INTO media_assets (
  asset_id, owner_user_id, storage_profile_id, storage_key, url, source_url,
  media_kind, status, metadata, reclaim_after, reclaim_generation
)
VALUES (
  sqlc.arg(asset_id), sqlc.arg(owner_user_id), sqlc.arg(storage_profile_id), sqlc.arg(storage_key), sqlc.arg(url), sqlc.arg(source_url),
  'image', 'uploading', sqlc.arg(metadata), sqlc.arg(reclaim_after), 1
)
ON CONFLICT (storage_profile_id, storage_key) DO UPDATE
SET source_url = EXCLUDED.source_url,
    status = CASE WHEN media_assets.status = 'ready' THEN 'ready' ELSE 'uploading' END,
    metadata = CASE WHEN media_assets.status = 'ready' THEN media_assets.metadata ELSE EXCLUDED.metadata END,
    updated_at = now(), deleted_at = NULL, reclaim_after = CASE WHEN media_assets.status = 'ready' THEN media_assets.reclaim_after ELSE EXCLUDED.reclaim_after END,
    reclaim_started_at = NULL, reclaim_claim_token = NULL
WHERE media_assets.owner_user_id = EXCLUDED.owner_user_id
RETURNING id;

-- name: UpsertLibraryAssetMediaLink :exec
INSERT INTO library_asset_media_assets (library_asset_id, owner_user_id, media_asset_id)
VALUES (sqlc.arg(library_asset_id), sqlc.arg(owner_user_id), sqlc.arg(media_asset_id))
ON CONFLICT (library_asset_id) DO UPDATE
SET media_asset_id = EXCLUDED.media_asset_id, updated_at = now()
WHERE library_asset_media_assets.owner_user_id = EXCLUDED.owner_user_id;

-- name: ListStaleUploadingMediaAssetIDs :many
SELECT id
FROM media_assets
WHERE status = 'uploading' AND updated_at < sqlc.arg(stale_before)
ORDER BY updated_at ASC, id ASC
LIMIT sqlc.arg(row_limit);

-- name: MarkStaleUploadingMediaAssetsFailed :execrows
UPDATE media_assets
SET status = 'failed', updated_at = now(), metadata = jsonb_set(
  metadata, '{ingestion_error}', to_jsonb('upload lease expired; reconciler marked ingestion failed'::text), true
)
WHERE id = ANY(sqlc.arg(ids)::bigint[]) AND status = 'uploading';

-- name: GetMediaAssetByID :one
SELECT
  id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256, object_generation,
  status, metadata, created_at, updated_at, deleted_at, reclaim_after, reclaim_started_at,
  reclaim_claim_token, reclaim_generation
FROM media_assets
WHERE id = sqlc.arg(id);

-- name: GetMediaAssetByStorageLocator :one
SELECT
  id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256, object_generation,
  status, metadata, created_at, updated_at, deleted_at, reclaim_after, reclaim_started_at,
  reclaim_claim_token, reclaim_generation
FROM media_assets
WHERE storage_profile_id = sqlc.arg(storage_profile_id)
  AND storage_key = sqlc.arg(storage_key);

-- name: GetMediaAssetByIDForOwner :one
SELECT
  id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256, object_generation,
  status, metadata, created_at, updated_at, deleted_at, reclaim_after, reclaim_started_at,
  reclaim_claim_token, reclaim_generation
FROM media_assets
WHERE id = sqlc.arg(id)
  AND owner_user_id = sqlc.arg(owner_user_id)
  AND deleted_at IS NULL;

-- name: GetMediaAssetByAssetIDForOwner :one
SELECT
  id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256, object_generation,
  status, metadata, created_at, updated_at, deleted_at, reclaim_after, reclaim_started_at,
  reclaim_claim_token, reclaim_generation
FROM media_assets
WHERE asset_id = sqlc.arg(asset_id)
  AND owner_user_id = sqlc.arg(owner_user_id)
  AND deleted_at IS NULL;

-- name: GetReadyMediaAssetStorageLocationByID :one
SELECT storage_profile_id, storage_key
FROM media_assets
WHERE id = sqlc.arg(id)
  AND status = 'ready'
  AND deleted_at IS NULL;

-- name: FindReadyOwnedMediaAssetIDByURL :one
SELECT id
FROM media_assets
WHERE owner_user_id = sqlc.arg(owner_user_id)
  AND url = sqlc.arg(url)
  AND status = 'ready'
  AND deleted_at IS NULL
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: ListReadyOwnedMediaAssetIDsByStorageKey :many
SELECT id
FROM media_assets
WHERE owner_user_id = sqlc.arg(owner_user_id)
  AND storage_key = sqlc.arg(storage_key)
  AND status = 'ready'
  AND deleted_at IS NULL
ORDER BY updated_at DESC, id DESC
LIMIT 2;

-- name: FindMediaAssetStorageLocationByURL :one
SELECT storage_profile_id, storage_key
FROM media_assets
WHERE url = sqlc.arg(url)
  AND status = 'ready'
  AND deleted_at IS NULL
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: ListMediaAssetStorageLocationsByKey :many
SELECT storage_profile_id, storage_key
FROM media_assets
WHERE storage_key = sqlc.arg(storage_key)
  AND status = 'ready'
  AND deleted_at IS NULL
ORDER BY updated_at DESC, id DESC
LIMIT 2;

-- name: UpsertMediaRef :one
INSERT INTO media_refs (
  media_asset_id, owner_user_id, ref_type, ref_id,
  board_id, task_id, role, input_index, metadata
)
VALUES (
  sqlc.arg(media_asset_id), sqlc.arg(owner_user_id), sqlc.arg(ref_type), sqlc.arg(ref_id),
  sqlc.arg(board_id), sqlc.arg(task_id), sqlc.arg(role), sqlc.arg(input_index), sqlc.arg(metadata)
)
ON CONFLICT (media_asset_id, ref_type, ref_id, board_id, role) DO UPDATE
SET board_id = EXCLUDED.board_id,
    task_id = EXCLUDED.task_id,
    input_index = EXCLUDED.input_index,
    metadata = EXCLUDED.metadata
WHERE media_refs.owner_user_id = EXCLUDED.owner_user_id
RETURNING id;

-- name: ListMediaRefsByBoardID :many
SELECT
  id, media_asset_id, owner_user_id, ref_type, ref_id,
  board_id, task_id, role, input_index, metadata, created_at, updated_at
FROM media_refs
WHERE owner_user_id = sqlc.arg(owner_user_id) AND board_id = sqlc.arg(board_id)
ORDER BY created_at ASC, id ASC;

-- name: FindPreparedGenerationMedia :one
SELECT asset.id, asset.storage_profile_id, asset.storage_key, asset.url,
  asset.source_url, asset.mime_type, asset.width, asset.height, asset.size_bytes, asset.checksum_sha256
FROM media_refs AS ref
JOIN media_assets AS asset ON asset.id = ref.media_asset_id AND asset.owner_user_id = ref.owner_user_id
WHERE ref.owner_user_id = sqlc.arg(owner_user_id)
  AND ref.ref_type = 'prompt_reference'
  AND ref.ref_id = sqlc.arg(ref_id)
  AND ref.role = sqlc.arg(role)
  AND ref.input_index = sqlc.arg(input_index)
  AND ref.task_id = ''
  AND asset.status = 'ready'
  AND asset.deleted_at IS NULL
LIMIT 1;

-- name: FindIngestingPreparedGenerationMedia :one
SELECT asset.id, asset.storage_profile_id, asset.storage_key, asset.url,
  asset.source_url, asset.mime_type, asset.width, asset.height, asset.size_bytes, asset.checksum_sha256
FROM media_refs AS ref
JOIN media_assets AS asset ON asset.id = ref.media_asset_id AND asset.owner_user_id = ref.owner_user_id
WHERE ref.owner_user_id = sqlc.arg(owner_user_id)
  AND ref.ref_type = 'prompt_reference'
  AND ref.ref_id = sqlc.arg(ref_id)
  AND ref.role = sqlc.arg(role)
  AND ref.input_index = sqlc.arg(input_index)
  AND ref.task_id = ''
  AND asset.status IN ('uploading', 'failed')
  AND asset.deleted_at IS NULL
LIMIT 1;

-- name: ListTaskInputMediaRefsForLineage :many
SELECT media_asset_id, role, input_index
FROM media_refs
WHERE task_id = sqlc.arg(task_id)
  AND ref_type = 'task_input'
ORDER BY input_index ASC, id ASC;

-- name: DeleteUnclaimedPreparedMediaAsset :execrows
DELETE FROM media_assets AS asset
WHERE asset.id = sqlc.arg(id)
  AND asset.owner_user_id = sqlc.arg(owner_user_id)
  AND NOT EXISTS (
    SELECT 1 FROM media_refs AS ref
    WHERE ref.media_asset_id = asset.id AND ref.task_id <> ''
  );

-- name: DeleteBoardMediaRefsByOwnerAndBoardID :exec
DELETE FROM media_refs
WHERE owner_user_id = sqlc.arg(owner_user_id)
  AND board_id = sqlc.arg(board_id)
  AND ref_type IN ('board', 'board_node');

-- name: DeleteTaskOutputMediaRefsByTaskID :exec
DELETE FROM media_refs
WHERE task_id = sqlc.arg(task_id)
  AND ref_type = 'task_output';

-- name: ReleaseTaskInputMediaRefs :many
WITH released AS (
  DELETE FROM media_refs
  WHERE task_id = sqlc.arg(task_id)
    AND ref_type = 'task_input'
  RETURNING media_asset_id
)
UPDATE media_assets AS asset
SET reclaim_after = COALESCE(asset.reclaim_after, now()),
    reclaim_started_at = NULL,
    reclaim_claim_token = NULL,
    reclaim_generation = CASE
      WHEN asset.reclaim_after IS NULL THEN asset.reclaim_generation + 1
      ELSE asset.reclaim_generation
    END
WHERE asset.id IN (SELECT media_asset_id FROM released)
  AND asset.deleted_at IS NULL
RETURNING asset.id;

-- name: CountMediaRefsByAssetID :one
SELECT COUNT(*)
FROM media_refs
WHERE media_asset_id = sqlc.arg(media_asset_id);

-- name: ScheduleUnreferencedMediaAssetsForReclamation :exec
UPDATE media_assets AS asset
SET reclaim_after = COALESCE(asset.reclaim_after, sqlc.arg(reclaim_after)),
    reclaim_started_at = NULL,
    reclaim_claim_token = NULL,
    reclaim_generation = CASE
      WHEN asset.reclaim_after IS NULL THEN asset.reclaim_generation + 1
      ELSE asset.reclaim_generation
    END
WHERE asset.id = ANY(sqlc.arg(media_asset_ids)::bigint[])
  AND asset.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1
    FROM media_refs AS ref
    WHERE ref.media_asset_id = asset.id
      AND ref.ref_type IN ('board', 'board_node', 'task_input')
  );

-- name: CancelMediaAssetReclamation :many
UPDATE media_assets
SET reclaim_after = NULL,
    reclaim_started_at = NULL,
    reclaim_claim_token = NULL
WHERE id = ANY(sqlc.arg(media_asset_ids)::bigint[])
  AND deleted_at IS NULL
  AND (
    reclaim_started_at IS NULL
    OR reclaim_started_at <= sqlc.arg(stale_claim_before)
  )
RETURNING id;

-- name: ListDueMediaAssetReclamationIDs :many
SELECT asset.id, asset.reclaim_generation
FROM media_assets AS asset
WHERE asset.deleted_at IS NULL
  AND asset.reclaim_after <= sqlc.arg(now_at)
  AND (
    asset.reclaim_started_at IS NULL
    OR asset.reclaim_started_at <= sqlc.arg(stale_claim_before)
  )
  AND NOT EXISTS (
    SELECT 1
    FROM media_refs AS ref
    WHERE ref.media_asset_id = asset.id
      AND ref.ref_type IN ('board', 'board_node', 'task_input')
  )
ORDER BY asset.reclaim_after ASC, asset.id ASC
LIMIT sqlc.arg(row_limit);

-- name: ClaimDueMediaAssetReclamation :one
UPDATE media_assets AS asset
SET reclaim_started_at = sqlc.arg(claimed_at),
    reclaim_claim_token = sqlc.arg(claim_token)
WHERE asset.id = sqlc.arg(id)
  AND asset.reclaim_generation = sqlc.arg(reclaim_generation)
  AND asset.deleted_at IS NULL
  AND asset.reclaim_after <= sqlc.arg(now_at)
  AND (
    asset.reclaim_started_at IS NULL
    OR asset.reclaim_started_at <= sqlc.arg(stale_claim_before)
  )
  AND NOT EXISTS (
    SELECT 1
    FROM media_refs AS ref
    WHERE ref.media_asset_id = asset.id
      AND ref.ref_type IN ('board', 'board_node', 'task_input')
  )
RETURNING asset.id, asset.asset_id, asset.owner_user_id, asset.storage_profile_id, asset.storage_key,
  asset.url, asset.source_url, asset.media_kind, asset.mime_type,
  asset.width, asset.height, asset.size_bytes, asset.checksum_sha256, asset.object_generation,
  asset.status, asset.metadata, asset.created_at, asset.updated_at, asset.deleted_at,
  asset.reclaim_after, asset.reclaim_started_at, asset.reclaim_claim_token, asset.reclaim_generation;

-- name: ReleaseMediaAssetReclamationClaim :exec
UPDATE media_assets
SET reclaim_started_at = NULL,
    reclaim_claim_token = NULL
WHERE id = sqlc.arg(id)
  AND reclaim_generation = sqlc.arg(reclaim_generation)
  AND deleted_at IS NULL
  AND reclaim_claim_token = sqlc.arg(claim_token);

-- name: MarkMediaAssetReclaimed :execrows
UPDATE media_assets
SET status = 'deleted',
    url = '',
    reclaim_after = NULL,
    reclaim_started_at = NULL,
    reclaim_claim_token = NULL,
    deleted_at = sqlc.arg(deleted_at)
WHERE media_assets.id = sqlc.arg(id)
  AND media_assets.reclaim_generation = sqlc.arg(reclaim_generation)
  AND media_assets.deleted_at IS NULL
  AND media_assets.reclaim_started_at IS NOT NULL
  AND media_assets.reclaim_claim_token = sqlc.arg(claim_token)
  AND NOT EXISTS (
    SELECT 1
    FROM media_refs AS ref
    WHERE ref.media_asset_id = media_assets.id
      AND ref.ref_type IN ('board', 'board_node', 'task_input')
  );

-- name: FindReadyOwnedMediaAssetByChecksum :one
SELECT id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type, width, height, size_bytes, checksum_sha256, object_generation,
  status, metadata, created_at, updated_at, deleted_at, reclaim_after, reclaim_started_at,
  reclaim_claim_token, reclaim_generation
FROM media_assets
WHERE owner_user_id = sqlc.arg(owner_user_id)
  AND storage_profile_id = sqlc.arg(storage_profile_id)
  AND checksum_sha256 = sqlc.arg(checksum_sha256)
  AND checksum_sha256 <> ''
  AND status = 'ready'
  AND deleted_at IS NULL
ORDER BY id ASC
LIMIT 1;
