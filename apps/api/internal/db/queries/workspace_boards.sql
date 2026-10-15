-- Workspace-rooted boards postgres store (#802 interface, #809 follow-up
-- to #817). boards.workspace_id is the workspace link (no separate link
-- table); board_revisions rows are INSERT-only, so no UPDATE/DELETE
-- revision query exists here by design. Every board-scoped query joins
-- workspaces on public_id so foreign-workspace rows stay invisible.
-- Scope guard: scripts/sql-workspace-scope-check.sh enforces one scope predicate per query.

-- name: CreateWorkspaceBoard :one
INSERT INTO boards (public_id, workspace_id, owner_user_id, title)
SELECT sqlc.arg(public_id), w.internal_id, sqlc.arg(owner_user_id), sqlc.arg(title)
FROM workspaces AS w
WHERE w.public_id = sqlc.arg(workspace_public_id)
RETURNING internal_id, public_id, owner_user_id, title, created_at, updated_at, archived_at;

-- name: GetWorkspaceBoard :one
SELECT b.internal_id, b.public_id, w.public_id AS workspace_public_id,
  b.owner_user_id, b.title, b.created_at, b.updated_at, b.archived_at,
  d.document_version, d.schema_version, d.snapshot_data,
  d.content_version, d.preview_scene
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
INNER JOIN board_documents AS d ON d.board_id = b.internal_id
WHERE w.public_id = sqlc.arg(workspace_public_id) AND b.public_id = sqlc.arg(board_public_id)
LIMIT 1;

-- name: ListWorkspaceBoardCatalog :many
-- Limit authorized identifiers before joining scene bodies. The count row
-- preserves offset validation even when the requested page is empty.
WITH authorized AS MATERIALIZED (
  SELECT b.internal_id, b.updated_at, b.public_id
  FROM boards AS b
  INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
  INNER JOIN board_documents AS d ON d.board_id = b.internal_id
  LEFT JOIN board_memberships AS m ON m.board_id = b.internal_id AND m.user_id = sqlc.arg(user_id)
  WHERE w.public_id = sqlc.arg(workspace_public_id)
    AND (b.archived_at IS NULL OR sqlc.arg(include_archived))
    AND b.owner_user_id IS NOT NULL
    AND (
      b.owner_user_id = sqlc.arg(user_id)
      OR (m.user_id IS NOT NULL AND btrim(m.role) IN ('owner', 'editor', 'viewer'))
      OR (m.user_id IS NULL AND sqlc.arg(workspace_role)::text IN ('owner', 'admin', 'member'))
    )
  ORDER BY b.updated_at DESC, b.public_id ASC
  LIMIT sqlc.arg(catalog_limit)
), page AS (
  SELECT internal_id FROM authorized
  ORDER BY updated_at DESC, public_id ASC
  LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset)
), total AS (
  SELECT count(*) AS total_count FROM authorized
)
SELECT total.total_count,
  b.internal_id, b.public_id, w.public_id AS workspace_public_id,
  b.owner_user_id, b.title, b.created_at, b.updated_at, b.archived_at,
  d.document_version, d.schema_version, d.content_version, d.preview_scene
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

-- name: GetWorkspaceBoardMembershipRole :one
SELECT m.role
FROM board_memberships AS m
INNER JOIN boards AS b ON b.internal_id = m.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND m.user_id = sqlc.arg(user_id)
LIMIT 1;

-- name: ListWorkspaceBoardMemberships :many
SELECT m.user_id, m.role, m.created_at
FROM board_memberships AS m
INNER JOIN boards AS b ON b.internal_id = m.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND m.user_id <> COALESCE(b.owner_user_id, -1)
ORDER BY m.user_id ASC
LIMIT sqlc.arg(row_limit);

-- name: InsertWorkspaceBoardMembershipIgnore :execrows
INSERT INTO board_memberships (board_id, user_id, role)
SELECT b.internal_id, sqlc.arg(user_id), sqlc.arg(role)
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
ON CONFLICT (board_id, user_id) DO NOTHING;

-- name: UpdateWorkspaceBoardMembershipRole :execrows
UPDATE board_memberships AS m
SET role = sqlc.arg(role)
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE m.board_id = b.internal_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND m.user_id = sqlc.arg(user_id);

-- name: DeleteWorkspaceBoardMembership :execrows
DELETE FROM board_memberships AS m
USING boards AS b, workspaces AS w
WHERE m.board_id = b.internal_id
  AND w.internal_id = b.workspace_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND m.user_id = sqlc.arg(user_id);

-- name: CountWorkspaceBoardOwners :one
SELECT COUNT(*)
FROM board_memberships AS m
INNER JOIN boards AS b ON b.internal_id = m.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND m.role = 'owner'
  AND m.user_id <> COALESCE(b.owner_user_id, -1);

-- name: InsertBoardShareToken :one
INSERT INTO board_share_tokens (public_id, board_id, token_id, token_secret, token_prefix, granted_role, created_by_user_id)
SELECT sqlc.arg(public_id), b.internal_id, sqlc.arg(token_id), sqlc.arg(token_secret), sqlc.arg(token_prefix), sqlc.arg(granted_role), sqlc.arg(created_by_user_id)
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
RETURNING internal_id, created_at;

-- name: ListBoardShareTokens :many
SELECT t.public_id, t.token_id, t.token_secret, t.token_prefix, t.granted_role,
  t.created_by_user_id, t.created_at, t.revoked_at, t.revoked_by_user_id, t.usage_count
FROM board_share_tokens AS t
INNER JOIN boards AS b ON b.internal_id = t.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
ORDER BY t.created_at DESC, t.internal_id DESC
LIMIT sqlc.arg(row_limit);

-- name: GetBoardShareTokenBySecret :one
SELECT t.internal_id, t.public_id, t.token_id, t.token_secret, t.token_prefix, t.granted_role,
  t.created_by_user_id, t.created_at, t.revoked_at, t.revoked_by_user_id, t.usage_count,
  b.internal_id AS board_internal_id, b.public_id AS board_public_id, b.archived_at AS board_archived_at,
  b.owner_user_id AS board_owner_user_id
FROM board_share_tokens AS t
INNER JOIN boards AS b ON b.internal_id = t.board_id
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE w.public_id = sqlc.arg(workspace_public_id)
  AND t.token_secret = sqlc.arg(token_secret)
LIMIT 1;

-- name: RevokeBoardShareToken :execrows
UPDATE board_share_tokens AS t
SET revoked_at = COALESCE(t.revoked_at, now()),
  revoked_by_user_id = COALESCE(t.revoked_by_user_id, sqlc.arg(revoked_by_user_id))
FROM boards AS b
INNER JOIN workspaces AS w ON w.internal_id = b.workspace_id
WHERE t.board_id = b.internal_id
  AND w.public_id = sqlc.arg(workspace_public_id)
  AND b.public_id = sqlc.arg(board_public_id)
  AND t.token_id = sqlc.arg(token_id);

-- name: IncrementBoardShareTokenUsage :exec
UPDATE board_share_tokens SET usage_count = usage_count + 1 WHERE internal_id = sqlc.arg(internal_id);

-- name: UpsertJoinBoardMembership :exec
INSERT INTO board_memberships (board_id, user_id, role)
VALUES (sqlc.arg(board_id), sqlc.arg(user_id), sqlc.arg(role))
ON CONFLICT (board_id, user_id) DO UPDATE SET role = EXCLUDED.role;
