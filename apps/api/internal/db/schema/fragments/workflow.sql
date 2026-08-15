CREATE TABLE feature_prompts (
  feature_slug VARCHAR(64) PRIMARY KEY,
  prompt TEXT NOT NULL DEFAULT '',
  updated_by BIGINT,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  CONSTRAINT ck_feature_prompts_feature_slug CHECK (feature_slug IN ('prompt-generation', 'local-inpaint', 'sketch-to-image', 'style-variation', 'remove-bg', 'style-fusion', 'hd-upscale', 'image-extend')),
  CONSTRAINT fk_feature_prompts_updated_by FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);
