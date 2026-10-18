-- Workspace-scoped Boards grant one common role to explicitly selected members.
CREATE TABLE boards (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at TIMESTAMPTZ NULL,
  -- Implicit board owner (creator). Nullable so the additive migration
  -- applies where boards rows already exist; the WorkspaceBoardStore
  -- postgres implementation always sets it on insert and reads owner-less
  -- rows as not found (fail-closed). The creator never needs a
  -- board_memberships row (see v1_workspace.go).
  owner_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  -- Sharing is disabled by none; viewer/editor applies only to selected members.
  workspace_access TEXT NOT NULL DEFAULT 'none',
  -- Access configuration changes atomically advance the independent ETag version.
  access_version BIGINT NOT NULL DEFAULT 1,
  CONSTRAINT uq_boards_public_id UNIQUE (public_id),
  -- Kernel prefix pin: mirrors contract.PrefixBoard; the SQL CHECK and
  -- contract.IsBoardID must accept the same shape.
  CONSTRAINT chk_boards_public_id CHECK (public_id LIKE 'board\_%'),
  CONSTRAINT chk_boards_title CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  CONSTRAINT chk_boards_workspace_access CHECK (workspace_access IN ('none', 'viewer', 'editor')),
  CONSTRAINT chk_boards_access_version CHECK (access_version > 0)
);

CREATE INDEX idx_boards_workspace ON boards (workspace_id, created_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_boards_archived ON boards (workspace_id, archived_at DESC) WHERE archived_at IS NOT NULL;

-- Board document snapshots with optimistic concurrency. The etag is a hash
-- of (document_version || snapshot_data) for conflict detection. Document
-- schema evolution uses a versioned discriminated union (not modeled here).
-- content_version is the opaque identity of the accepted persisted content
-- (hex SHA-256 of the exact stored envelope bytes, assigned in the
-- accepting transaction; see boards/previewscene/contract.md §7).
-- preview_scene is the stored versioned Board Preview Scene projection for
-- the Gallery catalog (NULL when the document schema is not extractable;
-- backfilled or derived on read for pre-cutover rows). The catalog list
-- reads these two columns and never snapshot_data.
CREATE TABLE board_documents (
  board_id BIGINT PRIMARY KEY REFERENCES boards(internal_id) ON DELETE CASCADE,
  document_version INT NOT NULL DEFAULT 1,
  snapshot_data JSONB NOT NULL,
  -- Envelope schema version denormalized from snapshot_data: the Gallery
  -- catalog serves it without reading (detoasting) full documents.
  -- Assigned in the accepting transaction from the validated envelope;
  -- pre-cutover rows are backfilled from the canonical bytes by the
  -- migration that adds the column.
  schema_version INT NOT NULL,
  etag TEXT NOT NULL,
  content_version TEXT NULL,
  preview_scene JSONB NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT chk_board_documents_version_positive CHECK (document_version > 0),
  CONSTRAINT chk_board_documents_schema_version_positive CHECK (schema_version > 0),
  CONSTRAINT chk_board_documents_etag CHECK (length(etag) BETWEEN 8 AND 128),
  CONSTRAINT chk_board_documents_content_version CHECK (content_version IS NULL OR length(content_version) BETWEEN 8 AND 128)
);

-- Immutable revision history. Each write creates a new revision referencing
-- the prior one. Revision data may be delta or full snapshot (not prescribed).
CREATE TABLE board_revisions (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  board_id BIGINT NOT NULL REFERENCES boards(internal_id) ON DELETE CASCADE,
  revision_number INT NOT NULL,
  author_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  parent_revision_id BIGINT NULL REFERENCES board_revisions(internal_id) ON DELETE RESTRICT,
  revision_data JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_board_revisions_public_id UNIQUE (public_id),
  CONSTRAINT uq_board_revisions_board_number UNIQUE (board_id, revision_number),
  CONSTRAINT chk_board_revisions_public_id CHECK (public_id LIKE 'rev\_%'),
  CONSTRAINT chk_board_revisions_number_positive CHECK (revision_number > 0)
);

CREATE INDEX idx_board_revisions_board_created ON board_revisions (board_id, created_at DESC);
CREATE INDEX idx_board_revisions_author ON board_revisions (author_user_id, created_at DESC);

-- Selected Workspace members. Their common role lives on boards.workspace_access.
CREATE TABLE board_memberships (
  board_id BIGINT NOT NULL REFERENCES boards(internal_id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (board_id, user_id)
);

CREATE INDEX idx_board_memberships_user ON board_memberships (user_id, board_id);

-- Board document media references (#936). One row per (board, media asset)
-- accepted into the board's persisted document snapshot: the durable join
-- fact behind asset authorization for board readers. Reconciled in the SAME
-- transaction as every document accept (ReplaceDocument): newly referenced
-- asset ids are upserted, ids no longer present are deleted. Multi-source
-- authorization stays independent — this table carries only the document
-- source; the upload-target source lives on upload_sessions.target_board_id
-- and owner/library/task sources on their own rows — so losing one board's
-- access (or one row here) never revokes an independent grant. Board removal
-- cascades; asset removal cascades; archive/restore change no row because
-- authorization revalidates the live board role on every read.
CREATE TABLE board_document_media_refs (
  board_id BIGINT NOT NULL REFERENCES boards(internal_id) ON DELETE CASCADE,
  media_asset_id BIGINT NOT NULL REFERENCES media_assets(internal_id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (board_id, media_asset_id)
);

CREATE INDEX idx_board_document_media_refs_asset ON board_document_media_refs (media_asset_id, board_id);

-- Restricted upload target (#936). The board a Canvas/Library/Search upload
-- is captured for, recorded at session create (pre-complete) and verified
-- by the handler against the caller's board write access — never trusted
-- from the client. NULL means no board binding yet (e.g. the board is not
-- autosaved): the resulting asset stays owner-only until a board document
-- accept, library save, or task output grants wider access, so there is
-- never a full-workspace readable window. Cleared when the board row goes
-- away; the asset itself survives (workspace-scoped immutability).
ALTER TABLE upload_sessions
  ADD COLUMN target_board_id BIGINT NULL REFERENCES boards(internal_id) ON DELETE SET NULL;
CREATE INDEX idx_upload_sessions_target_board ON upload_sessions (target_board_id) WHERE target_board_id IS NOT NULL;
