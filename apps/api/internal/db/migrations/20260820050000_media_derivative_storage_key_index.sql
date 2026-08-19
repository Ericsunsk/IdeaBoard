ALTER TABLE media_derivatives
  DROP CONSTRAINT uq_media_derivatives_storage_key;

CREATE UNIQUE INDEX uq_media_derivatives_storage_key
  ON media_derivatives (storage_profile_id, storage_key)
  WHERE btrim(storage_key) <> '';
