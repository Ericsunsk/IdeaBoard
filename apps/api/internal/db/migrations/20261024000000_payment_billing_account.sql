-- destructive: approved
-- Reviewed scope: replace only obsolete User-scoped order uniqueness and
-- Customer primary-key constraints with Billing Account equivalents below.
-- No payment rows, balances, Ledger entries, or ownership facts are deleted.
-- Isolated validation uses ATLAS_ALLOW_DESTRUCTIVE=1; production apply needs
-- its own reviewed rollout and is not authorized by this implementation.

-- Payment Order + Stripe Customer payer attribution (#919).
--
-- A Payment Order is a durable intent to buy Ideas for one Billing Account;
-- the operator (user_id) is recorded independently and never conflated with
-- the payer. Stripe Customers are reused per
-- (billing_account_id, stripe_account_id, stripe_mode); unpaid-slot
-- accounting and the (account, client key) idempotency scope both address
-- the payer account. Checkout, continue/abandon, Return, Webhook, and
-- background recovery converge on the persisted billing_account_id, so
-- workspace switches, member exits, and management changes never move the
-- payment obligation.
--
-- Existing orders keep their money facts untouched and gain their
-- determined original payer through
-- ensure_personal_billing_account_for_user(user_id) (the same deterministic
-- attribution #916 used for balances and Holds; organization/shared accounts
-- start at zero and receive no injected funds). No legacy wallet branch is
-- kept: the credited guard now requires the purchase Credit transaction on
-- the order's own billing_account_id instead of re-deriving a personal
-- account from the operator. Operator-keyed uniqueness is replaced with
-- account-keyed constraints; no payment rows or money facts are deleted.

ALTER TABLE payment_orders ADD COLUMN billing_account_id BIGINT;

UPDATE payment_orders
SET billing_account_id = ensure_personal_billing_account_for_user(user_id)
WHERE billing_account_id IS NULL;

ALTER TABLE payment_orders ADD CONSTRAINT ck_payment_order_billing_account CHECK (billing_account_id IS NOT NULL);

-- Idempotency scope is the payer account: same key + same hash replays,
-- same key + different hash conflicts. Operators may manage independent
-- accounts, so the obsolete per-operator uniqueness must be removed.
ALTER TABLE payment_orders DROP CONSTRAINT uq_payment_order_idempotency;
ALTER TABLE payment_orders ADD CONSTRAINT uq_payment_order_billing_idempotency UNIQUE (billing_account_id, client_idempotency_key);

-- Account-scoped order reads: pending lists, recharge history, and unpaid
-- accounting address the payer account, never the operator.
CREATE INDEX idx_payment_orders_billing_history
  ON payment_orders (billing_account_id, created_at DESC, id DESC);

-- Unpaid-slot accounting is per payer account: one account holds at most
-- one unpaid active Checkout per Idea quantity.
DROP INDEX uq_payment_orders_user_active_quantity;
CREATE UNIQUE INDEX uq_payment_orders_billing_active_quantity
  ON payment_orders (billing_account_id, idea_quantity)
  WHERE status IN ('creating', 'checkout_open', 'async_payment_pending');

ALTER TABLE payment_customers ADD COLUMN billing_account_id BIGINT;

UPDATE payment_customers
SET billing_account_id = ensure_personal_billing_account_for_user(user_id)
WHERE billing_account_id IS NULL;

ALTER TABLE payment_customers ADD CONSTRAINT ck_payment_customer_billing_account CHECK (billing_account_id IS NOT NULL);

ALTER TABLE payment_customers DROP CONSTRAINT payment_customers_pkey;
ALTER TABLE payment_customers ADD CONSTRAINT payment_customers_pkey PRIMARY KEY (billing_account_id, stripe_account_id, stripe_mode);

CREATE OR REPLACE FUNCTION guard_payment_order_lifecycle()
RETURNS TRIGGER AS $$
BEGIN
  IF OLD.id <> NEW.id
     OR OLD.public_order_number <> NEW.public_order_number
     OR OLD.public_token <> NEW.public_token
     OR OLD.user_id <> NEW.user_id
     OR OLD.billing_account_id IS DISTINCT FROM NEW.billing_account_id
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
  IF OLD.checkout_url <> '' AND NEW.checkout_url <> '' AND OLD.checkout_url <> NEW.checkout_url THEN
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
    SELECT 1
    FROM credit_transactions transaction_fact
    WHERE transaction_fact.transaction_id = NEW.credit_transaction_id
      AND transaction_fact.command_type = 'purchase_credit'
      AND transaction_fact.billing_account_id = NEW.billing_account_id
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
