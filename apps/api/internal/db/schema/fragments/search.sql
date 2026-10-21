CREATE TABLE ai_search_operations (
  operation_id UUID PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  workspace_id BIGINT,
  billing_account_id BIGINT,
  idempotency_key VARCHAR(160) NOT NULL,
  request_hash CHAR(64) NOT NULL,
  sku VARCHAR(128) NOT NULL,
  price_version_id BIGINT NOT NULL,
  unit_credits BIGINT NOT NULL,
  hold_transaction_id TEXT NOT NULL REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  query TEXT NOT NULL DEFAULT '',
  error_code VARCHAR(64) NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  settled_at TIMESTAMPTZ(3),
  CONSTRAINT ck_ai_search_operations_billing_account CHECK (billing_account_id IS NOT NULL),
  CONSTRAINT uq_ai_search_operations_user_key UNIQUE (user_id, idempotency_key),
  CONSTRAINT ck_ai_search_operations_hash CHECK (request_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT ck_ai_search_operations_status CHECK (status IN ('pending', 'succeeded', 'failed', 'reconciliation_pending')),
  CONSTRAINT ck_ai_search_operations_unit_credits CHECK (unit_credits > 0),
  CONSTRAINT ck_ai_search_operations_settled_shape CHECK (
    (status IN ('pending', 'reconciliation_pending') AND settled_at IS NULL)
    OR (status IN ('succeeded', 'failed') AND settled_at IS NOT NULL)
  )
);

CREATE INDEX idx_ai_search_operations_pending
  ON ai_search_operations (status, created_at)
  WHERE status IN ('pending', 'reconciliation_pending');

CREATE INDEX idx_ai_search_operations_workspace
  ON ai_search_operations (workspace_id, created_at DESC);

CREATE UNIQUE INDEX uq_ai_search_operations_workspace_key
  ON ai_search_operations (workspace_id, idempotency_key)
  WHERE workspace_id IS NOT NULL;
