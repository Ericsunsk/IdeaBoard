-- migrate:up
CREATE TABLE credit_wallet_restrictions (
  restriction_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  wallet_user_id BIGINT NOT NULL,
  restriction_type VARCHAR(64) NOT NULL,
  source VARCHAR(64) NOT NULL,
  related_object_type VARCHAR(64) NOT NULL,
  related_object_id VARCHAR(160) NOT NULL,
  reason TEXT NOT NULL,
  created_by_actor_type VARCHAR(24) NOT NULL,
  created_by_actor_id VARCHAR(128) NOT NULL,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  released_at TIMESTAMPTZ(3),
  released_by_actor_type VARCHAR(24),
  released_by_actor_id VARCHAR(128),
  release_reason TEXT,
  CONSTRAINT fk_credit_wallet_restrictions_wallet_user_id
    FOREIGN KEY (wallet_user_id) REFERENCES credit_wallets(user_id) ON DELETE RESTRICT,
  CONSTRAINT ck_credit_wallet_restrictions_type
    CHECK (btrim(restriction_type) <> ''),
  CONSTRAINT ck_credit_wallet_restrictions_source
    CHECK (btrim(source) <> ''),
  CONSTRAINT ck_credit_wallet_restrictions_related_object
    CHECK (btrim(related_object_type) <> '' AND btrim(related_object_id) <> ''),
  CONSTRAINT ck_credit_wallet_restrictions_reason
    CHECK (length(btrim(reason)) BETWEEN 1 AND 1000),
  CONSTRAINT ck_credit_wallet_restrictions_created_actor
    CHECK (
      created_by_actor_type IN ('admin', 'system', 'service')
      AND btrim(created_by_actor_id) <> ''
    ),
  CONSTRAINT ck_credit_wallet_restrictions_release
    CHECK (
      (released_at IS NULL AND released_by_actor_type IS NULL AND released_by_actor_id IS NULL AND release_reason IS NULL)
      OR
      (
        released_at IS NOT NULL
        AND released_at >= created_at
        AND released_by_actor_type IN ('admin', 'system', 'service')
        AND btrim(released_by_actor_id) <> ''
        AND length(btrim(release_reason)) BETWEEN 1 AND 1000
      )
    )
);

CREATE UNIQUE INDEX uq_credit_wallet_restrictions_active_source
  ON credit_wallet_restrictions (
    wallet_user_id,
    restriction_type,
    source,
    related_object_type,
    related_object_id
  )
  WHERE released_at IS NULL;

CREATE INDEX idx_credit_wallet_restrictions_wallet_active
  ON credit_wallet_restrictions (wallet_user_id, created_at DESC, restriction_id)
  WHERE released_at IS NULL;

-- statement:begin
CREATE FUNCTION create_credit_wallet_restriction(
  p_user_id BIGINT,
  p_restriction_type TEXT,
  p_source TEXT,
  p_related_object_type TEXT,
  p_related_object_id TEXT,
  p_reason TEXT,
  p_actor_type TEXT,
  p_actor_id TEXT
)
RETURNS TABLE (restriction_id TEXT, replayed BOOLEAN)
AS $$
DECLARE
  v_restriction_id UUID;
BEGIN
  p_restriction_type := btrim(p_restriction_type);
  p_source := btrim(p_source);
  p_related_object_type := btrim(p_related_object_type);
  p_related_object_id := btrim(p_related_object_id);
  p_reason := btrim(p_reason);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);

  IF p_user_id <= 0
     OR p_restriction_type = '' OR length(p_restriction_type) > 64
     OR p_source = '' OR length(p_source) > 64
     OR p_related_object_type = '' OR length(p_related_object_type) > 64
     OR p_related_object_id = '' OR length(p_related_object_id) > 160
     OR p_reason = '' OR length(p_reason) > 1000
     OR p_actor_type NOT IN ('admin', 'system', 'service')
     OR p_actor_id = '' OR length(p_actor_id) > 128 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit wallet restriction';
  END IF;

  PERFORM 1 FROM credit_wallets wallets WHERE wallets.user_id = p_user_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;

  INSERT INTO credit_wallet_restrictions (
    wallet_user_id,
    restriction_type,
    source,
    related_object_type,
    related_object_id,
    reason,
    created_by_actor_type,
    created_by_actor_id
  )
  VALUES (
    p_user_id,
    p_restriction_type,
    p_source,
    p_related_object_type,
    p_related_object_id,
    p_reason,
    p_actor_type,
    p_actor_id
  )
  ON CONFLICT (
    wallet_user_id,
    restriction_type,
    source,
    related_object_type,
    related_object_id
  ) WHERE released_at IS NULL
  DO NOTHING
  RETURNING credit_wallet_restrictions.restriction_id INTO v_restriction_id;

  IF FOUND THEN
    RETURN QUERY SELECT v_restriction_id::TEXT, FALSE;
    RETURN;
  END IF;

  SELECT restrictions.restriction_id INTO v_restriction_id
  FROM credit_wallet_restrictions restrictions
  WHERE restrictions.wallet_user_id = p_user_id
    AND restrictions.restriction_type = p_restriction_type
    AND restrictions.source = p_source
    AND restrictions.related_object_type = p_related_object_type
    AND restrictions.related_object_id = p_related_object_id
    AND restrictions.released_at IS NULL;

  RETURN QUERY SELECT v_restriction_id::TEXT, TRUE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE FUNCTION release_credit_wallet_restriction(
  p_restriction_id TEXT,
  p_actor_type TEXT,
  p_actor_id TEXT,
  p_reason TEXT
)
RETURNS TABLE (restriction_id TEXT, replayed BOOLEAN)
AS $$
DECLARE
  v_restriction_id UUID;
  v_wallet_user_id BIGINT;
  v_replayed BOOLEAN;
BEGIN
  p_restriction_id := btrim(p_restriction_id);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  p_reason := btrim(p_reason);

  IF p_restriction_id !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
     OR p_actor_type NOT IN ('admin', 'system', 'service')
     OR p_actor_id = '' OR length(p_actor_id) > 128
     OR p_reason = '' OR length(p_reason) > 1000 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit wallet restriction release';
  END IF;

  v_restriction_id := p_restriction_id::UUID;
  SELECT restrictions.wallet_user_id INTO v_wallet_user_id
  FROM credit_wallet_restrictions restrictions
  WHERE restrictions.restriction_id = v_restriction_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB012', MESSAGE = 'credit wallet restriction not found';
  END IF;

  PERFORM 1 FROM credit_wallets wallets WHERE wallets.user_id = v_wallet_user_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;

  UPDATE credit_wallet_restrictions restrictions
  SET released_at = clock_timestamp(),
      released_by_actor_type = p_actor_type,
      released_by_actor_id = p_actor_id,
      release_reason = p_reason
  WHERE restrictions.restriction_id = v_restriction_id
    AND restrictions.released_at IS NULL;

  v_replayed := NOT FOUND;
  RETURN QUERY SELECT v_restriction_id::TEXT, v_replayed;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

ALTER FUNCTION execute_credit_command_v2(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT)
  RENAME TO execute_credit_command_v2_before_wallet_restrictions;

ALTER FUNCTION execute_credit_command_v2_before_wallet_restrictions(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT)
  SECURITY INVOKER;

REVOKE ALL ON FUNCTION execute_credit_command_v2_before_wallet_restrictions(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;

-- Commands replay an existing idempotency key before evaluating current Wallet
-- gates. New exposure is rejected for inactive or restricted Wallets, while
-- existing Hold settlement and authenticated Admin repair commands can converge.
-- statement:begin
CREATE FUNCTION execute_credit_command_v2(
  p_command_type TEXT, p_user_id BIGINT, p_amount BIGINT, p_related_transaction_id TEXT,
  p_idempotency_key TEXT, p_actor_type TEXT, p_actor_id TEXT, p_source TEXT, p_reason TEXT,
  p_business_reference_type TEXT, p_business_reference_id TEXT
)
RETURNS TABLE (
  transaction_id VARCHAR(64), command_type VARCHAR(24), command_amount BIGINT,
  wallet_user_id BIGINT, available_balance_after BIGINT, reserved_balance_after BIGINT,
  replayed BOOLEAN, created_at TIMESTAMPTZ(3)
)
AS $$
DECLARE
  v_payload JSONB; v_hash TEXT; v_existing_hash TEXT; v_transaction_id VARCHAR(64);
  v_amount BIGINT := p_amount; v_wallet_status VARCHAR(20); v_hold_status VARCHAR(20);
  v_charge_amount BIGINT; v_refunded_amount BIGINT; v_original_command_type VARCHAR(24); v_original_amount BIGINT;
  v_original_actor_type VARCHAR(24); v_original_source VARCHAR(64); v_original_posting_count INT; v_corrected_amount BIGINT;
  v_original_from_id BIGINT; v_original_to_id BIGINT; v_original_positive_amount BIGINT;
  v_original_negative_amount BIGINT; v_original_posting_rows BIGINT;
  v_original_related_transaction_id VARCHAR(64);
  v_available_id BIGINT; v_reserved_id BIGINT;
  v_issuance_id BIGINT; v_revenue_id BIGINT; v_sink_id BIGINT; v_available_balance BIGINT;
  v_reserved_balance BIGINT; v_available_after BIGINT; v_reserved_after BIGINT;
  v_from_id BIGINT; v_to_id BIGINT; v_terminal_status VARCHAR(20); v_shard_key VARCHAR(128);
BEGIN
  p_command_type := btrim(p_command_type); p_related_transaction_id := btrim(COALESCE(p_related_transaction_id, ''));
  p_idempotency_key := btrim(p_idempotency_key); p_actor_type := btrim(p_actor_type); p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source); p_reason := btrim(p_reason); p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_user_id <= 0 OR p_command_type NOT IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction')
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160 OR p_actor_type NOT IN ('admin', 'system', 'service', 'user')
     OR p_actor_id = '' OR length(p_actor_id) > 128 OR p_source = '' OR length(p_source) > 64 OR p_reason = ''
     OR p_business_reference_type = '' OR length(p_business_reference_type) > 64
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160
     OR (p_command_type IN ('deduction', 'correction') AND p_actor_type <> 'admin')
     OR (p_command_type IN ('grant', 'deduction', 'hold', 'refund', 'correction') AND (p_amount <= 0 OR p_amount > 9000000000000000))
     OR (p_command_type IN ('capture', 'release') AND (p_amount <> 0 OR p_related_transaction_id = ''))
     OR (p_command_type IN ('refund', 'correction') AND (p_related_transaction_id = '' OR length(p_related_transaction_id) > 64)) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit command';
  END IF;
  v_payload := jsonb_build_object('command_type', p_command_type, 'user_id', p_user_id, 'amount', p_amount,
    'related_transaction_id', p_related_transaction_id, 'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id),
    'source', p_source, 'reason', p_reason, 'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id));
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));
  SELECT request_hash INTO v_existing_hash FROM credit_transactions WHERE idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict'; END IF;
    RETURN QUERY SELECT existing.transaction_id, existing.command_type, existing.command_amount, existing.wallet_user_id,
      existing.available_balance_after, existing.reserved_balance_after, TRUE, existing.created_at
      FROM credit_transactions existing WHERE existing.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;
  v_shard_key := 'wallet:' || p_user_id::TEXT;
  IF p_command_type = 'grant' THEN
    INSERT INTO credit_wallets (user_id) VALUES (p_user_id) ON CONFLICT DO NOTHING;
    INSERT INTO credit_accounts (wallet_user_id, account_role) VALUES (p_user_id, 'user_available'), (p_user_id, 'user_reserved') ON CONFLICT DO NOTHING;
    INSERT INTO credit_accounts (account_role, shard_key, metadata) VALUES
      ('system_issuance', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id)),
      ('system_revenue', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id)),
      ('system_sink', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id)) ON CONFLICT DO NOTHING;
  END IF;
  IF p_command_type IN ('capture', 'release') THEN
    SELECT amount, status INTO v_amount, v_hold_status FROM credit_holds holds
      WHERE holds.hold_transaction_id = p_related_transaction_id AND holds.wallet_user_id = p_user_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB005', MESSAGE = 'credit hold not found'; END IF;
    IF v_hold_status <> 'held' THEN RAISE EXCEPTION USING ERRCODE = 'IB006', MESSAGE = 'credit hold already settled'; END IF;
  ELSIF p_command_type = 'refund' THEN
    SELECT amount, refunded_amount INTO v_charge_amount, v_refunded_amount FROM credit_charges charges
      WHERE charges.capture_transaction_id = p_related_transaction_id AND charges.wallet_user_id = p_user_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB007', MESSAGE = 'credit charge not found'; END IF;
    IF p_amount > v_charge_amount - v_refunded_amount THEN RAISE EXCEPTION USING ERRCODE = 'IB008', MESSAGE = 'credit refund exceeds remaining charge'; END IF;
  ELSIF p_command_type = 'correction' THEN
    SELECT original_transaction.command_type, original_transaction.command_amount, original_transaction.actor_type, original_transaction.source, original_transaction.posting_count, COALESCE(original_transaction.related_transaction_id, '') INTO v_original_command_type, v_original_amount, v_original_actor_type, v_original_source, v_original_posting_count, v_original_related_transaction_id
    FROM credit_transactions original_transaction
    WHERE original_transaction.transaction_id = p_related_transaction_id AND original_transaction.wallet_user_id = p_user_id
    FOR UPDATE;
    IF NOT FOUND OR v_original_command_type NOT IN ('grant', 'deduction', 'refund') OR (v_original_actor_type <> 'admin' AND NOT (v_original_command_type = 'grant' AND v_original_source = 'reward_redemption')) THEN
      RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit correction';
    END IF;
    SELECT COUNT(*), MAX(amount) FILTER (WHERE amount > 0), MIN(amount) FILTER (WHERE amount < 0),
      MAX(account_id) FILTER (WHERE amount < 0), MAX(account_id) FILTER (WHERE amount > 0)
    INTO v_original_posting_rows, v_original_positive_amount, v_original_negative_amount, v_original_from_id, v_original_to_id
    FROM credit_postings original_posting WHERE original_posting.transaction_id = p_related_transaction_id;
    IF v_original_posting_count <> 2 OR v_original_posting_rows <> 2 OR v_original_positive_amount <> v_original_amount OR v_original_negative_amount <> -v_original_amount OR v_original_from_id IS NULL OR v_original_to_id IS NULL THEN
      RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit correction postings';
    END IF;
    SELECT COALESCE(SUM(correction_transaction.command_amount), 0) INTO v_corrected_amount
    FROM credit_transactions correction_transaction
    WHERE correction_transaction.related_transaction_id = p_related_transaction_id AND correction_transaction.command_type = 'correction';
    IF p_amount > v_original_amount - v_corrected_amount THEN
      RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'credit correction exceeds original amount';
    END IF;
    IF v_original_command_type = 'refund' THEN
      SELECT refunded_amount INTO v_refunded_amount FROM credit_charges charge
      WHERE charge.capture_transaction_id = v_original_related_transaction_id AND charge.wallet_user_id = p_user_id FOR UPDATE;
      IF NOT FOUND OR p_amount > v_refunded_amount THEN
        RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid refund correction';
      END IF;
    END IF;
  END IF;
  SELECT status INTO v_wallet_status FROM credit_wallets WHERE user_id = p_user_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found'; END IF;
  IF v_wallet_status <> 'active'
     AND p_command_type NOT IN ('capture', 'release')
     AND NOT (p_command_type IN ('refund', 'correction') AND p_actor_type = 'admin') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB003', MESSAGE = 'credit wallet frozen';
  END IF;
  IF p_command_type IN ('grant', 'deduction', 'hold')
     AND EXISTS (
       SELECT 1 FROM credit_wallet_restrictions restrictions
       WHERE restrictions.wallet_user_id = p_user_id AND restrictions.released_at IS NULL
     ) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB011', MESSAGE = 'credit wallet restricted';
  END IF;
  PERFORM id FROM credit_accounts accounts WHERE (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role IN ('system_issuance', 'system_revenue', 'system_sink')) ORDER BY accounts.id FOR UPDATE;
  SELECT MAX(id) FILTER (WHERE account_role = 'user_available'), MAX(id) FILTER (WHERE account_role = 'user_reserved'),
    MAX(id) FILTER (WHERE account_role = 'system_issuance'), MAX(id) FILTER (WHERE account_role = 'system_revenue'),
    MAX(id) FILTER (WHERE account_role = 'system_sink'), MAX(balance) FILTER (WHERE account_role = 'user_available'),
    MAX(balance) FILTER (WHERE account_role = 'user_reserved') INTO v_available_id, v_reserved_id, v_issuance_id, v_revenue_id,
    v_sink_id, v_available_balance, v_reserved_balance FROM credit_accounts accounts WHERE
    (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved')) OR
    (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role IN ('system_issuance', 'system_revenue', 'system_sink'));
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_issuance_id IS NULL OR v_revenue_id IS NULL OR v_sink_id IS NULL THEN
    RAISE EXCEPTION 'credit wallet account set is incomplete';
  END IF;
  CASE p_command_type
    WHEN 'grant' THEN v_from_id := v_issuance_id; v_to_id := v_available_id;
    WHEN 'deduction' THEN v_from_id := v_available_id; v_to_id := v_sink_id;
    WHEN 'correction' THEN v_from_id := v_original_to_id; v_to_id := v_original_from_id;
    WHEN 'hold' THEN v_from_id := v_available_id; v_to_id := v_reserved_id;
    WHEN 'capture' THEN v_from_id := v_reserved_id; v_to_id := v_revenue_id; v_terminal_status := 'captured';
    WHEN 'release' THEN v_from_id := v_reserved_id; v_to_id := v_available_id; v_terminal_status := 'released';
    WHEN 'refund' THEN v_from_id := v_revenue_id; v_to_id := v_available_id;
  END CASE;
  v_available_after := v_available_balance; v_reserved_after := v_reserved_balance;
  IF v_from_id = v_available_id THEN v_available_after := v_available_after - v_amount; END IF;
  IF v_to_id = v_available_id THEN v_available_after := v_available_after + v_amount; END IF;
  IF v_from_id = v_reserved_id THEN v_reserved_after := v_reserved_after - v_amount; END IF;
  IF v_to_id = v_reserved_id THEN v_reserved_after := v_reserved_after + v_amount; END IF;
  IF v_available_after < 0 OR v_reserved_after < 0 THEN RAISE EXCEPTION USING ERRCODE = 'IB004', MESSAGE = 'insufficient credits'; END IF;
  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8')), 'hex'), 1, 32);
  INSERT INTO credit_transactions (transaction_id, wallet_user_id, command_type, idempotency_key, request_hash, actor_type, actor_id,
    source, reason, business_reference_type, business_reference_id, related_transaction_id, command_amount, posting_count, available_balance_after, reserved_balance_after)
    VALUES (v_transaction_id, p_user_id, p_command_type, p_idempotency_key, v_hash, p_actor_type, p_actor_id, p_source, p_reason,
    p_business_reference_type, p_business_reference_id, NULLIF(p_related_transaction_id, ''), v_amount, 2, v_available_after, v_reserved_after);
  INSERT INTO credit_postings (transaction_id, account_id, amount) VALUES (v_transaction_id, v_from_id, -v_amount), (v_transaction_id, v_to_id, v_amount);
  IF p_command_type = 'hold' THEN
    INSERT INTO credit_holds (hold_transaction_id, wallet_user_id, amount) VALUES (v_transaction_id, p_user_id, v_amount);
  ELSIF p_command_type IN ('capture', 'release') THEN
    UPDATE credit_holds SET status = v_terminal_status, terminal_transaction_id = v_transaction_id, settled_at = now() WHERE hold_transaction_id = p_related_transaction_id;
    IF p_command_type = 'capture' THEN INSERT INTO credit_charges (capture_transaction_id, wallet_user_id, amount) VALUES (v_transaction_id, p_user_id, v_amount); END IF;
  ELSIF p_command_type = 'refund' THEN
    UPDATE credit_charges SET refunded_amount = refunded_amount + v_amount, updated_at = now() WHERE capture_transaction_id = p_related_transaction_id;
  ELSIF p_command_type = 'correction' AND v_original_command_type = 'refund' THEN
    UPDATE credit_charges SET refunded_amount = refunded_amount - v_amount, updated_at = now() WHERE capture_transaction_id = v_original_related_transaction_id;
  END IF;
  RETURN QUERY SELECT created.transaction_id, created.command_type, created.command_amount, created.wallet_user_id, created.available_balance_after,
    created.reserved_balance_after, FALSE, created.created_at FROM credit_transactions created WHERE created.transaction_id = v_transaction_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format('ALTER FUNCTION %I.create_credit_wallet_restriction(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.release_credit_wallet_restriction(TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.execute_credit_command_v2(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION create_credit_wallet_restriction(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION release_credit_wallet_restriction(TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION execute_credit_command_v2(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;

-- migrate:down
DROP FUNCTION IF EXISTS execute_credit_command_v2(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);
ALTER FUNCTION execute_credit_command_v2_before_wallet_restrictions(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT)
  SECURITY DEFINER;
ALTER FUNCTION execute_credit_command_v2_before_wallet_restrictions(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT)
  RENAME TO execute_credit_command_v2;
REVOKE ALL ON FUNCTION execute_credit_command_v2(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
DROP FUNCTION IF EXISTS release_credit_wallet_restriction(TEXT, TEXT, TEXT, TEXT);
DROP FUNCTION IF EXISTS create_credit_wallet_restriction(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);
DROP TABLE IF EXISTS credit_wallet_restrictions;
