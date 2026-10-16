CREATE TABLE library_folders (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  is_system BOOLEAN NOT NULL DEFAULT FALSE,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_library_folders_public_id UNIQUE (public_id),
  CONSTRAINT uq_library_folders_workspace_name UNIQUE (workspace_id, name),
  CONSTRAINT chk_library_folders_public_id CHECK (public_id LIKE 'lfolder\_%'),
  CONSTRAINT chk_library_folders_name CHECK (length(btrim(name)) BETWEEN 1 AND 100),
  CONSTRAINT chk_library_folders_system_shape CHECK (NOT is_system OR is_default)
);

CREATE INDEX idx_library_folders_workspace ON library_folders (workspace_id, created_at);

-- Library Source contract (issues #930/#931): one library_assets row is
-- either an external provider reference (source 'external') or a managed
-- Media save (source 'media', derived from media_asset_id IS NOT NULL;
-- no redundant source column). Media rows carry no provider/source
-- identity and no URLs (all NULL); their display dimensions are
-- projected at read time from the joined media_assets row (Media is the
-- dimension fact source; Library never copies them, so media width/height
-- stay NULL here). task_id/task_result_index are task provenance only
-- (never a dedup key, always paired); saved_by_user_id is the server-side
-- operator identity (RESTRICT, never client-supplied, never serialized).
-- Title/author are durable display metadata now (were validated-but-dropped
-- before this contract). Dedup identity: external rows on
-- (workspace_id, provider, source_id) WHERE media_asset_id IS NULL;
-- media rows on (workspace_id, media_asset_id) WHERE NOT NULL.
CREATE TABLE library_assets (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  folder_id BIGINT NOT NULL REFERENCES library_folders(internal_id) ON DELETE RESTRICT,
  provider TEXT NULL,
  source_id TEXT NULL,
  source_url TEXT NULL,
  preview_url TEXT NULL,
  original_url TEXT NULL,
  media_asset_id BIGINT NULL REFERENCES media_assets(internal_id) ON DELETE RESTRICT,
  title TEXT NOT NULL DEFAULT '',
  author TEXT NOT NULL DEFAULT '',
  width INT NULL,
  height INT NULL,
  task_id TEXT NULL,
  task_result_index INT NULL,
  saved_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_library_assets_public_id UNIQUE (public_id),
  CONSTRAINT chk_library_assets_public_id CHECK (public_id LIKE 'lasset\_%'),
  CONSTRAINT chk_library_assets_title CHECK (length(title) <= 255),
  CONSTRAINT chk_library_assets_author CHECK (length(author) <= 255),
  CONSTRAINT chk_library_assets_dimensions CHECK (
    (media_asset_id IS NULL AND width IS NOT NULL AND height IS NOT NULL AND width >= 0 AND height >= 0)
    OR (media_asset_id IS NOT NULL AND width IS NULL AND height IS NULL)
  ),
  CONSTRAINT chk_library_assets_lineage CHECK (
    (task_id IS NULL AND task_result_index IS NULL)
    OR (task_id IS NOT NULL AND task_id LIKE 'task\_%' AND task_result_index IS NOT NULL AND task_result_index >= 0)
  ),
  CONSTRAINT chk_library_assets_reference_shape CHECK (
    (media_asset_id IS NOT NULL
      AND provider IS NULL AND source_id IS NULL
      AND source_url IS NULL AND preview_url IS NULL AND original_url IS NULL)
    OR (media_asset_id IS NULL
      AND provider IN ('unsplash','pexels')
      AND source_id IS NOT NULL AND length(btrim(source_id)) > 0 AND length(source_id) <= 128
      AND source_url IS NOT NULL AND source_url <> ''
      AND preview_url IS NOT NULL AND preview_url <> ''
      AND original_url IS NOT NULL AND original_url <> '')
  )
);

-- Partial dedup identities: external rows by provider identity, media
-- rows by managed asset identity. Plain UNIQUE constraints cannot be
-- partial, so these are UNIQUE INDEXes by construction.
CREATE UNIQUE INDEX uq_library_assets_workspace_source
  ON library_assets (workspace_id, provider, source_id)
  WHERE media_asset_id IS NULL;
CREATE UNIQUE INDEX uq_library_assets_workspace_media
  ON library_assets (workspace_id, media_asset_id)
  WHERE media_asset_id IS NOT NULL;

CREATE INDEX idx_library_assets_workspace ON library_assets (workspace_id, created_at DESC);
CREATE INDEX idx_library_assets_folder ON library_assets (folder_id, created_at DESC);
CREATE INDEX idx_library_assets_media_asset ON library_assets (media_asset_id)
  WHERE media_asset_id IS NOT NULL;

-- Materialization creates a durable job that eventually produces a Media Asset.
CREATE TABLE library_materializations (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  library_asset_id BIGINT NOT NULL REFERENCES library_assets(internal_id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'pending',
  media_asset_id BIGINT NULL REFERENCES media_assets(internal_id) ON DELETE RESTRICT,
  error_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ NULL,
  CONSTRAINT uq_library_materializations_public_id UNIQUE (public_id),
  CONSTRAINT chk_library_materializations_public_id CHECK (public_id LIKE 'lmat\_%'),
  CONSTRAINT chk_library_materializations_status CHECK (status IN ('pending','processing','completed','failed')),
  CONSTRAINT chk_library_materializations_completion CHECK (
    (status = 'completed' AND media_asset_id IS NOT NULL AND completed_at IS NOT NULL)
    OR (status = 'failed' AND completed_at IS NOT NULL)
    OR (status IN ('pending','processing'))
  )
);

CREATE INDEX idx_library_materializations_workspace ON library_materializations (workspace_id, created_at DESC);
CREATE INDEX idx_library_materializations_library_asset ON library_materializations (library_asset_id);
CREATE INDEX idx_library_materializations_status ON library_materializations (status, created_at);

-- A Library asset is URL-backed until a user imports it. This association is
-- intentionally not a media_ref: only actual Canvas/Task use keeps Media
-- alive, so unused imports remain reclaimable.
CREATE TABLE library_asset_media_assets (
  library_asset_id BIGINT PRIMARY KEY,
  owner_user_id BIGINT NOT NULL,
  media_asset_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  CONSTRAINT uq_library_asset_media_assets_media_asset UNIQUE (media_asset_id)
);

CREATE INDEX idx_library_asset_media_assets_owner ON library_asset_media_assets (owner_user_id, media_asset_id);
