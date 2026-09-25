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

CREATE TABLE library_assets (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  folder_id BIGINT NOT NULL REFERENCES library_folders(internal_id) ON DELETE RESTRICT,
  provider TEXT NOT NULL,
  source_id TEXT NOT NULL,
  source_url TEXT NOT NULL,
  preview_url TEXT NOT NULL,
  original_url TEXT NOT NULL,
  media_asset_id BIGINT NULL REFERENCES media_assets(internal_id) ON DELETE RESTRICT,
  width INT NOT NULL DEFAULT 0,
  height INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_library_assets_public_id UNIQUE (public_id),
  CONSTRAINT uq_library_assets_workspace_source UNIQUE (workspace_id, provider, source_id),
  CONSTRAINT chk_library_assets_public_id CHECK (public_id LIKE 'lasset\_%'),
  CONSTRAINT chk_library_assets_provider CHECK (provider IN ('unsplash','pexels','workspace_media')),
  CONSTRAINT chk_library_assets_source_id CHECK (length(btrim(source_id)) > 0),
  CONSTRAINT chk_library_assets_dimensions CHECK (width >= 0 AND height >= 0),
  CONSTRAINT chk_library_assets_reference_shape CHECK (
    (provider = 'workspace_media' AND media_asset_id IS NOT NULL AND source_url = '' AND preview_url = '' AND original_url = '')
    OR (provider IN ('unsplash','pexels') AND media_asset_id IS NULL AND source_url <> '' AND preview_url <> '' AND original_url <> '')
  )
);

CREATE INDEX idx_library_assets_workspace ON library_assets (workspace_id, created_at DESC);
CREATE INDEX idx_library_assets_folder ON library_assets (folder_id, created_at DESC);

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

-- Legacy user-scoped board tables (deprecated, remove in #809)
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
