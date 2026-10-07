-- Control Plane durable idempotency facts (#891). Mirrors
-- fragments/control_audit.sql: one row per guarded mutation attempt,
-- keyed by (operator_identity, operation, target_type, target_id,
-- idempotency_key). See the fragment for the lifecycle contract.
CREATE TABLE control_idempotency_records (
  operator_identity TEXT NOT NULL,
  operation TEXT NOT NULL,
  target_type TEXT NOT NULL DEFAULT '',
  target_id TEXT NOT NULL DEFAULT '',
  idempotency_key VARCHAR(128) NOT NULL,
  request_hash VARCHAR(64) NOT NULL,
  completed BOOLEAN NOT NULL DEFAULT FALSE,
  response_status INTEGER NULL,
  response_content_type TEXT NOT NULL DEFAULT '',
  response_body TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ NULL,
  expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '24 hours'),
  CONSTRAINT control_idempotency_records_pkey PRIMARY KEY (operator_identity, operation, target_type, target_id, idempotency_key),
  CONSTRAINT ck_control_idempotency_records_operation CHECK (char_length(btrim(operation)) BETWEEN 1 AND 64),
  CONSTRAINT ck_control_idempotency_records_key CHECK (char_length(btrim(idempotency_key)) BETWEEN 1 AND 128),
  CONSTRAINT ck_control_idempotency_records_request_hash CHECK (request_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT ck_control_idempotency_records_response CHECK (
    (NOT completed AND response_status IS NULL AND completed_at IS NULL) OR
    (completed AND response_status BETWEEN 200 AND 299 AND completed_at IS NOT NULL)
  )
);

CREATE INDEX idx_control_idempotency_records_expiry ON control_idempotency_records (expires_at) WHERE completed;
