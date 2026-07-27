-- migrate:up
DROP TABLE feature_prompt_revisions;
DROP TABLE feature_prompts;

CREATE TABLE workflow_features (
  feature_slug VARCHAR(64) PRIMARY KEY,
  canonical_model VARCHAR(128) NOT NULL,
  system_prompt TEXT NOT NULL DEFAULT '',
  variables JSONB NOT NULL DEFAULT '[]'::jsonb,
  user_prompt_merge_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  updated_by BIGINT,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  CONSTRAINT ck_workflow_features_feature_slug CHECK (feature_slug IN ('sketch-to-image', 'style-variation', 'partial-edit', 'remove-bg', 'style-fusion', 'pattern-gen', 'hd-upscale', 'image-extend')),
  CONSTRAINT ck_workflow_features_model CHECK (btrim(canonical_model) <> ''),
  CONSTRAINT ck_workflow_features_variables_array CHECK (jsonb_typeof(variables) = 'array'),
  CONSTRAINT ck_workflow_features_merge_policy_object CHECK (jsonb_typeof(user_prompt_merge_policy) = 'object'),
  CONSTRAINT fk_workflow_features_updated_by FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TRIGGER set_workflow_features_updated_at BEFORE UPDATE ON workflow_features FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- migrate:down
-- statement:begin
DO $$
BEGIN
  RAISE EXCEPTION 'workflow_features migration is irreversible; restore a pre-migration backup instead';
END
$$;
-- statement:end
