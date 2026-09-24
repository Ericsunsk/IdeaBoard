-- Control Plane audit records (#808, durable follow-up #809). Every
-- high-risk operation records operator, target, capability, reason,
-- outcome, and timestamp. Capability enforcement happens at middleware;
-- this table is the immutable audit trail.
CREATE TABLE control_audit_records (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  operator_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  operation_type TEXT NOT NULL,
  capability TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
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
