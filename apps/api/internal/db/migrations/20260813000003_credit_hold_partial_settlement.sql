ALTER TABLE credit_holds
  DROP CONSTRAINT ck_credit_holds_terminal_consistency,
  DROP CONSTRAINT uq_credit_holds_terminal_transaction_id,
  DROP CONSTRAINT fk_credit_holds_terminal_transaction_id;

ALTER TABLE credit_holds
  ADD COLUMN captured_amount BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN released_amount BIGINT NOT NULL DEFAULT 0;

ALTER TABLE credit_holds
  ADD COLUMN capture_transaction_id VARCHAR(64),
  ADD COLUMN release_transaction_id VARCHAR(64);

ALTER TABLE credit_holds
  DROP COLUMN terminal_transaction_id,
  ADD CONSTRAINT fk_credit_holds_capture_transaction_id
    FOREIGN KEY (capture_transaction_id) REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_credit_holds_release_transaction_id
    FOREIGN KEY (release_transaction_id) REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT,
  ADD CONSTRAINT uq_credit_holds_capture_transaction_id UNIQUE (capture_transaction_id),
  ADD CONSTRAINT uq_credit_holds_release_transaction_id UNIQUE (release_transaction_id),
  ADD CONSTRAINT ck_credit_holds_settlement_amounts CHECK (
    captured_amount >= 0
    AND released_amount >= 0
    AND captured_amount + released_amount <= amount
  ),
  ADD CONSTRAINT ck_credit_holds_transaction_consistency CHECK (
    (captured_amount = 0) = (capture_transaction_id IS NULL)
    AND (released_amount = 0) = (release_transaction_id IS NULL)
  ),
  ADD CONSTRAINT ck_credit_holds_terminal_consistency CHECK (
    (status = 'held' AND captured_amount + released_amount < amount AND settled_at IS NULL)
    OR
    (status = 'captured' AND captured_amount > 0 AND captured_amount + released_amount = amount AND settled_at IS NOT NULL)
    OR
    (status = 'released' AND captured_amount = 0 AND released_amount = amount AND settled_at IS NOT NULL)
  );

CREATE OR REPLACE FUNCTION execute_credit_purchase_refund_v1(
  p_user_id BIGINT, p_hold_transaction_id TEXT, p_idempotency_key TEXT,
  p_actor_type TEXT, p_actor_id TEXT, p_source TEXT, p_reason TEXT,
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
  v_amount BIGINT; v_hold_status VARCHAR(20); v_captured_amount BIGINT;
  v_available_id BIGINT; v_reserved_id BIGINT; v_issuance_id BIGINT;
  v_available_balance BIGINT; v_reserved_balance BIGINT; v_shard_key VARCHAR(128);
BEGIN
  p_hold_transaction_id := btrim(p_hold_transaction_id); p_idempotency_key := btrim(p_idempotency_key);
  p_actor_type := btrim(p_actor_type); p_actor_id := btrim(p_actor_id); p_source := btrim(p_source);
  p_reason := btrim(p_reason); p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_user_id <= 0 OR p_hold_transaction_id = '' OR length(p_hold_transaction_id) > 64
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments' OR p_source <> 'stripe_refund'
     OR p_reason = '' OR p_business_reference_type <> 'payment_refund'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit purchase refund command';
  END IF;
  v_payload := jsonb_build_object('command_type', 'purchase_refund', 'user_id', p_user_id,
    'related_transaction_id', p_hold_transaction_id,
    'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id), 'source', p_source,
    'reason', p_reason, 'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id));
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
  SELECT holds.amount, holds.status, holds.captured_amount INTO v_amount, v_hold_status, v_captured_amount
  FROM credit_holds holds WHERE holds.hold_transaction_id = p_hold_transaction_id AND holds.wallet_user_id = p_user_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB005', MESSAGE = 'credit hold not found'; END IF;
  IF v_hold_status <> 'held' OR v_captured_amount <> 0 THEN RAISE EXCEPTION USING ERRCODE = 'IB006', MESSAGE = 'credit hold already settled'; END IF;
  PERFORM 1 FROM credit_wallets wallets WHERE wallets.user_id = p_user_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found'; END IF;
  v_shard_key := 'wallet:' || p_user_id::TEXT;
  PERFORM accounts.id FROM credit_accounts accounts
  WHERE (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_issuance')
  ORDER BY accounts.id FOR UPDATE;
  SELECT MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'), MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'system_issuance'), MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_issuance_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_issuance');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_issuance_id IS NULL OR v_reserved_balance < v_amount THEN
    RAISE EXCEPTION USING ERRCODE = 'IB004', MESSAGE = 'insufficient credits';
  END IF;
  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8')), 'hex'), 1, 32);
  INSERT INTO credit_transactions (transaction_id, wallet_user_id, command_type, idempotency_key, request_hash, actor_type, actor_id,
    source, reason, business_reference_type, business_reference_id, related_transaction_id, command_amount, posting_count, available_balance_after, reserved_balance_after)
  VALUES (v_transaction_id, p_user_id, 'purchase_refund', p_idempotency_key, v_hash, p_actor_type, p_actor_id, p_source, p_reason,
    p_business_reference_type, p_business_reference_id, p_hold_transaction_id, v_amount, 2, v_available_balance, v_reserved_balance - v_amount);
  INSERT INTO credit_postings (transaction_id, account_id, amount) VALUES (v_transaction_id, v_reserved_id, -v_amount), (v_transaction_id, v_issuance_id, v_amount);
  UPDATE credit_holds SET status = 'captured', captured_amount = captured_amount + v_amount, capture_transaction_id = v_transaction_id, settled_at = now()
  WHERE hold_transaction_id = p_hold_transaction_id;
  RETURN QUERY SELECT created.transaction_id, created.command_type, created.command_amount, created.wallet_user_id,
    created.available_balance_after, created.reserved_balance_after, FALSE, created.created_at FROM credit_transactions created WHERE created.transaction_id = v_transaction_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

CREATE OR REPLACE FUNCTION execute_credit_command_v2(
  p_command_type TEXT, p_user_id BIGINT, p_amount BIGINT, p_related_transaction_id TEXT,
  p_idempotency_key TEXT, p_actor_type TEXT, p_actor_id TEXT, p_source TEXT, p_reason TEXT,
  p_business_reference_type TEXT, p_business_reference_id TEXT
)
RETURNS TABLE (transaction_id VARCHAR(64), command_type VARCHAR(24), command_amount BIGINT,
  wallet_user_id BIGINT, available_balance_after BIGINT, reserved_balance_after BIGINT,
  replayed BOOLEAN, created_at TIMESTAMPTZ(3))
AS $$
DECLARE
  v_payload JSONB; v_hash TEXT; v_existing_hash TEXT; v_transaction_id VARCHAR(64);
  v_amount BIGINT := p_amount; v_wallet_status VARCHAR(20); v_hold_status VARCHAR(20);
  v_hold_amount BIGINT; v_captured_amount BIGINT; v_released_amount BIGINT;
  v_capture_transaction_id VARCHAR(64); v_release_transaction_id VARCHAR(64);
  v_charge_amount BIGINT; v_refunded_amount BIGINT; v_original_command_type VARCHAR(24); v_original_amount BIGINT;
  v_original_actor_type VARCHAR(24); v_original_source VARCHAR(64); v_original_posting_count INT; v_corrected_amount BIGINT;
  v_original_from_id BIGINT; v_original_to_id BIGINT; v_original_positive_amount BIGINT; v_original_negative_amount BIGINT;
  v_original_posting_rows BIGINT; v_original_related_transaction_id VARCHAR(64);
  v_available_id BIGINT; v_reserved_id BIGINT; v_issuance_id BIGINT; v_revenue_id BIGINT; v_sink_id BIGINT;
  v_available_balance BIGINT; v_reserved_balance BIGINT; v_available_after BIGINT; v_reserved_after BIGINT;
  v_from_id BIGINT; v_to_id BIGINT; v_terminal_status VARCHAR(20); v_shard_key VARCHAR(128);
BEGIN
  p_command_type := btrim(p_command_type); p_related_transaction_id := btrim(COALESCE(p_related_transaction_id, ''));
  p_idempotency_key := btrim(p_idempotency_key); p_actor_type := btrim(p_actor_type); p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source); p_reason := btrim(p_reason); p_business_reference_type := btrim(p_business_reference_type); p_business_reference_id := btrim(p_business_reference_id);
  IF p_user_id <= 0 OR p_command_type NOT IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction')
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160 OR p_actor_type NOT IN ('admin', 'system', 'service', 'user')
     OR p_actor_id = '' OR length(p_actor_id) > 128 OR p_source = '' OR length(p_source) > 64 OR p_reason = ''
     OR p_business_reference_type = '' OR length(p_business_reference_type) > 64 OR p_business_reference_id = '' OR length(p_business_reference_id) > 160
     OR (p_command_type IN ('deduction', 'correction') AND p_actor_type <> 'admin')
     OR (p_command_type IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction') AND (p_amount <= 0 OR p_amount > 9000000000000000))
     OR (p_command_type IN ('capture', 'release') AND p_related_transaction_id = '')
     OR (p_command_type IN ('refund', 'correction') AND (p_related_transaction_id = '' OR length(p_related_transaction_id) > 64)) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit command';
  END IF;
  v_payload := jsonb_build_object('command_type', p_command_type, 'user_id', p_user_id, 'amount', p_amount, 'related_transaction_id', p_related_transaction_id,
    'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id), 'source', p_source, 'reason', p_reason,
    'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id));
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));
  SELECT request_hash INTO v_existing_hash FROM credit_transactions WHERE idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict'; END IF;
    RETURN QUERY SELECT existing.transaction_id, existing.command_type, existing.command_amount, existing.wallet_user_id, existing.available_balance_after, existing.reserved_balance_after, TRUE, existing.created_at FROM credit_transactions existing WHERE existing.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;
  v_shard_key := 'wallet:' || p_user_id::TEXT;
  IF p_command_type = 'grant' THEN
    INSERT INTO credit_wallets (user_id) VALUES (p_user_id) ON CONFLICT DO NOTHING;
    INSERT INTO credit_accounts (wallet_user_id, account_role) VALUES (p_user_id, 'user_available'), (p_user_id, 'user_reserved') ON CONFLICT DO NOTHING;
    INSERT INTO credit_accounts (account_role, shard_key, metadata) VALUES ('system_issuance', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id)), ('system_revenue', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id)), ('system_sink', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id)) ON CONFLICT DO NOTHING;
  END IF;
  IF p_command_type IN ('capture', 'release') THEN
    SELECT amount, status, captured_amount, released_amount, capture_transaction_id, release_transaction_id INTO v_hold_amount, v_hold_status, v_captured_amount, v_released_amount, v_capture_transaction_id, v_release_transaction_id FROM credit_holds WHERE hold_transaction_id = p_related_transaction_id AND wallet_user_id = p_user_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB005', MESSAGE = 'credit hold not found'; END IF;
    IF v_hold_status <> 'held' THEN RAISE EXCEPTION USING ERRCODE = 'IB006', MESSAGE = 'credit hold already settled'; END IF;
    IF (p_command_type = 'capture' AND v_capture_transaction_id IS NOT NULL) OR (p_command_type = 'release' AND v_release_transaction_id IS NOT NULL) THEN RAISE EXCEPTION USING ERRCODE = 'IB006', MESSAGE = 'credit hold settlement command already recorded'; END IF;
    IF v_amount > v_hold_amount - v_captured_amount - v_released_amount THEN RAISE EXCEPTION USING ERRCODE = 'IB013', MESSAGE = 'credit hold settlement exceeds remaining amount'; END IF;
  ELSIF p_command_type = 'refund' THEN
    SELECT amount, refunded_amount INTO v_charge_amount, v_refunded_amount FROM credit_charges WHERE capture_transaction_id = p_related_transaction_id AND wallet_user_id = p_user_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB007', MESSAGE = 'credit charge not found'; END IF;
    IF p_amount > v_charge_amount - v_refunded_amount THEN RAISE EXCEPTION USING ERRCODE = 'IB008', MESSAGE = 'credit refund exceeds remaining charge'; END IF;
  ELSIF p_command_type = 'correction' THEN
    SELECT original_transaction.command_type, original_transaction.command_amount, original_transaction.actor_type,
      original_transaction.source, original_transaction.posting_count, COALESCE(original_transaction.related_transaction_id, '')
    INTO v_original_command_type, v_original_amount, v_original_actor_type, v_original_source,
      v_original_posting_count, v_original_related_transaction_id
    FROM credit_transactions original_transaction
    WHERE original_transaction.transaction_id = p_related_transaction_id
      AND original_transaction.wallet_user_id = p_user_id
    FOR UPDATE;
    IF NOT FOUND OR v_original_command_type NOT IN ('grant', 'deduction', 'refund')
       OR (v_original_actor_type <> 'admin' AND NOT (v_original_command_type = 'grant' AND v_original_source = 'reward_redemption')) THEN
      RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit correction';
    END IF;
    SELECT COUNT(*), MAX(amount) FILTER (WHERE amount > 0), MIN(amount) FILTER (WHERE amount < 0),
      MAX(account_id) FILTER (WHERE amount < 0), MAX(account_id) FILTER (WHERE amount > 0)
    INTO v_original_posting_rows, v_original_positive_amount, v_original_negative_amount,
      v_original_from_id, v_original_to_id
    FROM credit_postings original_posting
    WHERE original_posting.transaction_id = p_related_transaction_id;
    IF v_original_posting_count <> 2 OR v_original_posting_rows <> 2
       OR v_original_positive_amount <> v_original_amount OR v_original_negative_amount <> -v_original_amount
       OR v_original_from_id IS NULL OR v_original_to_id IS NULL THEN
      RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit correction postings';
    END IF;
    SELECT COALESCE(SUM(correction_transaction.command_amount), 0) INTO v_corrected_amount
    FROM credit_transactions correction_transaction
    WHERE correction_transaction.related_transaction_id = p_related_transaction_id
      AND correction_transaction.command_type = 'correction';
    IF p_amount > v_original_amount - v_corrected_amount THEN
      RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'credit correction exceeds original amount';
    END IF;
    IF v_original_command_type = 'refund' THEN
      SELECT refunded_amount INTO v_refunded_amount FROM credit_charges
      WHERE capture_transaction_id = v_original_related_transaction_id AND wallet_user_id = p_user_id FOR UPDATE;
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
     AND EXISTS (SELECT 1 FROM credit_wallet_restrictions WHERE wallet_user_id = p_user_id AND released_at IS NULL) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB011', MESSAGE = 'credit wallet restricted';
  END IF;
  PERFORM id FROM credit_accounts WHERE (wallet_user_id = p_user_id AND account_role IN ('user_available', 'user_reserved')) OR (wallet_user_id IS NULL AND shard_key = v_shard_key AND account_role IN ('system_issuance', 'system_revenue', 'system_sink')) ORDER BY id FOR UPDATE;
  SELECT MAX(id) FILTER (WHERE account_role = 'user_available'), MAX(id) FILTER (WHERE account_role = 'user_reserved'), MAX(id) FILTER (WHERE account_role = 'system_issuance'), MAX(id) FILTER (WHERE account_role = 'system_revenue'), MAX(id) FILTER (WHERE account_role = 'system_sink'), MAX(balance) FILTER (WHERE account_role = 'user_available'), MAX(balance) FILTER (WHERE account_role = 'user_reserved') INTO v_available_id, v_reserved_id, v_issuance_id, v_revenue_id, v_sink_id, v_available_balance, v_reserved_balance FROM credit_accounts WHERE (wallet_user_id = p_user_id AND account_role IN ('user_available', 'user_reserved')) OR (wallet_user_id IS NULL AND shard_key = v_shard_key AND account_role IN ('system_issuance', 'system_revenue', 'system_sink'));
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_issuance_id IS NULL OR v_revenue_id IS NULL OR v_sink_id IS NULL THEN RAISE EXCEPTION 'credit wallet account set is incomplete'; END IF;
  CASE p_command_type WHEN 'grant' THEN v_from_id := v_issuance_id; v_to_id := v_available_id; WHEN 'deduction' THEN v_from_id := v_available_id; v_to_id := v_sink_id; WHEN 'correction' THEN v_from_id := v_original_to_id; v_to_id := v_original_from_id; WHEN 'hold' THEN v_from_id := v_available_id; v_to_id := v_reserved_id; WHEN 'capture' THEN v_from_id := v_reserved_id; v_to_id := v_revenue_id; WHEN 'release' THEN v_from_id := v_reserved_id; v_to_id := v_available_id; WHEN 'refund' THEN v_from_id := v_revenue_id; v_to_id := v_available_id; END CASE;
  v_available_after := v_available_balance; v_reserved_after := v_reserved_balance;
  IF v_from_id = v_available_id THEN v_available_after := v_available_after - v_amount; END IF; IF v_to_id = v_available_id THEN v_available_after := v_available_after + v_amount; END IF; IF v_from_id = v_reserved_id THEN v_reserved_after := v_reserved_after - v_amount; END IF; IF v_to_id = v_reserved_id THEN v_reserved_after := v_reserved_after + v_amount; END IF;
  IF v_available_after < 0 OR v_reserved_after < 0 THEN RAISE EXCEPTION USING ERRCODE = 'IB004', MESSAGE = 'insufficient credits'; END IF;
  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8')), 'hex'), 1, 32);
  INSERT INTO credit_transactions (transaction_id, wallet_user_id, command_type, idempotency_key, request_hash, actor_type, actor_id, source, reason, business_reference_type, business_reference_id, related_transaction_id, command_amount, posting_count, available_balance_after, reserved_balance_after) VALUES (v_transaction_id, p_user_id, p_command_type, p_idempotency_key, v_hash, p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id, NULLIF(p_related_transaction_id, ''), v_amount, 2, v_available_after, v_reserved_after);
  INSERT INTO credit_postings (transaction_id, account_id, amount) VALUES (v_transaction_id, v_from_id, -v_amount), (v_transaction_id, v_to_id, v_amount);
  IF p_command_type = 'hold' THEN INSERT INTO credit_holds (hold_transaction_id, wallet_user_id, amount) VALUES (v_transaction_id, p_user_id, v_amount);
  ELSIF p_command_type IN ('capture', 'release') THEN
    IF p_command_type = 'capture' THEN v_captured_amount := v_captured_amount + v_amount; v_capture_transaction_id := v_transaction_id; ELSE v_released_amount := v_released_amount + v_amount; v_release_transaction_id := v_transaction_id; END IF;
    v_terminal_status := CASE WHEN v_captured_amount + v_released_amount < v_hold_amount THEN 'held' WHEN v_captured_amount > 0 THEN 'captured' ELSE 'released' END;
    UPDATE credit_holds SET captured_amount = v_captured_amount, released_amount = v_released_amount, status = v_terminal_status, capture_transaction_id = v_capture_transaction_id, release_transaction_id = v_release_transaction_id, settled_at = CASE WHEN v_terminal_status = 'held' THEN NULL ELSE now() END WHERE hold_transaction_id = p_related_transaction_id;
    IF p_command_type = 'capture' THEN INSERT INTO credit_charges (capture_transaction_id, wallet_user_id, amount) VALUES (v_transaction_id, p_user_id, v_amount); END IF;
  ELSIF p_command_type = 'refund' THEN UPDATE credit_charges SET refunded_amount = refunded_amount + v_amount, updated_at = now() WHERE capture_transaction_id = p_related_transaction_id;
  ELSIF p_command_type = 'correction' AND v_original_command_type = 'refund' THEN UPDATE credit_charges SET refunded_amount = refunded_amount - v_amount, updated_at = now() WHERE capture_transaction_id = v_original_related_transaction_id;
  END IF;
  RETURN QUERY SELECT created.transaction_id, created.command_type, created.command_amount, created.wallet_user_id, created.available_balance_after, created.reserved_balance_after, FALSE, created.created_at FROM credit_transactions created WHERE created.transaction_id = v_transaction_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
