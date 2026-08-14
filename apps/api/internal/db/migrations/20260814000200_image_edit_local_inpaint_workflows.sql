ALTER TABLE feature_prompts
  DROP CONSTRAINT ck_feature_prompts_feature_slug;

INSERT INTO feature_prompts (
  feature_slug,
  prompt,
  updated_by,
  created_at,
  updated_at
)
SELECT
  replacement.feature_slug,
  source.prompt,
  source.updated_by,
  source.created_at,
  source.updated_at
FROM feature_prompts AS source
CROSS JOIN (VALUES ('image-edit'), ('local-inpaint')) AS replacement(feature_slug)
WHERE source.feature_slug = 'partial-edit'
ON CONFLICT (feature_slug) DO NOTHING;

DELETE FROM feature_prompts
WHERE feature_slug = 'partial-edit';

ALTER TABLE feature_prompts
  ADD CONSTRAINT ck_feature_prompts_feature_slug
  CHECK (feature_slug IN ('prompt-generation', 'image-edit', 'local-inpaint', 'sketch-to-image', 'style-variation', 'remove-bg', 'style-fusion', 'hd-upscale', 'image-extend'));
