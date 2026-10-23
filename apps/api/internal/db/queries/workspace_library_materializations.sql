-- Workspace library materialization jobs (split from workspace_library.sql).
-- Status words: the public API exposes pending/running/completed/failed
-- while the durable CHECK stores pending/processing/completed/failed.
-- Scope guard: scripts/sql-workspace-scope-check.sh enforces one workspace_id key per query.

-- name: LockLibraryMaterializationAsset :exec
SELECT pg_advisory_xact_lock(hashtextextended(
  sqlc.arg(workspace_id)::bigint::text || ':' || sqlc.arg(library_asset_id)::bigint::text,
  0
));


-- name: GetActiveLibraryMaterializationByAsset :one
SELECT m.internal_id, m.public_id, m.workspace_id, m.library_asset_id,
  a.public_id AS asset_public_id,
  m.status, m.media_asset_id,
  ma.public_id AS media_asset_public_id,
  m.error_message, m.created_at, m.completed_at
FROM library_materializations AS m
INNER JOIN library_assets AS a ON a.internal_id = m.library_asset_id
LEFT JOIN media_assets AS ma ON ma.internal_id = m.media_asset_id
WHERE m.workspace_id = sqlc.arg(workspace_id)
  AND m.library_asset_id = sqlc.arg(library_asset_id)
  AND m.status IN ('pending', 'processing', 'completed')
ORDER BY m.created_at DESC, m.internal_id DESC
LIMIT 1;


-- name: CreateLibraryMaterialization :one
INSERT INTO library_materializations (public_id, workspace_id, library_asset_id, status)
VALUES (sqlc.arg(public_id), sqlc.arg(workspace_id), sqlc.arg(library_asset_id), 'pending')
RETURNING internal_id, public_id, workspace_id, library_asset_id, status, media_asset_id, error_message, created_at, completed_at;


-- name: GetLibraryMaterializationByPublicID :one
SELECT m.internal_id, m.public_id, m.workspace_id, m.library_asset_id,
  a.public_id AS asset_public_id,
  m.status, m.media_asset_id,
  ma.public_id AS media_asset_public_id,
  m.error_message, m.created_at, m.completed_at
FROM library_materializations AS m
INNER JOIN library_assets AS a ON a.internal_id = m.library_asset_id
LEFT JOIN media_assets AS ma ON ma.internal_id = m.media_asset_id
WHERE m.workspace_id = sqlc.arg(workspace_id)
  AND m.public_id = sqlc.arg(public_id)
LIMIT 1;

-- Transitions read their result back through the data-modifying
-- CTE's RETURNING list (joined to the untouched asset/media tables):
-- the outer statement shares the CTE snapshot, so re-scanning the
-- target table would return the pre-update row version.


-- name: StartLibraryMaterialization :one
WITH updated AS (
  UPDATE library_materializations AS m
  SET status = 'processing', error_message = ''
  WHERE m.workspace_id = sqlc.arg(workspace_id)
    AND m.public_id = sqlc.arg(public_id)
    AND m.status = 'pending'
  RETURNING m.internal_id, m.public_id, m.workspace_id, m.library_asset_id,
    m.status, m.media_asset_id, m.error_message, m.created_at, m.completed_at
)
SELECT u.internal_id, u.public_id, u.workspace_id, u.library_asset_id,
  a.public_id AS asset_public_id,
  u.status, u.media_asset_id,
  ma.public_id AS media_asset_public_id,
  u.error_message, u.created_at, u.completed_at
FROM updated AS u
INNER JOIN library_assets AS a ON a.internal_id = u.library_asset_id
LEFT JOIN media_assets AS ma ON ma.internal_id = u.media_asset_id
LIMIT 1;


-- name: CompleteLibraryMaterialization :one
WITH updated AS (
  UPDATE library_materializations AS m
  SET status = 'completed', media_asset_id = sqlc.arg(media_asset_id), error_message = '', completed_at = now()
  WHERE m.workspace_id = sqlc.arg(workspace_id)
    AND m.public_id = sqlc.arg(public_id)
    AND m.status IN ('pending', 'processing')
  RETURNING m.internal_id, m.public_id, m.workspace_id, m.library_asset_id,
    m.status, m.media_asset_id, m.error_message, m.created_at, m.completed_at
)
SELECT u.internal_id, u.public_id, u.workspace_id, u.library_asset_id,
  a.public_id AS asset_public_id,
  u.status, u.media_asset_id,
  ma.public_id AS media_asset_public_id,
  u.error_message, u.created_at, u.completed_at
FROM updated AS u
INNER JOIN library_assets AS a ON a.internal_id = u.library_asset_id
LEFT JOIN media_assets AS ma ON ma.internal_id = u.media_asset_id
LIMIT 1;


-- name: FailLibraryMaterialization :one
WITH updated AS (
  UPDATE library_materializations AS m
  SET status = 'failed', media_asset_id = NULL, error_message = sqlc.arg(error_message), completed_at = now()
  WHERE m.workspace_id = sqlc.arg(workspace_id)
    AND m.public_id = sqlc.arg(public_id)
    AND m.status IN ('pending', 'processing')
  RETURNING m.internal_id, m.public_id, m.workspace_id, m.library_asset_id,
    m.status, m.media_asset_id, m.error_message, m.created_at, m.completed_at
)
SELECT u.internal_id, u.public_id, u.workspace_id, u.library_asset_id,
  a.public_id AS asset_public_id,
  u.status, u.media_asset_id,
  ma.public_id AS media_asset_public_id,
  u.error_message, u.created_at, u.completed_at
FROM updated AS u
INNER JOIN library_assets AS a ON a.internal_id = u.library_asset_id
LEFT JOIN media_assets AS ma ON ma.internal_id = u.media_asset_id
LIMIT 1;


-- name: ListLibraryMaterializations :many
SELECT m.internal_id, m.public_id, m.workspace_id, m.library_asset_id,
  a.public_id AS asset_public_id,
  m.status, m.media_asset_id,
  ma.public_id AS media_asset_public_id,
  m.error_message, m.created_at, m.completed_at
FROM library_materializations AS m
INNER JOIN library_assets AS a ON a.internal_id = m.library_asset_id
LEFT JOIN media_assets AS ma ON ma.internal_id = m.media_asset_id
WHERE m.workspace_id = sqlc.arg(workspace_id)
ORDER BY m.created_at ASC, m.internal_id ASC
LIMIT sqlc.arg(row_limit);
