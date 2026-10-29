-- destructive: approved
-- Service Model Pool provider/model split: pool entries store provider
-- (registry slug) plus the bare model name. No dual-write, no backfill:
-- dev databases are rebuilt (pre-launch, pool tables hold no retained
-- data); a non-empty pool table must be cleared before applying.
--
-- provider is the registry slug (lowercased at the write boundary;
-- unknown slugs synthesize the default custom-<slug> namespace at
-- execution, never vanish). model_id is now the bare model name (no @path,
-- no whitespace; nested slashes inside the bare name, e.g. openrouter's
-- vendor/model, are preserved).

ALTER TABLE service_model_pool_entry
  ADD COLUMN provider TEXT NOT NULL DEFAULT 'custom';

ALTER TABLE service_model_pool_entry
  ALTER COLUMN provider DROP DEFAULT;

ALTER TABLE service_model_pool_entry
  DROP CONSTRAINT uq_service_model_pool_entry_service_model;

ALTER TABLE service_model_pool_entry
  ADD CONSTRAINT uq_service_model_pool_entry_service_provider_model UNIQUE (service, provider, model_id);

ALTER TABLE service_model_pool_entry
  ADD CONSTRAINT ck_service_model_pool_entry_provider CHECK (length(btrim(provider)) BETWEEN 1 AND 64);

ALTER TABLE service_model_pool_entry
  ADD CONSTRAINT ck_service_model_pool_entry_provider_no_slash CHECK (provider NOT LIKE '%/%' AND provider NOT LIKE '%@%' AND provider NOT LIKE '% %');

ALTER TABLE service_model_pool_entry
  ADD CONSTRAINT ck_service_model_pool_entry_model_no_prefix CHECK (model_id NOT LIKE '%@%' AND model_id NOT LIKE '% %');

CREATE INDEX IF NOT EXISTS idx_service_model_pool_entry_service_provider ON service_model_pool_entry (service, provider);
