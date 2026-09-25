-- Durable webhook delivery identity and per-attempt history (#833).
-- Existing event_deliveries rows are the aggregate delivery state; this
-- table keeps every River attempt readable after restart and discard.
CREATE UNIQUE INDEX IF NOT EXISTS ux_event_deliveries_event_subscription
  ON event_deliveries (event_id, subscription_id);

CREATE TABLE IF NOT EXISTS event_delivery_attempts (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  delivery_id BIGINT NOT NULL REFERENCES event_deliveries(internal_id) ON DELETE CASCADE,
  attempt_number INT NOT NULL,
  status_code INT NOT NULL DEFAULT 0,
  outcome TEXT NOT NULL,
  error_message TEXT NOT NULL DEFAULT '',
  attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_event_delivery_attempts_delivery_attempt UNIQUE (delivery_id, attempt_number),
  CONSTRAINT chk_event_delivery_attempts_number CHECK (attempt_number > 0),
  CONSTRAINT chk_event_delivery_attempts_outcome CHECK (outcome IN ('delivered', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_event_delivery_attempts_delivery
  ON event_delivery_attempts (delivery_id, attempt_number DESC);
