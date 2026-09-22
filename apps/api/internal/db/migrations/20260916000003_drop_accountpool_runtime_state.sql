-- destructive: approved
-- ai-gateway T5: drop the retired account/lease runtime state. Tasks and
-- billing history stay; account/endpoint rows stay read-only for their
-- foreign keys. Secret material is purged in place (rows remain for history
-- joins); the gateway is the only credential holder afterwards.
DROP TABLE IF EXISTS task_attempts;
DROP TABLE IF EXISTS task_leases;
DROP TABLE IF EXISTS provider_endpoint_capabilities;
DROP TABLE IF EXISTS provider_endpoint_models;
DROP TABLE IF EXISTS api_account_models;

UPDATE api_account_secrets
SET secret_payload = '',
    fingerprint = '',
    key_version = '',
    status = 'retired',
    updated_at = now()
WHERE status <> 'retired';
