-- migrate:up
CREATE TABLE payment_checkout_shutdowns (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  stripe_account_id TEXT NOT NULL,
  cutoff_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TIMESTAMPTZ,
  CONSTRAINT ck_payment_checkout_shutdowns_account CHECK (char_length(stripe_account_id) > 0)
);

CREATE INDEX idx_payment_checkout_shutdowns_active
  ON payment_checkout_shutdowns (created_at, id)
  WHERE completed_at IS NULL;

-- migrate:down
DROP TABLE IF EXISTS payment_checkout_shutdowns;
