-- migrate:up

ALTER TABLE payment_stripe_credentials
  ADD COLUMN payment_method_configuration_id TEXT,
  ADD CONSTRAINT ck_payment_stripe_credential_payment_method_configuration CHECK (
    payment_method_configuration_id IS NULL
    OR (
      char_length(payment_method_configuration_id) <= 255
      AND payment_method_configuration_id ~ '^pmc_[A-Za-z0-9]+$'
    )
  );

UPDATE payment_stripe_configurations
SET purchases_enabled = FALSE
WHERE purchases_enabled;
