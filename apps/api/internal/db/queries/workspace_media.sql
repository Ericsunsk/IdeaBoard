-- Workspace-scoped media queries (#820, Phase 4-03 of #809).
--
-- Upload sessions are durable coordination rows (unlike the pre-#809
-- ephemeral process-local sessions): pending -> transferring ->
-- completed, with transferring -> failed and pending/transferring ->
-- expired. Completion-only durability: only a successful complete
-- creates the durable Media Asset row; abandoned sessions linger as
-- pending/transferring rows until lazily expired on read (no sweeper).
-- The chk_upload_sessions_completion gate requires completed_at +
-- storage_key on completed rows and expired_at on expired rows.
--
-- Media Assets are immutable content references (public_id LIKE
-- 'asset_%', kernel PrefixAsset). There is no status column: an asset
-- row exists only after a successful completion, so existence means
-- ready; archived_at marks the archive lifecycle (metadata stays
-- readable, bytes go 410). Image rows carry width/height (NULL reads
-- back as 0); non-image kinds leave dimensions NULL.
--
-- Every lookup scopes by workspace public_id first so cross-workspace
-- ids miss by construction and read as not found (never a distinct
-- status). Membership gating stays at the v1 handler layer; the
-- repository enforces scoping + transitions.
-- Scope guard: scripts/sql-workspace-scope-check.sh enforces one scope predicate per query.

-- name: CreateUploadSession :one
INSERT INTO upload_sessions (public_id, workspace_id, uploader_user_id, declared_mime_type, declared_size_bytes, declared_checksum, storage_key, target_board_id, status)
VALUES (sqlc.arg(public_id), sqlc.arg(workspace_id), sqlc.arg(uploader_user_id), sqlc.arg(declared_mime_type), sqlc.arg(declared_size_bytes), sqlc.narg(declared_checksum), sqlc.narg(storage_key), sqlc.narg(target_board_id), 'pending')
RETURNING internal_id, public_id, workspace_id, uploader_user_id, declared_mime_type, declared_size_bytes, declared_checksum, storage_key, status, completed_at, expired_at, created_at, target_board_id;

-- name: GetUploadSessionByPublicID :one
SELECT u.internal_id, u.public_id, u.workspace_id, u.uploader_user_id, u.declared_mime_type, u.declared_size_bytes, u.declared_checksum, u.storage_key, u.status, u.completed_at, u.expired_at, u.created_at, u.target_board_id
FROM upload_sessions AS u
INNER JOIN workspaces AS w ON w.internal_id = u.workspace_id
WHERE u.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
LIMIT 1;

-- MarkUploadSessionTransferring signals transfer start (first read
-- after create). Zero rows means already moved or invisible; the
-- caller distinguishes via GetUploadSessionByPublicID.
-- name: MarkUploadSessionTransferring :one
UPDATE upload_sessions AS u
SET status = 'transferring'
FROM workspaces AS w
WHERE u.workspace_id = w.internal_id
AND u.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
AND u.status = 'pending'
RETURNING u.internal_id, u.public_id, u.workspace_id, u.uploader_user_id, u.declared_mime_type, u.declared_size_bytes, u.declared_checksum, u.storage_key, u.status, u.completed_at, u.expired_at, u.created_at, u.target_board_id;

-- CompleteUploadSession flips a live session to completed with its
-- reserved storage key. The durable Media Asset row is inserted
-- separately in the same caller-owned transaction.
-- name: CompleteUploadSession :one
UPDATE upload_sessions AS u
SET status = 'completed', storage_key = sqlc.arg(storage_key), completed_at = now()
FROM workspaces AS w
WHERE u.workspace_id = w.internal_id
AND u.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
AND u.status IN ('pending', 'transferring')
RETURNING u.internal_id, u.public_id, u.workspace_id, u.uploader_user_id, u.declared_mime_type, u.declared_size_bytes, u.declared_checksum, u.storage_key, u.status, u.completed_at, u.expired_at, u.created_at, u.target_board_id;

-- FailUploadSession flips a live session to failed (chk gate carries
-- no extra columns for failed rows; the reason stays server-side).
-- name: FailUploadSession :one
UPDATE upload_sessions AS u
SET status = 'failed'
FROM workspaces AS w
WHERE u.workspace_id = w.internal_id
AND u.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
AND u.status IN ('pending', 'transferring')
RETURNING u.internal_id, u.public_id, u.workspace_id, u.uploader_user_id, u.declared_mime_type, u.declared_size_bytes, u.declared_checksum, u.storage_key, u.status, u.completed_at, u.expired_at, u.created_at, u.target_board_id;

-- ExpireUploadSession lazily expires a live session past its TTL.
-- name: ExpireUploadSession :one
UPDATE upload_sessions AS u
SET status = 'expired', expired_at = now()
FROM workspaces AS w
WHERE u.workspace_id = w.internal_id
AND u.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
AND u.status IN ('pending', 'transferring')
RETURNING u.internal_id, u.public_id, u.workspace_id, u.uploader_user_id, u.declared_mime_type, u.declared_size_bytes, u.declared_checksum, u.storage_key, u.status, u.completed_at, u.expired_at, u.created_at, u.target_board_id;

-- CreateMediaAsset inserts the durable asset row. Width/height ride
-- NULL when unknown (0 maps to NULL; the image CHECK passes on NULL
-- and read-back maps NULL to 0).
-- name: CreateMediaAsset :one
INSERT INTO media_assets (public_id, workspace_id, upload_session_id, owner_user_id, storage_key, mime_type, size_bytes, checksum, media_kind, width, height, duration_ms, metadata)
VALUES (sqlc.arg(public_id), sqlc.arg(workspace_id), sqlc.arg(upload_session_id), sqlc.arg(owner_user_id), sqlc.arg(storage_key), sqlc.arg(mime_type), sqlc.arg(size_bytes), sqlc.arg(checksum), sqlc.arg(media_kind), sqlc.narg(width), sqlc.narg(height), sqlc.narg(duration_ms), sqlc.arg(metadata))
RETURNING internal_id, public_id, workspace_id, upload_session_id, owner_user_id, storage_key, mime_type, size_bytes, checksum, media_kind, width, height, duration_ms, metadata, created_at, archived_at;

-- GetMediaAssetByPublicID returns the asset row including archived
-- ones (archived assets stay metadata-readable; bytes go 410).
-- name: GetMediaAssetByPublicID :one
SELECT m.internal_id, m.public_id, m.workspace_id, m.upload_session_id, m.owner_user_id, m.storage_key, m.mime_type, m.size_bytes, m.checksum, m.media_kind, m.width, m.height, m.duration_ms, m.metadata, m.created_at, m.archived_at
FROM media_assets AS m
INNER JOIN workspaces AS w ON w.internal_id = m.workspace_id
WHERE m.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
LIMIT 1;

-- ListMediaAssets pages live (non-archived) workspace assets
-- newest-first with limit/offset (the v1 layer converts cursor to
-- offset and over-fetches by one for has_more).
-- name: ListMediaAssets :many
SELECT m.internal_id, m.public_id, m.workspace_id, m.upload_session_id, m.owner_user_id, m.storage_key, m.mime_type, m.size_bytes, m.checksum, m.media_kind, m.width, m.height, m.duration_ms, m.metadata, m.created_at, m.archived_at
FROM media_assets AS m
INNER JOIN workspaces AS w ON w.internal_id = m.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id) AND m.archived_at IS NULL
ORDER BY m.created_at DESC, m.internal_id DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- ArchiveMediaAsset stamps the archive lifecycle marker. Repeat
-- archives miss (archived_at IS NULL guard); the caller reads back
-- the stored row so replays converge instead of 404ing.
-- name: ArchiveMediaAsset :one
UPDATE media_assets AS m
SET archived_at = now()
FROM workspaces AS w
WHERE m.workspace_id = w.internal_id
AND m.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
AND m.archived_at IS NULL
RETURNING m.internal_id, m.public_id, m.workspace_id, m.upload_session_id, m.owner_user_id, m.storage_key, m.mime_type, m.size_bytes, m.checksum, m.media_kind, m.width, m.height, m.duration_ms, m.metadata, m.created_at, m.archived_at;

-- Compensation locks the asset before checking durable reference facts.
-- The second statement gets a fresh snapshot after an in-flight writer commits.
-- name: LockMediaAssetForCompensation :one
SELECT m.internal_id
FROM media_assets AS m
INNER JOIN workspaces AS w ON w.internal_id = m.workspace_id
WHERE m.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
FOR UPDATE OF m;

-- name: ArchiveUnreferencedMediaAsset :execrows
UPDATE media_assets AS m
SET archived_at = COALESCE(m.archived_at, now())
WHERE m.internal_id = sqlc.arg(internal_id)
AND NOT EXISTS (SELECT 1 FROM task_outputs AS o WHERE o.media_asset_id = m.internal_id)
AND NOT EXISTS (SELECT 1 FROM media_asset_task_origins AS o WHERE o.media_asset_id = m.internal_id)
AND NOT EXISTS (SELECT 1 FROM board_document_media_refs AS r WHERE r.media_asset_id = m.internal_id)
AND NOT EXISTS (SELECT 1 FROM library_assets AS l WHERE l.media_asset_id = m.internal_id)
AND NOT EXISTS (SELECT 1 FROM library_materializations AS l WHERE l.media_asset_id = m.internal_id);

-- UpdateMediaAssetMetadata refreshes dimensions and merges metadata
-- (read-modify-write merge: unknown keys from other writers survive).
-- name: UpdateMediaAssetMetadata :one
UPDATE media_assets AS m
SET width = sqlc.narg(width), height = sqlc.narg(height), duration_ms = sqlc.narg(duration_ms),
    metadata = m.metadata || sqlc.arg(metadata)::jsonb
FROM workspaces AS w
WHERE m.workspace_id = w.internal_id
AND m.public_id = sqlc.arg(public_id)
AND w.public_id = sqlc.arg(workspace_public_id)
RETURNING m.internal_id, m.public_id, m.workspace_id, m.upload_session_id, m.owner_user_id, m.storage_key, m.mime_type, m.size_bytes, m.checksum, m.media_kind, m.width, m.height, m.duration_ms, m.metadata, m.created_at, m.archived_at;

-- ListTaskMediaAssets queries media by task: asset rows attached to a
-- workspace-scoped task via task_outputs, ordered by output_index.
-- Remote-URL outputs carry no asset and are excluded. The output role
-- rides in task_outputs.metadata (see #819's mapping); it is
-- projected alongside the asset row.
-- name: ListTaskMediaAssets :many
SELECT m.internal_id, m.public_id, m.workspace_id, m.upload_session_id, m.owner_user_id, m.storage_key, m.mime_type, m.size_bytes, m.checksum, m.media_kind, m.width, m.height, m.duration_ms, m.metadata, m.created_at, m.archived_at,
    o.output_index, o.metadata AS output_metadata
FROM task_outputs AS o
INNER JOIN tasks AS t ON t.internal_id = o.task_id
INNER JOIN workspaces AS w ON w.internal_id = t.workspace_id
INNER JOIN media_assets AS m ON m.internal_id = o.media_asset_id
WHERE t.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
AND o.output_kind = 'media_asset'
ORDER BY o.output_index ASC;

-- ResolveBoardInternalID maps a workspace-scoped board public id to its
-- internal id for upload-target binding (#936). Misses (unknown board or
-- foreign workspace) return no row; the service maps that to ErrNotFound
-- so the handler renders 404 without leaking existence.
-- name: ResolveBoardInternalID :one
SELECT b.internal_id
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE b.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
LIMIT 1;

-- GetMediaAssetUploadTargetBoard resolves the restricted upload-target
-- board (#936) for one asset through its creating session. Assets whose
-- session carries no target (or whose board row went away) return a NULL
-- board id on the single row; unknown or cross-workspace assets return no
-- row at all (ErrNotFound, never a distinct status).
-- name: GetMediaAssetUploadTargetBoard :one
SELECT b.public_id AS target_board_public_id
FROM media_assets AS m
INNER JOIN workspaces AS w ON w.internal_id = m.workspace_id
INNER JOIN upload_sessions AS u ON u.internal_id = m.upload_session_id
LEFT JOIN boards AS b ON b.internal_id = u.target_board_id
WHERE m.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
LIMIT 1;

-- MediaAssetHasLibraryLink reports whether a workspace-scoped asset backs
-- at least one published Workspace Library row (#936: shared-library
-- source). Library rows are member-visible, so existence authorizes every
-- workspace member. Unknown or cross-workspace assets report false, never
-- a distinct status.
-- name: MediaAssetHasLibraryLink :one
SELECT EXISTS(
  SELECT 1
  FROM media_assets AS m
  INNER JOIN workspaces AS w ON w.internal_id = m.workspace_id
  INNER JOIN library_assets AS l ON l.media_asset_id = m.internal_id
  WHERE m.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
);

-- ListMediaAssetTaskOrigins preserves producing authority independently of
-- Task retention and Board deletion. An ordinary upload has no Task origin.
-- name: ListMediaAssetTaskOrigins :many
SELECT o.task_public_id,
       w.public_id AS workspace_public_id,
       o.submitter_user_id,
       COALESCE(o.board_public_id, '')::text AS board_public_id
FROM media_asset_task_origins AS o
INNER JOIN workspaces AS w ON w.internal_id = o.workspace_id
INNER JOIN media_assets AS m ON m.internal_id = o.media_asset_id AND m.workspace_id = o.workspace_id
WHERE m.public_id = sqlc.arg(public_id)
  AND w.public_id = sqlc.arg(workspace_public_id)
ORDER BY o.task_public_id ASC, o.output_index ASC;
