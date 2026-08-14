UPDATE feature_prompts
SET feature_slug = 'prompt-generation'
WHERE feature_slug = 'pattern-gen';

ALTER TABLE feature_prompts
  DROP CONSTRAINT ck_feature_prompts_feature_slug;

ALTER TABLE feature_prompts
  ADD CONSTRAINT ck_feature_prompts_feature_slug
  CHECK (feature_slug IN ('prompt-generation', 'sketch-to-image', 'style-variation', 'partial-edit', 'remove-bg', 'style-fusion', 'hd-upscale', 'image-extend'));
