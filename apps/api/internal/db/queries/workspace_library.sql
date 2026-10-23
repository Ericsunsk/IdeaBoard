-- Workspace-scoped library queries (#818, Phase 4-01 of #809).
--
-- All lookups are keyed by workspace first: every folder/asset row is
-- addressed as (workspace_id, public_id) so cross-workspace ids miss by
-- construction and read as not found (never a distinct status).
-- Scope guard: scripts/sql-workspace-scope-check.sh enforces one workspace_id key per query.
-- Durable public ids follow the schema CHECK patterns
-- (lfolder_% / lasset_% / lmat_%); callers mint them through the library
-- seam (NewV1FolderPublicID / NewV1AssetPublicID) and pass them opaquely.
-- Integer database ids (internal_id) never leave the store.
--
-- Folder uniqueness is (workspace_id, name) per
-- uq_library_folders_workspace_name; the *_CI lookup additionally guards
-- the seam's case-insensitive contract (parity with the in-memory store)
-- while the constraint remains the race backstop. System folders carry
-- (is_system, is_default) = (TRUE, TRUE) per
-- chk_library_folders_system_shape and are immutable: rename/delete
-- statements exclude them and the store maps the miss explicitly.
--
-- Assets always live in a folder (folder_id NOT NULL): the workspace
-- Quick Saves system folder is the unfiled bucket. Save/move callers
-- pass an empty folder to mean Quick Saves and the store ensures it.
-- Title/author are durable display metadata; task_id/task_result_index
-- are task provenance (paired, never a dedup key); saved_by_user_id is
-- the server-side operator identity (never serialized on the wire).
-- Source is derived (media_asset_id IS NOT NULL -> 'media', else
-- 'external'); there is no source column. Media display dimensions are
-- projected at read time from the joined media_assets row (Media owns
-- the fact; library width/height stay NULL on media rows), paired by
-- the seam (either both set or both NULL).
-- Dedup identity: external rows (workspace_id, provider, source_id)
-- WHERE media_asset_id IS NULL; media rows (workspace_id, media_asset_id)
-- WHERE NOT NULL. Asset reads LEFT JOIN media_assets for the managed
-- link identity + dimension projection in one round trip.

-- name: CreateMediaLibraryAsset :one
INSERT INTO library_assets (
  public_id, workspace_id, folder_id, media_asset_id,
  title, author, task_id, task_result_index, saved_by_user_id
)
VALUES (
  sqlc.arg(public_id), sqlc.arg(workspace_id), sqlc.arg(folder_id), sqlc.arg(media_asset_id),
  sqlc.arg(title), sqlc.arg(author), sqlc.arg(task_id), sqlc.arg(task_result_index), sqlc.arg(saved_by_user_id)
)
RETURNING internal_id, public_id, workspace_id, folder_id, provider, source_id, source_url, preview_url, original_url, media_asset_id, title, author, width, height, task_id, task_result_index, saved_by_user_id, created_at, updated_at;


-- name: CreateLibraryFolder :one
INSERT INTO library_folders (public_id, workspace_id, name, is_system, is_default)
VALUES (sqlc.arg(public_id), sqlc.arg(workspace_id), sqlc.arg(name), sqlc.arg(is_system), sqlc.arg(is_default))
RETURNING internal_id, public_id, workspace_id, name, is_system, is_default, created_at, updated_at;


-- name: GetLibraryFolderByPublicID :one
SELECT internal_id, public_id, workspace_id, name, is_system, is_default, created_at, updated_at
FROM library_folders AS f
WHERE f.workspace_id = sqlc.arg(workspace_id)
  AND f.public_id = sqlc.arg(public_id)
LIMIT 1;


-- name: GetLibraryFolderByWorkspaceAndNameCI :one
SELECT internal_id, public_id, workspace_id, name, is_system, is_default, created_at, updated_at
FROM library_folders AS f
WHERE f.workspace_id = sqlc.arg(workspace_id)
  AND lower(f.name) = lower(sqlc.arg(name))
  AND (sqlc.narg(exclude_public_id)::TEXT IS NULL OR f.public_id <> sqlc.narg(exclude_public_id)::TEXT)
LIMIT 1;


-- name: ListLibraryFolders :many
SELECT internal_id, public_id, workspace_id, name, is_system, is_default, created_at, updated_at
FROM library_folders AS f
WHERE f.workspace_id = sqlc.arg(workspace_id)
ORDER BY f.is_system DESC, f.is_default DESC, f.created_at ASC, f.internal_id ASC
LIMIT sqlc.arg(row_limit);


-- name: EnsureQuickSavesFolder :exec
INSERT INTO library_folders (public_id, workspace_id, name, is_system, is_default)
VALUES (sqlc.arg(public_id), sqlc.arg(workspace_id), 'Quick Saves', TRUE, TRUE)
ON CONFLICT (workspace_id, name) DO NOTHING;


-- name: GetQuickSavesFolder :one
SELECT internal_id, public_id, workspace_id, name, is_system, is_default, created_at, updated_at
FROM library_folders AS f
WHERE f.workspace_id = sqlc.arg(workspace_id)
  AND f.name = 'Quick Saves'
  AND f.is_system = TRUE
  AND f.is_default = TRUE
LIMIT 1;


-- name: RenameLibraryFolder :one
UPDATE library_folders AS f
SET name = sqlc.arg(name), updated_at = now()
WHERE f.workspace_id = sqlc.arg(workspace_id)
  AND f.public_id = sqlc.arg(public_id)
  AND f.is_system = FALSE
RETURNING f.internal_id, f.public_id, f.workspace_id, f.name, f.is_system, f.is_default, f.created_at, f.updated_at;


-- name: MoveLibraryAssetsToFolder :execrows
UPDATE library_assets AS a
SET folder_id = sqlc.arg(target_folder_id), updated_at = now()
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.folder_id = sqlc.arg(source_folder_id);


-- name: DeleteLibraryFolder :execrows
DELETE FROM library_folders AS f
WHERE f.workspace_id = sqlc.arg(workspace_id)
  AND f.public_id = sqlc.arg(public_id)
  AND f.is_system = FALSE;


-- name: CreateLibraryAsset :one
INSERT INTO library_assets (
  public_id, workspace_id, folder_id, provider, source_id,
  source_url, preview_url, original_url, title, author, width, height, saved_by_user_id
)
VALUES (
  sqlc.arg(public_id), sqlc.arg(workspace_id), sqlc.arg(folder_id), sqlc.arg(provider), sqlc.arg(source_id),
  sqlc.arg(source_url), sqlc.arg(preview_url), sqlc.arg(original_url), sqlc.arg(title), sqlc.arg(author), sqlc.arg(width), sqlc.arg(height), sqlc.arg(saved_by_user_id)
)
RETURNING internal_id, public_id, workspace_id, folder_id, provider, source_id, source_url, preview_url, original_url, media_asset_id, title, author, width, height, task_id, task_result_index, saved_by_user_id, created_at, updated_at;


-- name: GetLibraryAssetByPublicID :one
SELECT sqlc.embed(a),
  f.public_id AS folder_public_id,
  ma.public_id AS media_asset_public_id,
  ma.width AS media_width, ma.height AS media_height
FROM library_assets AS a
INNER JOIN library_folders AS f ON f.internal_id = a.folder_id
LEFT JOIN media_assets AS ma ON ma.internal_id = a.media_asset_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.public_id = sqlc.arg(public_id)
LIMIT 1;


-- name: GetLibraryAssetByWorkspaceSource :one
SELECT sqlc.embed(a),
  f.public_id AS folder_public_id,
  ma.public_id AS media_asset_public_id,
  ma.width AS media_width, ma.height AS media_height
FROM library_assets AS a
INNER JOIN library_folders AS f ON f.internal_id = a.folder_id
LEFT JOIN media_assets AS ma ON ma.internal_id = a.media_asset_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.media_asset_id IS NULL
  AND a.provider = sqlc.arg(provider)
  AND a.source_id = sqlc.arg(source_id)
LIMIT 1;


-- name: GetLibraryAssetByWorkspaceMedia :one
SELECT sqlc.embed(a),
  f.public_id AS folder_public_id,
  ma.public_id AS media_asset_public_id,
  ma.width AS media_width, ma.height AS media_height
FROM library_assets AS a
INNER JOIN library_folders AS f ON f.internal_id = a.folder_id
LEFT JOIN media_assets AS ma ON ma.internal_id = a.media_asset_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.media_asset_id = sqlc.arg(media_asset_id)
LIMIT 1;


-- name: ListLibraryAssets :many
SELECT sqlc.embed(a),
  f.public_id AS folder_public_id,
  ma.public_id AS media_asset_public_id,
  ma.width AS media_width, ma.height AS media_height
FROM library_assets AS a
INNER JOIN library_folders AS f ON f.internal_id = a.folder_id
LEFT JOIN media_assets AS ma ON ma.internal_id = a.media_asset_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
ORDER BY a.created_at ASC, a.internal_id ASC
LIMIT sqlc.arg(row_limit);


-- name: ListLibraryAssetsByWorkspaceProvider :many
SELECT sqlc.embed(a),
  f.public_id AS folder_public_id,
  ma.public_id AS media_asset_public_id,
  ma.width AS media_width, ma.height AS media_height
FROM library_assets AS a
INNER JOIN library_folders AS f ON f.internal_id = a.folder_id
LEFT JOIN media_assets AS ma ON ma.internal_id = a.media_asset_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.media_asset_id IS NULL
  AND a.provider = sqlc.arg(provider)
ORDER BY a.created_at ASC, a.internal_id ASC
LIMIT sqlc.arg(row_limit);


-- name: MoveLibraryAsset :one
WITH moved AS (
  UPDATE library_assets AS a
  SET folder_id = sqlc.arg(folder_id), updated_at = now()
  WHERE a.workspace_id = sqlc.arg(workspace_id)
    AND a.public_id = sqlc.arg(public_id)
  RETURNING a.*
)
SELECT sqlc.embed(a),
  f.public_id AS folder_public_id,
  ma.public_id AS media_asset_public_id,
  ma.width AS media_width, ma.height AS media_height
FROM moved AS a
INNER JOIN library_folders AS f ON f.internal_id = a.folder_id
LEFT JOIN media_assets AS ma ON ma.internal_id = a.media_asset_id;


-- name: DeleteLibraryAsset :execrows
DELETE FROM library_assets AS a
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.public_id = sqlc.arg(public_id);

-- Materialization durable jobs (#828 slice A, Phase 4 of #809).
--
-- Status words: the public API exposes pending/running/completed/failed
-- while the durable CHECK stores pending/processing/completed/failed
-- (chk_library_materializations_status). The store maps running <->
-- processing at the boundary; SQL below uses the durable words.
-- Terminal states (completed/failed) are immutable: transition
-- statements guard on status IN ('pending','processing') so concurrent
-- settlers converge on the winner; a guarded miss reads back the stored
-- snapshot so idempotent replays converge instead of resurrecting jobs.
-- Completion CHECK: completed needs media_asset_id + completed_at,
-- failed needs completed_at. The result link resolves the caller's
-- asset_... public id to media_assets.internal_id workspace-scoped
-- (GetMediaAssetByPublicID in workspace_media.sql). error_code has no
-- durable column: failures persist error_message only and reads report
-- an empty code (follow-up migration if the taxonomy must persist).
-- Rows carry no updated_at: reads report COALESCE(completed_at,
-- created_at) as updated_at. No dedup key: one asset may back many
-- jobs; request idempotency stays handler-side (library:materialize).
