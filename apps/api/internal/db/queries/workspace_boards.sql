-- Workspace-scoped Boards use one common role and explicit selected members.

-- name: CreateWorkspaceBoard :one
INSERT INTO boards (public_id, workspace_id, owner_user_id, title)
SELECT sqlc.arg(public_id), w.internal_id, sqlc.arg(owner_user_id), sqlc.arg(title)
FROM workspaces AS w
WHERE w.public_id = sqlc.arg(workspace_public_id)
RETURNING internal_id, public_id, owner_user_id, title, workspace_access, access_version, created_at, updated_at, archived_at;

-- name: GetWorkspaceBoard :one
SELECT b.internal_id, b.public_id, w.public_id AS workspace_public_id,
  b.owner_user_id, b.title, b.workspace_access, b.access_version,
  b.created_at, b.updated_at, b.archived_at,
  d.document_version, d.schema_version, d.snapshot_data,
  d.content_version, d.preview_scene
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
INNER JOIN board_documents AS d ON d.board_id = b.internal_id
WHERE w.public_id = sqlc.arg(workspace_public_id) AND b.public_id = sqlc.arg(board_public_id)
LIMIT 1;

-- name: GetBoardByPublicID :one
-- Global presence-ticket lookup: board public ids are globally unique
-- (uq_boards_public_id), so the realtime admit path resolves the owning
-- workspace from the ticket bid alone without a wid query param.
SELECT b.internal_id, b.public_id, w.public_id AS workspace_public_id,
  b.owner_user_id, b.title, b.workspace_access, b.access_version,
  b.created_at, b.updated_at, b.archived_at,
  d.document_version, d.schema_version, d.snapshot_data,
  d.content_version, d.preview_scene
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
INNER JOIN board_documents AS d ON d.board_id = b.internal_id
WHERE b.public_id = sqlc.arg(board_public_id)
LIMIT 1;

-- name: ListWorkspaceBoardCatalog :many
-- Limit authorized identifiers before joining scene bodies. The count row
-- preserves offset validation even when the requested page is empty. The
-- predicate mirrors ResolveRole: current members read their own Boards or
-- selected Boards with a configured Viewer/Editor role.
WITH authorized AS MATERIALIZED (
  SELECT b.internal_id, b.updated_at, b.public_id,
    (m.user_id IS NOT NULL) AS has_direct_grant,
    CASE
      WHEN b.owner_user_id = sqlc.arg(user_id) THEN 'owner'
      WHEN m.user_id IS NOT NULL AND b.workspace_access IN ('viewer', 'editor') THEN b.workspace_access
      ELSE ''
    END AS effective_role
  FROM boards AS b
  INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
  INNER JOIN board_documents AS d ON d.board_id = b.internal_id
  LEFT JOIN board_memberships AS m ON m.board_id = b.internal_id AND m.user_id = sqlc.arg(user_id)
  WHERE w.public_id = sqlc.arg(workspace_public_id)
    AND (b.archived_at IS NULL OR sqlc.arg(include_archived))
    AND b.owner_user_id IS NOT NULL
    AND sqlc.arg(is_workspace_member)
    AND (
      EXISTS (SELECT 1 FROM workspace_memberships AS wm WHERE wm.workspace_id = w.internal_id AND wm.user_id = sqlc.arg(user_id) AND btrim(wm.role) IN ('owner', 'admin', 'member'))
      OR EXISTS (
        SELECT 1 FROM organization_memberships AS om
        WHERE om.organization_id = w.organization_id
          AND w.organization_id IS NOT NULL
          AND om.user_id = sqlc.arg(user_id)
          AND btrim(om.role) IN ('owner', 'admin')
      )
    )
    AND (
      b.owner_user_id = sqlc.arg(user_id)
      OR (m.user_id IS NOT NULL AND b.workspace_access IN ('viewer', 'editor'))
    )
  ORDER BY b.updated_at DESC, b.public_id ASC
  LIMIT sqlc.arg(catalog_limit)
), page AS (
  SELECT internal_id, has_direct_grant, effective_role FROM authorized
  ORDER BY updated_at DESC, public_id ASC
  LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset)
), total AS (
  SELECT count(*) AS total_count FROM authorized
)
SELECT total.total_count,
  b.internal_id, b.public_id, w.public_id AS workspace_public_id,
  b.owner_user_id, b.title, b.workspace_access, b.access_version,
  b.created_at, b.updated_at, b.archived_at,
  d.document_version, d.schema_version, d.content_version, d.preview_scene,
  page.has_direct_grant, page.effective_role,
  EXISTS (SELECT 1 FROM board_memberships AS g WHERE g.board_id = b.internal_id) AS has_any_grants
FROM total
LEFT JOIN page ON true
LEFT JOIN boards AS b ON b.internal_id = page.internal_id
LEFT JOIN workspaces AS w ON w.internal_id = b.workspace_id
LEFT JOIN board_documents AS d ON d.board_id = b.internal_id
ORDER BY b.updated_at DESC, b.public_id ASC;

-- name: RenameWorkspaceBoard :execrows
UPDATE boards AS b
SET title = sqlc.arg(title), updated_at = now()
FROM workspaces AS w
WHERE w.internal_id = b.workspace_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND b.archived_at IS NULL;

-- name: ArchiveWorkspaceBoard :execrows
UPDATE boards AS b
SET archived_at = now(), updated_at = now()
FROM workspaces AS w
WHERE w.internal_id = b.workspace_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND b.archived_at IS NULL;

-- name: RestoreWorkspaceBoard :execrows
UPDATE boards AS b
SET archived_at = NULL, updated_at = now()
FROM workspaces AS w
WHERE w.internal_id = b.workspace_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND b.archived_at IS NOT NULL;

-- name: TouchWorkspaceBoard :exec
UPDATE boards SET updated_at = now() WHERE internal_id = sqlc.arg(internal_id);

-- name: InsertBoardDocument :exec
INSERT INTO board_documents (board_id, document_version, snapshot_data, schema_version, etag, content_version, preview_scene)
VALUES (sqlc.arg(board_id), sqlc.arg(document_version), sqlc.arg(snapshot_data), sqlc.arg(schema_version), sqlc.arg(etag), sqlc.arg(content_version), sqlc.arg(preview_scene));

-- name: LockWorkspaceBoard :one
SELECT b.internal_id, b.archived_at, d.document_version
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
INNER JOIN board_documents AS d ON d.board_id = b.internal_id
WHERE w.public_id = sqlc.arg(workspace_public_id) AND b.public_id = sqlc.arg(board_public_id)
LIMIT 1
FOR UPDATE;

-- name: GetWorkspaceBoardAccess :one
SELECT b.owner_user_id, b.workspace_access, b.access_version, b.archived_at
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id) AND b.public_id = sqlc.arg(board_public_id)
LIMIT 1;

-- name: LockWorkspaceBoardAccess :one
-- Configuration writes lock Workspace membership before the Board row.
SELECT b.internal_id, b.owner_user_id, b.workspace_access, b.access_version, b.archived_at
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id) AND b.public_id = sqlc.arg(board_public_id)
LIMIT 1
FOR UPDATE;

-- name: SetBoardWorkspaceAccess :execrows
-- Version-guarded access transition: bumps access_version exactly once.
-- Zero rows means the board vanished or a concurrent writer moved the
-- version first (callers distinguish via a re-read).
UPDATE boards AS b
SET workspace_access = sqlc.arg(workspace_access), access_version = access_version + 1, updated_at = now()
FROM workspaces AS w
WHERE w.internal_id = b.workspace_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND b.access_version = sqlc.arg(expected_version)
  AND b.archived_at IS NULL;

-- name: ClearBoardMemberships :execrows
-- Configuration replacement removes previous selections in the accepting transaction.
DELETE FROM board_memberships AS m
USING boards AS b, workspaces AS w
WHERE m.board_id = b.internal_id
  AND w.internal_id = b.workspace_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id);

-- name: ListWorkspaceMemberBoardGrants :many
-- Every currently readable Board affected by the membership departure.
SELECT b.public_id, b.access_version, b.archived_at IS NOT NULL AS is_archived
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
LEFT JOIN board_memberships AS m ON m.board_id = b.internal_id AND m.user_id = sqlc.arg(user_id)
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND (b.owner_user_id = sqlc.arg(user_id)
    OR (m.user_id IS NOT NULL AND b.workspace_access IN ('viewer', 'editor')))
ORDER BY b.public_id ASC;

-- name: LockWorkspaceMemberBoards :many
-- Lock affected Boards in a stable order before removing grants.
SELECT b.internal_id, b.public_id, b.access_version, b.owner_user_id
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = ANY(sqlc.arg(board_public_ids)::text[])
ORDER BY b.public_id ASC
FOR UPDATE;

-- name: BumpWorkspaceMemberBoardsAccess :many
-- Every changed grant advances its Board access version, including archived Boards.
UPDATE boards AS b
SET access_version = access_version + 1, updated_at = now()
FROM workspaces AS w
WHERE w.internal_id = b.workspace_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = ANY(sqlc.arg(board_public_ids)::text[])
RETURNING b.public_id, b.access_version;

-- name: DeleteWorkspaceMemberBoardGrants :execrows
-- Removes every direct grant of one user across the workspace in the
-- same transaction as the version bumps above: rejoining later finds no
-- stale row to resurrect (fail-closed), while the serve-time membership
-- precondition already denies the departed member before this cleanup
-- lands. Board/document/Task/asset rows are untouched.
DELETE FROM board_memberships AS m
USING boards AS b, workspaces AS w
WHERE m.board_id = b.internal_id
  AND w.internal_id = b.workspace_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND m.user_id = sqlc.arg(user_id);

-- name: UpdateBoardDocument :execrows
UPDATE board_documents
SET document_version = sqlc.arg(document_version), snapshot_data = sqlc.arg(snapshot_data), schema_version = sqlc.arg(schema_version), etag = sqlc.arg(etag),
  content_version = sqlc.arg(content_version), preview_scene = sqlc.arg(preview_scene), updated_at = now()
WHERE board_id = sqlc.arg(board_id) AND document_version = sqlc.arg(expected_version);

-- name: ScanBoardDocumentsMissingPreview :many
-- Operator backfill sweep (backfill-board-previews): internal-id ordered
-- pages of rows whose content identity was never assigned. Internal-id
-- keyed like TouchWorkspaceBoard/InsertBoardDocument: ids come from the
-- scan itself, never from caller input (see the scope-check allowlist).
SELECT d.board_id, d.snapshot_data
FROM board_documents AS d
WHERE d.content_version IS NULL
ORDER BY d.board_id ASC
LIMIT sqlc.arg(row_limit);

-- name: UpdateBoardDocumentPreview :execrows
-- Operator backfill write: assigns the content identity (and the scene
-- when the stored document is extractable) without touching the document
-- version, the snapshot bytes, or board ordering. Internal-id keyed like
-- UpdateBoardDocument.
UPDATE board_documents
SET content_version = sqlc.arg(content_version), preview_scene = sqlc.arg(preview_scene)
WHERE board_id = sqlc.arg(board_id) AND content_version IS NULL;

-- name: InsertBoardRevision :one
INSERT INTO board_revisions (public_id, board_id, revision_number, author_user_id, parent_revision_id, revision_data)
VALUES (sqlc.arg(public_id), sqlc.arg(board_id), sqlc.arg(revision_number), sqlc.arg(author_user_id), sqlc.arg(parent_revision_id), sqlc.arg(revision_data))
RETURNING internal_id, created_at;

-- name: GetBoardRevisionByNumber :one
SELECT r.internal_id, r.revision_number, (r.revision_data ->> 'schema_version')::INT AS schema_version,
  r.revision_data, COALESCE(p.revision_number, 0) AS source_revision, r.author_user_id, r.created_at
FROM board_revisions AS r
INNER JOIN boards AS b ON b.internal_id = r.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
LEFT JOIN board_revisions AS p ON p.internal_id = r.parent_revision_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND r.revision_number = sqlc.arg(revision_number)
LIMIT 1;

-- name: GetBoardRevisionByID :one
SELECT r.internal_id, r.revision_number, (r.revision_data ->> 'schema_version')::INT AS schema_version,
  r.revision_data, COALESCE(p.revision_number, 0) AS source_revision, r.author_user_id, r.created_at
FROM board_revisions AS r
INNER JOIN boards AS b ON b.internal_id = r.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
LEFT JOIN board_revisions AS p ON p.internal_id = r.parent_revision_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND r.internal_id = sqlc.arg(internal_id)
LIMIT 1;

-- name: ListBoardRevisions :many
SELECT r.internal_id, r.revision_number, (r.revision_data ->> 'schema_version')::INT AS schema_version,
  r.revision_data, COALESCE(p.revision_number, 0) AS source_revision, r.author_user_id, r.created_at
FROM board_revisions AS r
INNER JOIN boards AS b ON b.internal_id = r.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
LEFT JOIN board_revisions AS p ON p.internal_id = r.parent_revision_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
ORDER BY r.revision_number ASC
LIMIT sqlc.arg(row_limit);

-- name: GetWorkspaceBoardMembership :one
SELECT m.user_id
FROM board_memberships AS m
INNER JOIN boards AS b ON b.internal_id = m.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND m.user_id = sqlc.arg(user_id)
LIMIT 1;

-- name: ListWorkspaceBoardMemberships :many
SELECT m.user_id, m.created_at
FROM board_memberships AS m
INNER JOIN boards AS b ON b.internal_id = m.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND m.user_id <> COALESCE(b.owner_user_id, -1)
ORDER BY m.user_id ASC
LIMIT sqlc.arg(row_limit);

-- name: InsertWorkspaceBoardMembershipIgnore :execrows
INSERT INTO board_memberships (board_id, user_id)
SELECT b.internal_id, sqlc.arg(user_id)
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
ON CONFLICT (board_id, user_id) DO NOTHING;

-- UpsertBoardDocumentMediaRef records one accepted board -> media asset
-- reference (#936: document source). Reconciled in the SAME transaction as
-- the document accept: the board and the asset must both resolve inside
-- the calling workspace or no row is written (fail-closed, never
-- cross-workspace). Replays converge via ON CONFLICT DO NOTHING.
-- name: UpsertBoardDocumentMediaRef :exec
INSERT INTO board_document_media_refs (board_id, media_asset_id)
SELECT b.internal_id, m.internal_id
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
INNER JOIN media_assets AS m ON m.workspace_id = w.internal_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND m.public_id = sqlc.arg(asset_public_id)
ON CONFLICT (board_id, media_asset_id) DO NOTHING;

-- DeleteBoardDocumentMediaRefs clears every accepted document reference
-- for one board in the calling workspace (#936). ReplaceDocument calls it
-- before re-inserting the accepted set, so removed references stop
-- authorizing in the same accepting transaction while upload-target,
-- owner, library, task, and other-board sources stay untouched.
-- name: DeleteBoardDocumentMediaRefs :exec
DELETE FROM board_document_media_refs AS r
USING boards AS b, workspaces AS w
WHERE r.board_id = b.internal_id
  AND w.internal_id = b.workspace_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id);

-- ListBoardsReferencingMediaAsset lists the workspace-scoped board public
-- ids whose accepted document references one asset (#936: board-reference
-- source, read from board_document_media_refs). Served through the narrow
-- BoardMediaRefReader port: Media authorization resolves each board through
-- RoleReader and only boards the caller can read authorize. Unknown or
-- cross-workspace assets list empty.
-- name: ListBoardsReferencingMediaAsset :many
SELECT b.public_id
FROM media_assets AS m
INNER JOIN workspaces AS w ON w.internal_id = m.workspace_id
INNER JOIN board_document_media_refs AS r ON r.media_asset_id = m.internal_id
INNER JOIN boards AS b ON b.internal_id = r.board_id
WHERE m.public_id = sqlc.arg(public_id) AND w.public_id = sqlc.arg(workspace_public_id)
ORDER BY b.public_id ASC;

-- Y.Doc collaboration update log (ADR-0020). Updates are opaque CRDT bytes
-- written by the realtime sidecar; the Go side never decodes them. Compaction
-- posts a sanitized snapshot that lands in board_documents + board_revisions
-- in one transaction and then trims this log (DeleteBoardYUpdates).

-- name: InsertBoardYUpdate :one
INSERT INTO board_y_updates (board_id, author_user_id, update_data)
SELECT b.internal_id, sqlc.arg(author_user_id), sqlc.arg(update_data)
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
RETURNING internal_id, created_at;

-- name: ListBoardYUpdates :many
SELECT u.internal_id, u.author_user_id, u.update_data, u.created_at
FROM board_y_updates AS u
INNER JOIN boards AS b ON b.internal_id = u.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
ORDER BY u.internal_id ASC
LIMIT sqlc.arg(row_limit);

-- name: CountBoardYUpdates :one
SELECT count(*)
FROM board_y_updates AS u
INNER JOIN boards AS b ON b.internal_id = u.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id);

-- name: DeleteBoardYUpdates :exec
DELETE FROM board_y_updates AS u
USING boards AS b, workspaces AS w
WHERE u.board_id = b.internal_id
  AND w.internal_id = b.workspace_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id);
