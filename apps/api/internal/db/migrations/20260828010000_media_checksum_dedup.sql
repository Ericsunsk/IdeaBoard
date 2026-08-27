CREATE UNIQUE INDEX uq_media_assets_owned_checksum
  ON media_assets (owner_user_id, storage_profile_id, checksum_sha256)
  WHERE checksum_sha256 <> '' AND status = 'ready' AND deleted_at IS NULL;
