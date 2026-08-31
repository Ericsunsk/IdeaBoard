-- Canonical workflow slugs intentionally drop provider/product prefixes.
-- This is a clean-break migration; the published baseline remains unchanged.

ALTER TABLE feature_prompts
  DROP CONSTRAINT IF EXISTS ck_feature_prompts_feature_slug;

UPDATE feature_prompts
SET feature_slug = CASE feature_slug
  WHEN 'prompt-generation' THEN 'generation'
  WHEN 'style-fusion' THEN 'fusion'
  WHEN 'hd-upscale' THEN 'upscale'
  ELSE feature_slug
END
WHERE feature_slug IN ('prompt-generation', 'style-fusion', 'hd-upscale');

ALTER TABLE feature_prompts
  ADD CONSTRAINT ck_feature_prompts_feature_slug
  CHECK (feature_slug IN ('generation', 'inpaint', 'render', 'variations', 'cutout', 'fusion', 'upscale', 'outpaint'));

UPDATE provider_endpoint_capabilities
SET capability = CASE capability
  WHEN 'image-generation' THEN 'generation'
  WHEN 'image-fill' THEN 'fill'
  WHEN 'hd-upscale' THEN 'upscale'
  ELSE capability
END
WHERE capability IN ('image-generation', 'image-fill', 'hd-upscale');

-- Rewrite persisted task facts and payloads to the v5 contract. The params
-- column is JSON text in the published schema, so key/value replacements are
-- performed only for exact JSON tokens.
UPDATE tasks
SET feature_slug = CASE feature_slug
  WHEN 'prompt-generation' THEN 'generation'
  WHEN 'style-fusion' THEN 'fusion'
  WHEN 'hd-upscale' THEN 'upscale'
  ELSE feature_slug
END,
    params = regexp_replace(
      regexp_replace(
        regexp_replace(
          regexp_replace(
            regexp_replace(
              regexp_replace(params, '"schema_version"\\s*:\\s*4', '"schema_version":5', 'g'),
              '"prompt-generation"', '"generation"', 'g'),
            '"style-fusion"', '"fusion"', 'g'),
          '"hd-upscale"', '"upscale"', 'g'),
        '"image-generation"', '"generation"', 'g'),
      '"image-fill"', '"fill"', 'g'),
      '"hd"\\s*:', '"upscale":', 'g');

UPDATE task_leases
SET feature_slug = CASE feature_slug
  WHEN 'prompt-generation' THEN 'generation'
  WHEN 'style-fusion' THEN 'fusion'
  WHEN 'hd-upscale' THEN 'upscale'
  ELSE feature_slug
END;

UPDATE task_daily_stats
SET feature_slug = CASE feature_slug
  WHEN 'prompt-generation' THEN 'generation'
  WHEN 'style-fusion' THEN 'fusion'
  WHEN 'hd-upscale' THEN 'upscale'
  ELSE feature_slug
END;

UPDATE task_stat_events
SET feature_slug = CASE feature_slug
  WHEN 'prompt-generation' THEN 'generation'
  WHEN 'style-fusion' THEN 'fusion'
  WHEN 'hd-upscale' THEN 'upscale'
  ELSE feature_slug
END;

UPDATE task_billing_snapshots
SET feature_slug = CASE feature_slug
  WHEN 'prompt-generation' THEN 'generation'
  WHEN 'style-fusion' THEN 'fusion'
  WHEN 'hd-upscale' THEN 'upscale'
  ELSE feature_slug
END
WHERE feature_slug IN ('prompt-generation', 'style-fusion', 'hd-upscale');
