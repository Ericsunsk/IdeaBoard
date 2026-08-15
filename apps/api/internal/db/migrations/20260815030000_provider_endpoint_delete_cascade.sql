-- A ProviderEndpoint owns its APIKey accounts and their model/secret configuration.
-- Deleting the endpoint should remove that subtree while task history remains protected
-- by the existing task/account foreign keys.

ALTER TABLE api_accounts
  DROP CONSTRAINT fk_api_accounts_provider_endpoint_id,
  ADD CONSTRAINT fk_api_accounts_provider_endpoint_id
    FOREIGN KEY (provider_endpoint_id)
    REFERENCES provider_endpoints(id)
    ON DELETE CASCADE;
