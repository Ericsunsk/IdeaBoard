-- Workspace-scoped library queries (#818, Phase 4-01 of #809).
--
-- All lookups are keyed by workspace first: every folder/asset row is
-- addressed as (workspace_id, public_id) so cross-workspace ids miss by
-- construction and read as not found (never a distinct status).
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
-- Title/author have no durable columns (the workspace tables keep
-- provider URL + source metadata only); they are validated by the seam
-- and intentionally not persisted here.

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
  source_url, preview_url, original_url, width, height
)
VALUES (
  sqlc.arg(public_id), sqlc.arg(workspace_id), sqlc.arg(folder_id), sqlc.arg(provider), sqlc.arg(source_id),
  sqlc.arg(source_url), sqlc.arg(preview_url), sqlc.arg(original_url), sqlc.arg(width), sqlc.arg(height)
)
RETURNING internal_id, public_id, workspace_id, folder_id, provider, source_id, source_url, preview_url, original_url, width, height, created_at, updated_at;

-- name: GetLibraryAssetByPublicID :one
SELECT a.internal_id, a.public_id, a.workspace_id, a.folder_id,
  f.public_id AS folder_public_id,
  a.provider, a.source_id, a.source_url, a.preview_url, a.original_url,
  a.width, a.height, a.created_at, a.updated_at
FROM library_assets AS a
INNER JOIN library_folders AS f ON f.internal_id = a.folder_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.public_id = sqlc.arg(public_id)
LIMIT 1;

-- name: GetLibraryAssetByWorkspaceSource :one
SELECT a.internal_id, a.public_id, a.workspace_id, a.folder_id,
  f.public_id AS folder_public_id,
  a.provider, a.source_id, a.source_url, a.preview_url, a.original_url,
  a.width, a.height, a.created_at, a.updated_at
FROM library_assets AS a
INNER JOIN library_folders AS f ON f.internal_id = a.folder_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.provider = sqlc.arg(provider)
  AND a.source_id = sqlc.arg(source_id)
LIMIT 1;

-- name: ListLibraryAssets :many
SELECT a.internal_id, a.public_id, a.workspace_id, a.folder_id,
  f.public_id AS folder_public_id,
  a.provider, a.source_id, a.source_url, a.preview_url, a.original_url,
  a.width, a.height, a.created_at, a.updated_at
FROM library_assets AS a
INNER JOIN library_folders AS f ON f.internal_id = a.folder_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
ORDER BY a.created_at ASC, a.internal_id ASC
LIMIT sqlc.arg(row_limit);

-- name: ListLibraryAssetsByFolder :many
SELECT a.internal_id, a.public_id, a.workspace_id, a.folder_id,
  f.public_id AS folder_public_id,
  a.provider, a.source_id, a.source_url, a.preview_url, a.original_url,
  a.width, a.height, a.created_at, a.updated_at
FROM library_assets AS a
INNER JOIN library_folders AS f ON f.internal_id = a.folder_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.folder_id = sqlc.arg(folder_id)
ORDER BY a.created_at ASC, a.internal_id ASC
LIMIT sqlc.arg(row_limit);

-- name: ListLibraryAssetsByWorkspaceProvider :many
SELECT a.internal_id, a.public_id, a.workspace_id, a.folder_id,
  f.public_id AS folder_public_id,
  a.provider, a.source_id, a.source_url, a.preview_url, a.original_url,
  a.width, a.height, a.created_at, a.updated_at
FROM library_assets AS a
INNER JOIN library_folders AS f ON f.internal_id = a.folder_id
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.provider = sqlc.arg(provider)
ORDER BY a.created_at ASC, a.internal_id ASC
LIMIT sqlc.arg(row_limit);

-- name: MoveLibraryAsset :one
UPDATE library_assets AS a
SET folder_id = sqlc.arg(folder_id), updated_at = now()
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.public_id = sqlc.arg(public_id)
RETURNING a.internal_id, a.public_id, a.workspace_id, a.folder_id, a.provider, a.source_id, a.source_url, a.preview_url, a.original_url, a.width, a.height, a.created_at, a.updated_at;

-- name: DeleteLibraryAsset :execrows
DELETE FROM library_assets AS a
WHERE a.workspace_id = sqlc.arg(workspace_id)
  AND a.public_id = sqlc.arg(public_id);
