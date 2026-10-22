-- destructive: approved
-- Pre-launch permanent closure: remove the unused restore lifecycle.
-- Drop index "idx_organization_closures_org" from table: "organization_closures"
DROP INDEX "idx_organization_closures_org";
-- Drop index "uq_organization_closures_open_per_org" from table: "organization_closures"
DROP INDEX "uq_organization_closures_open_per_org";
-- Modify "organization_closures" table
ALTER TABLE "organization_closures" DROP CONSTRAINT "chk_organization_closures_restored_shape", DROP CONSTRAINT "chk_organization_closures_disposition", ADD CONSTRAINT "chk_organization_closures_disposition" CHECK (disposition = 'expired'::text), ALTER COLUMN "disposition" SET DEFAULT 'expired', DROP COLUMN "restored_at", DROP COLUMN "restored_by";
-- Create index "uq_organization_closures_org" to table: "organization_closures"
CREATE UNIQUE INDEX "uq_organization_closures_org" ON "organization_closures" ("organization_id");

-- Permanent closure also guards the canonical Wallet activation command.
CREATE OR REPLACE FUNCTION set_billing_account_wallet_status(p_billing_account_id bigint, p_status text) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  p_status := btrim(p_status);
  IF p_billing_account_id <= 0 OR p_status NOT IN ('active', 'frozen') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid billing account wallet status command';
  END IF;
  -- Wallet locking serializes activation with permanent Organization closure.
  PERFORM billing_account_id FROM credit_wallets
  WHERE billing_account_id = p_billing_account_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
  IF p_status = 'active' AND EXISTS (
    SELECT 1 FROM billing_accounts accounts
    JOIN organizations org ON org.internal_id = accounts.organization_id
    WHERE accounts.internal_id = p_billing_account_id AND org.closed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB003', MESSAGE = 'closed organization wallet cannot be activated';
  END IF;
  UPDATE credit_wallets SET status = p_status, updated_at = now() WHERE billing_account_id = p_billing_account_id;
END;
$$;

DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.set_billing_account_wallet_status(BIGINT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
END;
$$;

-- Preserve reviewed exception refunds on the closed original payer pool.
CREATE OR REPLACE FUNCTION execute_credit_purchase_refund_hold_v2(
  p_billing_account_id BIGINT,
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
  billing_account_id BIGINT, available_balance_after BIGINT, reserved_balance_after BIGINT,
  replayed BOOLEAN, created_at TIMESTAMPTZ(3)
)
AS $$
DECLARE
  v_payload JSONB;
  v_hash TEXT;
  v_existing_hash TEXT;
  v_transaction_id VARCHAR(64);
  v_wallet_status VARCHAR(20);
  v_available_id BIGINT;
  v_reserved_id BIGINT;
  v_available_balance BIGINT;
  v_reserved_balance BIGINT;
BEGIN
  p_idempotency_key := btrim(p_idempotency_key);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source);
  p_reason := btrim(p_reason);
  p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_billing_account_id <= 0 OR p_amount <= 0 OR p_amount > 9000000000000000
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments'
     OR p_source <> 'stripe_refund_hold' OR p_reason = ''
     OR p_business_reference_type <> 'payment_refund'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit purchase refund hold command';
  END IF;

  v_payload := jsonb_build_object(
    'command_type', 'hold', 'billing_account_id', p_billing_account_id, 'amount', p_amount,
    'related_transaction_id', '',
    'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id),
    'source', p_source, 'reason', p_reason,
    'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id)
  );
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));
  SELECT request_hash INTO v_existing_hash
  FROM credit_transactions
  WHERE idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN
      RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
    END IF;
    RETURN QUERY SELECT existing.transaction_id, existing.command_type, existing.command_amount,
      existing.billing_account_id, existing.available_balance_after, existing.reserved_balance_after,
      TRUE, existing.created_at
    FROM credit_transactions existing
    WHERE existing.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;

  SELECT wallets.status INTO v_wallet_status
  FROM credit_wallets wallets
  WHERE wallets.billing_account_id = p_billing_account_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
  -- Permanent Organization closure forbids spending, but does not cancel
  -- legal or purchase-promise refund duties on the original payer account.
  -- Only this Payments-typed reservation may use that closed pool; other
  -- frozen Wallets retain their lifecycle gate.
  IF v_wallet_status <> 'active' AND NOT EXISTS (
    SELECT 1 FROM billing_accounts accounts
    JOIN organizations org ON org.internal_id = accounts.organization_id
    WHERE accounts.internal_id = p_billing_account_id AND org.closed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB003', MESSAGE = 'credit wallet frozen';
  END IF;

  PERFORM accounts.id
  FROM credit_accounts accounts
  WHERE accounts.billing_account_id = p_billing_account_id
    AND accounts.account_role IN ('user_available', 'user_reserved')
  ORDER BY accounts.id
  FOR UPDATE;
  SELECT MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE accounts.billing_account_id = p_billing_account_id
    AND accounts.account_role IN ('user_available', 'user_reserved');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_available_balance < p_amount THEN
    RAISE EXCEPTION USING ERRCODE = 'IB004', MESSAGE = 'insufficient credits';
  END IF;

  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(
    clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8'
  )), 'hex'), 1, 32);
  INSERT INTO credit_transactions (
    transaction_id, billing_account_id, command_type, idempotency_key, request_hash,
    actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
    command_amount, posting_count, available_balance_after, reserved_balance_after
  ) VALUES (
    v_transaction_id, p_billing_account_id, 'hold', p_idempotency_key, v_hash,
    p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id,
    p_amount, 2, v_available_balance - p_amount, v_reserved_balance + p_amount
  );
  INSERT INTO credit_postings (transaction_id, account_id, amount)
  VALUES (v_transaction_id, v_available_id, -p_amount), (v_transaction_id, v_reserved_id, p_amount);
  INSERT INTO credit_holds (hold_transaction_id, billing_account_id, amount)
  VALUES (v_transaction_id, p_billing_account_id, p_amount);

  RETURN QUERY SELECT created.transaction_id, created.command_type, created.command_amount,
    created.billing_account_id, created.available_balance_after, created.reserved_balance_after,
    FALSE, created.created_at
  FROM credit_transactions created
  WHERE created.transaction_id = v_transaction_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_purchase_refund_hold_v2(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
END;
$$;

REVOKE ALL ON FUNCTION execute_credit_purchase_refund_hold_v2(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
