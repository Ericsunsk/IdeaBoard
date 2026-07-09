-- name: CreateLineDraftMode :one
INSERT INTO line_draft_modes (name, cover_image, prompt, sort_order, is_enabled)
VALUES (sqlc.arg(name), sqlc.arg(cover_image), sqlc.arg(prompt), sqlc.arg(sort_order), sqlc.arg(is_enabled))
RETURNING id;

-- name: GetLineDraftModeByID :one
SELECT id, name, cover_image, prompt, sort_order, is_enabled, created_at, updated_at
FROM line_draft_modes
WHERE id = sqlc.arg(id);

-- name: ListLineDraftModes :many
SELECT id, name, cover_image, prompt, sort_order, is_enabled, created_at, updated_at
FROM line_draft_modes
ORDER BY sort_order ASC, id ASC;

-- name: ListEnabledLineDraftModes :many
SELECT id, name, cover_image, prompt, sort_order, is_enabled, created_at, updated_at
FROM line_draft_modes
WHERE is_enabled
ORDER BY sort_order ASC, id ASC;

-- name: UpdateLineDraftMode :exec
UPDATE line_draft_modes
SET name = sqlc.arg(name),
    cover_image = sqlc.arg(cover_image),
    prompt = sqlc.arg(prompt),
    sort_order = sqlc.arg(sort_order),
    is_enabled = sqlc.arg(is_enabled)
WHERE id = sqlc.arg(id);

-- name: DeleteLineDraftMode :exec
DELETE FROM line_draft_modes
WHERE id = sqlc.arg(id);

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
    deleted_at = NULL
WHERE media_assets.owner_user_id = EXCLUDED.owner_user_id
RETURNING id;

-- name: GetMediaAssetByID :one
SELECT
  id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256,
  status, metadata, created_at, updated_at, deleted_at
FROM media_assets
WHERE id = sqlc.arg(id);

-- name: GetMediaAssetByAssetID :one
SELECT
  id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256,
  status, metadata, created_at, updated_at, deleted_at
FROM media_assets
WHERE asset_id = sqlc.arg(asset_id);

-- name: ListMediaAssetsByOwner :many
SELECT
  id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256,
  status, metadata, created_at, updated_at, deleted_at
FROM media_assets
WHERE owner_user_id = sqlc.arg(owner_user_id)
  AND (
    (sqlc.arg(has_status)::boolean AND status = sqlc.arg(status))
    OR (NOT sqlc.arg(has_status)::boolean AND deleted_at IS NULL AND status <> 'deleted')
  )
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: MarkMediaAssetDeleted :exec
UPDATE media_assets
SET status = 'deleted',
    deleted_at = COALESCE(deleted_at, now())
WHERE id = sqlc.arg(id) AND owner_user_id = sqlc.arg(owner_user_id);

-- name: UpsertMediaRef :one
INSERT INTO media_refs (
  media_asset_id, owner_user_id, ref_type, ref_id,
  board_id, task_id, role, metadata
)
VALUES (
  sqlc.arg(media_asset_id), sqlc.arg(owner_user_id), sqlc.arg(ref_type), sqlc.arg(ref_id),
  sqlc.arg(board_id), sqlc.arg(task_id), sqlc.arg(role), sqlc.arg(metadata)
)
ON CONFLICT (media_asset_id, ref_type, ref_id, role) DO UPDATE
SET board_id = EXCLUDED.board_id,
    task_id = EXCLUDED.task_id,
    metadata = EXCLUDED.metadata
WHERE media_refs.owner_user_id = EXCLUDED.owner_user_id
RETURNING id;

-- name: ListMediaRefsByAssetID :many
SELECT
  id, media_asset_id, owner_user_id, ref_type, ref_id,
  board_id, task_id, role, metadata, created_at, updated_at
FROM media_refs
WHERE media_asset_id = sqlc.arg(media_asset_id)
ORDER BY created_at ASC, id ASC;

-- name: ListMediaRefsByBoardID :many
SELECT
  id, media_asset_id, owner_user_id, ref_type, ref_id,
  board_id, task_id, role, metadata, created_at, updated_at
FROM media_refs
WHERE owner_user_id = sqlc.arg(owner_user_id) AND board_id = sqlc.arg(board_id)
ORDER BY created_at ASC, id ASC;

-- name: DeleteMediaRef :exec
DELETE FROM media_refs
WHERE media_asset_id = sqlc.arg(media_asset_id)
  AND ref_type = sqlc.arg(ref_type)
  AND ref_id = sqlc.arg(ref_id)
  AND role = sqlc.arg(role);

-- name: DeleteBoardMediaRefsByOwnerAndBoardID :exec
DELETE FROM media_refs
WHERE owner_user_id = sqlc.arg(owner_user_id)
  AND board_id = sqlc.arg(board_id)
  AND ref_type IN ('board', 'board_node');

-- name: DeleteTaskOutputMediaRefsByTaskID :exec
DELETE FROM media_refs
WHERE task_id = sqlc.arg(task_id)
  AND ref_type = 'task_output';

-- name: DeleteTaskOutputMediaRefsByTaskIDs :exec
DELETE FROM media_refs
WHERE task_id = ANY(sqlc.arg(task_ids)::text[])
  AND ref_type = 'task_output';

-- name: DeleteTaskOutputMediaRefsByUserTaskID :exec
DELETE FROM media_refs AS mr
USING tasks AS t
WHERE mr.task_id = sqlc.arg(task_id)
  AND mr.ref_type = 'task_output'
  AND t.task_id = mr.task_id
  AND t.user_id = sqlc.arg(user_id);

-- name: CountMediaRefsByAssetID :one
SELECT COUNT(*)
FROM media_refs
WHERE media_asset_id = sqlc.arg(media_asset_id);
