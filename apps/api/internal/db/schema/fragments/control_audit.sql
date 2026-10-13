-- Control Plane audit records. Every high-risk operation records operator,
-- target, capability, outcome, and timestamp. Capability enforcement
-- happens at middleware; this append-only table is the audit trail.
CREATE TABLE control_audit_records (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  operator_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  operation_type TEXT NOT NULL,
  capability TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  outcome TEXT NOT NULL,
  error_message TEXT NOT NULL DEFAULT '',
  request_correlation_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_control_audit_records_public_id UNIQUE (public_id),
  CONSTRAINT chk_control_audit_records_public_id CHECK (public_id LIKE 'audit\_%'),
  CONSTRAINT chk_control_audit_records_operation CHECK (length(btrim(operation_type)) > 0),
  CONSTRAINT chk_control_audit_records_capability CHECK (capability LIKE 'control:%'),
  CONSTRAINT chk_control_audit_records_target CHECK (length(btrim(target_type)) > 0 AND length(btrim(target_id)) > 0),
  CONSTRAINT chk_control_audit_records_outcome CHECK (outcome IN ('success','failure','denied')),
  CONSTRAINT chk_control_audit_records_metadata CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_control_audit_records_operator ON control_audit_records (operator_user_id, created_at DESC);
CREATE INDEX idx_control_audit_records_target ON control_audit_records (target_type, target_id, created_at DESC);
CREATE INDEX idx_control_audit_records_capability ON control_audit_records (capability, created_at DESC);
CREATE INDEX idx_control_audit_records_correlation ON control_audit_records (request_correlation_id);

-- Control Plane durable idempotency facts (#891). One row per guarded
-- mutation attempt, keyed by the durable scope
-- (operator_identity, operation, target_type, target_id, idempotency_key).
-- The request_hash pins the canonical request identity
-- (method + route template + target + raw body); a key reused with a
-- different hash is a 409 conflict, never a second execution.
--
-- Atomic mutations keep the claim, business writes and response in one
-- transaction. Detached mutations commit settlement_unknown before external
-- I/O: uncertain outcomes cannot be reclaimed merely because time elapsed.
-- attempt_token fences every settlement and lease takeover. Only completed
-- responses expire after 24h. Public operator subjects deliberately have no
-- users FK so internal-worker operators use the same guarantee.
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
  attempt_token TEXT NOT NULL DEFAULT gen_random_uuid()::text,
  settlement_unknown BOOLEAN NOT NULL DEFAULT FALSE,
  CONSTRAINT control_idempotency_records_pkey PRIMARY KEY (operator_identity, operation, target_type, target_id, idempotency_key),
  CONSTRAINT ck_control_idempotency_records_operation CHECK (char_length(btrim(operation)) BETWEEN 1 AND 64),
  CONSTRAINT ck_control_idempotency_records_key CHECK (char_length(btrim(idempotency_key)) BETWEEN 1 AND 128),
  CONSTRAINT ck_control_idempotency_records_request_hash CHECK (request_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT ck_control_idempotency_records_attempt_token CHECK (length(btrim(attempt_token)) > 0),
  CONSTRAINT ck_control_idempotency_records_response CHECK (
    (NOT completed AND response_status IS NULL AND completed_at IS NULL) OR
    (completed AND response_status BETWEEN 200 AND 299 AND completed_at IS NOT NULL)
  )
);

CREATE INDEX idx_control_idempotency_records_expiry ON control_idempotency_records (expires_at) WHERE completed;
