-- migrate:up
-- A Payment Order is a durable Checkout intent. Keep the validated return origin
-- with that intent so a timed-out create can be safely replayed after a restart.
ALTER TABLE payment_orders
  ADD COLUMN return_base_url TEXT NOT NULL;

CREATE TABLE payment_order_reconciliations (
  payment_order_id UUID PRIMARY KEY REFERENCES payment_orders(id) ON DELETE RESTRICT,
  claim_token UUID,
  claim_generation BIGINT NOT NULL DEFAULT 0,
  claimed_at TIMESTAMPTZ,
  claimed_until TIMESTAMPTZ,
  last_checked_at TIMESTAMPTZ,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_error_code TEXT NOT NULL DEFAULT '',
  last_error_summary TEXT NOT NULL DEFAULT '',
  anomalous_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT ck_payment_order_reconciliation_claim
    CHECK ((claim_token IS NULL AND claimed_at IS NULL AND claimed_until IS NULL)
      OR (claim_token IS NOT NULL AND claimed_at IS NOT NULL AND claimed_until IS NOT NULL AND claimed_until > claimed_at)),
  CONSTRAINT ck_payment_order_reconciliation_generation CHECK (claim_generation >= 0),
  CONSTRAINT ck_payment_order_reconciliation_error CHECK (
    char_length(last_error_code) <= 64 AND char_length(last_error_summary) <= 500
  )
);

CREATE INDEX idx_payment_orders_reconciliation_candidates
  ON payment_orders (updated_at, id)
  WHERE status IN ('creating', 'checkout_open', 'async_payment_pending', 'paid_pending_credit');

CREATE INDEX idx_payment_order_reconciliations_due
  ON payment_order_reconciliations (next_attempt_at, claimed_until, payment_order_id);

CREATE INDEX idx_payment_order_reconciliations_anomaly
  ON payment_order_reconciliations (anomalous_at DESC, payment_order_id DESC)
  WHERE anomalous_at IS NOT NULL;

CREATE OR REPLACE FUNCTION guard_payment_order_lifecycle()
RETURNS TRIGGER AS $$
BEGIN
  IF OLD.id <> NEW.id
     OR OLD.public_order_number <> NEW.public_order_number
     OR OLD.public_token <> NEW.public_token
     OR OLD.user_id <> NEW.user_id
     OR OLD.client_idempotency_key <> NEW.client_idempotency_key
     OR OLD.request_hash <> NEW.request_hash
     OR OLD.stripe_account_id <> NEW.stripe_account_id
     OR OLD.stripe_product_id <> NEW.stripe_product_id
     OR OLD.idea_quantity <> NEW.idea_quantity
     OR OLD.base_currency <> NEW.base_currency
     OR OLD.base_amount_minor <> NEW.base_amount_minor
     OR OLD.terms_version <> NEW.terms_version
     OR OLD.refund_policy_version <> NEW.refund_policy_version
     OR OLD.return_base_url <> NEW.return_base_url
     OR OLD.created_at <> NEW.created_at THEN
    RAISE EXCEPTION 'payment order identity is immutable';
  END IF;
  IF OLD.stripe_customer_id <> '' AND OLD.stripe_customer_id <> NEW.stripe_customer_id THEN
    RAISE EXCEPTION 'payment order customer is immutable once assigned';
  END IF;
  IF OLD.stripe_checkout_session_id IS NOT NULL
     AND OLD.stripe_checkout_session_id IS DISTINCT FROM NEW.stripe_checkout_session_id THEN
    RAISE EXCEPTION 'payment order Checkout Session is immutable once assigned';
  END IF;
  IF OLD.checkout_url <> '' AND OLD.checkout_url <> NEW.checkout_url THEN
    RAISE EXCEPTION 'payment order Checkout URL is immutable once assigned';
  END IF;
  IF OLD.stripe_checkout_session_id IS NOT NULL AND OLD.expires_at <> NEW.expires_at THEN
    RAISE EXCEPTION 'payment order Checkout expiry is immutable once assigned';
  END IF;
  IF OLD.status IN ('paid_pending_credit', 'credited') AND (
    OLD.stripe_payment_intent_id <> NEW.stripe_payment_intent_id
    OR OLD.stripe_charge_id <> NEW.stripe_charge_id
    OR OLD.paid_subtotal_minor IS DISTINCT FROM NEW.paid_subtotal_minor
    OR OLD.paid_tax_minor IS DISTINCT FROM NEW.paid_tax_minor
    OR OLD.paid_total_minor IS DISTINCT FROM NEW.paid_total_minor
    OR OLD.paid_currency <> NEW.paid_currency
    OR OLD.receipt_url <> NEW.receipt_url
    OR OLD.paid_at IS DISTINCT FROM NEW.paid_at
  ) THEN
    RAISE EXCEPTION 'paid payment order facts are immutable';
  END IF;
  IF OLD.status = 'credited' AND (
    OLD.credit_transaction_id IS DISTINCT FROM NEW.credit_transaction_id
    OR OLD.credited_at IS DISTINCT FROM NEW.credited_at
  ) THEN
    RAISE EXCEPTION 'credited payment order facts are immutable';
  END IF;
  IF OLD.status IN ('cancelled', 'expired', 'payment_failed') AND NEW.status <> 'paid_pending_credit' AND (
    OLD.stripe_customer_id <> NEW.stripe_customer_id
    OR OLD.stripe_checkout_session_id IS DISTINCT FROM NEW.stripe_checkout_session_id
    OR OLD.checkout_url <> NEW.checkout_url
    OR OLD.stripe_payment_intent_id <> NEW.stripe_payment_intent_id
    OR OLD.stripe_charge_id <> NEW.stripe_charge_id
    OR OLD.paid_subtotal_minor IS DISTINCT FROM NEW.paid_subtotal_minor
    OR OLD.paid_tax_minor IS DISTINCT FROM NEW.paid_tax_minor
    OR OLD.paid_total_minor IS DISTINCT FROM NEW.paid_total_minor
    OR OLD.paid_currency <> NEW.paid_currency
    OR OLD.receipt_url <> NEW.receipt_url
    OR OLD.credit_transaction_id IS DISTINCT FROM NEW.credit_transaction_id
    OR OLD.paid_at IS DISTINCT FROM NEW.paid_at
    OR OLD.credited_at IS DISTINCT FROM NEW.credited_at
  ) THEN
    RAISE EXCEPTION 'terminal unpaid payment order facts are immutable';
  END IF;
  IF OLD.status <> NEW.status AND NOT (
    (OLD.status = 'creating' AND NEW.status IN ('checkout_open', 'paid_pending_credit', 'expired', 'payment_failed'))
    OR (OLD.status = 'checkout_open' AND NEW.status IN ('async_payment_pending', 'paid_pending_credit', 'cancelled', 'expired', 'payment_failed'))
    OR (OLD.status = 'async_payment_pending' AND NEW.status IN ('paid_pending_credit', 'expired', 'payment_failed'))
    OR (OLD.status = 'paid_pending_credit' AND NEW.status = 'credited')
    OR (OLD.status IN ('cancelled', 'expired', 'payment_failed') AND NEW.status = 'paid_pending_credit')
  ) THEN
    RAISE EXCEPTION 'invalid payment order state transition: % -> %', OLD.status, NEW.status;
  END IF;
  IF NEW.status = 'credited' AND OLD.status <> 'credited' AND NOT EXISTS (
    SELECT 1 FROM credit_transactions transaction_fact
    WHERE transaction_fact.transaction_id = NEW.credit_transaction_id
      AND transaction_fact.command_type = 'purchase_credit'
      AND transaction_fact.wallet_user_id = NEW.user_id
      AND transaction_fact.command_amount = NEW.idea_quantity
      AND transaction_fact.actor_type = 'service'
      AND transaction_fact.actor_id = 'payments'
      AND transaction_fact.source = 'stripe_purchase'
      AND transaction_fact.business_reference_type = 'payment_order'
      AND transaction_fact.business_reference_id = NEW.id::TEXT
  ) THEN
    RAISE EXCEPTION 'credited payment order requires its matching purchase Credit transaction';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER set_payment_order_reconciliations_updated_at
BEFORE UPDATE ON payment_order_reconciliations
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE FUNCTION initialize_payment_order_reconciliation()
RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO payment_order_reconciliations (payment_order_id)
  VALUES (NEW.id);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER initialize_payment_order_reconciliation
AFTER INSERT ON payment_orders
FOR EACH ROW EXECUTE FUNCTION initialize_payment_order_reconciliation();
