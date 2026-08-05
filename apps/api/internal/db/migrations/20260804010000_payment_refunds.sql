-- migrate:up
ALTER TABLE credit_transactions
  DROP CONSTRAINT ck_credit_transactions_command_type,
  ADD CONSTRAINT ck_credit_transactions_command_type CHECK (
    command_type IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction', 'purchase_credit', 'purchase_refund')
  );

-- statement:begin
CREATE FUNCTION execute_credit_purchase_refund_v1(
  p_user_id BIGINT,
  p_hold_transaction_id TEXT,
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
  v_amount BIGINT;
  v_hold_status VARCHAR(20);
  v_available_id BIGINT;
  v_reserved_id BIGINT;
  v_issuance_id BIGINT;
  v_available_balance BIGINT;
  v_reserved_balance BIGINT;
  v_shard_key VARCHAR(128);
BEGIN
  p_hold_transaction_id := btrim(p_hold_transaction_id);
  p_idempotency_key := btrim(p_idempotency_key);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source);
  p_reason := btrim(p_reason);
  p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_user_id <= 0 OR p_hold_transaction_id = '' OR length(p_hold_transaction_id) > 64
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments'
     OR p_source <> 'stripe_refund' OR p_reason = ''
     OR p_business_reference_type <> 'payment_refund'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit purchase refund command';
  END IF;

  v_payload := jsonb_build_object(
    'command_type', 'purchase_refund', 'user_id', p_user_id,
    'related_transaction_id', p_hold_transaction_id,
    'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id),
    'source', p_source, 'reason', p_reason,
    'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id)
  );
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));
  SELECT request_hash INTO v_existing_hash FROM credit_transactions WHERE idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN
      RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
    END IF;
    RETURN QUERY SELECT existing.transaction_id, existing.command_type, existing.command_amount,
      existing.wallet_user_id, existing.available_balance_after, existing.reserved_balance_after,
      TRUE, existing.created_at
    FROM credit_transactions existing WHERE existing.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;

  SELECT holds.amount, holds.status INTO v_amount, v_hold_status
  FROM credit_holds holds
  WHERE holds.hold_transaction_id = p_hold_transaction_id AND holds.wallet_user_id = p_user_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB005', MESSAGE = 'credit hold not found';
  END IF;
  IF v_hold_status <> 'held' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB006', MESSAGE = 'credit hold already settled';
  END IF;

  PERFORM 1 FROM credit_wallets wallets WHERE wallets.user_id = p_user_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
  v_shard_key := 'wallet:' || p_user_id::TEXT;
  PERFORM accounts.id FROM credit_accounts accounts
  WHERE (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_issuance')
  ORDER BY accounts.id FOR UPDATE;
  SELECT MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'system_issuance'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_issuance_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_issuance');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_issuance_id IS NULL OR v_reserved_balance < v_amount THEN
    RAISE EXCEPTION USING ERRCODE = 'IB004', MESSAGE = 'insufficient credits';
  END IF;

  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(
    clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8'
  )), 'hex'), 1, 32);
  INSERT INTO credit_transactions (
    transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
    actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
    related_transaction_id, command_amount, posting_count, available_balance_after, reserved_balance_after
  ) VALUES (
    v_transaction_id, p_user_id, 'purchase_refund', p_idempotency_key, v_hash,
    p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id,
    p_hold_transaction_id, v_amount, 2, v_available_balance, v_reserved_balance - v_amount
  );
  INSERT INTO credit_postings (transaction_id, account_id, amount)
  VALUES (v_transaction_id, v_reserved_id, -v_amount), (v_transaction_id, v_issuance_id, v_amount);
  UPDATE credit_holds SET status = 'captured', terminal_transaction_id = v_transaction_id, settled_at = now()
  WHERE hold_transaction_id = p_hold_transaction_id;

  RETURN QUERY SELECT created.transaction_id, created.command_type, created.command_amount,
    created.wallet_user_id, created.available_balance_after, created.reserved_balance_after,
    FALSE, created.created_at
  FROM credit_transactions created WHERE created.transaction_id = v_transaction_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_purchase_refund_v1(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION execute_credit_purchase_refund_v1(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;

ALTER TABLE payment_admin_actions
  DROP CONSTRAINT ck_payment_admin_action_type,
  DROP CONSTRAINT ck_payment_admin_action_target,
  ADD CONSTRAINT ck_payment_admin_action_type CHECK (action_type IN ('configure_stripe', 'refund_payment')),
  ADD CONSTRAINT ck_payment_admin_action_target CHECK (
    (target_type = 'stripe_configuration' AND target_id = 'current')
    OR (target_type = 'payment_refund' AND target_id <> '')
  );

CREATE TABLE payment_refunds (
  id UUID PRIMARY KEY,
  payment_order_id UUID NOT NULL REFERENCES payment_orders(id) ON DELETE RESTRICT,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  idempotency_key TEXT NOT NULL,
  idea_quantity BIGINT NOT NULL,
  amount_minor BIGINT NOT NULL,
  currency TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  stripe_status TEXT NOT NULL DEFAULT '',
  stripe_refund_id TEXT NOT NULL DEFAULT '',
  stripe_charge_id TEXT NOT NULL DEFAULT '',
  credit_hold_transaction_id TEXT NOT NULL,
  credit_transaction_id TEXT NOT NULL DEFAULT '',
  actor_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  reason TEXT NOT NULL DEFAULT '',
  error_summary TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  succeeded_at TIMESTAMPTZ,
  failed_at TIMESTAMPTZ,
  CONSTRAINT uq_payment_refund_idempotency UNIQUE (payment_order_id, idempotency_key),
  CONSTRAINT ck_payment_refund_status CHECK (status IN ('pending', 'succeeded', 'failed', 'anomaly')),
  CONSTRAINT ck_payment_refund_ideas CHECK (idea_quantity > 0),
  CONSTRAINT ck_payment_refund_amount CHECK (amount_minor > 0),
  CONSTRAINT ck_payment_refund_currency CHECK (currency ~ '^[a-z]{3}$'),
  CONSTRAINT ck_payment_refund_ids CHECK (char_length(stripe_refund_id) <= 255 AND char_length(stripe_charge_id) BETWEEN 1 AND 255),
  CONSTRAINT ck_payment_refund_error CHECK (char_length(error_summary) <= 500),
  CONSTRAINT ck_payment_refund_terminal_facts CHECK (
    (status = 'pending' AND succeeded_at IS NULL AND failed_at IS NULL)
    OR (status = 'succeeded' AND succeeded_at IS NOT NULL AND failed_at IS NULL AND credit_transaction_id <> '')
    OR (status = 'failed' AND failed_at IS NOT NULL AND succeeded_at IS NULL)
    OR (status = 'anomaly' AND succeeded_at IS NULL AND failed_at IS NULL)
  )
);

CREATE INDEX idx_payment_refunds_order ON payment_refunds (payment_order_id, created_at DESC, id DESC);
CREATE INDEX idx_payment_refunds_pending ON payment_refunds (status, updated_at, id) WHERE status = 'pending';
CREATE UNIQUE INDEX uq_payment_refund_stripe_id ON payment_refunds (stripe_refund_id) WHERE stripe_refund_id <> '';

CREATE TRIGGER set_payment_refunds_updated_at
BEFORE UPDATE ON payment_refunds
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- statement:begin
CREATE FUNCTION enforce_payment_refund_transition()
RETURNS TRIGGER AS $$
BEGIN
  IF OLD.status IN ('succeeded', 'failed', 'anomaly') AND ROW(
    NEW.status, NEW.stripe_refund_id, NEW.stripe_charge_id, NEW.credit_hold_transaction_id,
    NEW.credit_transaction_id, NEW.succeeded_at, NEW.failed_at
  ) IS DISTINCT FROM ROW(
    OLD.status, OLD.stripe_refund_id, OLD.stripe_charge_id, OLD.credit_hold_transaction_id,
    OLD.credit_transaction_id, OLD.succeeded_at, OLD.failed_at
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB408', MESSAGE = 'terminal payment refund facts are immutable';
  END IF;
  IF OLD.status = 'pending' AND NEW.status NOT IN ('pending', 'succeeded', 'failed') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB408', MESSAGE = 'invalid payment refund transition';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- statement:end

CREATE TRIGGER enforce_payment_refunds_transition
BEFORE UPDATE ON payment_refunds
FOR EACH ROW EXECUTE FUNCTION enforce_payment_refund_transition();

-- migrate:down
DROP TRIGGER IF EXISTS enforce_payment_refunds_transition ON payment_refunds;
DROP FUNCTION IF EXISTS enforce_payment_refund_transition();
DROP TRIGGER IF EXISTS set_payment_refunds_updated_at ON payment_refunds;
DROP TABLE IF EXISTS payment_refunds;
DROP FUNCTION IF EXISTS execute_credit_purchase_refund_v1(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);
ALTER TABLE credit_transactions
  DROP CONSTRAINT ck_credit_transactions_command_type,
  ADD CONSTRAINT ck_credit_transactions_command_type CHECK (
    command_type IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction', 'purchase_credit')
  );
ALTER TABLE payment_admin_actions
  DROP CONSTRAINT ck_payment_admin_action_type,
  DROP CONSTRAINT ck_payment_admin_action_target,
  ADD CONSTRAINT ck_payment_admin_action_type CHECK (action_type IN ('configure_stripe')),
  ADD CONSTRAINT ck_payment_admin_action_target CHECK (target_type = 'stripe_configuration' AND target_id = 'current');
