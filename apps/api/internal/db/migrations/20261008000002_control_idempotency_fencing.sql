-- Fence each Control mutation attempt and retain uncertain external outcomes.
ALTER TABLE control_idempotency_records
  ADD COLUMN attempt_token TEXT NOT NULL DEFAULT gen_random_uuid()::text,
  ADD COLUMN settlement_unknown BOOLEAN NOT NULL DEFAULT FALSE,
  ADD CONSTRAINT ck_control_idempotency_records_attempt_token CHECK (length(btrim(attempt_token)) > 0);

-- Legacy committed in-progress claims do not prove whether dispatch started.
-- Keep them unavailable rather than replaying an external effect during rollout.
UPDATE control_idempotency_records SET settlement_unknown = TRUE WHERE NOT completed;
