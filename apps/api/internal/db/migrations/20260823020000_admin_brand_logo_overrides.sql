CREATE TABLE admin_brand_logo_overrides (
  scope VARCHAR(32) NOT NULL,
  target_key VARCHAR(128) NOT NULL,
  logo_key VARCHAR(64) NOT NULL,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  CONSTRAINT pk_admin_brand_logo_overrides PRIMARY KEY (scope, target_key),
  CONSTRAINT ck_admin_brand_logo_overrides_scope CHECK (
    scope IN ('provider-endpoint', 'storage-profile', 'search-provider', 'oauth-provider')
  ),
  CONSTRAINT ck_admin_brand_logo_overrides_target_key CHECK (
    target_key ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$'
  ),
  CONSTRAINT ck_admin_brand_logo_overrides_logo_key CHECK (
    logo_key IN (
      'flux', 'midjourney', 'stability', 'fal', 'replicate', 'ideogram', 'recraft', 'runway',
      'zhipu', 'aliyun', 'doubao', 'moonshot', 'minimax', 'baidu', 'tencent', 'lingyi',
      'openai', 'gemini', 'anthropic', 'deepseek', 'cloudflare', 'siliconflow', 'mistral',
      'groq', 'together', 'fireworks', 'huggingface', 'perplexity', 'cohere', 'novita',
      'lepton', 'google', 'azure', 'bedrock', 'ollama', 'amazons3', 'cloudflare-r2',
      'alibabacloud', 'tencent-cos', 'minio', 'unsplash', 'pexels', 'github', 'google-oauth',
      'discord'
    )
  )
);

CREATE TRIGGER set_admin_brand_logo_overrides_updated_at
BEFORE UPDATE ON admin_brand_logo_overrides
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
