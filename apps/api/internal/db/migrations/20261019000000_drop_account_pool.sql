-- destructive: approved
-- Remove account_pool three tables (provider_endpoints/api_accounts/api_account_secrets).
-- The accountpool package is already gone with no production traffic; usage
-- dimension columns (provider_endpoint_id BIGINT, not FK) stay untouched.
DROP TABLE IF EXISTS api_account_secrets;
DROP TABLE IF EXISTS api_accounts;
DROP TABLE IF EXISTS provider_endpoints;
DROP FUNCTION IF EXISTS prevent_api_account_provider_endpoint_change();
