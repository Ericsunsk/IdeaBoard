ALTER TABLE media_derivatives
  DROP CONSTRAINT ck_media_derivatives_tier;

ALTER TABLE media_derivatives
  ADD CONSTRAINT ck_media_derivatives_tier
  CHECK (tier IN ('thumb', 'preview', 'large-preview', 'processing'));
