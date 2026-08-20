-- name: EnsureQuickSavesFolder :exec
INSERT INTO library_folders (owner_user_id, name, is_system, is_default)
VALUES (sqlc.arg(owner_user_id), 'Quick Saves', TRUE, TRUE)
ON CONFLICT DO NOTHING;

-- name: GetQuickSavesFolder :one
SELECT id, owner_user_id, name, is_system, is_default, created_at, updated_at
FROM library_folders
WHERE owner_user_id = sqlc.arg(owner_user_id)
  AND is_system = TRUE
  AND is_default = TRUE
  AND name = 'Quick Saves';

-- name: EnsureLibraryFolderByName :exec
INSERT INTO library_folders (owner_user_id, name)
VALUES (sqlc.arg(owner_user_id), sqlc.arg(name))
ON CONFLICT DO NOTHING;

-- name: GetLibraryFolderByOwnerAndName :one
SELECT id, owner_user_id, name, is_system, is_default, created_at, updated_at
FROM library_folders
WHERE owner_user_id = sqlc.arg(owner_user_id)
  AND lower(name) = lower(sqlc.arg(name));

-- name: ListLibraryFolders :many
SELECT
  folders.id,
  folders.owner_user_id,
  folders.name,
  folders.is_system,
  folders.is_default,
  folders.created_at,
  folders.updated_at,
  COUNT(assets.id)::BIGINT AS asset_count
FROM library_folders AS folders
LEFT JOIN library_assets AS assets
  ON assets.folder_id = folders.id
 AND assets.owner_user_id = folders.owner_user_id
WHERE folders.owner_user_id = sqlc.arg(owner_user_id)
GROUP BY folders.id
ORDER BY folders.is_system DESC, folders.is_default DESC, folders.created_at ASC, folders.id ASC;

-- name: CreateLibraryFolder :one
INSERT INTO library_folders (owner_user_id, name)
VALUES (sqlc.arg(owner_user_id), sqlc.arg(name))
RETURNING id, owner_user_id, name, is_system, is_default, created_at, updated_at;

-- name: GetLibraryFolderByIDForOwner :one
SELECT id, owner_user_id, name, is_system, is_default, created_at, updated_at
FROM library_folders
WHERE id = sqlc.arg(id) AND owner_user_id = sqlc.arg(owner_user_id);

-- name: RenameLibraryFolder :one
UPDATE library_folders
SET name = sqlc.arg(name), updated_at = now()
WHERE id = sqlc.arg(id)
  AND owner_user_id = sqlc.arg(owner_user_id)
  AND is_system = FALSE
RETURNING id, owner_user_id, name, is_system, is_default, created_at, updated_at;

-- name: MoveLibraryAssetsToFolder :execrows
UPDATE library_assets
SET folder_id = sqlc.arg(target_folder_id), updated_at = now()
WHERE owner_user_id = sqlc.arg(owner_user_id)
  AND folder_id = sqlc.arg(source_folder_id);

-- name: DeleteLibraryFolder :execrows
DELETE FROM library_folders
WHERE id = sqlc.arg(id)
  AND owner_user_id = sqlc.arg(owner_user_id)
  AND is_system = FALSE;

-- name: ListLibraryAssets :many
SELECT
  assets.id,
  assets.owner_user_id,
  assets.folder_id,
  assets.provider,
  assets.source_id,
  assets.source_url,
  assets.preview_url,
  assets.original_url,
  assets.title,
  assets.author,
  assets.width,
  assets.height,
  assets.created_at,
  assets.updated_at
FROM library_assets AS assets
WHERE assets.owner_user_id = sqlc.arg(owner_user_id)
  AND (
    sqlc.narg(folder_id)::BIGINT IS NULL
    OR assets.folder_id = sqlc.narg(folder_id)::BIGINT
  )
  AND (
    sqlc.narg(cursor_created_at)::TIMESTAMPTZ IS NULL
    OR (assets.created_at, assets.id) < (
      sqlc.narg(cursor_created_at)::TIMESTAMPTZ,
      sqlc.narg(cursor_id)::BIGINT
    )
  )
ORDER BY assets.created_at DESC, assets.id DESC
LIMIT sqlc.arg(row_limit);

-- name: UpsertLibraryAsset :one
INSERT INTO library_assets (
  owner_user_id, folder_id, provider, source_id,
  source_url, preview_url, original_url, title, author, width, height
)
VALUES (
  sqlc.arg(owner_user_id), sqlc.arg(folder_id), sqlc.arg(provider), sqlc.arg(source_id),
  sqlc.arg(source_url), sqlc.arg(preview_url), sqlc.arg(original_url), sqlc.arg(title),
  sqlc.arg(author), sqlc.arg(width), sqlc.arg(height)
)
ON CONFLICT (owner_user_id, provider, source_id) DO UPDATE
SET folder_id = EXCLUDED.folder_id,
    updated_at = now()
RETURNING
  id, owner_user_id, folder_id, provider, source_id,
  source_url, preview_url, original_url, title, author,
  width, height, created_at, updated_at;

-- name: MoveLibraryAsset :one
UPDATE library_assets
SET folder_id = sqlc.arg(folder_id), updated_at = now()
WHERE id = sqlc.arg(id) AND owner_user_id = sqlc.arg(owner_user_id)
RETURNING
  id, owner_user_id, folder_id, provider, source_id,
  source_url, preview_url, original_url, title, author,
  width, height, created_at, updated_at;

-- name: DeleteLibraryAsset :execrows
DELETE FROM library_assets
WHERE id = sqlc.arg(id) AND owner_user_id = sqlc.arg(owner_user_id);

-- name: GetLibraryAssetByIDForOwner :one
SELECT
  assets.id,
  assets.owner_user_id,
  assets.folder_id,
  assets.provider,
  assets.source_id,
  assets.source_url,
  assets.preview_url,
  assets.original_url,
  assets.title,
  assets.author,
  assets.width,
  assets.height,
  assets.created_at,
  assets.updated_at
FROM library_assets AS assets
WHERE assets.id = sqlc.arg(id)
  AND assets.owner_user_id = sqlc.arg(owner_user_id);

-- name: LookupLibraryAssets :many
SELECT
  assets.id,
  assets.owner_user_id,
  assets.folder_id,
  assets.provider,
  assets.source_id,
  assets.source_url,
  assets.preview_url,
  assets.original_url,
  assets.title,
  assets.author,
  assets.width,
  assets.height,
  assets.created_at,
  assets.updated_at
FROM library_assets AS assets
WHERE assets.owner_user_id = sqlc.arg(owner_user_id)
  AND (assets.provider, assets.source_id) IN (
    SELECT lookup.provider, lookup.source_id
    FROM jsonb_to_recordset(sqlc.arg(keys)::JSONB)
      AS lookup(provider TEXT, source_id TEXT)
  );
