-- Workspace-scoped media assets (issue #803). The workspace binding reuses
-- media_assets.metadata (workspace_id) with NO schema change; the archive
-- lifecycle marker travels in metadata.archived_at. Storage keys never
-- leave the server: content URLs carry asset_... only.
--
-- Retention: CreateWorkspaceMediaAsset stamps reclaim_after at
-- complete + 7d (WorkspaceAssetAbandonedTTL) for never-verified assets;
-- the existing media reclamation scanner reclaims them once due unless
-- Board/Task/Library refs protect them (#805/#806 own those refs).
-- ArchiveWorkspaceMediaAsset restamps reclaim_after at archive + 30d.

-- name: CreateWorkspaceMediaAsset :one
INSERT INTO media_assets (
  asset_id, owner_user_id, storage_profile_id, storage_key,
  url, media_kind, mime_type, width, height, size_bytes,
  checksum_sha256, status, metadata, reclaim_after
)
VALUES (
  sqlc.arg(asset_id), sqlc.arg(owner_user_id), sqlc.arg(storage_profile_id), sqlc.arg(storage_key),
  sqlc.arg(url), sqlc.arg(media_kind), sqlc.arg(mime_type), sqlc.arg(width), sqlc.arg(height), sqlc.arg(size_bytes),
  sqlc.arg(checksum_sha256), sqlc.arg(status), sqlc.arg(metadata), sqlc.arg(reclaim_after)
)
RETURNING
  id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256, object_generation,
  status, metadata, created_at, updated_at, deleted_at, reclaim_after, reclaim_started_at,
  reclaim_claim_token, reclaim_generation;

-- name: GetWorkspaceMediaAsset :one
SELECT
  id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256, object_generation,
  status, metadata, created_at, updated_at, deleted_at, reclaim_after, reclaim_started_at,
  reclaim_claim_token, reclaim_generation
FROM media_assets
WHERE asset_id = sqlc.arg(asset_id)
  AND (metadata->>'workspace_id') = sqlc.arg(workspace_id)::text
  AND deleted_at IS NULL;

-- name: ListWorkspaceMediaAssets :many
SELECT
  id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256, object_generation,
  status, metadata, created_at, updated_at, deleted_at, reclaim_after, reclaim_started_at,
  reclaim_claim_token, reclaim_generation
FROM media_assets
WHERE (metadata->>'workspace_id') = sqlc.arg(workspace_id)::text
  AND deleted_at IS NULL
  AND (metadata->>'archived_at' IS NULL)
ORDER BY id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: ArchiveWorkspaceMediaAsset :one
UPDATE media_assets
SET metadata = metadata || jsonb_build_object('archived_at', sqlc.arg(archived_at)::text),
    reclaim_after = sqlc.arg(reclaim_after),
    updated_at = now()
WHERE asset_id = sqlc.arg(asset_id)
  AND (metadata->>'workspace_id') = sqlc.arg(workspace_id)::text
  AND deleted_at IS NULL
  AND (metadata->>'archived_at' IS NULL)
RETURNING
  id, asset_id, owner_user_id, storage_profile_id, storage_key,
  url, source_url, media_kind, mime_type,
  width, height, size_bytes, checksum_sha256, object_generation,
  status, metadata, created_at, updated_at, deleted_at, reclaim_after, reclaim_started_at,
  reclaim_claim_token, reclaim_generation;
