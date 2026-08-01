-- migrate:up
ALTER TABLE credit_transactions
  DROP CONSTRAINT ck_credit_transactions_command_type,
  ADD CONSTRAINT ck_credit_transactions_command_type
    CHECK (command_type IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction', 'purchase_credit'));

CREATE TABLE payment_customers (
  user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
  stripe_account_id TEXT NOT NULL,
  stripe_customer_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT ck_payment_customer_account_id CHECK (char_length(stripe_account_id) BETWEEN 1 AND 255),
  CONSTRAINT uq_payment_customer_account UNIQUE (stripe_account_id, stripe_customer_id),
  CONSTRAINT ck_payment_customer_id CHECK (char_length(stripe_customer_id) BETWEEN 1 AND 255)
);

CREATE TABLE payment_orders (
  id UUID PRIMARY KEY,
  public_order_number TEXT NOT NULL UNIQUE,
  public_token UUID NOT NULL UNIQUE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  client_idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  status TEXT NOT NULL,
  stripe_account_id TEXT NOT NULL,
  stripe_customer_id TEXT NOT NULL DEFAULT '',
  stripe_product_id TEXT NOT NULL,
  stripe_checkout_session_id TEXT,
  stripe_payment_intent_id TEXT NOT NULL DEFAULT '',
  stripe_charge_id TEXT NOT NULL DEFAULT '',
  idea_quantity BIGINT NOT NULL,
  base_currency TEXT NOT NULL DEFAULT 'usd',
  base_amount_minor BIGINT NOT NULL,
  paid_subtotal_minor BIGINT,
  paid_tax_minor BIGINT,
  paid_total_minor BIGINT,
  paid_currency TEXT NOT NULL DEFAULT '',
  terms_version TEXT NOT NULL,
  refund_policy_version TEXT NOT NULL,
  checkout_url TEXT NOT NULL DEFAULT '',
  receipt_url TEXT NOT NULL DEFAULT '',
  credit_transaction_id TEXT UNIQUE REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT,
  expires_at TIMESTAMPTZ NOT NULL,
  paid_at TIMESTAMPTZ,
  credited_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT uq_payment_order_idempotency UNIQUE (user_id, client_idempotency_key),
  CONSTRAINT uq_payment_order_checkout_session UNIQUE (stripe_account_id, stripe_checkout_session_id),
  CONSTRAINT ck_payment_order_public_number CHECK (public_order_number ~ '^IBP-[0-9]{8}-[A-Z0-9]{12}$'),
  CONSTRAINT ck_payment_order_idempotency CHECK (char_length(client_idempotency_key) BETWEEN 8 AND 160),
  CONSTRAINT ck_payment_order_request_hash CHECK (request_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT ck_payment_order_status CHECK (status IN (
    'creating', 'checkout_open', 'async_payment_pending', 'paid_pending_credit',
    'credited', 'cancelled', 'expired', 'payment_failed'
  )),
  CONSTRAINT ck_payment_order_stripe_ids CHECK (
    char_length(stripe_account_id) BETWEEN 1 AND 255
    AND char_length(stripe_customer_id) <= 255
    AND char_length(stripe_product_id) BETWEEN 1 AND 255
    AND (stripe_checkout_session_id IS NULL OR char_length(stripe_checkout_session_id) BETWEEN 1 AND 255)
    AND char_length(stripe_payment_intent_id) <= 255
    AND char_length(stripe_charge_id) <= 255
  ),
  CONSTRAINT ck_payment_order_urls CHECK (char_length(checkout_url) <= 4096 AND char_length(receipt_url) <= 4096),
  CONSTRAINT ck_payment_order_ideas CHECK (idea_quantity BETWEEN 500 AND 500000),
  CONSTRAINT ck_payment_order_base_amount CHECK (base_currency = 'usd' AND base_amount_minor = idea_quantity),
  CONSTRAINT ck_payment_order_paid_amounts CHECK (
    (paid_subtotal_minor IS NULL OR paid_subtotal_minor >= 0)
    AND (paid_tax_minor IS NULL OR paid_tax_minor >= 0)
    AND (paid_total_minor IS NULL OR paid_total_minor > 0)
    AND (paid_currency = '' OR paid_currency ~ '^[a-z]{3}$')
    AND (paid_total_minor IS NULL OR paid_subtotal_minor + paid_tax_minor = paid_total_minor)
  ),
  CONSTRAINT ck_payment_order_versions CHECK (
    char_length(terms_version) BETWEEN 1 AND 64
    AND char_length(refund_policy_version) BETWEEN 1 AND 64
  ),
  CONSTRAINT ck_payment_order_state_facts CHECK (
    (status = 'creating' AND stripe_customer_id = '' AND stripe_checkout_session_id IS NULL
      AND checkout_url = '' AND credit_transaction_id IS NULL AND paid_total_minor IS NULL)
    OR (status IN ('checkout_open', 'async_payment_pending') AND stripe_customer_id <> ''
      AND stripe_checkout_session_id IS NOT NULL AND checkout_url <> ''
      AND credit_transaction_id IS NULL AND paid_total_minor IS NULL)
    OR (status = 'paid_pending_credit' AND stripe_customer_id <> '' AND stripe_checkout_session_id IS NOT NULL
      AND stripe_payment_intent_id <> '' AND credit_transaction_id IS NULL
      AND paid_subtotal_minor IS NOT NULL AND paid_tax_minor IS NOT NULL AND paid_total_minor IS NOT NULL
      AND paid_currency <> '' AND paid_at IS NOT NULL AND credited_at IS NULL)
    OR (status = 'credited' AND stripe_customer_id <> '' AND stripe_checkout_session_id IS NOT NULL
      AND stripe_payment_intent_id <> '' AND credit_transaction_id IS NOT NULL
      AND paid_subtotal_minor IS NOT NULL AND paid_tax_minor IS NOT NULL AND paid_total_minor IS NOT NULL
      AND paid_currency <> '' AND paid_at IS NOT NULL AND credited_at IS NOT NULL)
    OR (status IN ('cancelled', 'expired', 'payment_failed') AND stripe_payment_intent_id = '' AND stripe_charge_id = ''
      AND paid_subtotal_minor IS NULL AND paid_tax_minor IS NULL AND paid_total_minor IS NULL
      AND paid_currency = '' AND receipt_url = '' AND credit_transaction_id IS NULL
      AND paid_at IS NULL AND credited_at IS NULL
      AND ((stripe_checkout_session_id IS NULL AND stripe_customer_id = '' AND checkout_url = '')
        OR (stripe_checkout_session_id IS NOT NULL AND stripe_customer_id <> '' AND checkout_url <> '')))
  )
);

CREATE INDEX idx_payment_orders_user_history
  ON payment_orders (user_id, created_at DESC, id DESC);

CREATE INDEX idx_payment_orders_open
  ON payment_orders (status, expires_at)
  WHERE status IN ('creating', 'checkout_open', 'async_payment_pending');

CREATE UNIQUE INDEX uq_payment_orders_user_active_quantity
  ON payment_orders (user_id, idea_quantity)
  WHERE status IN ('creating', 'checkout_open', 'async_payment_pending');

-- statement:begin
CREATE FUNCTION guard_payment_order_lifecycle()
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
    SELECT 1
    FROM credit_transactions transaction_fact
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
-- statement:end

CREATE TRIGGER guard_payment_orders
BEFORE UPDATE ON payment_orders
FOR EACH ROW EXECUTE FUNCTION guard_payment_order_lifecycle();

CREATE TRIGGER set_payment_customers_updated_at
BEFORE UPDATE ON payment_customers
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER set_payment_orders_updated_at
BEFORE UPDATE ON payment_orders
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- statement:begin
CREATE FUNCTION execute_credit_purchase_v1(
  p_user_id BIGINT,
  p_amount BIGINT,
  p_idempotency_key TEXT,
  p_actor_type TEXT,
  p_actor_id TEXT,
  p_source TEXT,
  p_reason TEXT,
  p_business_reference_type TEXT,
  p_business_reference_id TEXT
)
RETURNS TABLE (
  transaction_id VARCHAR(64), command_type VARCHAR(24), command_amount BIGINT,
  wallet_user_id BIGINT, available_balance_after BIGINT, reserved_balance_after BIGINT,
  replayed BOOLEAN, created_at TIMESTAMPTZ(3)
)
AS $$
DECLARE
  v_payload JSONB;
  v_hash TEXT;
  v_existing_hash TEXT;
  v_transaction_id VARCHAR(64);
  v_available_id BIGINT;
  v_reserved_id BIGINT;
  v_issuance_id BIGINT;
  v_available_balance BIGINT;
  v_reserved_balance BIGINT;
  v_shard_key VARCHAR(128);
BEGIN
  p_idempotency_key := btrim(p_idempotency_key);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source);
  p_reason := btrim(p_reason);
  p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_user_id <= 0 OR p_amount <= 0 OR p_amount > 9000000000000000
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments'
     OR p_source <> 'stripe_purchase' OR p_reason = ''
     OR p_business_reference_type <> 'payment_order'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit purchase command';
  END IF;

  v_payload := jsonb_build_object(
    'command_type', 'purchase_credit',
    'user_id', p_user_id,
    'amount', p_amount,
    'related_transaction_id', '',
    'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id),
    'source', p_source,
    'reason', p_reason,
    'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id)
  );
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));
  SELECT existing.request_hash INTO v_existing_hash
  FROM credit_transactions existing
  WHERE existing.idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN
      RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
    END IF;
    RETURN QUERY
    SELECT existing.transaction_id, existing.command_type, existing.command_amount,
      existing.wallet_user_id, existing.available_balance_after, existing.reserved_balance_after,
      TRUE, existing.created_at
    FROM credit_transactions existing
    WHERE existing.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;

  PERFORM 1 FROM credit_wallets wallets WHERE wallets.user_id = p_user_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
  v_shard_key := 'wallet:' || p_user_id::TEXT;
  PERFORM accounts.id
  FROM credit_accounts accounts
  WHERE (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_issuance')
  ORDER BY accounts.id
  FOR UPDATE;
  SELECT
    MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'),
    MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
    MAX(accounts.id) FILTER (WHERE accounts.account_role = 'system_issuance'),
    MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
    MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_issuance_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_issuance');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_issuance_id IS NULL THEN
    RAISE EXCEPTION 'credit wallet account set is incomplete';
  END IF;

  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(
    clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8'
  )), 'hex'), 1, 32);
  INSERT INTO credit_transactions (
    transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
    actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
    command_amount, posting_count, available_balance_after, reserved_balance_after
  ) VALUES (
    v_transaction_id, p_user_id, 'purchase_credit', p_idempotency_key, v_hash,
    p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id,
    p_amount, 2, v_available_balance + p_amount, v_reserved_balance
  );
  INSERT INTO credit_postings (transaction_id, account_id, amount)
  VALUES (v_transaction_id, v_issuance_id, -p_amount), (v_transaction_id, v_available_id, p_amount);

  RETURN QUERY
  SELECT created.transaction_id, created.command_type, created.command_amount,
    created.wallet_user_id, created.available_balance_after, created.reserved_balance_after,
    FALSE, created.created_at
  FROM credit_transactions created
  WHERE created.transaction_id = v_transaction_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_purchase_v1(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION execute_credit_purchase_v1(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;

-- migrate:down
DROP TRIGGER IF EXISTS set_payment_orders_updated_at ON payment_orders;
DROP TRIGGER IF EXISTS set_payment_customers_updated_at ON payment_customers;
DROP TRIGGER IF EXISTS guard_payment_orders ON payment_orders;
DROP FUNCTION IF EXISTS guard_payment_order_lifecycle();
DROP FUNCTION IF EXISTS execute_credit_purchase_v1(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);
DROP TABLE IF EXISTS payment_orders;
DROP TABLE IF EXISTS payment_customers;

ALTER TABLE credit_transactions
  DROP CONSTRAINT ck_credit_transactions_command_type,
  ADD CONSTRAINT ck_credit_transactions_command_type
    CHECK (command_type IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction'));
