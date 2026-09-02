ALTER TABLE media_derivatives
  ADD COLUMN IF NOT EXISTS source_object_generation BIGINT NOT NULL DEFAULT 0;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'ck_media_derivatives_source_object_generation'
      AND conrelid = 'media_derivatives'::regclass
  ) THEN
    ALTER TABLE media_derivatives
      ADD CONSTRAINT ck_media_derivatives_source_object_generation
      CHECK (source_object_generation >= 0);
  END IF;
END $$;

-- Existing ready rows predate source generation tracking. Mark them obsolete so
-- the next scheduling pass regenerates them against the current source.
UPDATE media_derivatives
SET status = 'obsolete', updated_at = now()
WHERE status = 'ready';
