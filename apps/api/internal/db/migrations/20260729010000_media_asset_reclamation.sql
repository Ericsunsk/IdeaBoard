-- migrate:up
-- Media reclamation is deliberately reference-driven. Task and billing facts retain
-- their immutable media asset identity; only the stored object URL is retired.
ALTER TABLE media_assets
  ADD COLUMN reclaim_after TIMESTAMPTZ(3),
  ADD COLUMN reclaim_started_at TIMESTAMPTZ(3),
  ADD COLUMN reclaim_claim_token UUID,
  ADD COLUMN reclaim_generation BIGINT NOT NULL DEFAULT 0;

CREATE INDEX idx_media_assets_reclamation_due
  ON media_assets (reclaim_after, id)
  WHERE deleted_at IS NULL AND reclaim_after IS NOT NULL;

ALTER TABLE media_refs DROP CONSTRAINT uq_media_refs_ref;
ALTER TABLE media_refs
  ADD CONSTRAINT uq_media_refs_ref UNIQUE (media_asset_id, ref_type, ref_id, board_id, role);

-- Once an asset records a profile locator, changing that locator would make
-- delayed reclamation target a different object store. Credentials and access
-- domains remain operational metadata and can still rotate in place.
-- statement:begin
CREATE OR REPLACE FUNCTION prevent_referenced_storage_profile_locator_change()
RETURNS trigger AS $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM media_assets
    WHERE storage_profile_id = OLD.profile_id
  ) AND (
    NEW.provider IS DISTINCT FROM OLD.provider
    OR COALESCE(NEW.config->>'endpoint', '') IS DISTINCT FROM COALESCE(OLD.config->>'endpoint', '')
    OR COALESCE(NEW.config->>'bucket_name', '') IS DISTINCT FROM COALESCE(OLD.config->>'bucket_name', '')
    OR COALESCE(NEW.config->>'region_id', '') IS DISTINCT FROM COALESCE(OLD.config->>'region_id', '')
    OR COALESCE(NEW.config->>'force_path_style', 'false') IS DISTINCT FROM COALESCE(OLD.config->>'force_path_style', 'false')
  ) THEN
    RAISE EXCEPTION 'referenced storage profile locator is immutable; create a new profile';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- statement:end

CREATE TRIGGER prevent_referenced_storage_profile_locator_change
BEFORE UPDATE OF provider, config ON storage_profiles
FOR EACH ROW EXECUTE FUNCTION prevent_referenced_storage_profile_locator_change();

-- migrate:down
DROP TRIGGER IF EXISTS prevent_referenced_storage_profile_locator_change ON storage_profiles;
DROP FUNCTION IF EXISTS prevent_referenced_storage_profile_locator_change();

ALTER TABLE media_refs DROP CONSTRAINT uq_media_refs_ref;
ALTER TABLE media_refs
  ADD CONSTRAINT uq_media_refs_ref UNIQUE (media_asset_id, ref_type, ref_id, role);

DROP INDEX idx_media_assets_reclamation_due;
ALTER TABLE media_assets
  DROP COLUMN reclaim_generation,
  DROP COLUMN reclaim_claim_token,
  DROP COLUMN reclaim_started_at,
  DROP COLUMN reclaim_after;
