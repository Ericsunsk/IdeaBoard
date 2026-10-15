-- Boards and collaboration (#802). Boards are workspace-scoped with separate
-- ACL from workspace membership. Document snapshots use optimistic concurrency
-- (ETag/If-Match). Revisions are immutable. Share tokens enable public join.
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
  CONSTRAINT uq_boards_public_id UNIQUE (public_id),
  CONSTRAINT chk_boards_public_id CHECK (public_id LIKE 'board\_%'),
  CONSTRAINT chk_boards_title CHECK (length(btrim(title)) BETWEEN 1 AND 200)
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

-- Board membership ACL, separate from workspace membership. Role enforcement
-- at application layer: owner > editor > viewer.
CREATE TABLE board_memberships (
  board_id BIGINT NOT NULL REFERENCES boards(internal_id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('owner','editor','viewer')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (board_id, user_id)
);

CREATE INDEX idx_board_memberships_user ON board_memberships (user_id, board_id);

-- Share tokens enable public join with configurable role. Token creation,
-- join, and revocation are idempotent commands (Idempotency-Key required).
CREATE TABLE board_share_tokens (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  board_id BIGINT NOT NULL REFERENCES boards(internal_id) ON DELETE CASCADE,
  token_secret TEXT NOT NULL,
  granted_role TEXT NOT NULL CHECK (granted_role IN ('editor','viewer')),
  created_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at TIMESTAMPTZ NULL,
  revoked_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  -- Opaque sht_ bearer id exposed in {token_id} URL segments. Nullable so
  -- the additive migration applies where token rows already exist; the
  -- store always sets it on insert. Only the bearer hash is kept in
  -- token_secret (never the bearer), so share secrets need no reversible
  -- encryption at rest.
  token_id TEXT NULL,
  token_prefix TEXT NOT NULL DEFAULT '',
  -- Join counter. The durable audit trail itself is a #808 follow-up.
  usage_count BIGINT NOT NULL DEFAULT 0,
  CONSTRAINT uq_board_share_tokens_public_id UNIQUE (public_id),
  CONSTRAINT uq_board_share_tokens_secret UNIQUE (token_secret),
  CONSTRAINT uq_board_share_tokens_token_id UNIQUE (token_id),
  CONSTRAINT chk_board_share_tokens_public_id CHECK (public_id LIKE 'bst\_%'),
  CONSTRAINT chk_board_share_tokens_secret CHECK (length(token_secret) >= 32),
  CONSTRAINT chk_board_share_tokens_token_id CHECK (token_id IS NULL OR token_id LIKE 'sht\_%'),
  CONSTRAINT chk_board_share_tokens_usage_count CHECK (usage_count >= 0),
  CONSTRAINT chk_board_share_tokens_revocation CHECK (
    (revoked_at IS NULL AND revoked_by_user_id IS NULL)
    OR (revoked_at IS NOT NULL AND revoked_by_user_id IS NOT NULL)
  )
);

CREATE INDEX idx_board_share_tokens_board ON board_share_tokens (board_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_board_share_tokens_secret ON board_share_tokens (token_secret) WHERE revoked_at IS NULL;
