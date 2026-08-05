-- migrate:up
CREATE TABLE payment_disputes (
  id UUID PRIMARY KEY,
  payment_order_id UUID NOT NULL REFERENCES payment_orders(id) ON DELETE RESTRICT,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  stripe_dispute_id TEXT NOT NULL UNIQUE,
  stripe_charge_id TEXT NOT NULL,
  stripe_payment_intent_id TEXT NOT NULL DEFAULT '',
  provider_status TEXT NOT NULL,
  status TEXT NOT NULL,
  amount_minor BIGINT NOT NULL,
  currency TEXT NOT NULL,
  target_idea_quantity BIGINT NOT NULL,
  debited_idea_quantity BIGINT NOT NULL,
  unrecovered_credits BIGINT NOT NULL,
  restriction_id UUID REFERENCES credit_wallet_restrictions(restriction_id) ON DELETE RESTRICT,
  debit_transaction_id TEXT UNIQUE REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT,
  restore_transaction_id TEXT UNIQUE REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT,
  opened_at TIMESTAMPTZ NOT NULL,
  closed_at TIMESTAMPTZ,
  next_reconciliation_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT ck_payment_dispute_provider_ids CHECK (
    char_length(stripe_dispute_id) BETWEEN 1 AND 255
    AND char_length(stripe_charge_id) BETWEEN 1 AND 255
    AND char_length(stripe_payment_intent_id) <= 255
  ),
  CONSTRAINT ck_payment_dispute_status CHECK (status IN ('open', 'won', 'lost')),
  CONSTRAINT ck_payment_dispute_amount CHECK (amount_minor > 0 AND currency ~ '^[a-z]{3}$'),
  CONSTRAINT ck_payment_dispute_recovery CHECK (
    target_idea_quantity >= 0
    AND debited_idea_quantity BETWEEN 0 AND target_idea_quantity
    AND ((status = 'won' AND unrecovered_credits = 0)
      OR (status IN ('open', 'lost') AND unrecovered_credits = target_idea_quantity - debited_idea_quantity))
    AND ((debited_idea_quantity = 0 AND debit_transaction_id IS NULL)
      OR (debited_idea_quantity > 0 AND debit_transaction_id IS NOT NULL))
    AND (restore_transaction_id IS NULL OR status = 'won')
  ),
  CONSTRAINT ck_payment_dispute_restriction CHECK (
    (status IN ('open', 'lost') AND restriction_id IS NOT NULL)
    OR status = 'won'
  ),
  CONSTRAINT ck_payment_dispute_lifecycle CHECK (
    (status = 'open' AND closed_at IS NULL)
    OR (status IN ('won', 'lost') AND closed_at IS NOT NULL AND closed_at >= opened_at)
  )
);

CREATE INDEX idx_payment_disputes_order
  ON payment_disputes (payment_order_id, opened_at, id);

CREATE INDEX idx_payment_disputes_reconciliation
  ON payment_disputes (next_reconciliation_at, id)
  WHERE status IN ('open', 'lost');

-- statement:begin
CREATE FUNCTION guard_payment_dispute_lifecycle()
RETURNS TRIGGER AS $$
BEGIN
  IF OLD.id <> NEW.id
     OR OLD.payment_order_id <> NEW.payment_order_id
     OR OLD.user_id <> NEW.user_id
     OR OLD.stripe_dispute_id <> NEW.stripe_dispute_id
     OR OLD.stripe_charge_id <> NEW.stripe_charge_id
     OR OLD.stripe_payment_intent_id <> NEW.stripe_payment_intent_id
     OR OLD.amount_minor <> NEW.amount_minor
     OR OLD.currency <> NEW.currency
     OR OLD.target_idea_quantity <> NEW.target_idea_quantity
     OR OLD.debited_idea_quantity <> NEW.debited_idea_quantity
     OR OLD.restriction_id IS DISTINCT FROM NEW.restriction_id
     OR OLD.debit_transaction_id IS DISTINCT FROM NEW.debit_transaction_id
     OR OLD.opened_at <> NEW.opened_at
     OR OLD.created_at <> NEW.created_at THEN
    RAISE EXCEPTION 'Payment Dispute recovery identity is immutable';
  END IF;
  IF OLD.restore_transaction_id IS NOT NULL
     AND OLD.restore_transaction_id IS DISTINCT FROM NEW.restore_transaction_id THEN
    RAISE EXCEPTION 'Payment Dispute restore identity is immutable';
  END IF;
  IF OLD.unrecovered_credits <> NEW.unrecovered_credits
     AND NOT (NEW.status = 'won' AND NEW.unrecovered_credits = 0) THEN
    RAISE EXCEPTION 'Payment Dispute unrecovered amount is immutable before win';
  END IF;
  IF OLD.status <> NEW.status AND NOT (
    (OLD.status = 'open' AND NEW.status IN ('won', 'lost'))
    OR (OLD.status = 'lost' AND NEW.status = 'won')
  ) THEN
    RAISE EXCEPTION 'invalid Payment Dispute state transition: % -> %', OLD.status, NEW.status;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- statement:end

CREATE TRIGGER guard_payment_disputes
BEFORE UPDATE ON payment_disputes
FOR EACH ROW EXECUTE FUNCTION guard_payment_dispute_lifecycle();

CREATE TRIGGER set_payment_disputes_updated_at
BEFORE UPDATE ON payment_disputes
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Identity deletion calls this read-only Payments-owned seam inside its
-- deletion transaction, keeping payment state ownership out of Identity SQL.
CREATE FUNCTION has_unsettled_payment_state(p_user_id BIGINT)
RETURNS BOOLEAN
AS $$
  SELECT EXISTS (
    SELECT 1 FROM payment_orders
    WHERE user_id = p_user_id
      AND status IN ('creating', 'checkout_open', 'async_payment_pending', 'paid_pending_credit')
    UNION ALL
    SELECT 1 FROM payment_refunds
    WHERE user_id = p_user_id
      AND status IN ('pending', 'anomaly')
    UNION ALL
    SELECT 1 FROM payment_disputes
    WHERE user_id = p_user_id
      AND status IN ('open', 'lost')
  )
$$ LANGUAGE SQL STABLE SECURITY DEFINER SET search_path = pg_catalog;

-- Bind the SECURITY DEFINER function to the Payments schema that owns its tables.
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.has_unsettled_payment_state(BIGINT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
END;
$$;

-- migrate:down
DROP FUNCTION IF EXISTS has_unsettled_payment_state(BIGINT);
DROP TABLE IF EXISTS payment_disputes;
DROP FUNCTION IF EXISTS guard_payment_dispute_lifecycle();
