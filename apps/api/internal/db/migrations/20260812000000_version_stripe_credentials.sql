-- migrate:up

CREATE TABLE payment_stripe_credentials (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  account_id TEXT NOT NULL,
  mode TEXT NOT NULL,
  charges_enabled BOOLEAN NOT NULL,
  stripe_product_id TEXT NOT NULL,
  stripe_webhook_endpoint_id TEXT NOT NULL,
  api_key_ciphertext TEXT NOT NULL,
  webhook_secret_ciphertext TEXT NOT NULL,
  secret_material_kind TEXT NOT NULL,
  created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT ck_payment_stripe_credential_mode CHECK (mode IN ('test', 'live')),
  CONSTRAINT ck_payment_stripe_credential_material CHECK (secret_material_kind = 'encrypted'),
  CONSTRAINT ck_payment_stripe_credential_ids CHECK (
    char_length(account_id) BETWEEN 1 AND 255
    AND char_length(stripe_product_id) BETWEEN 1 AND 255
    AND char_length(stripe_webhook_endpoint_id) BETWEEN 1 AND 255
  )
);

ALTER TABLE payment_stripe_configurations
  ADD COLUMN active_credential_id UUID,
  ADD COLUMN revision BIGINT NOT NULL DEFAULT 1;

INSERT INTO payment_stripe_credentials (
  account_id, mode, charges_enabled, stripe_product_id, stripe_webhook_endpoint_id,
  api_key_ciphertext, webhook_secret_ciphertext, secret_material_kind, created_by, created_at
)
SELECT account_id, mode, charges_enabled, stripe_product_id, stripe_webhook_endpoint_id,
       api_key_ciphertext, webhook_secret_ciphertext, secret_material_kind, created_by, created_at
FROM payment_stripe_configurations;

UPDATE payment_stripe_configurations configuration
SET active_credential_id = credential.id
FROM payment_stripe_credentials credential
WHERE configuration.singleton_id = 1;

ALTER TABLE payment_stripe_configurations
  ALTER COLUMN active_credential_id SET NOT NULL,
  ADD CONSTRAINT payment_stripe_configurations_active_credential_id_fkey
    FOREIGN KEY (active_credential_id) REFERENCES payment_stripe_credentials(id) ON DELETE RESTRICT,
  ADD CONSTRAINT ck_payment_stripe_configuration_revision CHECK (revision > 0),
  DROP CONSTRAINT ck_payment_stripe_configuration_mode,
  DROP CONSTRAINT ck_payment_stripe_configuration_material,
  DROP COLUMN account_id,
  DROP COLUMN mode,
  DROP COLUMN charges_enabled,
  DROP COLUMN stripe_product_id,
  DROP COLUMN stripe_webhook_endpoint_id,
  DROP COLUMN api_key_ciphertext,
  DROP COLUMN webhook_secret_ciphertext,
  DROP COLUMN secret_material_kind,
  DROP COLUMN created_by;

ALTER TABLE payment_orders ADD COLUMN stripe_credential_id UUID;
UPDATE payment_orders orders
SET stripe_credential_id = credential.id
FROM payment_stripe_credentials credential
WHERE credential.account_id = orders.stripe_account_id;
ALTER TABLE payment_orders
  ALTER COLUMN stripe_credential_id SET NOT NULL,
  ADD CONSTRAINT payment_orders_stripe_credential_id_fkey
    FOREIGN KEY (stripe_credential_id) REFERENCES payment_stripe_credentials(id) ON DELETE RESTRICT;

ALTER TABLE payment_checkout_shutdowns ADD COLUMN stripe_credential_id UUID;
UPDATE payment_checkout_shutdowns shutdown
SET stripe_credential_id = credential.id
FROM payment_stripe_credentials credential
WHERE credential.account_id = shutdown.stripe_account_id;
ALTER TABLE payment_checkout_shutdowns
  ALTER COLUMN stripe_credential_id SET NOT NULL,
  ADD CONSTRAINT payment_checkout_shutdowns_stripe_credential_id_fkey
    FOREIGN KEY (stripe_credential_id) REFERENCES payment_stripe_credentials(id) ON DELETE RESTRICT;
