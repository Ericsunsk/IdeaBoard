-- AI gateway connection keys retirement: delete the six offline gateway
-- connection rows from system_configs. Gateway credentials now resolve
-- from Worker env; the repository no longer reads system settings for
-- connection data (see modelregistry/infrastructure/config_repository.go).
--
-- Deleted keys (exact match only, no LIKE prefix):
--   ai_gateway_base_url, ai_gateway_account_id, ai_gateway_gateway_id,
--   ai_gateway_token_ciphertext, ai_gateway_token_material_kind,
--   ai_gateway_updated_at
-- Retained keys (must NOT match this statement):
--   ai_gateway_vision_model, ai_gateway_model_catalog
--
-- Rollback: no automatic down migration (Atlas versioned, forward-only).
-- To restore, re-insert from the pre-deploy system_configs backup; the
-- admin audit trail does not retain credential ciphertext. Required pre-step:
-- dump system_configs before applying this migration on each environment.

DELETE FROM system_configs
WHERE config_key IN (
  'ai_gateway_base_url',
  'ai_gateway_account_id',
  'ai_gateway_gateway_id',
  'ai_gateway_token_ciphertext',
  'ai_gateway_token_material_kind',
  'ai_gateway_updated_at'
);
