-- Upload sessions and Media Assets (#803). Upload sessions have explicit
-- lifecycle (pending → completed/expired/failed). Only successful completion
-- creates a durable Media Asset. Storage keys are never exposed in the API.
CREATE TABLE upload_sessions (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  uploader_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  declared_mime_type TEXT NOT NULL,
  declared_size_bytes BIGINT NOT NULL,
  declared_checksum TEXT NULL,
  storage_key TEXT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  completed_at TIMESTAMPTZ NULL,
  expired_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_upload_sessions_public_id UNIQUE (public_id),
  -- Kernel prefix pin: mirrors contract.PrefixUpload (see also
  -- contract.IsUploadID).
  CONSTRAINT chk_upload_sessions_public_id CHECK (public_id LIKE 'upload\_%'),
  CONSTRAINT chk_upload_sessions_size_positive CHECK (declared_size_bytes > 0),
  CONSTRAINT chk_upload_sessions_status CHECK (status IN ('pending','transferring','completed','expired','failed')),
  CONSTRAINT chk_upload_sessions_completion CHECK (
    (status = 'completed' AND completed_at IS NOT NULL AND storage_key IS NOT NULL)
    OR (status = 'expired' AND expired_at IS NOT NULL)
    OR (status IN ('pending','transferring','failed'))
  )
);

CREATE INDEX idx_upload_sessions_workspace ON upload_sessions (workspace_id, created_at DESC);
CREATE INDEX idx_upload_sessions_uploader ON upload_sessions (uploader_user_id, created_at DESC);
CREATE INDEX idx_upload_sessions_pending ON upload_sessions (created_at) WHERE status IN ('pending','transferring');

-- Media Assets are immutable references to stored content with typed metadata.
-- Assets are created only from successful upload completion. Content delivery
-- supports original and derivative variants without exposing storage_key.
CREATE TABLE media_assets (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  upload_session_id BIGINT NOT NULL REFERENCES upload_sessions(internal_id) ON DELETE RESTRICT,
  owner_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  storage_key TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL,
  checksum TEXT NOT NULL,
  media_kind TEXT NOT NULL,
  width INT NULL,
  height INT NULL,
  duration_ms INT NULL,
  metadata JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at TIMESTAMPTZ NULL,
  CONSTRAINT uq_media_assets_public_id UNIQUE (public_id),
  CONSTRAINT uq_media_assets_storage_key UNIQUE (storage_key),
  -- Kernel prefix pin: mirrors contract.PrefixAsset (see also
  -- contract.IsAssetID).
  CONSTRAINT chk_media_assets_public_id CHECK (public_id LIKE 'asset\_%'),
  CONSTRAINT chk_media_assets_size_positive CHECK (size_bytes > 0),
  CONSTRAINT chk_media_assets_media_kind CHECK (media_kind IN ('image','video','audio','document','other')),
  CONSTRAINT chk_media_assets_dimensions CHECK (
    (media_kind = 'image' AND width > 0 AND height > 0)
    OR (media_kind != 'image')
  ),
  CONSTRAINT chk_media_assets_metadata CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_media_assets_workspace ON media_assets (workspace_id, created_at DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_media_assets_owner ON media_assets (owner_user_id, created_at DESC);
CREATE INDEX idx_media_assets_upload ON media_assets (upload_session_id);

CREATE TABLE media_asset_task_origins (
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  task_public_id TEXT NOT NULL,
  output_index INT NOT NULL,
  media_asset_id BIGINT NOT NULL REFERENCES media_assets(internal_id) ON DELETE CASCADE,
  submitter_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  board_public_id TEXT NULL,
  CONSTRAINT media_asset_task_origins_pkey PRIMARY KEY (workspace_id, task_public_id, output_index),
  -- Kernel prefix pins: task_public_id mirrors contract.PrefixTask and
  -- board_public_id mirrors contract.PrefixBoard.
  CONSTRAINT chk_media_asset_task_origins_task CHECK (task_public_id LIKE 'task\_%'),
  CONSTRAINT chk_media_asset_task_origins_output_index CHECK (output_index >= 0),
  CONSTRAINT chk_media_asset_task_origins_board CHECK (board_public_id IS NULL OR board_public_id LIKE 'board\_%')
);

CREATE INDEX idx_media_asset_task_origins_asset ON media_asset_task_origins (media_asset_id);
