-- Serialize Stripe webhook processing across API instances. A processing row
-- is reclaimable after the lease so a crashed worker cannot strand an event.
ALTER TABLE payment_webhook_events
  DROP CONSTRAINT ck_payment_webhook_processing_result;

ALTER TABLE payment_webhook_events
  ADD CONSTRAINT ck_payment_webhook_processing_result
  CHECK (processing_result IN ('received', 'processing', 'ignored', 'processed', 'failed'));

CREATE INDEX idx_payment_webhook_events_processing_lease
  ON payment_webhook_events (processing_result, processed_at)
  WHERE processing_result = 'processing';
