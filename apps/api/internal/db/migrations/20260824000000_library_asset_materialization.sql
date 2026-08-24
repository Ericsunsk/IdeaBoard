ALTER TABLE library_assets
  ADD CONSTRAINT uq_library_assets_id_owner UNIQUE (id, owner_user_id);

CREATE TABLE library_asset_media_assets (
  library_asset_id BIGINT PRIMARY KEY,
  owner_user_id BIGINT NOT NULL,
  media_asset_id BIGINT NOT NULL,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  CONSTRAINT uq_library_asset_media_assets_media_asset UNIQUE (media_asset_id),
  CONSTRAINT fk_library_asset_media_assets_library_owner FOREIGN KEY (library_asset_id, owner_user_id)
    REFERENCES library_assets(id, owner_user_id) ON DELETE CASCADE,
  CONSTRAINT fk_library_asset_media_assets_media_owner FOREIGN KEY (media_asset_id, owner_user_id)
    REFERENCES media_assets(id, owner_user_id) ON DELETE CASCADE
);

CREATE INDEX idx_library_asset_media_assets_owner ON library_asset_media_assets (owner_user_id, media_asset_id);
