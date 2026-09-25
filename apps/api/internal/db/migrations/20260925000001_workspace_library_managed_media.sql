-- Workspace task outputs are managed Media Assets, not provider URL refs.
ALTER TABLE library_assets
  ADD COLUMN media_asset_id BIGINT NULL REFERENCES media_assets(internal_id) ON DELETE RESTRICT;

ALTER TABLE library_assets
  DROP CONSTRAINT chk_library_assets_provider,
  ADD CONSTRAINT chk_library_assets_provider
    CHECK (provider IN ('unsplash', 'pexels', 'workspace_media'));

ALTER TABLE library_assets
  ADD CONSTRAINT chk_library_assets_reference_shape CHECK (
    (provider = 'workspace_media' AND media_asset_id IS NOT NULL AND source_url = '' AND preview_url = '' AND original_url = '')
    OR (provider IN ('unsplash', 'pexels') AND media_asset_id IS NULL AND source_url <> '' AND preview_url <> '' AND original_url <> '')
  );

CREATE INDEX idx_library_assets_media_asset ON library_assets (media_asset_id)
  WHERE media_asset_id IS NOT NULL;
