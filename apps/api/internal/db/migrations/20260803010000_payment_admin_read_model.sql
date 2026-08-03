-- migrate:up
ALTER TABLE payment_webhook_events
  ADD COLUMN payment_order_id UUID REFERENCES payment_orders(id) ON DELETE RESTRICT;

CREATE INDEX idx_payment_webhook_events_order_history
  ON payment_webhook_events (payment_order_id, stripe_created_at DESC, stripe_event_id DESC)
  WHERE payment_order_id IS NOT NULL;

CREATE UNIQUE INDEX uq_payment_orders_stripe_payment_intent
  ON payment_orders (stripe_payment_intent_id)
  WHERE stripe_payment_intent_id <> '';

CREATE UNIQUE INDEX uq_payment_orders_stripe_charge
  ON payment_orders (stripe_charge_id)
  WHERE stripe_charge_id <> '';

CREATE INDEX idx_payment_orders_admin_history
  ON payment_orders (created_at DESC, id DESC);

CREATE INDEX idx_payment_orders_admin_status_history
  ON payment_orders (status, created_at DESC, id DESC);

-- migrate:down
DROP INDEX IF EXISTS idx_payment_orders_admin_status_history;
DROP INDEX IF EXISTS idx_payment_orders_admin_history;
DROP INDEX IF EXISTS uq_payment_orders_stripe_charge;
DROP INDEX IF EXISTS uq_payment_orders_stripe_payment_intent;
DROP INDEX IF EXISTS idx_payment_webhook_events_order_history;

ALTER TABLE payment_webhook_events
  DROP COLUMN IF EXISTS payment_order_id;
