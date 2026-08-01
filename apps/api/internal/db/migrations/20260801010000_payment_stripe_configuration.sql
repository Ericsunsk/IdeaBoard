-- migrate:up
CREATE TABLE payment_stripe_configurations (
  singleton_id SMALLINT PRIMARY KEY DEFAULT 1,
  account_id TEXT NOT NULL,
  mode TEXT NOT NULL,
  charges_enabled BOOLEAN NOT NULL,
  product_tax_code TEXT NOT NULL,
  purchases_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  stripe_product_id TEXT NOT NULL,
  stripe_webhook_endpoint_id TEXT NOT NULL,
  api_key_ciphertext TEXT NOT NULL,
  webhook_secret_ciphertext TEXT NOT NULL,
  secret_material_kind TEXT NOT NULL,
  created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  updated_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT ck_payment_stripe_configuration_singleton CHECK (singleton_id = 1),
  CONSTRAINT ck_payment_stripe_configuration_mode CHECK (mode IN ('test', 'live')),
  CONSTRAINT ck_payment_stripe_configuration_tax_code CHECK (product_tax_code ~ '^txcd_[0-9]{8}$'),
  CONSTRAINT ck_payment_stripe_configuration_material CHECK (secret_material_kind = 'encrypted')
);

CREATE TABLE payment_admin_actions (
  id BIGSERIAL PRIMARY KEY,
  action_type TEXT NOT NULL,
  actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  outcome TEXT NOT NULL,
  error_code TEXT NOT NULL DEFAULT '',
  error_summary TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT ck_payment_admin_action_type CHECK (action_type IN ('configure_stripe')),
  CONSTRAINT ck_payment_admin_action_target CHECK (target_type = 'stripe_configuration' AND target_id = 'current'),
  CONSTRAINT ck_payment_admin_action_outcome CHECK (outcome IN ('succeeded', 'failed')),
  CONSTRAINT ck_payment_admin_action_error_code_length CHECK (char_length(error_code) <= 64),
  CONSTRAINT ck_payment_admin_action_error_summary_length CHECK (char_length(error_summary) <= 500),
  CONSTRAINT ck_payment_admin_action_metadata_object CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE TABLE payment_webhook_events (
  stripe_event_id TEXT PRIMARY KEY,
  event_type TEXT NOT NULL,
  stripe_object_id TEXT NOT NULL DEFAULT '',
  livemode BOOLEAN NOT NULL,
  stripe_created_at TIMESTAMPTZ NOT NULL,
  processing_result TEXT NOT NULL,
  error_summary TEXT NOT NULL DEFAULT '',
  received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  processed_at TIMESTAMPTZ,
  CONSTRAINT ck_payment_webhook_event_id CHECK (char_length(stripe_event_id) BETWEEN 1 AND 255),
  CONSTRAINT ck_payment_webhook_event_type CHECK (char_length(event_type) BETWEEN 1 AND 255),
  CONSTRAINT ck_payment_webhook_processing_result CHECK (processing_result IN ('received', 'ignored', 'processed', 'failed')),
  CONSTRAINT ck_payment_webhook_error_summary_length CHECK (char_length(error_summary) <= 500)
);

CREATE INDEX idx_payment_webhook_events_received
  ON payment_webhook_events (received_at DESC, stripe_event_id DESC);

-- statement:begin
CREATE FUNCTION prevent_payment_admin_action_mutation()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION 'payment admin actions are append-only';
END;
$$ LANGUAGE plpgsql;
-- statement:end

CREATE TRIGGER payment_admin_actions_append_only
BEFORE UPDATE OR DELETE ON payment_admin_actions
FOR EACH ROW EXECUTE FUNCTION prevent_payment_admin_action_mutation();

CREATE TRIGGER set_payment_stripe_configurations_updated_at
BEFORE UPDATE ON payment_stripe_configurations
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- migrate:down
DROP TRIGGER IF EXISTS set_payment_stripe_configurations_updated_at ON payment_stripe_configurations;
DROP TRIGGER IF EXISTS payment_admin_actions_append_only ON payment_admin_actions;
DROP FUNCTION IF EXISTS prevent_payment_admin_action_mutation();
DROP TABLE IF EXISTS payment_webhook_events;
DROP TABLE IF EXISTS payment_admin_actions;
DROP TABLE IF EXISTS payment_stripe_configurations;
