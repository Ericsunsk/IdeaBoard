-- migrate:up

-- Stripe test and live mode share an Account ID but not Customer resources.
-- This table is a recreatable lookup cache; existing rows cannot be assigned a
-- mode reliably, so discard them instead of carrying ambiguous provider state.
DELETE FROM payment_customers;

ALTER TABLE payment_customers
  DROP CONSTRAINT payment_customers_pkey,
  DROP CONSTRAINT uq_payment_customer_account,
  ADD COLUMN stripe_mode TEXT NOT NULL,
  ADD CONSTRAINT payment_customers_pkey PRIMARY KEY (user_id, stripe_account_id, stripe_mode),
  ADD CONSTRAINT ck_payment_customer_mode CHECK (stripe_mode IN ('test', 'live')),
  ADD CONSTRAINT uq_payment_customer_namespace UNIQUE (stripe_account_id, stripe_mode, stripe_customer_id);
