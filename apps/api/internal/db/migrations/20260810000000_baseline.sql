-- migrate:up
--
--




--
--



--
-- Name: SCHEMA public; Type: COMMENT; Schema: -; Owner: -
--



--
-- Name: apply_credit_posting_balances(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION apply_credit_posting_balances() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  posting_delta RECORD;
BEGIN
  FOR posting_delta IN
    SELECT account_id, asset_code, SUM(amount)::BIGINT AS amount
    FROM inserted_credit_postings
    GROUP BY account_id, asset_code
    ORDER BY account_id
  LOOP
    UPDATE credit_accounts
    SET balance = balance + posting_delta.amount
    WHERE id = posting_delta.account_id
      AND asset_code = posting_delta.asset_code;
    IF NOT FOUND THEN
      RAISE EXCEPTION 'credit account % for asset % does not exist', posting_delta.account_id, posting_delta.asset_code;
    END IF;
  END LOOP;
  RETURN NULL;
END;
$$;


--
-- Name: bind_credit_welcome_identity_key(text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION bind_credit_welcome_identity_key(p_key_fingerprint text) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $_$
DECLARE
  v_bound_fingerprint TEXT;
BEGIN
  p_key_fingerprint := btrim(p_key_fingerprint);
  IF p_key_fingerprint !~ '^[0-9a-f]{64}$' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid Welcome identity key fingerprint';
  END IF;

  PERFORM pg_advisory_xact_lock(5917936940700615137);
  SELECT key_fingerprint INTO v_bound_fingerprint
  FROM credit_welcome_identity_key_guard
  WHERE singleton = TRUE;

  IF NOT FOUND THEN
    INSERT INTO credit_welcome_identity_key_guard (singleton, key_fingerprint)
    VALUES (TRUE, p_key_fingerprint);
    RETURN;
  END IF;

  IF v_bound_fingerprint <> p_key_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE = 'IB010', MESSAGE = 'Welcome identity key does not match the permanently bound key';
  END IF;
END;
$_$;


--
-- Name: claim_credit_welcome(text, bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION claim_credit_welcome(p_identity_fingerprint text, p_user_id bigint) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $_$
BEGIN
  p_identity_fingerprint := btrim(p_identity_fingerprint);
  IF p_identity_fingerprint !~ '^[0-9a-f]{64}$'
     OR p_user_id <= 0 OR NOT EXISTS (SELECT 1 FROM users WHERE id = p_user_id) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid Credit Welcome claim';
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(p_identity_fingerprint, 8163442859039458091));
  IF EXISTS (
    SELECT 1 FROM credit_welcome_claims
    WHERE identity_fingerprint = p_identity_fingerprint OR claimed_user_id = p_user_id
  ) THEN
    RETURN FALSE;
  END IF;
  INSERT INTO credit_welcome_claims (identity_fingerprint, claimed_user_id)
  VALUES (p_identity_fingerprint, p_user_id);
  RETURN TRUE;
END;
$_$;


--
-- Name: create_credit_wallet_restriction(bigint, text, text, text, text, text, text, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION create_credit_wallet_restriction(p_user_id bigint, p_restriction_type text, p_source text, p_related_object_type text, p_related_object_id text, p_reason text, p_actor_type text, p_actor_id text) RETURNS TABLE(restriction_id text, replayed boolean)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
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
$$;


--
-- Name: delete_archived_billing_item(text, bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION delete_archived_billing_item(p_sku text, p_actor_id bigint) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $_$
DECLARE
  v_display_name TEXT;
  v_description TEXT;
  v_price_count BIGINT;
BEGIN
  p_sku := lower(btrim(p_sku));
  IF p_sku IS NULL
     OR p_sku !~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
     OR char_length(p_sku) > 128
     OR p_actor_id IS NULL
     OR p_actor_id <= 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid archived billing item delete command';
  END IF;

  SELECT skus.display_name, skus.description
  INTO v_display_name, v_description
  FROM billing_skus skus
  WHERE skus.sku = p_sku
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB103', MESSAGE = 'billing SKU not found';
  END IF;

  -- The row lock serializes this check with concurrent snapshot FK acquisition.
  IF EXISTS (SELECT 1 FROM task_billing_snapshots snapshots WHERE snapshots.sku = p_sku) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB108', MESSAGE = 'billing item has durable references';
  END IF;

  SELECT count(*) INTO v_price_count
  FROM billing_price_versions prices
  WHERE prices.sku = p_sku;

  INSERT INTO billing_admin_actions (
    action_type, sku, price_version_id, actor_id, reason, metadata
  ) VALUES (
    'delete_archived', p_sku, NULL, p_actor_id, 'archived item deleted',
    jsonb_build_object(
      'display_name', v_display_name,
      'description', v_description,
      'price_version_count', v_price_count,
      'outcome', 'deleted'
    )
  );

  PERFORM set_config('ideaboard.billing_archive_delete', p_sku, TRUE);
  UPDATE billing_skus SET current_price_version_id = NULL WHERE sku = p_sku;
  DELETE FROM billing_price_versions WHERE sku = p_sku;
  DELETE FROM billing_skus WHERE sku = p_sku;
  RETURN p_sku;
END;
$_$;


--
-- Name: delete_reward_code_v1(bigint, bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION delete_reward_code_v1(p_reward_code_id bigint, p_actor_user_id bigint) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  target_code RECORD;
BEGIN
  SELECT * INTO target_code FROM reward_codes WHERE id = p_reward_code_id FOR UPDATE;
  IF NOT FOUND THEN RETURN FALSE; END IF;
  IF target_code.redemption_count <> 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB516', MESSAGE = 'redeemed reward Code cannot be deleted';
  END IF;
  INSERT INTO reward_admin_actions (
    reward_code_id, reward_code_id_snapshot, action_type, actor_user_id, metadata, target_snapshot
  ) VALUES (
    target_code.id, target_code.id, 'delete_reward_code', p_actor_user_id, '{}'::JSONB,
    jsonb_build_object(
      'id', target_code.id, 'display_name', target_code.display_name,
      'masked_code_hint', '•••• ' || target_code.code_hint,
      'credit_amount', target_code.credit_amount,
      'max_redemptions', target_code.max_redemptions,
      'redemption_count', target_code.redemption_count,
      'expires_at', target_code.expires_at, 'enabled', target_code.enabled
    )
  );
  DELETE FROM reward_code_digest_aliases WHERE reward_code_id = p_reward_code_id;
  DELETE FROM reward_codes WHERE id = p_reward_code_id;
  RETURN TRUE;
END;
$$;


--
-- Name: enforce_payment_refund_transition(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION enforce_payment_refund_transition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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
$$;


--
-- Name: enforce_task_output_media_asset_owner(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION enforce_task_output_media_asset_owner() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  task_owner_id BIGINT;
  asset_owner_id BIGINT;
BEGIN
  IF NEW.media_asset_id IS NULL THEN
    RETURN NEW;
  END IF;

  SELECT t.user_id, a.owner_user_id
  INTO task_owner_id, asset_owner_id
  FROM tasks AS t
  JOIN media_assets AS a ON a.id = NEW.media_asset_id
  WHERE t.task_id = NEW.task_id;

  IF task_owner_id IS NULL THEN
    RAISE EXCEPTION 'task output media ownership check failed for task_id %, media_asset_id %', NEW.task_id, NEW.media_asset_id;
  END IF;
  IF task_owner_id IS DISTINCT FROM asset_owner_id THEN
    RAISE EXCEPTION 'task output media asset owner does not match task owner';
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: execute_credit_command_v2(text, bigint, bigint, text, text, text, text, text, text, text, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION execute_credit_command_v2(p_command_type text, p_user_id bigint, p_amount bigint, p_related_transaction_id text, p_idempotency_key text, p_actor_type text, p_actor_id text, p_source text, p_reason text, p_business_reference_type text, p_business_reference_id text) RETURNS TABLE(transaction_id character varying, command_type character varying, command_amount bigint, wallet_user_id bigint, available_balance_after bigint, reserved_balance_after bigint, replayed boolean, created_at timestamp with time zone)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
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
$$;


--
-- Name: execute_credit_payment_dispute_debit_v1(bigint, bigint, text, text, text, text, text, text, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION execute_credit_payment_dispute_debit_v1(p_user_id bigint, p_requested_amount bigint, p_idempotency_key text, p_actor_type text, p_actor_id text, p_source text, p_reason text, p_business_reference_type text, p_business_reference_id text) RETURNS TABLE(transaction_id character varying, command_type character varying, command_amount bigint, wallet_user_id bigint, available_balance_after bigint, reserved_balance_after bigint, replayed boolean, created_at timestamp with time zone, requested_amount bigint, unrecovered_amount bigint)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  v_payload JSONB;
  v_hash TEXT;
  v_existing_hash TEXT;
  v_transaction_id VARCHAR(64);
  v_debited_amount BIGINT;
  v_available_id BIGINT;
  v_reserved_id BIGINT;
  v_sink_id BIGINT;
  v_available_balance BIGINT;
  v_reserved_balance BIGINT;
  v_shard_key VARCHAR(128);
  v_created_at TIMESTAMPTZ(3);
BEGIN
  p_idempotency_key := btrim(p_idempotency_key);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source);
  p_reason := btrim(p_reason);
  p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_user_id <= 0 OR p_requested_amount <= 0 OR p_requested_amount > 9000000000000000
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments'
     OR p_source <> 'stripe_dispute' OR p_reason = ''
     OR p_business_reference_type <> 'payment_dispute'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit payment dispute debit command';
  END IF;

  v_payload := jsonb_build_object(
    'command_type', 'payment_dispute_debit', 'user_id', p_user_id, 'requested_amount', p_requested_amount,
    'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id),
    'source', p_source, 'reason', p_reason,
    'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id)
  );
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));

  SELECT request_hash INTO v_existing_hash
  FROM payment_dispute_debit_requests
  WHERE idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN
      RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
    END IF;
    RETURN QUERY
    SELECT COALESCE(request.credit_transaction_id, ''), 'payment_dispute_debit'::VARCHAR(24), request.debited_amount,
      request.wallet_user_id, request.available_balance_after, request.reserved_balance_after, TRUE, request.created_at,
      request.requested_amount, request.requested_amount - request.debited_amount
    FROM payment_dispute_debit_requests request
    WHERE request.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;
  IF EXISTS (SELECT 1 FROM credit_transactions WHERE idempotency_key = p_idempotency_key) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
  END IF;

  PERFORM 1 FROM credit_wallets WHERE user_id = p_user_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
  v_shard_key := 'wallet:' || p_user_id::TEXT;
  PERFORM accounts.id FROM credit_accounts accounts
  WHERE (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_sink')
  ORDER BY accounts.id FOR UPDATE;
  SELECT MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'system_sink'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_sink_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_sink');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_sink_id IS NULL THEN
    RAISE EXCEPTION 'credit wallet account set is incomplete';
  END IF;
  v_debited_amount := LEAST(p_requested_amount, v_available_balance);

  IF v_debited_amount = 0 THEN
    INSERT INTO payment_dispute_debit_requests (
      id, wallet_user_id, idempotency_key, request_hash, requested_amount, debited_amount,
      available_balance_after, reserved_balance_after,
      business_reference_type, business_reference_id
    ) VALUES (
      gen_random_uuid(), p_user_id, p_idempotency_key, v_hash, p_requested_amount, 0,
      v_available_balance, v_reserved_balance,
      p_business_reference_type, p_business_reference_id
    ) RETURNING payment_dispute_debit_requests.created_at INTO v_created_at;
    RETURN QUERY SELECT ''::VARCHAR(64), 'payment_dispute_debit'::VARCHAR(24), 0::BIGINT,
      p_user_id, v_available_balance, v_reserved_balance, FALSE, v_created_at, p_requested_amount, p_requested_amount;
    RETURN;
  END IF;

  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(
    clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8'
  )), 'hex'), 1, 32);
  INSERT INTO credit_transactions (
    transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
    actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
    command_amount, posting_count, available_balance_after, reserved_balance_after
  ) VALUES (
    v_transaction_id, p_user_id, 'payment_dispute_debit', p_idempotency_key, v_hash,
    p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id,
    v_debited_amount, 2, v_available_balance - v_debited_amount, v_reserved_balance
  );
  INSERT INTO credit_postings (transaction_id, account_id, amount)
  VALUES (v_transaction_id, v_available_id, -v_debited_amount), (v_transaction_id, v_sink_id, v_debited_amount);
  INSERT INTO payment_dispute_debit_requests (
      id, wallet_user_id, idempotency_key, request_hash, requested_amount, debited_amount,
      available_balance_after, reserved_balance_after,
      credit_transaction_id, business_reference_type, business_reference_id
    ) VALUES (
      gen_random_uuid(), p_user_id, p_idempotency_key, v_hash, p_requested_amount, v_debited_amount,
      v_available_balance - v_debited_amount, v_reserved_balance,
      v_transaction_id, p_business_reference_type, p_business_reference_id
  ) RETURNING payment_dispute_debit_requests.created_at INTO v_created_at;
  RETURN QUERY SELECT v_transaction_id, 'payment_dispute_debit'::VARCHAR(24), v_debited_amount,
    p_user_id, v_available_balance - v_debited_amount, v_reserved_balance, FALSE, v_created_at,
    p_requested_amount, p_requested_amount - v_debited_amount;
END;
$$;


--
-- Name: execute_credit_payment_dispute_restore_v1(bigint, text, bigint, text, text, text, text, text, text, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION execute_credit_payment_dispute_restore_v1(p_user_id bigint, p_original_transaction_id text, p_amount bigint, p_idempotency_key text, p_actor_type text, p_actor_id text, p_source text, p_reason text, p_business_reference_type text, p_business_reference_id text) RETURNS TABLE(transaction_id character varying, command_type character varying, command_amount bigint, wallet_user_id bigint, available_balance_after bigint, reserved_balance_after bigint, replayed boolean, created_at timestamp with time zone)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  v_payload JSONB;
  v_hash TEXT;
  v_existing_hash TEXT;
  v_transaction_id VARCHAR(64);
  v_original_amount BIGINT;
  v_restored_amount BIGINT;
  v_available_id BIGINT;
  v_reserved_id BIGINT;
  v_sink_id BIGINT;
  v_available_balance BIGINT;
  v_reserved_balance BIGINT;
  v_shard_key VARCHAR(128);
BEGIN
  p_original_transaction_id := btrim(p_original_transaction_id);
  p_idempotency_key := btrim(p_idempotency_key);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source);
  p_reason := btrim(p_reason);
  p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_user_id <= 0 OR p_original_transaction_id = '' OR length(p_original_transaction_id) > 64
     OR p_amount <= 0 OR p_amount > 9000000000000000
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments'
     OR p_source <> 'stripe_dispute' OR p_reason = ''
     OR p_business_reference_type <> 'payment_dispute'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit payment dispute restore command';
  END IF;

  v_payload := jsonb_build_object(
    'command_type', 'payment_dispute_restore', 'user_id', p_user_id, 'amount', p_amount,
    'related_transaction_id', p_original_transaction_id,
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
  IF EXISTS (SELECT 1 FROM payment_dispute_debit_requests WHERE idempotency_key = p_idempotency_key) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
  END IF;

  SELECT original.command_amount INTO v_original_amount
  FROM credit_transactions original
  WHERE original.transaction_id = p_original_transaction_id AND original.wallet_user_id = p_user_id
    AND original.command_type = 'payment_dispute_debit' AND original.actor_type = 'service' AND original.actor_id = 'payments'
    AND original.source = 'stripe_dispute' AND original.business_reference_type = p_business_reference_type
    AND original.business_reference_id = p_business_reference_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid payment dispute debit reference';
  END IF;
  SELECT COALESCE(SUM(restores.command_amount), 0) INTO v_restored_amount
  FROM credit_transactions restores
  WHERE restores.related_transaction_id = p_original_transaction_id AND restores.command_type = 'payment_dispute_restore';
  IF p_amount > v_original_amount - v_restored_amount THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'payment dispute restore exceeds debited amount';
  END IF;

  PERFORM 1 FROM credit_wallets WHERE user_id = p_user_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
  v_shard_key := 'wallet:' || p_user_id::TEXT;
  PERFORM accounts.id FROM credit_accounts accounts
  WHERE (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_sink')
  ORDER BY accounts.id FOR UPDATE;
  SELECT MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'system_sink'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_sink_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE (accounts.wallet_user_id = p_user_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_sink');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_sink_id IS NULL THEN
    RAISE EXCEPTION 'credit wallet account set is incomplete';
  END IF;
  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(
    clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8'
  )), 'hex'), 1, 32);
  INSERT INTO credit_transactions (
    transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
    actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
    related_transaction_id, command_amount, posting_count, available_balance_after, reserved_balance_after
  ) VALUES (
    v_transaction_id, p_user_id, 'payment_dispute_restore', p_idempotency_key, v_hash,
    p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id,
    p_original_transaction_id, p_amount, 2, v_available_balance + p_amount, v_reserved_balance
  );
  INSERT INTO credit_postings (transaction_id, account_id, amount)
  VALUES (v_transaction_id, v_sink_id, -p_amount), (v_transaction_id, v_available_id, p_amount);
  RETURN QUERY SELECT created.transaction_id, created.command_type, created.command_amount,
    created.wallet_user_id, created.available_balance_after, created.reserved_balance_after,
    FALSE, created.created_at
  FROM credit_transactions created WHERE created.transaction_id = v_transaction_id;
END;
$$;


--
-- Name: execute_credit_purchase_refund_hold_v1(bigint, bigint, text, text, text, text, text, text, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION execute_credit_purchase_refund_hold_v1(p_user_id bigint, p_amount bigint, p_idempotency_key text, p_actor_type text, p_actor_id text, p_source text, p_reason text, p_business_reference_type text, p_business_reference_id text) RETURNS TABLE(transaction_id character varying, command_type character varying, command_amount bigint, wallet_user_id bigint, available_balance_after bigint, reserved_balance_after bigint, replayed boolean, created_at timestamp with time zone)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
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
  IF p_user_id <= 0 OR p_amount <= 0 OR p_amount > 9000000000000000
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments'
     OR p_source <> 'stripe_refund_hold' OR p_reason = ''
     OR p_business_reference_type <> 'payment_refund'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit purchase refund hold command';
  END IF;

  v_payload := jsonb_build_object(
    'command_type', 'hold', 'user_id', p_user_id, 'amount', p_amount,
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
      existing.wallet_user_id, existing.available_balance_after, existing.reserved_balance_after,
      TRUE, existing.created_at
    FROM credit_transactions existing
    WHERE existing.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;

  SELECT wallets.status INTO v_wallet_status
  FROM credit_wallets wallets
  WHERE wallets.user_id = p_user_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
  IF v_wallet_status <> 'active' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB003', MESSAGE = 'credit wallet frozen';
  END IF;

  PERFORM accounts.id
  FROM credit_accounts accounts
  WHERE accounts.wallet_user_id = p_user_id
    AND accounts.account_role IN ('user_available', 'user_reserved')
  ORDER BY accounts.id
  FOR UPDATE;
  SELECT MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE accounts.wallet_user_id = p_user_id
    AND accounts.account_role IN ('user_available', 'user_reserved');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_available_balance < p_amount THEN
    RAISE EXCEPTION USING ERRCODE = 'IB004', MESSAGE = 'insufficient credits';
  END IF;

  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(
    clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8'
  )), 'hex'), 1, 32);
  INSERT INTO credit_transactions (
    transaction_id, wallet_user_id, command_type, idempotency_key, request_hash,
    actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
    command_amount, posting_count, available_balance_after, reserved_balance_after
  ) VALUES (
    v_transaction_id, p_user_id, 'hold', p_idempotency_key, v_hash,
    p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id,
    p_amount, 2, v_available_balance - p_amount, v_reserved_balance + p_amount
  );
  INSERT INTO credit_postings (transaction_id, account_id, amount)
  VALUES (v_transaction_id, v_available_id, -p_amount), (v_transaction_id, v_reserved_id, p_amount);
  INSERT INTO credit_holds (hold_transaction_id, wallet_user_id, amount)
  VALUES (v_transaction_id, p_user_id, p_amount);

  RETURN QUERY SELECT created.transaction_id, created.command_type, created.command_amount,
    created.wallet_user_id, created.available_balance_after, created.reserved_balance_after,
    FALSE, created.created_at
  FROM credit_transactions created
  WHERE created.transaction_id = v_transaction_id;
END;
$$;


--
-- Name: execute_credit_purchase_refund_v1(bigint, text, text, text, text, text, text, text, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION execute_credit_purchase_refund_v1(p_user_id bigint, p_hold_transaction_id text, p_idempotency_key text, p_actor_type text, p_actor_id text, p_source text, p_reason text, p_business_reference_type text, p_business_reference_id text) RETURNS TABLE(transaction_id character varying, command_type character varying, command_amount bigint, wallet_user_id bigint, available_balance_after bigint, reserved_balance_after bigint, replayed boolean, created_at timestamp with time zone)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
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
$$;


--
-- Name: execute_credit_purchase_v1(bigint, bigint, text, text, text, text, text, text, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION execute_credit_purchase_v1(p_user_id bigint, p_amount bigint, p_idempotency_key text, p_actor_type text, p_actor_id text, p_source text, p_reason text, p_business_reference_type text, p_business_reference_id text) RETURNS TABLE(transaction_id character varying, command_type character varying, command_amount bigint, wallet_user_id bigint, available_balance_after bigint, reserved_balance_after bigint, replayed boolean, created_at timestamp with time zone)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
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
$$;


--
-- Name: execute_credit_reconciliation(bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION execute_credit_reconciliation(p_run_id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  v_stale_after INTERVAL;
  v_finding RECORD;
  v_restriction_id TEXT;
BEGIN
  IF NOT pg_try_advisory_xact_lock(hashtextextended(current_schema(), 5283921922134398278)) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB302', MESSAGE = 'credit reconciliation already running';
  END IF;
  UPDATE credit_reconciliation_runs SET status = 'running', started_at = clock_timestamp(), completed_at = NULL,
    error_message = '', attempt_count = attempt_count + 1
  WHERE id = p_run_id AND status IN ('queued', 'failed')
  RETURNING stale_hold_after INTO v_stale_after;
  IF NOT FOUND THEN RETURN; END IF;

  CREATE TEMP TABLE reconciliation_scan (
    finding_type TEXT, severity TEXT, risk TEXT, wallet_user_id BIGINT,
    subject_type TEXT, subject_id TEXT, details JSONB
  ) ON COMMIT DROP;

  INSERT INTO reconciliation_scan
  SELECT 'ledger_postings_invalid', 'critical', 'high', transactions.wallet_user_id,
    'transaction', transactions.transaction_id,
    jsonb_build_object('declared_posting_count', transactions.posting_count, 'actual_posting_count', COUNT(postings.id),
      'asset_count', COUNT(DISTINCT postings.asset_code), 'posting_sum', COALESCE(SUM(postings.amount), 0),
      'debit_amount', COALESCE(-SUM(postings.amount) FILTER (WHERE postings.amount < 0), 0),
      'credit_amount', COALESCE(SUM(postings.amount) FILTER (WHERE postings.amount > 0), 0),
      'command_amount', transactions.command_amount)
  FROM credit_transactions transactions
  LEFT JOIN credit_postings postings ON postings.transaction_id = transactions.transaction_id
  GROUP BY transactions.transaction_id, transactions.wallet_user_id, transactions.posting_count, transactions.command_amount
  HAVING COUNT(postings.id) < 2 OR COUNT(postings.id) <> transactions.posting_count
      OR COUNT(DISTINCT postings.asset_code) <> 1
      OR BOOL_OR(postings.asset_code IS DISTINCT FROM transactions.asset_code)
      OR COALESCE(SUM(postings.amount), 0) <> 0
      OR COALESCE(-SUM(postings.amount) FILTER (WHERE postings.amount < 0), 0) <> transactions.command_amount
      OR COALESCE(SUM(postings.amount) FILTER (WHERE postings.amount > 0), 0) <> transactions.command_amount;

  INSERT INTO reconciliation_scan
  SELECT 'command_shape_invalid', 'critical', 'high', transactions.wallet_user_id, 'transaction', transactions.transaction_id,
    jsonb_build_object('command_type', transactions.command_type, 'actor_type', transactions.actor_type,
      'related_transaction_id', transactions.related_transaction_id, 'command_amount', transactions.command_amount)
  FROM credit_transactions transactions
  WHERE transactions.command_type NOT IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction', 'purchase_credit', 'purchase_refund', 'payment_dispute_debit', 'payment_dispute_restore')
    OR (transactions.command_type IN ('deduction', 'correction') AND transactions.actor_type <> 'admin')
    OR (transactions.command_type = 'purchase_credit' AND (
      transactions.actor_type <> 'service' OR transactions.actor_id <> 'payments'
      OR transactions.source <> 'stripe_purchase'
      OR transactions.business_reference_type <> 'payment_order'
    ))
    OR (transactions.command_type = 'purchase_refund' AND (
      transactions.actor_type <> 'service' OR transactions.actor_id <> 'payments'
      OR transactions.source <> 'stripe_refund'
      OR transactions.business_reference_type <> 'payment_refund'
    ))
    OR (transactions.command_type = 'payment_dispute_debit' AND (
      transactions.actor_type <> 'service' OR transactions.actor_id <> 'payments'
      OR transactions.source <> 'stripe_dispute'
      OR transactions.business_reference_type <> 'payment_dispute'
    ))
    OR (transactions.command_type = 'payment_dispute_restore' AND (
      transactions.actor_type <> 'service' OR transactions.actor_id <> 'payments'
      OR transactions.source <> 'stripe_dispute'
      OR transactions.business_reference_type <> 'payment_dispute'
    ))
    OR transactions.command_amount <= 0;

  INSERT INTO reconciliation_scan
  SELECT 'wallet_projection_mismatch', 'critical', 'high', accounts.wallet_user_id,
    'account', accounts.id::TEXT,
    jsonb_build_object('account_role', accounts.account_role, 'projected_balance', accounts.balance,
      'ledger_balance', COALESCE(SUM(postings.amount), 0))
  FROM credit_accounts accounts
  LEFT JOIN credit_postings postings ON postings.account_id = accounts.id
  WHERE accounts.wallet_user_id IS NOT NULL
  GROUP BY accounts.id, accounts.wallet_user_id, accounts.account_role, accounts.balance
  HAVING accounts.balance <> COALESCE(SUM(postings.amount), 0);

  INSERT INTO reconciliation_scan
  SELECT 'stale_hold', 'warning', 'low', holds.wallet_user_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('created_at', holds.created_at, 'age_seconds', EXTRACT(EPOCH FROM clock_timestamp() - holds.created_at)::BIGINT)
  FROM credit_holds holds WHERE holds.status = 'held' AND holds.created_at < clock_timestamp() - v_stale_after;

  INSERT INTO reconciliation_scan
  SELECT 'task_hold_missing_billing_snapshot', 'critical', 'high', transactions.wallet_user_id,
    'hold', transactions.transaction_id,
    jsonb_build_object('task_id', transactions.business_reference_id, 'billing_snapshot_exists', FALSE)
  FROM credit_transactions transactions
  LEFT JOIN task_billing_snapshots snapshots ON snapshots.hold_transaction_id = transactions.transaction_id
  WHERE transactions.command_type = 'hold' AND transactions.business_reference_type = 'task'
    AND snapshots.task_id IS NULL;

  INSERT INTO reconciliation_scan
  SELECT 'task_trace_missing', 'warning', 'low', snapshots.user_id,
    'task', snapshots.task_id,
    jsonb_build_object('hold_transaction_id', snapshots.hold_transaction_id, 'settlement_state', settlements.state)
  FROM task_billing_snapshots snapshots
  JOIN credit_transactions transactions ON transactions.transaction_id = snapshots.hold_transaction_id
    AND transactions.command_type = 'hold' AND transactions.command_amount = snapshots.total_credits
    AND transactions.wallet_user_id = snapshots.user_id
    AND transactions.business_reference_type = 'task' AND transactions.business_reference_id = snapshots.task_id
  JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  JOIN credit_holds holds ON holds.hold_transaction_id = snapshots.hold_transaction_id
    AND holds.wallet_user_id = snapshots.user_id AND holds.amount = snapshots.total_credits
    AND holds.status = settlements.state AND holds.terminal_transaction_id IS NOT DISTINCT FROM settlements.settlement_transaction_id
  LEFT JOIN tasks ON tasks.task_id = snapshots.task_id
  WHERE snapshots.total_credits > 0 AND tasks.task_id IS NULL
    AND settlements.state IN ('held', 'captured', 'released')
    AND (settlements.state = 'held' OR EXISTS (
      SELECT 1 FROM credit_transactions terminal
      WHERE terminal.transaction_id = settlements.settlement_transaction_id
        AND terminal.command_type = CASE settlements.state WHEN 'captured' THEN 'capture' ELSE 'release' END
        AND terminal.command_amount = snapshots.total_credits
        AND terminal.wallet_user_id = snapshots.user_id
        AND terminal.related_transaction_id = snapshots.hold_transaction_id
        AND terminal.business_reference_type = 'task' AND terminal.business_reference_id = snapshots.task_id
    ))
    AND NOT EXISTS (
      SELECT 1
      FROM credit_transactions ledger_transactions
      LEFT JOIN LATERAL (
        SELECT COUNT(*) AS posting_count, COUNT(DISTINCT postings.asset_code) AS asset_count,
          BOOL_OR(postings.asset_code IS DISTINCT FROM ledger_transactions.asset_code) AS asset_mismatch,
          COALESCE(SUM(postings.amount), 0) AS posting_sum,
          COALESCE(-SUM(postings.amount) FILTER (WHERE postings.amount < 0), 0) AS debit_amount,
          COALESCE(SUM(postings.amount) FILTER (WHERE postings.amount > 0), 0) AS credit_amount
        FROM credit_postings postings
        WHERE postings.transaction_id = ledger_transactions.transaction_id
      ) ledger ON TRUE
      WHERE ledger_transactions.transaction_id IN (transactions.transaction_id, settlements.settlement_transaction_id)
        AND (ledger.posting_count < 2 OR ledger.posting_count <> ledger_transactions.posting_count
          OR ledger.asset_count <> 1 OR ledger.asset_mismatch OR ledger.posting_sum <> 0
          OR ledger.debit_amount <> ledger_transactions.command_amount
          OR ledger.credit_amount <> ledger_transactions.command_amount)
    );

  INSERT INTO reconciliation_scan
  SELECT 'task_billing_fact_missing', 'critical', 'high', snapshots.user_id, 'task', snapshots.task_id,
    jsonb_build_object('hold_transaction_id', snapshots.hold_transaction_id, 'hold_exists', holds.hold_transaction_id IS NOT NULL,
      'settlement_exists', settlements.task_id IS NOT NULL, 'hold_command', hold_transactions.command_type,
      'hold_command_amount', hold_transactions.command_amount, 'expected_amount', snapshots.total_credits,
      'hold_wallet_matches', hold_transactions.wallet_user_id = snapshots.user_id,
      'hold_reference_matches', hold_transactions.business_reference_type = 'task' AND hold_transactions.business_reference_id = snapshots.task_id,
      'hold_projection_matches', holds.wallet_user_id = snapshots.user_id AND holds.amount = snapshots.total_credits,
      'settlement_state', settlements.state, 'terminal_command', terminal.command_type,
      'terminal_command_amount', terminal.command_amount)
  FROM task_billing_snapshots snapshots
  LEFT JOIN credit_transactions hold_transactions ON hold_transactions.transaction_id = snapshots.hold_transaction_id
  LEFT JOIN credit_holds holds ON holds.hold_transaction_id = snapshots.hold_transaction_id
  LEFT JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_transactions terminal ON terminal.transaction_id = settlements.settlement_transaction_id
  WHERE snapshots.total_credits > 0 AND (
    holds.hold_transaction_id IS NULL OR settlements.task_id IS NULL
    OR hold_transactions.command_type IS DISTINCT FROM 'hold'
    OR hold_transactions.command_amount IS DISTINCT FROM snapshots.total_credits
    OR hold_transactions.wallet_user_id IS DISTINCT FROM snapshots.user_id
    OR hold_transactions.business_reference_type IS DISTINCT FROM 'task'
    OR hold_transactions.business_reference_id IS DISTINCT FROM snapshots.task_id
    OR holds.wallet_user_id IS DISTINCT FROM snapshots.user_id
    OR holds.amount IS DISTINCT FROM snapshots.total_credits
    OR settlements.state NOT IN ('held', 'captured', 'released')
    OR holds.status IS DISTINCT FROM settlements.state
    OR holds.terminal_transaction_id IS DISTINCT FROM settlements.settlement_transaction_id
    OR (settlements.state IN ('captured', 'released') AND (
      terminal.transaction_id IS NULL
      OR terminal.command_type IS DISTINCT FROM CASE settlements.state WHEN 'captured' THEN 'capture' ELSE 'release' END
      OR terminal.command_amount IS DISTINCT FROM snapshots.total_credits
      OR terminal.wallet_user_id IS DISTINCT FROM snapshots.user_id
      OR terminal.related_transaction_id IS DISTINCT FROM snapshots.hold_transaction_id
      OR terminal.business_reference_type IS DISTINCT FROM 'task'
      OR terminal.business_reference_id IS DISTINCT FROM snapshots.task_id
    ))
  );

  INSERT INTO reconciliation_scan
  SELECT CASE
      WHEN tasks.status IN ('pending', 'running') THEN 'active_task_not_held'
      WHEN tasks.status = 'completed' AND (settlements.state IS DISTINCT FROM 'captured' OR holds.status IS DISTINCT FROM 'captured') THEN 'completed_task_not_captured'
      WHEN tasks.status IN ('failed', 'cancelled') AND (settlements.state IS DISTINCT FROM 'released' OR holds.status IS DISTINCT FROM 'released') THEN 'failed_task_not_released'
    END,
    'critical', 'high', snapshots.user_id, 'task', tasks.task_id,
    jsonb_build_object('task_status', tasks.status, 'settlement_state', settlements.state,
      'hold_status', holds.status, 'settlement_transaction_id', settlements.settlement_transaction_id)
  FROM task_billing_snapshots snapshots
  JOIN tasks ON tasks.task_id = snapshots.task_id
  LEFT JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_holds holds ON holds.hold_transaction_id = snapshots.hold_transaction_id
  WHERE snapshots.total_credits > 0 AND (
    (tasks.status IN ('pending', 'running') AND (settlements.state IS DISTINCT FROM 'held' OR holds.status IS DISTINCT FROM 'held'))
    OR (tasks.status = 'completed' AND (settlements.state IS DISTINCT FROM 'captured' OR holds.status IS DISTINCT FROM 'captured'))
    OR (tasks.status IN ('failed', 'cancelled') AND (settlements.state IS DISTINCT FROM 'released' OR holds.status IS DISTINCT FROM 'released')));

  INSERT INTO reconciliation_scan
  SELECT 'task_terminal_transaction_count', 'critical', 'high', snapshots.user_id, 'task', tasks.task_id,
    jsonb_build_object('task_status', tasks.status, 'expected_command',
      CASE WHEN tasks.status = 'completed' THEN 'capture' ELSE 'release' END, 'actual_count', COUNT(terminals.transaction_id))
  FROM task_billing_snapshots snapshots
  JOIN tasks ON tasks.task_id = snapshots.task_id
  LEFT JOIN credit_transactions terminals ON terminals.related_transaction_id = snapshots.hold_transaction_id
    AND terminals.command_type = CASE WHEN tasks.status = 'completed' THEN 'capture' ELSE 'release' END
    AND terminals.wallet_user_id = snapshots.user_id
    AND terminals.business_reference_type = 'task' AND terminals.business_reference_id = tasks.task_id
  WHERE snapshots.total_credits > 0 AND tasks.status IN ('completed', 'failed', 'cancelled')
  GROUP BY snapshots.user_id, tasks.task_id, tasks.status
  HAVING COUNT(terminals.transaction_id) <> 1;

  INSERT INTO reconciliation_scan
  WITH conflicts AS (
    SELECT idempotency_key, COUNT(*) AS transaction_count, COUNT(DISTINCT request_hash) AS request_hash_count
    FROM credit_transactions GROUP BY idempotency_key HAVING COUNT(DISTINCT request_hash) > 1
  )
  SELECT 'idempotency_request_hash_conflict', 'critical', 'high', transactions.wallet_user_id,
    'idempotency_key', conflicts.idempotency_key,
    jsonb_build_object('transaction_count', conflicts.transaction_count, 'request_hash_count', conflicts.request_hash_count)
  FROM conflicts JOIN credit_transactions transactions USING (idempotency_key)
  GROUP BY conflicts.idempotency_key, conflicts.transaction_count, conflicts.request_hash_count, transactions.wallet_user_id;

  INSERT INTO reconciliation_scan
  SELECT 'task_settlement_reference_invalid', 'critical', 'high', snapshots.user_id, 'task', snapshots.task_id,
    jsonb_build_object('settlement_state', settlements.state, 'settlement_transaction_id', settlements.settlement_transaction_id,
      'transaction_command', terminal.command_type, 'transaction_wallet_user_id', terminal.wallet_user_id,
      'business_reference_type', terminal.business_reference_type, 'business_reference_id', terminal.business_reference_id)
  FROM task_billing_snapshots snapshots
  JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_transactions terminal ON terminal.transaction_id = settlements.settlement_transaction_id
  WHERE settlements.state IN ('captured', 'released') AND (
    terminal.transaction_id IS NULL
    OR terminal.command_type IS DISTINCT FROM CASE settlements.state WHEN 'captured' THEN 'capture' ELSE 'release' END
    OR terminal.wallet_user_id IS DISTINCT FROM snapshots.user_id
    OR terminal.related_transaction_id IS DISTINCT FROM snapshots.hold_transaction_id
    OR terminal.business_reference_type IS DISTINCT FROM 'task'
    OR terminal.business_reference_id IS DISTINCT FROM snapshots.task_id
  );

  INSERT INTO reconciliation_scan
  SELECT 'duplicate_business_reference', 'error', 'high', wallet_user_id, 'business_reference',
    business_reference_type || ':' || business_reference_id || ':' || command_type,
    jsonb_build_object('command_type', command_type, 'transaction_count', COUNT(*))
  FROM credit_transactions
  GROUP BY wallet_user_id, business_reference_type, business_reference_id, command_type HAVING COUNT(*) > 1;

  INSERT INTO reconciliation_scan
  SELECT 'correction_amount_exceeds_original', 'critical', 'high', original.wallet_user_id,
    'transaction', original.transaction_id,
    jsonb_build_object('original_command_type', original.command_type, 'original_amount', original.command_amount,
      'corrected_amount', SUM(corrections.command_amount))
  FROM credit_transactions original
  JOIN credit_transactions corrections ON corrections.command_type = 'correction'
    AND corrections.related_transaction_id = original.transaction_id
  GROUP BY original.transaction_id, original.wallet_user_id, original.command_type, original.command_amount, original.actor_type
  HAVING original.command_type NOT IN ('grant', 'deduction', 'refund')
    OR original.actor_type <> 'admin' OR SUM(corrections.command_amount) > original.command_amount;

  INSERT INTO reconciliation_scan
  SELECT 'refund_amount_invalid', 'critical', 'high', charges.wallet_user_id, 'charge', charges.capture_transaction_id,
    jsonb_build_object('charge_amount', charges.amount, 'projected_refunded_amount', charges.refunded_amount,
      'ledger_refunded_amount', COALESCE(refunds.amount, 0) - COALESCE(corrections.amount, 0))
  FROM credit_charges charges
  LEFT JOIN LATERAL (
    SELECT SUM(command_amount) AS amount FROM credit_transactions
    WHERE command_type = 'refund' AND related_transaction_id = charges.capture_transaction_id
  ) refunds ON TRUE
  LEFT JOIN LATERAL (
    SELECT SUM(correction.command_amount) AS amount
    FROM credit_transactions correction
    JOIN credit_transactions refund ON refund.transaction_id = correction.related_transaction_id
    WHERE correction.command_type = 'correction' AND refund.command_type = 'refund'
      AND refund.related_transaction_id = charges.capture_transaction_id
  ) corrections ON TRUE
  WHERE charges.refunded_amount < 0 OR charges.refunded_amount > charges.amount
    OR charges.refunded_amount <> COALESCE(refunds.amount, 0) - COALESCE(corrections.amount, 0);

  INSERT INTO reconciliation_scan
  SELECT 'illegal_reference_chain', 'critical', 'high', transactions.wallet_user_id, 'transaction', transactions.transaction_id,
    jsonb_build_object('command_type', transactions.command_type, 'related_transaction_id', transactions.related_transaction_id,
      'related_command_type', related.command_type, 'related_wallet_user_id', related.wallet_user_id)
  FROM credit_transactions transactions
  LEFT JOIN credit_transactions related ON related.transaction_id = transactions.related_transaction_id
  WHERE (transactions.command_type IN ('grant', 'deduction', 'hold', 'purchase_credit', 'payment_dispute_debit') AND transactions.related_transaction_id IS NOT NULL)
    OR (transactions.command_type IN ('capture', 'release') AND
      (related.transaction_id IS NULL OR related.command_type <> 'hold' OR related.wallet_user_id <> transactions.wallet_user_id))
    OR (transactions.command_type = 'refund' AND
      (related.transaction_id IS NULL OR related.command_type <> 'capture' OR related.wallet_user_id <> transactions.wallet_user_id))
    OR (transactions.command_type = 'purchase_refund' AND
      (related.transaction_id IS NULL OR related.command_type <> 'hold' OR related.wallet_user_id <> transactions.wallet_user_id))
    OR (transactions.command_type = 'payment_dispute_restore' AND
      (related.transaction_id IS NULL OR related.command_type <> 'payment_dispute_debit' OR related.wallet_user_id <> transactions.wallet_user_id
       OR related.business_reference_type <> transactions.business_reference_type
       OR related.business_reference_id <> transactions.business_reference_id))
    OR (transactions.command_type = 'correction' AND
      (related.transaction_id IS NULL OR related.command_type NOT IN ('grant', 'deduction', 'refund')
       OR related.actor_type <> 'admin' OR related.wallet_user_id <> transactions.wallet_user_id));

  INSERT INTO reconciliation_scan
  SELECT 'payment_dispute_restore_exceeds_debit', 'critical', 'high', debit.wallet_user_id,
    'transaction', debit.transaction_id,
    jsonb_build_object('debited_amount', debit.command_amount, 'restored_amount', COALESCE(restores.amount, 0))
  FROM credit_transactions debit
  LEFT JOIN LATERAL (
    SELECT SUM(command_amount) AS amount
    FROM credit_transactions restore
    WHERE restore.command_type = 'payment_dispute_restore'
      AND restore.related_transaction_id = debit.transaction_id
  ) restores ON TRUE
  WHERE debit.command_type = 'payment_dispute_debit'
    AND COALESCE(restores.amount, 0) > debit.command_amount;

  INSERT INTO reconciliation_scan
  SELECT 'hold_state_invalid', 'critical', 'high', holds.wallet_user_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('hold_status', holds.status, 'hold_amount', holds.amount, 'hold_command', hold_tx.command_type,
      'terminal_command', terminal.command_type)
  FROM credit_holds holds
  LEFT JOIN credit_transactions hold_tx ON hold_tx.transaction_id = holds.hold_transaction_id
  LEFT JOIN credit_transactions terminal ON terminal.transaction_id = holds.terminal_transaction_id
  WHERE hold_tx.command_type IS DISTINCT FROM 'hold' OR hold_tx.wallet_user_id IS DISTINCT FROM holds.wallet_user_id
    OR hold_tx.command_amount IS DISTINCT FROM holds.amount
    OR (holds.status = 'held' AND holds.terminal_transaction_id IS NOT NULL)
    OR (holds.status = 'captured' AND ((terminal.command_type IS NULL OR terminal.command_type NOT IN ('capture', 'purchase_refund')) OR terminal.related_transaction_id IS DISTINCT FROM holds.hold_transaction_id))
    OR (holds.status = 'released' AND (terminal.command_type IS DISTINCT FROM 'release' OR terminal.related_transaction_id IS DISTINCT FROM holds.hold_transaction_id));

  INSERT INTO reconciliation_scan
  SELECT 'hold_terminal_transaction_count', 'critical', 'high', holds.wallet_user_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('hold_status', holds.status, 'terminal_transaction_count', COUNT(terminals.transaction_id))
  FROM credit_holds holds
  LEFT JOIN credit_transactions terminals ON terminals.related_transaction_id = holds.hold_transaction_id
    AND terminals.command_type IN ('capture', 'release', 'purchase_refund')
  GROUP BY holds.hold_transaction_id, holds.wallet_user_id, holds.status
  HAVING COUNT(terminals.transaction_id) <> CASE WHEN holds.status = 'held' THEN 0 ELSE 1 END;

  INSERT INTO reconciliation_scan
  SELECT 'charge_state_invalid', 'critical', 'high', charges.wallet_user_id, 'charge', charges.capture_transaction_id,
    jsonb_build_object('charge_amount', charges.amount, 'capture_command', capture.command_type, 'capture_amount', capture.command_amount)
  FROM credit_charges charges
  LEFT JOIN credit_transactions capture ON capture.transaction_id = charges.capture_transaction_id
  WHERE capture.command_type IS DISTINCT FROM 'capture' OR capture.wallet_user_id IS DISTINCT FROM charges.wallet_user_id
    OR capture.command_amount IS DISTINCT FROM charges.amount;

  WITH normalized AS (
    SELECT scan.*, encode(sha256(convert_to(concat_ws(E'\x1f', finding_type, COALESCE(wallet_user_id::TEXT, ''), subject_type, subject_id), 'UTF8')), 'hex') AS fingerprint
    FROM reconciliation_scan scan WHERE finding_type IS NOT NULL
  ), upserted AS (
    INSERT INTO credit_reconciliation_findings (
      fingerprint, finding_type, severity, risk, wallet_user_id, subject_type, subject_id, details,
      first_seen_run_id, last_seen_run_id
    ) SELECT fingerprint, finding_type, severity, risk, wallet_user_id, subject_type, subject_id, details, p_run_id, p_run_id
      FROM normalized
    ON CONFLICT (fingerprint) DO UPDATE SET
      severity = EXCLUDED.severity, risk = EXCLUDED.risk, wallet_user_id = EXCLUDED.wallet_user_id,
      details = EXCLUDED.details, status = 'open', isolated = FALSE, last_seen_run_id = p_run_id,
      last_seen_at = clock_timestamp(), occurrence_count = credit_reconciliation_findings.occurrence_count + 1,
      resolved_at = NULL
    RETURNING id
  ) INSERT INTO credit_reconciliation_run_findings (run_id, finding_id)
    SELECT p_run_id, id FROM upserted ON CONFLICT DO NOTHING;

  UPDATE credit_reconciliation_findings
  SET status = 'resolved', resolved_at = clock_timestamp()
  WHERE status = 'open' AND last_seen_run_id <> p_run_id;

  FOR v_finding IN
    SELECT restrictions.restriction_id::TEXT AS restriction_id,
           restrictions.wallet_user_id,
           findings.id AS finding_id
    FROM credit_wallet_restrictions restrictions
    JOIN credit_reconciliation_findings findings
      ON restrictions.related_object_type = 'finding'
     AND restrictions.related_object_id = findings.id::TEXT
    WHERE restrictions.restriction_type = 'ledger_integrity'
      AND restrictions.source = 'reconciliation'
      AND restrictions.released_at IS NULL
      AND (
        findings.status <> 'open'
        OR findings.risk <> 'high'
        OR findings.wallet_user_id IS NULL
        OR findings.wallet_user_id <> restrictions.wallet_user_id
        OR findings.last_seen_run_id <> p_run_id
      )
    ORDER BY restrictions.wallet_user_id, findings.id
  LOOP
    PERFORM release_credit_wallet_restriction(
      v_finding.restriction_id,
      'system',
      'credits-reconciliation',
      'finding no longer requires ledger integrity isolation'
    );
  END LOOP;

  FOR v_finding IN
    SELECT findings.id, findings.wallet_user_id, findings.finding_type
    FROM credit_reconciliation_findings findings
    WHERE findings.last_seen_run_id = p_run_id
      AND findings.status = 'open'
      AND findings.risk = 'high'
      AND findings.wallet_user_id IS NOT NULL
    ORDER BY findings.wallet_user_id, findings.id
  LOOP
    SELECT restriction_id INTO v_restriction_id
    FROM create_credit_wallet_restriction(
      v_finding.wallet_user_id,
      'ledger_integrity',
      'reconciliation',
      'finding',
      v_finding.id::TEXT,
      'high-risk reconciliation finding: ' || v_finding.finding_type,
      'system',
      'credits-reconciliation'
    );
  END LOOP;

  UPDATE credit_reconciliation_findings findings
  SET isolated = EXISTS (
    SELECT 1
    FROM credit_wallet_restrictions restrictions
    WHERE restrictions.wallet_user_id = findings.wallet_user_id
      AND restrictions.restriction_type = 'ledger_integrity'
      AND restrictions.source = 'reconciliation'
      AND restrictions.related_object_type = 'finding'
      AND restrictions.related_object_id = findings.id::TEXT
      AND restrictions.released_at IS NULL
  );

  UPDATE credit_reconciliation_run_findings links SET
    observed_severity = findings.severity,
    observed_risk = findings.risk,
    observed_details = findings.details,
    observed_status = findings.status,
    observed_isolated = findings.isolated,
    observed_occurrence_count = findings.occurrence_count,
    observed_first_seen_at = findings.first_seen_at,
    observed_last_seen_at = findings.last_seen_at,
    observed_resolved_at = findings.resolved_at
  FROM credit_reconciliation_findings findings
  WHERE links.run_id = p_run_id AND links.finding_id = findings.id;

  UPDATE credit_reconciliation_runs runs SET status = 'completed', completed_at = clock_timestamp(),
    finding_count = (SELECT COUNT(*) FROM credit_reconciliation_run_findings WHERE run_id = p_run_id),
    critical_count = (SELECT COUNT(*) FROM credit_reconciliation_findings WHERE last_seen_run_id = p_run_id AND severity = 'critical'),
    isolated_wallet_count = (SELECT COUNT(DISTINCT wallet_user_id) FROM credit_reconciliation_findings
      WHERE last_seen_run_id = p_run_id AND isolated)
  WHERE runs.id = p_run_id;
END;
$$;


--
-- Name: fail_credit_reconciliation(bigint, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION fail_credit_reconciliation(p_run_id bigint, p_error text) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  UPDATE credit_reconciliation_runs
  SET status = 'failed', started_at = COALESCE(started_at, clock_timestamp()), completed_at = clock_timestamp(),
      error_message = left(COALESCE(p_error, ''), 2000)
  WHERE id = p_run_id AND status IN ('queued', 'running');
END;
$$;


--
-- Name: get_auth_runtime_settings(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION get_auth_runtime_settings() RETURNS TABLE(registration_enabled boolean)
    LANGUAGE plpgsql STABLE SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  RETURN QUERY
  SELECT COALESCE(
      (
        SELECT lower(btrim(config.value)) = 'true'
        FROM system_configs AS config
        WHERE config.config_key = 'auth_registration_enabled'
        LIMIT 1
      ),
      FALSE
    );
END;
$$;


--
-- Name: guard_credit_account_projection(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION guard_credit_account_projection() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.balance IS DISTINCT FROM OLD.balance
     AND pg_trigger_depth() < 2 THEN
    RAISE EXCEPTION 'credit account balances are ledger projections and cannot be overwritten directly';
  END IF;
  IF NEW.wallet_user_id IS DISTINCT FROM OLD.wallet_user_id
     OR NEW.asset_code IS DISTINCT FROM OLD.asset_code
     OR NEW.account_role IS DISTINCT FROM OLD.account_role
     OR NEW.shard_key IS DISTINCT FROM OLD.shard_key THEN
    RAISE EXCEPTION 'credit account identity is immutable';
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: guard_credit_posting_transaction_scope(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION guard_credit_posting_transaction_scope() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM credit_transactions
    WHERE transaction_id = NEW.transaction_id
      AND xmin = pg_current_xact_id()::xid
  ) THEN
    RAISE EXCEPTION 'credit postings must be inserted in the transaction that created their ledger transaction';
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: guard_payment_dispute_lifecycle(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION guard_payment_dispute_lifecycle() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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
$$;


--
-- Name: guard_payment_order_lifecycle(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION guard_payment_order_lifecycle() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
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
$$;


--
-- Name: guard_reward_code_identity(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION guard_reward_code_identity() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
  IF NEW.redemption_count < OLD.redemption_count
     OR NEW.redemption_count > OLD.redemption_count + 1
     OR NEW.created_by IS DISTINCT FROM OLD.created_by
     OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
     OR NEW.request_fingerprint IS DISTINCT FROM OLD.request_fingerprint
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION USING ERRCODE = 'IB512', MESSAGE = 'reward Code immutable identity changed';
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: guard_task_billing_settlement_transition(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION guard_task_billing_settlement_transition() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  snapshot_total BIGINT;
  snapshot_user BIGINT;
  snapshot_hold TEXT;
  valid_terminal BOOLEAN;
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB202', MESSAGE = 'task billing settlements cannot be deleted';
  END IF;
  SELECT total_credits, user_id, hold_transaction_id
  INTO snapshot_total, snapshot_user, snapshot_hold
  FROM task_billing_snapshots
  WHERE task_id = NEW.task_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB205', MESSAGE = 'task billing settlement snapshot is missing';
  END IF;
  IF TG_OP = 'INSERT' THEN
    IF (snapshot_total = 0 AND NEW.state <> 'free')
       OR (snapshot_total > 0 AND NEW.state <> 'held') THEN
      RAISE EXCEPTION USING ERRCODE = 'IB206', MESSAGE = 'task billing initial settlement state does not match snapshot';
    END IF;
    RETURN NEW;
  END IF;
  IF NEW.task_id <> OLD.task_id
     OR OLD.state <> 'held'
     OR NEW.state NOT IN ('captured', 'released')
     OR NEW.settlement_transaction_id IS NULL
     OR NEW.settled_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = 'IB203', MESSAGE = 'invalid task billing settlement transition';
  END IF;
  SELECT EXISTS (
    SELECT 1
    FROM credit_transactions transactions
    WHERE transactions.transaction_id = NEW.settlement_transaction_id
      AND transactions.command_type = CASE NEW.state WHEN 'captured' THEN 'capture' ELSE 'release' END
      AND transactions.wallet_user_id = snapshot_user
      AND transactions.related_transaction_id = snapshot_hold
      AND transactions.business_reference_type = 'task'
      AND transactions.business_reference_id = NEW.task_id
  ) INTO valid_terminal;
  IF NOT valid_terminal THEN
    RAISE EXCEPTION USING ERRCODE = 'IB207', MESSAGE = 'task billing terminal settlement does not match Credit Ledger';
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: guard_task_submission_claim(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION guard_task_submission_claim() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  valid_claim BOOLEAN;
BEGIN
  IF TG_OP <> 'INSERT' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB208', MESSAGE = 'task submission claims are immutable';
  END IF;
  SELECT EXISTS (
    SELECT 1
    FROM task_billing_snapshots snapshots
    JOIN tasks ON tasks.task_id = snapshots.task_id
    WHERE snapshots.task_id = NEW.task_id
      AND snapshots.user_id = NEW.user_id
      AND tasks.user_id = NEW.user_id
      AND tasks.client_action_id = NEW.client_action_id
  ) INTO valid_claim;
  IF NOT valid_claim THEN
    RAISE EXCEPTION USING ERRCODE = 'IB209', MESSAGE = 'task submission claim does not match Task Billing facts';
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: has_unsettled_payment_state(bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION has_unsettled_payment_state(p_user_id bigint) RETURNS boolean
    LANGUAGE plpgsql STABLE SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  RETURN EXISTS (
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
  );
END;
$$;


--
-- Name: initialize_payment_order_reconciliation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION initialize_payment_order_reconciliation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  INSERT INTO payment_order_reconciliations (payment_order_id)
  VALUES (NEW.id);
  RETURN NEW;
END;
$$;


--
-- Name: prevent_api_account_provider_endpoint_change(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_api_account_provider_endpoint_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.provider_endpoint_id IS DISTINCT FROM OLD.provider_endpoint_id THEN
    RAISE EXCEPTION 'api_accounts.provider_endpoint_id is immutable; create a new account to switch ProviderEndpoints';
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: prevent_billing_admin_action_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_billing_admin_action_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB107', MESSAGE = 'billing admin actions are append-only';
END;
$$;


--
-- Name: prevent_billing_price_version_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_billing_price_version_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
  IF TG_OP = 'DELETE'
     AND current_setting('ideaboard.billing_archive_delete', TRUE) = OLD.sku THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION USING ERRCODE = 'IB105', MESSAGE = 'billing price versions are immutable';
END;
$$;


--
-- Name: prevent_billing_sku_identity_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_billing_sku_identity_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    IF current_setting('ideaboard.billing_archive_delete', TRUE) = OLD.sku THEN
      RETURN OLD;
    END IF;
    RAISE EXCEPTION USING ERRCODE = 'IB104', MESSAGE = 'billing SKU cannot be deleted or reused';
  END IF;
  IF NEW.sku <> OLD.sku THEN
    RAISE EXCEPTION USING ERRCODE = 'IB104', MESSAGE = 'billing SKU cannot be renamed';
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: prevent_credit_ledger_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_credit_ledger_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  RAISE EXCEPTION '% is immutable; append a reversing credit transaction instead', TG_TABLE_NAME;
END;
$$;


--
-- Name: prevent_payment_admin_action_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_payment_admin_action_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  RAISE EXCEPTION 'payment admin actions are append-only';
END;
$$;


--
-- Name: prevent_referenced_storage_profile_locator_change(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_referenced_storage_profile_locator_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM media_assets
    WHERE storage_profile_id = OLD.profile_id
  ) AND (
    NEW.provider IS DISTINCT FROM OLD.provider
    OR COALESCE(NEW.config->>'endpoint', '') IS DISTINCT FROM COALESCE(OLD.config->>'endpoint', '')
    OR COALESCE(NEW.config->>'bucket_name', '') IS DISTINCT FROM COALESCE(OLD.config->>'bucket_name', '')
    OR COALESCE(NEW.config->>'region_id', '') IS DISTINCT FROM COALESCE(OLD.config->>'region_id', '')
    OR COALESCE(NEW.config->>'force_path_style', 'false') IS DISTINCT FROM COALESCE(OLD.config->>'force_path_style', 'false')
  ) THEN
    RAISE EXCEPTION 'referenced storage profile locator is immutable; create a new profile';
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: prevent_reward_admin_action_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_reward_admin_action_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  IF TG_OP = 'UPDATE'
     AND OLD.reward_code_id IS NOT NULL
     AND NEW.reward_code_id IS NULL
     AND NEW.id IS NOT DISTINCT FROM OLD.id
     AND NEW.reward_code_id_snapshot IS NOT DISTINCT FROM OLD.reward_code_id_snapshot
     AND NEW.action_type IS NOT DISTINCT FROM OLD.action_type
     AND NEW.actor_user_id IS NOT DISTINCT FROM OLD.actor_user_id
     AND NEW.metadata IS NOT DISTINCT FROM OLD.metadata
     AND NEW.target_snapshot IS NOT DISTINCT FROM OLD.target_snapshot
     AND NEW.created_at IS NOT DISTINCT FROM OLD.created_at
     AND NOT EXISTS (SELECT 1 FROM reward_codes WHERE id = OLD.reward_code_id) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION USING ERRCODE = 'IB511', MESSAGE = 'reward admin actions are append-only';
END;
$$;


--
-- Name: prevent_system_settings_admin_action_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_system_settings_admin_action_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB401', MESSAGE = 'system settings admin actions are append-only';
END;
$$;


--
-- Name: prevent_task_billing_snapshot_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_task_billing_snapshot_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB201', MESSAGE = 'task billing snapshots are immutable';
END;
$$;


--
-- Name: provision_credit_wallet(bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION provision_credit_wallet(p_user_id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  v_shard_key VARCHAR(128);
BEGIN
  IF p_user_id <= 0 OR NOT EXISTS (SELECT 1 FROM users WHERE id = p_user_id) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit wallet owner';
  END IF;

  v_shard_key := 'wallet:' || p_user_id::TEXT;
  INSERT INTO credit_wallets (user_id) VALUES (p_user_id) ON CONFLICT DO NOTHING;
  INSERT INTO credit_accounts (wallet_user_id, account_role)
  VALUES (p_user_id, 'user_available'), (p_user_id, 'user_reserved')
  ON CONFLICT DO NOTHING;
  INSERT INTO credit_accounts (account_role, shard_key, metadata)
  VALUES
    ('system_issuance', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id)),
    ('system_revenue', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id)),
    ('system_sink', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id))
  ON CONFLICT DO NOTHING;
END;
$$;


--
-- Name: publish_billing_price(text, text, text, bigint, bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION publish_billing_price(p_sku text, p_display_name text, p_description text, p_unit_credits bigint, p_actor_id bigint) RETURNS TABLE(price_version_id bigint, version bigint, unit_credits bigint, created_by bigint, created_at timestamp with time zone)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $_$
DECLARE
  v_old_price_version_id BIGINT;
  v_old_version BIGINT;
  v_old_unit_credits BIGINT;
  v_version BIGINT;
  v_price_version_id BIGINT;
  v_materialized BOOLEAN;
  v_inserted BIGINT;
BEGIN
  p_sku := lower(btrim(p_sku));
  p_display_name := btrim(p_display_name);
  p_description := btrim(COALESCE(p_description, ''));
  IF p_sku IS NULL
     OR p_sku !~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
     OR char_length(p_sku) > 128
     OR p_display_name IS NULL
     OR char_length(p_display_name) NOT BETWEEN 1 AND 120
     OR char_length(p_description) > 1000
     OR p_unit_credits IS NULL
     OR p_unit_credits < 0
     OR p_unit_credits > 9000000000000000
     OR p_actor_id IS NULL
     OR p_actor_id <= 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing price publish command';
  END IF;

  INSERT INTO billing_skus (
    sku, display_name, description, enabled, created_by, updated_by
  ) VALUES (
    p_sku, p_display_name, p_description, TRUE, p_actor_id, p_actor_id
  ) ON CONFLICT (sku) DO NOTHING;
  GET DIAGNOSTICS v_inserted = ROW_COUNT;
  v_materialized := v_inserted > 0;

  SELECT skus.current_price_version_id, prices.version, prices.unit_credits
  INTO v_old_price_version_id, v_old_version, v_old_unit_credits
  FROM billing_skus skus
  LEFT JOIN billing_price_versions prices
    ON prices.sku = skus.sku
   AND prices.price_version_id = skus.current_price_version_id
  WHERE skus.sku = p_sku
  FOR UPDATE OF skus;

  UPDATE billing_skus
  SET display_name = p_display_name,
      description = p_description,
      enabled = CASE WHEN v_old_price_version_id IS NULL THEN TRUE ELSE enabled END,
      updated_by = p_actor_id,
      updated_at = clock_timestamp()
  WHERE sku = p_sku;

  SELECT COALESCE(max(prices.version), 0) + 1
  INTO v_version
  FROM billing_price_versions prices
  WHERE prices.sku = p_sku;

  INSERT INTO billing_price_versions (sku, version, unit_credits, created_by)
  VALUES (p_sku, v_version, p_unit_credits, p_actor_id)
  RETURNING billing_price_versions.price_version_id INTO v_price_version_id;

  UPDATE billing_skus
  SET current_price_version_id = v_price_version_id,
      updated_by = p_actor_id,
      updated_at = clock_timestamp()
  WHERE sku = p_sku;

  INSERT INTO billing_admin_actions (
    action_type, sku, price_version_id, actor_id, reason, metadata
  ) VALUES (
    'create_price', p_sku, v_price_version_id, p_actor_id, 'price published',
    jsonb_build_object(
      'materialized', v_materialized,
      'old', CASE
        WHEN v_old_price_version_id IS NULL THEN NULL
        ELSE jsonb_build_object(
          'price_version_id', v_old_price_version_id,
          'version', v_old_version,
          'unit_credits', v_old_unit_credits
        )
      END,
      'new', jsonb_build_object(
        'price_version_id', v_price_version_id,
        'version', v_version,
        'unit_credits', p_unit_credits
      ),
      'outcome', 'published'
    )
  );

  RETURN QUERY
  SELECT prices.price_version_id, prices.version, prices.unit_credits,
         prices.created_by, prices.created_at
  FROM billing_price_versions prices
  WHERE prices.price_version_id = v_price_version_id;
END;
$_$;


--
-- Name: release_credit_wallet_restriction(text, text, text, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION release_credit_wallet_restriction(p_restriction_id text, p_actor_type text, p_actor_id text, p_reason text) RETURNS TABLE(restriction_id text, replayed boolean)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $_$
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
$_$;


--
-- Name: reserve_reward_code_redemption_v1(bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION reserve_reward_code_redemption_v1(p_reward_code_id bigint) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  changed BOOLEAN;
BEGIN
  UPDATE reward_codes
  SET redemption_count = redemption_count + 1
  WHERE id = p_reward_code_id
    AND enabled = TRUE
    AND (expires_at IS NULL OR expires_at > clock_timestamp())
    AND (max_redemptions = 0 OR redemption_count < max_redemptions);
  changed := FOUND;
  RETURN changed;
END;
$$;


--
-- Name: rotate_reward_code_v1(bigint, text, bytea, text, text, bytea, bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION rotate_reward_code_v1(p_reward_code_id bigint, p_key_version text, p_code_digest bytea, p_code_hint text, p_previous_key_version text, p_previous_code_digest bytea, p_actor_user_id bigint) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  before_code RECORD;
  after_code RECORD;
BEGIN
  SELECT * INTO before_code FROM reward_codes WHERE id = p_reward_code_id FOR UPDATE;
  IF NOT FOUND THEN RETURN FALSE; END IF;
  UPDATE reward_codes
  SET key_version = p_key_version, code_digest = p_code_digest, code_hint = p_code_hint
  WHERE id = p_reward_code_id RETURNING * INTO after_code;
  DELETE FROM reward_code_digest_aliases WHERE reward_code_id = p_reward_code_id;
  INSERT INTO reward_code_digest_aliases (reward_code_id, key_version, code_digest)
  VALUES (p_reward_code_id, p_key_version, p_code_digest);
  IF p_previous_key_version IS NOT NULL THEN
    INSERT INTO reward_code_digest_aliases (reward_code_id, key_version, code_digest)
    VALUES (p_reward_code_id, p_previous_key_version, p_previous_code_digest);
  END IF;
  INSERT INTO reward_admin_actions (
    reward_code_id, reward_code_id_snapshot, action_type, actor_user_id, metadata, target_snapshot
  ) VALUES (
    after_code.id, after_code.id, 'rotate_reward_code', p_actor_user_id,
    jsonb_build_object('previous_masked_code_hint', '•••• ' || before_code.code_hint),
    jsonb_build_object(
      'id', after_code.id, 'display_name', after_code.display_name,
      'masked_code_hint', '•••• ' || after_code.code_hint,
      'credit_amount', after_code.credit_amount,
      'max_redemptions', after_code.max_redemptions,
      'redemption_count', after_code.redemption_count,
      'expires_at', after_code.expires_at, 'enabled', after_code.enabled
    )
  );
  RETURN TRUE;
END;
$$;


--
-- Name: safe_jsonb_object(text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION safe_jsonb_object(raw text) RETURNS jsonb
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
  parsed JSONB;
BEGIN
  IF raw IS NULL OR btrim(raw) = '' THEN
    RETURN NULL;
  END IF;

  BEGIN
    parsed := raw::jsonb;
  EXCEPTION WHEN others THEN
    RETURN NULL;
  END;

  IF jsonb_typeof(parsed) = 'object' THEN
    RETURN parsed;
  END IF;
  RETURN NULL;
END;
$$;


--
-- Name: set_billing_item_availability(text, boolean, bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION set_billing_item_availability(p_sku text, p_enabled boolean, p_actor_id bigint) RETURNS TABLE(sku text, enabled boolean, updated_at timestamp with time zone)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $_$
DECLARE
  v_old_enabled BOOLEAN;
  v_price_version_id BIGINT;
  v_updated_at TIMESTAMPTZ(3);
  v_outcome TEXT;
BEGIN
  p_sku := lower(btrim(p_sku));
  IF p_sku IS NULL
     OR p_sku !~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
     OR char_length(p_sku) > 128
     OR p_enabled IS NULL
     OR p_actor_id IS NULL
     OR p_actor_id <= 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing availability command';
  END IF;

  SELECT skus.enabled, skus.current_price_version_id, skus.updated_at
  INTO v_old_enabled, v_price_version_id, v_updated_at
  FROM billing_skus skus
  WHERE skus.sku = p_sku
  FOR UPDATE;

  IF NOT FOUND OR v_price_version_id IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = 'IB104', MESSAGE = 'billing item has no published price';
  END IF;

  IF v_old_enabled IS DISTINCT FROM p_enabled THEN
    v_updated_at := clock_timestamp();
    UPDATE billing_skus
    SET enabled = p_enabled,
        updated_by = p_actor_id,
        updated_at = v_updated_at
    WHERE billing_skus.sku = p_sku;
    v_outcome := 'changed';
  ELSE
    v_outcome := 'unchanged';
  END IF;

  INSERT INTO billing_admin_actions (
    action_type, sku, price_version_id, actor_id, reason, metadata
  ) VALUES (
    'set_availability', p_sku, v_price_version_id, p_actor_id, 'availability updated',
    jsonb_build_object(
      'old', CASE WHEN v_old_enabled THEN 'active' ELSE 'disabled' END,
      'new', CASE WHEN p_enabled THEN 'active' ELSE 'disabled' END,
      'outcome', v_outcome
    )
  );

  RETURN QUERY SELECT p_sku, p_enabled, v_updated_at;
END;
$_$;


--
-- Name: set_credit_wallet_status(bigint, text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION set_credit_wallet_status(p_user_id bigint, p_status text) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  p_status := btrim(p_status);
  IF p_user_id <= 0 OR p_status NOT IN ('active', 'frozen') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit wallet status command';
  END IF;
  UPDATE credit_wallets SET status = p_status, updated_at = now() WHERE user_id = p_user_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
END;
$$;


--
-- Name: set_reward_code_enabled_v1(bigint, boolean, bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION set_reward_code_enabled_v1(p_reward_code_id bigint, p_enabled boolean, p_actor_user_id bigint) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  before_code RECORD;
  after_code RECORD;
BEGIN
  SELECT * INTO before_code FROM reward_codes WHERE id = p_reward_code_id FOR UPDATE;
  IF NOT FOUND THEN RETURN FALSE; END IF;
  IF before_code.enabled = p_enabled THEN RETURN TRUE; END IF;
  UPDATE reward_codes SET enabled = p_enabled WHERE id = p_reward_code_id RETURNING * INTO after_code;
  INSERT INTO reward_admin_actions (
    reward_code_id, reward_code_id_snapshot, action_type, actor_user_id, metadata, target_snapshot
  ) VALUES (
    after_code.id, after_code.id, 'set_reward_code_enabled', p_actor_user_id,
    jsonb_build_object('previous_enabled', before_code.enabled),
    jsonb_build_object(
      'id', after_code.id, 'display_name', after_code.display_name,
      'masked_code_hint', '•••• ' || after_code.code_hint,
      'credit_amount', after_code.credit_amount,
      'max_redemptions', after_code.max_redemptions,
      'redemption_count', after_code.redemption_count,
      'expires_at', after_code.expires_at, 'enabled', after_code.enabled
    )
  );
  RETURN TRUE;
END;
$$;


--
-- Name: set_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$;


--
-- Name: start_credit_reconciliation(text, bigint, text, timestamp with time zone, boolean); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION start_credit_reconciliation(p_trigger_type text, p_requested_by bigint, p_reason text, p_scheduled_for timestamp with time zone, p_only_if_overdue boolean) RETURNS TABLE(run_id bigint, enqueue_required boolean)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  v_run_id BIGINT;
  v_run_status VARCHAR(16);
BEGIN
  p_trigger_type := btrim(p_trigger_type);
  p_reason := NULLIF(btrim(p_reason, E' \t\n\r\f'), '');
  IF p_trigger_type IS NULL
     OR p_only_if_overdue IS NULL
     OR (
       p_trigger_type = 'manual'
       AND (
         p_requested_by IS NULL OR p_requested_by <= 0
         OR p_reason IS NULL OR char_length(p_reason) > 1000
         OR p_scheduled_for IS NOT NULL
         OR p_only_if_overdue
       )
     )
     OR (
       p_trigger_type = 'scheduled'
       AND (
         p_requested_by IS NOT NULL
         OR p_reason IS NOT NULL
         OR p_scheduled_for IS NULL
       )
     )
     OR p_trigger_type NOT IN ('manual', 'scheduled') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB301', MESSAGE = 'invalid credit reconciliation request';
  END IF;

  PERFORM pg_advisory_xact_lock(5283921922134398277);

  IF p_only_if_overdue AND EXISTS (
    SELECT 1
    FROM credit_reconciliation_runs
    WHERE status = 'completed'
      AND completed_at >= clock_timestamp() - INTERVAL '24 hours'
  ) THEN
    RETURN QUERY SELECT NULL::BIGINT, FALSE;
    RETURN;
  END IF;

  SELECT runs.id INTO v_run_id
  FROM credit_reconciliation_runs runs
  WHERE runs.status IN ('queued', 'running')
  ORDER BY runs.id DESC
  LIMIT 1;
  IF FOUND THEN
    RETURN QUERY SELECT v_run_id, FALSE;
    RETURN;
  END IF;

  IF p_trigger_type = 'scheduled' THEN
    SELECT runs.id, runs.status INTO v_run_id, v_run_status
    FROM credit_reconciliation_runs runs
    WHERE runs.trigger_type = 'scheduled'
      AND runs.scheduled_for = p_scheduled_for;
    IF FOUND THEN
      IF p_only_if_overdue AND v_run_status = 'failed' THEN
        UPDATE credit_reconciliation_runs
        SET status = 'queued',
            error_message = '',
            started_at = NULL,
            completed_at = NULL
        WHERE id = v_run_id;
        RETURN QUERY SELECT v_run_id, TRUE;
        RETURN;
      END IF;
      RETURN QUERY SELECT v_run_id, FALSE;
      RETURN;
    END IF;
  END IF;

  INSERT INTO credit_reconciliation_runs (
    trigger_type,
    requested_by,
    reason,
    scheduled_for
  )
  VALUES (
    p_trigger_type,
    p_requested_by,
    p_reason,
    p_scheduled_for
  )
  RETURNING id INTO v_run_id;

  RETURN QUERY SELECT v_run_id, TRUE;
END;
$$;


--
-- Name: update_reward_code_definition_v1(bigint, text, bigint, bigint, timestamp with time zone, bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION update_reward_code_definition_v1(p_reward_code_id bigint, p_display_name text, p_credit_amount bigint, p_max_redemptions bigint, p_expires_at timestamp with time zone, p_actor_user_id bigint) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  before_code RECORD;
  after_code RECORD;
BEGIN
  SELECT * INTO before_code FROM reward_codes WHERE id = p_reward_code_id FOR UPDATE;
  IF NOT FOUND THEN RETURN FALSE; END IF;
  IF p_max_redemptions > 0 AND p_max_redemptions < before_code.redemption_count THEN
    RAISE EXCEPTION USING ERRCODE = 'IB515', MESSAGE = 'reward Code cap is below redemption count';
  END IF;
  IF before_code.display_name IS NOT DISTINCT FROM p_display_name
     AND before_code.credit_amount IS NOT DISTINCT FROM p_credit_amount
     AND before_code.max_redemptions IS NOT DISTINCT FROM p_max_redemptions
     AND before_code.expires_at IS NOT DISTINCT FROM p_expires_at THEN
    RETURN TRUE;
  END IF;
  UPDATE reward_codes
  SET display_name = p_display_name, credit_amount = p_credit_amount,
      max_redemptions = p_max_redemptions, expires_at = p_expires_at
  WHERE id = p_reward_code_id RETURNING * INTO after_code;
  INSERT INTO reward_admin_actions (
    reward_code_id, reward_code_id_snapshot, action_type, actor_user_id, metadata, target_snapshot
  ) VALUES (
    after_code.id, after_code.id, 'update_reward_code', p_actor_user_id,
    jsonb_build_object(
      'previous_display_name', before_code.display_name,
      'previous_credit_amount', before_code.credit_amount,
      'previous_max_redemptions', before_code.max_redemptions,
      'previous_expires_at', before_code.expires_at
    ),
    jsonb_build_object(
      'id', after_code.id, 'display_name', after_code.display_name,
      'masked_code_hint', '•••• ' || after_code.code_hint,
      'credit_amount', after_code.credit_amount,
      'max_redemptions', after_code.max_redemptions,
      'redemption_count', after_code.redemption_count,
      'expires_at', after_code.expires_at, 'enabled', after_code.enabled
    )
  );
  RETURN TRUE;
END;
$$;


--
-- Name: validate_credit_transaction_balance(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION validate_credit_transaction_balance() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  checked_transaction_id VARCHAR(64);
  expected_count SMALLINT;
  expected_asset VARCHAR(32);
  actual_count BIGINT;
  actual_total NUMERIC;
  actual_assets BIGINT;
BEGIN
  checked_transaction_id := NEW.transaction_id;
  SELECT posting_count, asset_code
  INTO expected_count, expected_asset
  FROM credit_transactions
  WHERE transaction_id = checked_transaction_id;

  SELECT COUNT(*), COALESCE(SUM(amount), 0), COUNT(DISTINCT asset_code)
  INTO actual_count, actual_total, actual_assets
  FROM credit_postings
  WHERE transaction_id = checked_transaction_id;

  IF actual_count <> expected_count
     OR actual_total <> 0
     OR actual_assets <> 1
     OR EXISTS (
       SELECT 1
       FROM credit_postings
       WHERE transaction_id = checked_transaction_id
         AND asset_code <> expected_asset
     ) THEN
    RAISE EXCEPTION 'credit transaction % is not balanced for asset %', checked_transaction_id, expected_asset;
  END IF;
  RETURN NULL;
END;
$$;


--
-- Name: validate_task_billing_snapshot_insert(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION validate_task_billing_snapshot_insert() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  valid_hold BOOLEAN;
BEGIN
  IF NEW.total_credits = 0 THEN
    RETURN NEW;
  END IF;
  SELECT EXISTS (
    SELECT 1
    FROM credit_transactions transactions
    WHERE transactions.transaction_id = NEW.hold_transaction_id
      AND transactions.command_type = 'hold'
      AND transactions.wallet_user_id = NEW.user_id
      AND transactions.command_amount = NEW.total_credits
      AND transactions.business_reference_type = 'task'
      AND transactions.business_reference_id = NEW.task_id
  ) INTO valid_hold;
  IF NOT valid_hold THEN
    RAISE EXCEPTION USING ERRCODE = 'IB204', MESSAGE = 'task billing snapshot Hold does not match Credit Ledger';
  END IF;
  RETURN NEW;
END;
$$;




--
-- Name: api_account_models; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE api_account_models (
    id bigint NOT NULL,
    account_id bigint NOT NULL,
    model_name character varying(128) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    weight integer DEFAULT 1 NOT NULL,
    max_concurrent integer,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_api_account_models_max_concurrent CHECK (((max_concurrent IS NULL) OR ((max_concurrent >= 1) AND (max_concurrent <= 64)))),
    CONSTRAINT chk_api_account_models_metadata_object CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT chk_api_account_models_model_name_not_blank CHECK ((btrim((model_name)::text) <> ''::text)),
    CONSTRAINT chk_api_account_models_weight CHECK ((weight > 0))
);


--
-- Name: api_account_models_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE api_account_models ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME api_account_models_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: api_account_secrets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE api_account_secrets (
    id bigint NOT NULL,
    account_id bigint NOT NULL,
    secret_material_kind character varying(20) DEFAULT 'encrypted'::character varying NOT NULL,
    secret_payload text DEFAULT ''::text NOT NULL,
    key_version character varying(64) DEFAULT ''::character varying NOT NULL,
    fingerprint character varying(128) DEFAULT ''::character varying NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    rotated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    expires_at timestamp(3) with time zone,
    retired_at timestamp(3) with time zone,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_api_account_secrets_material CHECK ((((status)::text <> 'active'::text) OR (((secret_material_kind)::text = 'encrypted'::text) AND (btrim(secret_payload) <> ''::text)))),
    CONSTRAINT chk_api_account_secrets_material_kind CHECK (((secret_material_kind)::text = 'encrypted'::text)),
    CONSTRAINT chk_api_account_secrets_status CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'retired'::character varying, 'revoked'::character varying])::text[])))
);


--
-- Name: api_account_secrets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE api_account_secrets ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME api_account_secrets_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: api_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE api_accounts (
    id bigint NOT NULL,
    name character varying(100) NOT NULL,
    provider_endpoint_id bigint NOT NULL,
    max_concurrent integer DEFAULT 3 NOT NULL,
    timeout integer DEFAULT 120 NOT NULL,
    extra_config text NOT NULL,
    status smallint DEFAULT 1 NOT NULL,
    fail_count integer DEFAULT 0 NOT NULL,
    weight integer DEFAULT 1 NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp(3) with time zone,
    CONSTRAINT chk_api_accounts_deleted_status CHECK (((deleted_at IS NULL) OR (status = 0))),
    CONSTRAINT chk_api_accounts_fail_count CHECK ((fail_count >= 0)),
    CONSTRAINT chk_api_accounts_max_concurrent CHECK (((max_concurrent >= 1) AND (max_concurrent <= 64))),
    CONSTRAINT chk_api_accounts_name_not_blank CHECK ((btrim((name)::text) <> ''::text)),
    CONSTRAINT chk_api_accounts_provider_endpoint_id_positive CHECK ((provider_endpoint_id > 0)),
    CONSTRAINT chk_api_accounts_status CHECK ((status = ANY (ARRAY[0, 1]))),
    CONSTRAINT chk_api_accounts_timeout CHECK (((timeout >= 1) AND (timeout <= 600))),
    CONSTRAINT chk_api_accounts_weight CHECK ((weight > 0))
);


--
-- Name: api_accounts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE api_accounts ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME api_accounts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: billing_admin_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE billing_admin_actions (
    id bigint NOT NULL,
    action_type character varying(32) NOT NULL,
    sku character varying(128) NOT NULL,
    price_version_id bigint,
    actor_id bigint NOT NULL,
    reason character varying(1000) NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT billing_admin_actions_actor_id_check CHECK ((actor_id > 0)),
    CONSTRAINT ck_billing_admin_actions_metadata CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT ck_billing_admin_actions_price_scope CHECK (((((action_type)::text = ANY ((ARRAY['create_sku'::character varying, 'update_sku'::character varying, 'delete_archived'::character varying])::text[])) AND (price_version_id IS NULL)) OR (((action_type)::text = ANY ((ARRAY['create_price'::character varying, 'set_price_enabled'::character varying, 'set_availability'::character varying])::text[])) AND (price_version_id IS NOT NULL)))),
    CONSTRAINT ck_billing_admin_actions_reason CHECK ((((reason)::text = btrim((reason)::text, E' \t\n\r\f'::text)) AND ((char_length((reason)::text) >= 1) AND (char_length((reason)::text) <= 1000)))),
    CONSTRAINT ck_billing_admin_actions_type CHECK (((action_type)::text = ANY ((ARRAY['create_sku'::character varying, 'update_sku'::character varying, 'create_price'::character varying, 'set_price_enabled'::character varying, 'set_availability'::character varying, 'delete_archived'::character varying])::text[])))
);


--
-- Name: billing_admin_actions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE billing_admin_actions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME billing_admin_actions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: billing_price_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE billing_price_versions (
    price_version_id bigint NOT NULL,
    sku character varying(128) NOT NULL,
    version bigint NOT NULL,
    unit_credits bigint NOT NULL,
    created_by bigint NOT NULL,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT billing_price_versions_created_by_check CHECK ((created_by > 0)),
    CONSTRAINT billing_price_versions_unit_credits_check CHECK (((unit_credits >= 0) AND (unit_credits <= '9000000000000000'::bigint))),
    CONSTRAINT billing_price_versions_version_check CHECK ((version > 0))
);


--
-- Name: billing_price_versions_price_version_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE billing_price_versions ALTER COLUMN price_version_id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME billing_price_versions_price_version_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: billing_skus; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE billing_skus (
    sku character varying(128) NOT NULL,
    display_name character varying(120) NOT NULL,
    description character varying(1000) DEFAULT ''::character varying NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    current_price_version_id bigint,
    created_by bigint NOT NULL,
    updated_by bigint NOT NULL,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT billing_skus_canonical_check CHECK ((((sku)::text = lower(btrim((sku)::text))) AND ((char_length((sku)::text) >= 3) AND (char_length((sku)::text) <= 128)) AND ((sku)::text ~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'::text))),
    CONSTRAINT billing_skus_created_by_check CHECK ((created_by > 0)),
    CONSTRAINT billing_skus_display_name_check CHECK (((char_length(btrim((display_name)::text)) >= 1) AND (char_length(btrim((display_name)::text)) <= 120))),
    CONSTRAINT billing_skus_updated_by_check CHECK ((updated_by > 0))
);


--
-- Name: board_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE board_members (
    id bigint NOT NULL,
    board_id character varying(100) NOT NULL,
    user_id bigint NOT NULL,
    role character varying(20) NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_board_members_role CHECK (((role)::text = ANY ((ARRAY['editor'::character varying, 'viewer'::character varying])::text[])))
);


--
-- Name: board_members_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE board_members ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME board_members_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: board_share_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE board_share_tokens (
    id bigint NOT NULL,
    board_id character varying(100) NOT NULL,
    token_hash character varying(128) NOT NULL,
    token_prefix character varying(16) NOT NULL,
    role character varying(20) NOT NULL,
    created_by bigint NOT NULL,
    revoked boolean DEFAULT false NOT NULL,
    usage_count bigint DEFAULT 0 NOT NULL,
    max_uses integer,
    last_used_at timestamp(3) with time zone,
    revoked_at timestamp(3) with time zone,
    revoked_by bigint,
    expires_at timestamp(3) with time zone,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_board_share_tokens_revoked_at CHECK (((NOT revoked) OR (revoked_at IS NOT NULL))),
    CONSTRAINT ck_board_share_tokens_role CHECK (((role)::text = ANY ((ARRAY['editor'::character varying, 'viewer'::character varying])::text[]))),
    CONSTRAINT ck_board_share_tokens_token_hash_not_blank CHECK ((btrim((token_hash)::text) <> ''::text)),
    CONSTRAINT ck_board_share_tokens_token_prefix_not_blank CHECK ((btrim((token_prefix)::text) <> ''::text)),
    CONSTRAINT ck_board_share_tokens_usage CHECK (((usage_count >= 0) AND ((max_uses IS NULL) OR (max_uses > 0))))
);


--
-- Name: board_share_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE board_share_tokens ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME board_share_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: boards; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE boards (
    id bigint NOT NULL,
    board_id character varying(100) NOT NULL,
    owner_user_id bigint NOT NULL,
    created_by bigint NOT NULL,
    name character varying(60) NOT NULL,
    state jsonb NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    state_schema_version integer DEFAULT 1 NOT NULL,
    last_saved_by bigint,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_boards_revision CHECK ((revision > 0)),
    CONSTRAINT ck_boards_state_schema_version CHECK ((state_schema_version > 0))
);


--
-- Name: boards_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE boards ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME boards_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: credit_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_accounts (
    id bigint NOT NULL,
    wallet_user_id bigint,
    asset_code character varying(32) DEFAULT 'credits'::character varying NOT NULL,
    account_role character varying(32) NOT NULL,
    shard_key character varying(128) DEFAULT ''::character varying NOT NULL,
    balance bigint DEFAULT 0 NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_credit_accounts_asset CHECK (((asset_code)::text = 'credits'::text)),
    CONSTRAINT ck_credit_accounts_metadata_object CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT ck_credit_accounts_owner CHECK ((((wallet_user_id IS NOT NULL) AND ((account_role)::text = ANY ((ARRAY['user_available'::character varying, 'user_reserved'::character varying])::text[])) AND ((shard_key)::text = ''::text)) OR ((wallet_user_id IS NULL) AND ((account_role)::text = ANY ((ARRAY['system_issuance'::character varying, 'system_revenue'::character varying, 'system_sink'::character varying])::text[])) AND (btrim((shard_key)::text) <> ''::text)))),
    CONSTRAINT ck_credit_accounts_role CHECK (((account_role)::text = ANY ((ARRAY['user_available'::character varying, 'user_reserved'::character varying, 'system_issuance'::character varying, 'system_revenue'::character varying, 'system_sink'::character varying])::text[]))),
    CONSTRAINT ck_credit_accounts_user_balance_non_negative CHECK ((((account_role)::text <> ALL ((ARRAY['user_available'::character varying, 'user_reserved'::character varying])::text[])) OR (balance >= 0)))
);


--
-- Name: credit_accounts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE credit_accounts ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME credit_accounts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: credit_charges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_charges (
    capture_transaction_id character varying(64) NOT NULL,
    wallet_user_id bigint NOT NULL,
    amount bigint NOT NULL,
    refunded_amount bigint DEFAULT 0 NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_credit_charges_amount_positive CHECK ((amount > 0)),
    CONSTRAINT ck_credit_charges_refunded_range CHECK (((refunded_amount >= 0) AND (refunded_amount <= amount)))
);


--
-- Name: credit_holds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_holds (
    hold_transaction_id character varying(64) NOT NULL,
    wallet_user_id bigint NOT NULL,
    amount bigint NOT NULL,
    status character varying(20) DEFAULT 'held'::character varying NOT NULL,
    terminal_transaction_id character varying(64),
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    settled_at timestamp(3) with time zone,
    CONSTRAINT ck_credit_holds_amount_positive CHECK ((amount > 0)),
    CONSTRAINT ck_credit_holds_status CHECK (((status)::text = ANY ((ARRAY['held'::character varying, 'captured'::character varying, 'released'::character varying])::text[]))),
    CONSTRAINT ck_credit_holds_terminal_consistency CHECK (((((status)::text = 'held'::text) AND (terminal_transaction_id IS NULL) AND (settled_at IS NULL)) OR (((status)::text = ANY ((ARRAY['captured'::character varying, 'released'::character varying])::text[])) AND (terminal_transaction_id IS NOT NULL) AND (settled_at IS NOT NULL))))
);


--
-- Name: credit_postings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_postings (
    id bigint NOT NULL,
    transaction_id character varying(64) NOT NULL,
    account_id bigint NOT NULL,
    asset_code character varying(32) DEFAULT 'credits'::character varying NOT NULL,
    amount bigint NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_credit_postings_amount_non_zero CHECK ((amount <> 0)),
    CONSTRAINT ck_credit_postings_asset CHECK (((asset_code)::text = 'credits'::text))
);


--
-- Name: credit_postings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE credit_postings ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME credit_postings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: credit_reconciliation_findings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_reconciliation_findings (
    id bigint NOT NULL,
    fingerprint character(64) NOT NULL,
    finding_type character varying(64) NOT NULL,
    severity character varying(16) NOT NULL,
    risk character varying(16) NOT NULL,
    wallet_user_id bigint,
    subject_type character varying(32) NOT NULL,
    subject_id character varying(160) NOT NULL,
    details jsonb DEFAULT '{}'::jsonb NOT NULL,
    status character varying(16) DEFAULT 'open'::character varying NOT NULL,
    isolated boolean DEFAULT false NOT NULL,
    first_seen_run_id bigint NOT NULL,
    last_seen_run_id bigint NOT NULL,
    occurrence_count bigint DEFAULT 1 NOT NULL,
    first_seen_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    last_seen_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    resolved_at timestamp(3) with time zone,
    CONSTRAINT ck_credit_reconciliation_finding_resolution CHECK (((((status)::text = 'open'::text) AND (resolved_at IS NULL)) OR (((status)::text = 'resolved'::text) AND (resolved_at IS NOT NULL)))),
    CONSTRAINT ck_credit_reconciliation_finding_risk CHECK (((risk)::text = ANY ((ARRAY['low'::character varying, 'high'::character varying])::text[]))),
    CONSTRAINT ck_credit_reconciliation_finding_severity CHECK (((severity)::text = ANY ((ARRAY['warning'::character varying, 'error'::character varying, 'critical'::character varying])::text[]))),
    CONSTRAINT ck_credit_reconciliation_finding_status CHECK (((status)::text = ANY ((ARRAY['open'::character varying, 'resolved'::character varying])::text[]))),
    CONSTRAINT credit_reconciliation_findings_details_check CHECK ((jsonb_typeof(details) = 'object'::text)),
    CONSTRAINT credit_reconciliation_findings_fingerprint_check CHECK ((fingerprint ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT credit_reconciliation_findings_occurrence_count_check CHECK ((occurrence_count > 0))
);


--
-- Name: credit_reconciliation_findings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE credit_reconciliation_findings ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME credit_reconciliation_findings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: credit_reconciliation_run_findings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_reconciliation_run_findings (
    run_id bigint NOT NULL,
    finding_id bigint NOT NULL,
    observed_severity character varying(16) DEFAULT 'warning'::character varying NOT NULL,
    observed_risk character varying(16) DEFAULT 'low'::character varying NOT NULL,
    observed_details jsonb DEFAULT '{}'::jsonb NOT NULL,
    observed_status character varying(16) DEFAULT 'open'::character varying NOT NULL,
    observed_isolated boolean DEFAULT false NOT NULL,
    observed_occurrence_count bigint DEFAULT 1 NOT NULL,
    observed_first_seen_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    observed_last_seen_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    observed_resolved_at timestamp(3) with time zone,
    CONSTRAINT credit_reconciliation_run_findi_observed_occurrence_count_check CHECK ((observed_occurrence_count > 0)),
    CONSTRAINT credit_reconciliation_run_findings_observed_details_check CHECK ((jsonb_typeof(observed_details) = 'object'::text)),
    CONSTRAINT credit_reconciliation_run_findings_observed_risk_check CHECK (((observed_risk)::text = ANY ((ARRAY['low'::character varying, 'high'::character varying])::text[]))),
    CONSTRAINT credit_reconciliation_run_findings_observed_severity_check CHECK (((observed_severity)::text = ANY ((ARRAY['warning'::character varying, 'error'::character varying, 'critical'::character varying])::text[]))),
    CONSTRAINT credit_reconciliation_run_findings_observed_status_check CHECK (((observed_status)::text = ANY ((ARRAY['open'::character varying, 'resolved'::character varying])::text[])))
);


--
-- Name: credit_reconciliation_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_reconciliation_runs (
    id bigint NOT NULL,
    status character varying(16) DEFAULT 'queued'::character varying NOT NULL,
    requested_by bigint,
    stale_hold_after interval DEFAULT '24:00:00'::interval NOT NULL,
    finding_count bigint DEFAULT 0 NOT NULL,
    critical_count bigint DEFAULT 0 NOT NULL,
    isolated_wallet_count bigint DEFAULT 0 NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    requested_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    started_at timestamp(3) with time zone,
    completed_at timestamp(3) with time zone,
    reason character varying(1000),
    trigger_type character varying(16) NOT NULL,
    scheduled_for timestamp(3) with time zone,
    CONSTRAINT ck_credit_reconciliation_run_audit CHECK (((((trigger_type)::text = 'manual'::text) AND (requested_by IS NOT NULL) AND (requested_by > 0) AND (reason IS NOT NULL) AND ((reason)::text = btrim((reason)::text, E' \t\n\r\f'::text)) AND ((char_length((reason)::text) >= 1) AND (char_length((reason)::text) <= 1000)) AND (scheduled_for IS NULL)) OR (((trigger_type)::text = 'scheduled'::text) AND (requested_by IS NULL) AND (reason IS NULL) AND (scheduled_for IS NOT NULL)))),
    CONSTRAINT ck_credit_reconciliation_run_stale_hold_fixed CHECK ((stale_hold_after = '24:00:00'::interval)),
    CONSTRAINT ck_credit_reconciliation_run_status CHECK (((status)::text = ANY ((ARRAY['queued'::character varying, 'running'::character varying, 'completed'::character varying, 'failed'::character varying])::text[]))),
    CONSTRAINT ck_credit_reconciliation_run_timestamps CHECK (((((status)::text = 'queued'::text) AND (completed_at IS NULL)) OR (((status)::text = 'running'::text) AND (started_at IS NOT NULL) AND (completed_at IS NULL)) OR (((status)::text = ANY ((ARRAY['completed'::character varying, 'failed'::character varying])::text[])) AND (started_at IS NOT NULL) AND (completed_at IS NOT NULL)))),
    CONSTRAINT ck_credit_reconciliation_run_trigger CHECK (((trigger_type)::text = ANY ((ARRAY['manual'::character varying, 'scheduled'::character varying])::text[]))),
    CONSTRAINT credit_reconciliation_runs_attempt_count_check CHECK ((attempt_count >= 0)),
    CONSTRAINT credit_reconciliation_runs_critical_count_check CHECK ((critical_count >= 0)),
    CONSTRAINT credit_reconciliation_runs_finding_count_check CHECK ((finding_count >= 0)),
    CONSTRAINT credit_reconciliation_runs_isolated_wallet_count_check CHECK ((isolated_wallet_count >= 0)),
    CONSTRAINT credit_reconciliation_runs_stale_hold_after_check CHECK ((stale_hold_after > '00:00:00'::interval))
);


--
-- Name: credit_reconciliation_runs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE credit_reconciliation_runs ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME credit_reconciliation_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: credit_transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_transactions (
    id bigint NOT NULL,
    transaction_id character varying(64) NOT NULL,
    wallet_user_id bigint NOT NULL,
    command_type character varying(24) NOT NULL,
    asset_code character varying(32) DEFAULT 'credits'::character varying NOT NULL,
    idempotency_key character varying(160) NOT NULL,
    request_hash character(64) NOT NULL,
    actor_type character varying(24) NOT NULL,
    actor_id character varying(128) NOT NULL,
    source character varying(64) NOT NULL,
    reason text NOT NULL,
    business_reference_type character varying(64) NOT NULL,
    business_reference_id character varying(160) NOT NULL,
    related_transaction_id character varying(64),
    command_amount bigint NOT NULL,
    posting_count smallint NOT NULL,
    available_balance_after bigint NOT NULL,
    reserved_balance_after bigint NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_credit_transactions_actor CHECK (((btrim((actor_type)::text) <> ''::text) AND (btrim((actor_id)::text) <> ''::text))),
    CONSTRAINT ck_credit_transactions_asset CHECK (((asset_code)::text = 'credits'::text)),
    CONSTRAINT ck_credit_transactions_balances_non_negative CHECK (((available_balance_after >= 0) AND (reserved_balance_after >= 0))),
    CONSTRAINT ck_credit_transactions_business_reference CHECK (((btrim((business_reference_type)::text) <> ''::text) AND (btrim((business_reference_id)::text) <> ''::text))),
    CONSTRAINT ck_credit_transactions_command_amount_positive CHECK ((command_amount > 0)),
    CONSTRAINT ck_credit_transactions_command_type CHECK (((command_type)::text = ANY ((ARRAY['grant'::character varying, 'deduction'::character varying, 'hold'::character varying, 'capture'::character varying, 'release'::character varying, 'refund'::character varying, 'correction'::character varying, 'purchase_credit'::character varying, 'purchase_refund'::character varying, 'payment_dispute_debit'::character varying, 'payment_dispute_restore'::character varying])::text[]))),
    CONSTRAINT ck_credit_transactions_posting_count CHECK ((posting_count >= 2)),
    CONSTRAINT ck_credit_transactions_reason CHECK ((btrim(reason) <> ''::text)),
    CONSTRAINT ck_credit_transactions_request_hash CHECK ((request_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT ck_credit_transactions_source CHECK ((btrim((source)::text) <> ''::text))
);


--
-- Name: credit_transactions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE credit_transactions ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME credit_transactions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: credit_wallet_restrictions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_wallet_restrictions (
    restriction_id uuid DEFAULT gen_random_uuid() NOT NULL,
    wallet_user_id bigint NOT NULL,
    restriction_type character varying(64) NOT NULL,
    source character varying(64) NOT NULL,
    related_object_type character varying(64) NOT NULL,
    related_object_id character varying(160) NOT NULL,
    reason text NOT NULL,
    created_by_actor_type character varying(24) NOT NULL,
    created_by_actor_id character varying(128) NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    released_at timestamp(3) with time zone,
    released_by_actor_type character varying(24),
    released_by_actor_id character varying(128),
    release_reason text,
    CONSTRAINT ck_credit_wallet_restrictions_created_actor CHECK ((((created_by_actor_type)::text = ANY ((ARRAY['admin'::character varying, 'system'::character varying, 'service'::character varying])::text[])) AND (btrim((created_by_actor_id)::text) <> ''::text))),
    CONSTRAINT ck_credit_wallet_restrictions_reason CHECK (((length(btrim(reason)) >= 1) AND (length(btrim(reason)) <= 1000))),
    CONSTRAINT ck_credit_wallet_restrictions_related_object CHECK (((btrim((related_object_type)::text) <> ''::text) AND (btrim((related_object_id)::text) <> ''::text))),
    CONSTRAINT ck_credit_wallet_restrictions_release CHECK ((((released_at IS NULL) AND (released_by_actor_type IS NULL) AND (released_by_actor_id IS NULL) AND (release_reason IS NULL)) OR ((released_at IS NOT NULL) AND (released_at >= created_at) AND ((released_by_actor_type)::text = ANY ((ARRAY['admin'::character varying, 'system'::character varying, 'service'::character varying])::text[])) AND (btrim((released_by_actor_id)::text) <> ''::text) AND ((length(btrim(release_reason)) >= 1) AND (length(btrim(release_reason)) <= 1000))))),
    CONSTRAINT ck_credit_wallet_restrictions_source CHECK ((btrim((source)::text) <> ''::text)),
    CONSTRAINT ck_credit_wallet_restrictions_type CHECK ((btrim((restriction_type)::text) <> ''::text))
);


--
-- Name: credit_wallets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_wallets (
    user_id bigint NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_credit_wallets_status CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'frozen'::character varying])::text[])))
);


--
-- Name: credit_welcome_claims; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_welcome_claims (
    identity_fingerprint character(64) NOT NULL,
    claimed_user_id bigint NOT NULL,
    claimed_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_credit_welcome_claims_fingerprint CHECK ((identity_fingerprint ~ '^[0-9a-f]{64}$'::text))
);


--
-- Name: credit_welcome_identity_key_guard; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE credit_welcome_identity_key_guard (
    singleton boolean DEFAULT true NOT NULL,
    key_fingerprint character(64) NOT NULL,
    bound_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_credit_welcome_identity_key_fingerprint CHECK ((key_fingerprint ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT credit_welcome_identity_key_guard_singleton_check CHECK (singleton)
);


--
-- Name: feature_prompts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE feature_prompts (
    feature_slug character varying(64) NOT NULL,
    prompt text DEFAULT ''::text NOT NULL,
    updated_by bigint,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_feature_prompts_feature_slug CHECK (((feature_slug)::text = ANY ((ARRAY['sketch-to-image'::character varying, 'style-variation'::character varying, 'partial-edit'::character varying, 'remove-bg'::character varying, 'style-fusion'::character varying, 'pattern-gen'::character varying, 'hd-upscale'::character varying, 'image-extend'::character varying])::text[])))
);


--
-- Name: media_assets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE media_assets (
    id bigint NOT NULL,
    asset_id character varying(64) NOT NULL,
    owner_user_id bigint NOT NULL,
    storage_profile_id character varying(64) DEFAULT 'builtin-local'::character varying NOT NULL,
    storage_key text NOT NULL,
    url text DEFAULT ''::text NOT NULL,
    source_url text DEFAULT ''::text NOT NULL,
    media_kind character varying(32) DEFAULT 'image'::character varying NOT NULL,
    mime_type character varying(64) DEFAULT ''::character varying NOT NULL,
    width integer DEFAULT 0 NOT NULL,
    height integer DEFAULT 0 NOT NULL,
    size_bytes bigint DEFAULT 0 NOT NULL,
    checksum_sha256 character varying(64) DEFAULT ''::character varying NOT NULL,
    status character varying(32) DEFAULT 'ready'::character varying NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp(3) with time zone,
    reclaim_after timestamp(3) with time zone,
    reclaim_started_at timestamp(3) with time zone,
    reclaim_claim_token uuid,
    reclaim_generation bigint DEFAULT 0 NOT NULL,
    CONSTRAINT ck_media_assets_asset_id_not_blank CHECK ((btrim((asset_id)::text) <> ''::text)),
    CONSTRAINT ck_media_assets_deleted_state CHECK ((((status)::text = 'deleted'::text) = (deleted_at IS NOT NULL))),
    CONSTRAINT ck_media_assets_dimensions CHECK (((width >= 0) AND (height >= 0) AND (size_bytes >= 0))),
    CONSTRAINT ck_media_assets_media_kind CHECK (((media_kind)::text = ANY ((ARRAY['image'::character varying, 'video'::character varying, 'audio'::character varying, 'document'::character varying, 'other'::character varying])::text[]))),
    CONSTRAINT ck_media_assets_metadata_object CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT ck_media_assets_status CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'ready'::character varying, 'quarantined'::character varying, 'deleted'::character varying, 'failed'::character varying])::text[]))),
    CONSTRAINT ck_media_assets_storage_key_not_blank CHECK ((btrim(storage_key) <> ''::text)),
    CONSTRAINT ck_media_assets_storage_profile_id_not_blank CHECK ((btrim((storage_profile_id)::text) <> ''::text))
);


--
-- Name: media_assets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE media_assets ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME media_assets_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: media_refs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE media_refs (
    id bigint NOT NULL,
    media_asset_id bigint NOT NULL,
    owner_user_id bigint NOT NULL,
    ref_type character varying(32) NOT NULL,
    ref_id character varying(128) NOT NULL,
    board_id character varying(100) DEFAULT ''::character varying NOT NULL,
    task_id character varying(64) DEFAULT ''::character varying NOT NULL,
    role character varying(32) DEFAULT ''::character varying NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_media_refs_metadata_object CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT ck_media_refs_ref_id_not_blank CHECK ((btrim((ref_id)::text) <> ''::text)),
    CONSTRAINT ck_media_refs_ref_type CHECK (((ref_type)::text = ANY ((ARRAY['upload'::character varying, 'task_output'::character varying, 'board'::character varying, 'board_node'::character varying, 'user_avatar'::character varying, 'prompt_reference'::character varying])::text[]))),
    CONSTRAINT ck_media_refs_target_fields CHECK (((((ref_type)::text = 'task_output'::text) AND (btrim((task_id)::text) <> ''::text)) OR (((ref_type)::text = ANY ((ARRAY['board'::character varying, 'board_node'::character varying])::text[])) AND (btrim((board_id)::text) <> ''::text)) OR ((ref_type)::text = ANY ((ARRAY['upload'::character varying, 'user_avatar'::character varying, 'prompt_reference'::character varying])::text[]))))
);


--
-- Name: media_refs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE media_refs ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME media_refs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: payment_admin_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payment_admin_actions (
    id bigint NOT NULL,
    action_type text NOT NULL,
    actor_id bigint NOT NULL,
    target_type text NOT NULL,
    target_id text NOT NULL,
    outcome text NOT NULL,
    error_code text DEFAULT ''::text NOT NULL,
    error_summary text DEFAULT ''::text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT ck_payment_admin_action_error_code_length CHECK ((char_length(error_code) <= 64)),
    CONSTRAINT ck_payment_admin_action_error_summary_length CHECK ((char_length(error_summary) <= 500)),
    CONSTRAINT ck_payment_admin_action_metadata_object CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT ck_payment_admin_action_outcome CHECK ((outcome = ANY (ARRAY['succeeded'::text, 'failed'::text]))),
    CONSTRAINT ck_payment_admin_action_target CHECK ((((target_type = 'stripe_configuration'::text) AND (target_id = 'current'::text)) OR ((target_type = 'payment_refund'::text) AND (target_id <> ''::text)))),
    CONSTRAINT ck_payment_admin_action_type CHECK ((action_type = ANY (ARRAY['configure_stripe'::text, 'refund_payment'::text])))
);


--
-- Name: payment_admin_actions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE payment_admin_actions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: payment_admin_actions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE payment_admin_actions_id_seq OWNED BY payment_admin_actions.id;


--
-- Name: payment_checkout_shutdowns; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payment_checkout_shutdowns (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    stripe_account_id text NOT NULL,
    cutoff_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT ck_payment_checkout_shutdowns_account CHECK ((char_length(stripe_account_id) > 0))
);


--
-- Name: payment_customers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payment_customers (
    user_id bigint NOT NULL,
    stripe_account_id text NOT NULL,
    stripe_customer_id text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT ck_payment_customer_account_id CHECK (((char_length(stripe_account_id) >= 1) AND (char_length(stripe_account_id) <= 255))),
    CONSTRAINT ck_payment_customer_id CHECK (((char_length(stripe_customer_id) >= 1) AND (char_length(stripe_customer_id) <= 255)))
);


--
-- Name: payment_dispute_debit_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payment_dispute_debit_requests (
    id uuid NOT NULL,
    wallet_user_id bigint NOT NULL,
    idempotency_key text NOT NULL,
    request_hash character(64) NOT NULL,
    requested_amount bigint NOT NULL,
    debited_amount bigint NOT NULL,
    available_balance_after bigint NOT NULL,
    reserved_balance_after bigint NOT NULL,
    credit_transaction_id text,
    business_reference_type text NOT NULL,
    business_reference_id text NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_payment_dispute_debit_request_amounts CHECK (((requested_amount > 0) AND ((debited_amount >= 0) AND (debited_amount <= requested_amount)) AND (available_balance_after >= 0) AND (reserved_balance_after >= 0))),
    CONSTRAINT ck_payment_dispute_debit_request_hash CHECK ((request_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT ck_payment_dispute_debit_request_reference CHECK (((business_reference_type = 'payment_dispute'::text) AND (btrim(business_reference_id) <> ''::text))),
    CONSTRAINT ck_payment_dispute_debit_request_transaction CHECK ((((debited_amount = 0) AND (credit_transaction_id IS NULL)) OR ((debited_amount > 0) AND (credit_transaction_id IS NOT NULL))))
);


--
-- Name: payment_disputes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payment_disputes (
    id uuid NOT NULL,
    payment_order_id uuid NOT NULL,
    user_id bigint NOT NULL,
    stripe_dispute_id text NOT NULL,
    stripe_charge_id text NOT NULL,
    stripe_payment_intent_id text DEFAULT ''::text NOT NULL,
    provider_status text NOT NULL,
    status text NOT NULL,
    amount_minor bigint NOT NULL,
    currency text NOT NULL,
    target_idea_quantity bigint NOT NULL,
    debited_idea_quantity bigint NOT NULL,
    unrecovered_credits bigint NOT NULL,
    restriction_id uuid,
    debit_transaction_id text,
    restore_transaction_id text,
    opened_at timestamp with time zone NOT NULL,
    closed_at timestamp with time zone,
    next_reconciliation_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT ck_payment_dispute_amount CHECK (((amount_minor > 0) AND (currency ~ '^[a-z]{3}$'::text))),
    CONSTRAINT ck_payment_dispute_lifecycle CHECK ((((status = 'open'::text) AND (closed_at IS NULL)) OR ((status = ANY (ARRAY['won'::text, 'lost'::text])) AND (closed_at IS NOT NULL) AND (closed_at >= opened_at)))),
    CONSTRAINT ck_payment_dispute_provider_ids CHECK ((((char_length(stripe_dispute_id) >= 1) AND (char_length(stripe_dispute_id) <= 255)) AND ((char_length(stripe_charge_id) >= 1) AND (char_length(stripe_charge_id) <= 255)) AND (char_length(stripe_payment_intent_id) <= 255))),
    CONSTRAINT ck_payment_dispute_recovery CHECK (((target_idea_quantity >= 0) AND ((debited_idea_quantity >= 0) AND (debited_idea_quantity <= target_idea_quantity)) AND (((status = 'won'::text) AND (unrecovered_credits = 0)) OR ((status = ANY (ARRAY['open'::text, 'lost'::text])) AND (unrecovered_credits = (target_idea_quantity - debited_idea_quantity)))) AND (((debited_idea_quantity = 0) AND (debit_transaction_id IS NULL)) OR ((debited_idea_quantity > 0) AND (debit_transaction_id IS NOT NULL))) AND ((restore_transaction_id IS NULL) OR (status = 'won'::text)))),
    CONSTRAINT ck_payment_dispute_restriction CHECK ((((status = ANY (ARRAY['open'::text, 'lost'::text])) AND (restriction_id IS NOT NULL)) OR (status = 'won'::text))),
    CONSTRAINT ck_payment_dispute_status CHECK ((status = ANY (ARRAY['open'::text, 'won'::text, 'lost'::text])))
);


--
-- Name: payment_order_reconciliations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payment_order_reconciliations (
    payment_order_id uuid NOT NULL,
    claim_token uuid,
    claim_generation bigint DEFAULT 0 NOT NULL,
    claimed_at timestamp with time zone,
    claimed_until timestamp with time zone,
    last_checked_at timestamp with time zone,
    next_attempt_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    last_error_code text DEFAULT ''::text NOT NULL,
    last_error_summary text DEFAULT ''::text NOT NULL,
    anomalous_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT ck_payment_order_reconciliation_claim CHECK ((((claim_token IS NULL) AND (claimed_at IS NULL) AND (claimed_until IS NULL)) OR ((claim_token IS NOT NULL) AND (claimed_at IS NOT NULL) AND (claimed_until IS NOT NULL) AND (claimed_until > claimed_at)))),
    CONSTRAINT ck_payment_order_reconciliation_error CHECK (((char_length(last_error_code) <= 64) AND (char_length(last_error_summary) <= 500))),
    CONSTRAINT ck_payment_order_reconciliation_generation CHECK ((claim_generation >= 0))
);


--
-- Name: payment_orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payment_orders (
    id uuid NOT NULL,
    public_order_number text NOT NULL,
    public_token uuid NOT NULL,
    user_id bigint NOT NULL,
    client_idempotency_key text NOT NULL,
    request_hash text NOT NULL,
    status text NOT NULL,
    stripe_account_id text NOT NULL,
    stripe_customer_id text DEFAULT ''::text NOT NULL,
    stripe_product_id text NOT NULL,
    stripe_checkout_session_id text,
    stripe_payment_intent_id text DEFAULT ''::text NOT NULL,
    stripe_charge_id text DEFAULT ''::text NOT NULL,
    idea_quantity bigint NOT NULL,
    base_currency text DEFAULT 'usd'::text NOT NULL,
    base_amount_minor bigint NOT NULL,
    paid_subtotal_minor bigint,
    paid_tax_minor bigint,
    paid_total_minor bigint,
    paid_currency text DEFAULT ''::text NOT NULL,
    terms_version text NOT NULL,
    refund_policy_version text NOT NULL,
    checkout_url text DEFAULT ''::text NOT NULL,
    receipt_url text DEFAULT ''::text NOT NULL,
    credit_transaction_id text,
    expires_at timestamp with time zone NOT NULL,
    paid_at timestamp with time zone,
    credited_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    return_base_url text NOT NULL,
    CONSTRAINT ck_payment_order_base_amount CHECK (((base_currency = 'usd'::text) AND (base_amount_minor = idea_quantity))),
    CONSTRAINT ck_payment_order_ideas CHECK (((idea_quantity >= 500) AND (idea_quantity <= 500000))),
    CONSTRAINT ck_payment_order_idempotency CHECK (((char_length(client_idempotency_key) >= 8) AND (char_length(client_idempotency_key) <= 160))),
    CONSTRAINT ck_payment_order_paid_amounts CHECK ((((paid_subtotal_minor IS NULL) OR (paid_subtotal_minor >= 0)) AND ((paid_tax_minor IS NULL) OR (paid_tax_minor >= 0)) AND ((paid_total_minor IS NULL) OR (paid_total_minor > 0)) AND ((paid_currency = ''::text) OR (paid_currency ~ '^[a-z]{3}$'::text)) AND ((paid_total_minor IS NULL) OR ((paid_subtotal_minor + paid_tax_minor) = paid_total_minor)))),
    CONSTRAINT ck_payment_order_public_number CHECK ((public_order_number ~ '^IBP-[0-9]{8}-[A-Z0-9]{12}$'::text)),
    CONSTRAINT ck_payment_order_request_hash CHECK ((request_hash ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT ck_payment_order_state_facts CHECK ((((status = 'creating'::text) AND (stripe_customer_id = ''::text) AND (stripe_checkout_session_id IS NULL) AND (checkout_url = ''::text) AND (credit_transaction_id IS NULL) AND (paid_total_minor IS NULL)) OR ((status = 'checkout_open'::text) AND (stripe_customer_id <> ''::text) AND (stripe_checkout_session_id IS NOT NULL) AND (checkout_url <> ''::text) AND (credit_transaction_id IS NULL) AND (paid_total_minor IS NULL)) OR ((status = 'async_payment_pending'::text) AND (stripe_customer_id <> ''::text) AND (stripe_checkout_session_id IS NOT NULL) AND (checkout_url = ''::text) AND (credit_transaction_id IS NULL) AND (paid_total_minor IS NULL)) OR ((status = 'paid_pending_credit'::text) AND (stripe_customer_id <> ''::text) AND (stripe_checkout_session_id IS NOT NULL) AND (stripe_payment_intent_id <> ''::text) AND (credit_transaction_id IS NULL) AND (paid_subtotal_minor IS NOT NULL) AND (paid_tax_minor IS NOT NULL) AND (paid_total_minor IS NOT NULL) AND (paid_currency <> ''::text) AND (paid_at IS NOT NULL) AND (credited_at IS NULL)) OR ((status = 'credited'::text) AND (stripe_customer_id <> ''::text) AND (stripe_checkout_session_id IS NOT NULL) AND (stripe_payment_intent_id <> ''::text) AND (credit_transaction_id IS NOT NULL) AND (paid_subtotal_minor IS NOT NULL) AND (paid_tax_minor IS NOT NULL) AND (paid_total_minor IS NOT NULL) AND (paid_currency <> ''::text) AND (paid_at IS NOT NULL) AND (credited_at IS NOT NULL)) OR ((status = ANY (ARRAY['cancelled'::text, 'expired'::text, 'payment_failed'::text])) AND (stripe_payment_intent_id = ''::text) AND (stripe_charge_id = ''::text) AND (paid_subtotal_minor IS NULL) AND (paid_tax_minor IS NULL) AND (paid_total_minor IS NULL) AND (paid_currency = ''::text) AND (receipt_url = ''::text) AND (credit_transaction_id IS NULL) AND (paid_at IS NULL) AND (credited_at IS NULL) AND (((stripe_checkout_session_id IS NULL) AND (stripe_customer_id = ''::text) AND (checkout_url = ''::text)) OR ((stripe_checkout_session_id IS NOT NULL) AND (stripe_customer_id <> ''::text) AND (checkout_url = ''::text)))))),
    CONSTRAINT ck_payment_order_status CHECK ((status = ANY (ARRAY['creating'::text, 'checkout_open'::text, 'async_payment_pending'::text, 'paid_pending_credit'::text, 'credited'::text, 'cancelled'::text, 'expired'::text, 'payment_failed'::text]))),
    CONSTRAINT ck_payment_order_stripe_ids CHECK ((((char_length(stripe_account_id) >= 1) AND (char_length(stripe_account_id) <= 255)) AND (char_length(stripe_customer_id) <= 255) AND ((char_length(stripe_product_id) >= 1) AND (char_length(stripe_product_id) <= 255)) AND ((stripe_checkout_session_id IS NULL) OR ((char_length(stripe_checkout_session_id) >= 1) AND (char_length(stripe_checkout_session_id) <= 255))) AND (char_length(stripe_payment_intent_id) <= 255) AND (char_length(stripe_charge_id) <= 255))),
    CONSTRAINT ck_payment_order_urls CHECK (((char_length(checkout_url) <= 4096) AND (char_length(receipt_url) <= 4096))),
    CONSTRAINT ck_payment_order_versions CHECK ((((char_length(terms_version) >= 1) AND (char_length(terms_version) <= 64)) AND ((char_length(refund_policy_version) >= 1) AND (char_length(refund_policy_version) <= 64))))
);


--
-- Name: payment_refunds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payment_refunds (
    id uuid NOT NULL,
    payment_order_id uuid NOT NULL,
    user_id bigint NOT NULL,
    idempotency_key text NOT NULL,
    idea_quantity bigint NOT NULL,
    amount_minor bigint NOT NULL,
    currency text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    stripe_status text DEFAULT ''::text NOT NULL,
    stripe_refund_id text DEFAULT ''::text NOT NULL,
    stripe_charge_id text DEFAULT ''::text NOT NULL,
    credit_hold_transaction_id text NOT NULL,
    credit_transaction_id text DEFAULT ''::text NOT NULL,
    actor_id bigint,
    reason text DEFAULT ''::text NOT NULL,
    error_summary text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    succeeded_at timestamp with time zone,
    failed_at timestamp with time zone,
    CONSTRAINT ck_payment_refund_amount CHECK ((amount_minor > 0)),
    CONSTRAINT ck_payment_refund_currency CHECK ((currency ~ '^[a-z]{3}$'::text)),
    CONSTRAINT ck_payment_refund_error CHECK ((char_length(error_summary) <= 500)),
    CONSTRAINT ck_payment_refund_ideas CHECK ((idea_quantity > 0)),
    CONSTRAINT ck_payment_refund_ids CHECK (((char_length(stripe_refund_id) <= 255) AND ((char_length(stripe_charge_id) >= 1) AND (char_length(stripe_charge_id) <= 255)))),
    CONSTRAINT ck_payment_refund_status CHECK ((status = ANY (ARRAY['pending'::text, 'succeeded'::text, 'failed'::text, 'anomaly'::text]))),
    CONSTRAINT ck_payment_refund_terminal_facts CHECK ((((status = 'pending'::text) AND (succeeded_at IS NULL) AND (failed_at IS NULL)) OR ((status = 'succeeded'::text) AND (succeeded_at IS NOT NULL) AND (failed_at IS NULL) AND (credit_transaction_id <> ''::text)) OR ((status = 'failed'::text) AND (failed_at IS NOT NULL) AND (succeeded_at IS NULL)) OR ((status = 'anomaly'::text) AND (succeeded_at IS NULL) AND (failed_at IS NULL))))
);


--
-- Name: payment_stripe_configurations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payment_stripe_configurations (
    singleton_id smallint DEFAULT 1 NOT NULL,
    account_id text NOT NULL,
    mode text NOT NULL,
    charges_enabled boolean NOT NULL,
    product_tax_code text NOT NULL,
    purchases_enabled boolean DEFAULT false NOT NULL,
    stripe_product_id text NOT NULL,
    stripe_webhook_endpoint_id text NOT NULL,
    api_key_ciphertext text NOT NULL,
    webhook_secret_ciphertext text NOT NULL,
    secret_material_kind text NOT NULL,
    created_by bigint NOT NULL,
    updated_by bigint NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT ck_payment_stripe_configuration_material CHECK ((secret_material_kind = 'encrypted'::text)),
    CONSTRAINT ck_payment_stripe_configuration_mode CHECK ((mode = ANY (ARRAY['test'::text, 'live'::text]))),
    CONSTRAINT ck_payment_stripe_configuration_singleton CHECK ((singleton_id = 1)),
    CONSTRAINT ck_payment_stripe_configuration_tax_code CHECK ((product_tax_code ~ '^txcd_[0-9]{8}$'::text))
);


--
-- Name: payment_webhook_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE payment_webhook_events (
    stripe_event_id text NOT NULL,
    event_type text NOT NULL,
    stripe_object_id text DEFAULT ''::text NOT NULL,
    livemode boolean NOT NULL,
    stripe_created_at timestamp with time zone NOT NULL,
    processing_result text NOT NULL,
    error_summary text DEFAULT ''::text NOT NULL,
    received_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    processed_at timestamp with time zone,
    payment_order_id uuid,
    CONSTRAINT ck_payment_webhook_error_summary_length CHECK ((char_length(error_summary) <= 500)),
    CONSTRAINT ck_payment_webhook_event_id CHECK (((char_length(stripe_event_id) >= 1) AND (char_length(stripe_event_id) <= 255))),
    CONSTRAINT ck_payment_webhook_event_type CHECK (((char_length(event_type) >= 1) AND (char_length(event_type) <= 255))),
    CONSTRAINT ck_payment_webhook_processing_result CHECK ((processing_result = ANY (ARRAY['received'::text, 'ignored'::text, 'processed'::text, 'failed'::text])))
);


--
-- Name: provider_endpoints; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE provider_endpoints (
    id bigint NOT NULL,
    api_type character varying(50) DEFAULT 'openai-compatible'::character varying NOT NULL,
    name character varying(100) NOT NULL,
    base_url character varying(500) NOT NULL,
    status smallint DEFAULT 1 NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_provider_endpoints_api_type_not_blank CHECK ((btrim((api_type)::text) <> ''::text)),
    CONSTRAINT chk_provider_endpoints_base_url_not_blank CHECK ((btrim((base_url)::text) <> ''::text)),
    CONSTRAINT chk_provider_endpoints_name_not_blank CHECK ((btrim((name)::text) <> ''::text)),
    CONSTRAINT chk_provider_endpoints_status CHECK ((status = ANY (ARRAY[0, 1])))
);


--
-- Name: provider_endpoints_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE provider_endpoints ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME provider_endpoints_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: reward_admin_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE reward_admin_actions (
    id bigint NOT NULL,
    reward_code_id bigint,
    reward_code_id_snapshot bigint NOT NULL,
    action_type character varying(32) NOT NULL,
    actor_user_id bigint NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    target_snapshot jsonb NOT NULL,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT ck_reward_admin_actions_code_snapshot CHECK ((reward_code_id_snapshot > 0)),
    CONSTRAINT ck_reward_admin_actions_metadata CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT ck_reward_admin_actions_target_snapshot CHECK ((jsonb_typeof(target_snapshot) = 'object'::text)),
    CONSTRAINT ck_reward_admin_actions_type CHECK (((action_type)::text = ANY ((ARRAY['create_reward_code'::character varying, 'update_reward_code'::character varying, 'rotate_reward_code'::character varying, 'set_reward_code_enabled'::character varying, 'delete_reward_code'::character varying])::text[])))
);


--
-- Name: reward_admin_actions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE reward_admin_actions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME reward_admin_actions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: reward_code_digest_aliases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE reward_code_digest_aliases (
    reward_code_id bigint NOT NULL,
    key_version character varying(32) NOT NULL,
    code_digest bytea NOT NULL,
    CONSTRAINT ck_reward_code_digest_aliases_digest CHECK ((octet_length(code_digest) = 32)),
    CONSTRAINT ck_reward_code_digest_aliases_key_version CHECK ((((key_version)::text = btrim((key_version)::text)) AND ((char_length((key_version)::text) >= 1) AND (char_length((key_version)::text) <= 32))))
);


--
-- Name: reward_codes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE reward_codes (
    id bigint NOT NULL,
    display_name character varying(120) NOT NULL,
    key_version character varying(32) NOT NULL,
    code_digest bytea NOT NULL,
    code_hint character varying(4) NOT NULL,
    credit_amount bigint NOT NULL,
    max_redemptions bigint DEFAULT 0 NOT NULL,
    redemption_count bigint DEFAULT 0 NOT NULL,
    expires_at timestamp(3) with time zone,
    enabled boolean DEFAULT true NOT NULL,
    created_by bigint NOT NULL,
    idempotency_key character varying(128) NOT NULL,
    request_fingerprint bytea NOT NULL,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT ck_reward_codes_capacity CHECK (((redemption_count >= 0) AND ((max_redemptions = 0) OR (redemption_count <= max_redemptions)))),
    CONSTRAINT ck_reward_codes_credit_amount CHECK (((credit_amount >= 1) AND (credit_amount <= '9000000000000000'::bigint))),
    CONSTRAINT ck_reward_codes_digest CHECK ((octet_length(code_digest) = 32)),
    CONSTRAINT ck_reward_codes_fingerprint CHECK ((octet_length(request_fingerprint) = 32)),
    CONSTRAINT ck_reward_codes_hint CHECK (((code_hint)::text ~ '^[A-Z0-9]{4}$'::text)),
    CONSTRAINT ck_reward_codes_idempotency CHECK ((((idempotency_key)::text = btrim((idempotency_key)::text, E' \t\n\r\f'::text)) AND ((char_length((idempotency_key)::text) >= 1) AND (char_length((idempotency_key)::text) <= 128)))),
    CONSTRAINT ck_reward_codes_key_version CHECK ((((key_version)::text = btrim((key_version)::text)) AND ((char_length((key_version)::text) >= 1) AND (char_length((key_version)::text) <= 32)))),
    CONSTRAINT ck_reward_codes_name CHECK ((((display_name)::text = btrim((display_name)::text, E' \t\n\r\f'::text)) AND ((char_length((display_name)::text) >= 1) AND (char_length((display_name)::text) <= 120))))
);


--
-- Name: reward_codes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE reward_codes ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME reward_codes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: reward_redemptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE reward_redemptions (
    id bigint NOT NULL,
    reward_code_id bigint NOT NULL,
    user_id bigint NOT NULL,
    reward_code_display_name character varying(120) NOT NULL,
    masked_code_hint character varying(9) NOT NULL,
    credit_amount bigint NOT NULL,
    credit_transaction_id character varying(64) NOT NULL,
    available_balance_after bigint NOT NULL,
    redeemed_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT ck_reward_redemptions_credit_amount CHECK (((credit_amount >= 1) AND (credit_amount <= '9000000000000000'::bigint))),
    CONSTRAINT ck_reward_redemptions_display_name CHECK (((char_length((reward_code_display_name)::text) >= 1) AND (char_length((reward_code_display_name)::text) <= 120))),
    CONSTRAINT ck_reward_redemptions_masked_code_hint CHECK (((masked_code_hint)::text ~ '^•••• [A-Z0-9]{4}$'::text))
);


--
-- Name: reward_redemptions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE reward_redemptions ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME reward_redemptions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: storage_profile_secret_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE storage_profile_secret_versions (
    id bigint NOT NULL,
    profile_id character varying(64) NOT NULL,
    secret_name character varying(64) NOT NULL,
    version_number integer NOT NULL,
    secret_material_kind character varying(20) DEFAULT 'encrypted'::character varying NOT NULL,
    secret_ref text DEFAULT ''::text NOT NULL,
    secret_payload text DEFAULT ''::text NOT NULL,
    fingerprint character varying(128) DEFAULT ''::character varying NOT NULL,
    key_version character varying(64) DEFAULT ''::character varying NOT NULL,
    status character varying(32) DEFAULT 'active'::character varying NOT NULL,
    rotated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    expires_at timestamp(3) with time zone,
    retired_at timestamp(3) with time zone,
    revoked_at timestamp(3) with time zone,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_storage_profile_secret_versions_material CHECK ((((status)::text <> 'active'::text) OR (((secret_material_kind)::text = 'encrypted'::text) AND (btrim(secret_payload) <> ''::text)) OR (((secret_material_kind)::text = 'external_ref'::text) AND (btrim(secret_ref) <> ''::text)))),
    CONSTRAINT ck_storage_profile_secret_versions_material_kind CHECK (((secret_material_kind)::text = ANY ((ARRAY['encrypted'::character varying, 'external_ref'::character varying])::text[]))),
    CONSTRAINT ck_storage_profile_secret_versions_status CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'retired'::character varying, 'revoked'::character varying])::text[]))),
    CONSTRAINT ck_storage_profile_secret_versions_timestamps CHECK (((((status)::text = 'active'::text) AND (retired_at IS NULL) AND (revoked_at IS NULL)) OR (((status)::text = 'retired'::text) AND (retired_at IS NOT NULL) AND (revoked_at IS NULL)) OR (((status)::text = 'revoked'::text) AND (revoked_at IS NOT NULL)))),
    CONSTRAINT ck_storage_profile_secret_versions_version CHECK ((version_number > 0))
);


--
-- Name: storage_profile_secret_versions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE storage_profile_secret_versions ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME storage_profile_secret_versions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: storage_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE storage_profiles (
    profile_id character varying(64) NOT NULL,
    name character varying(100) NOT NULL,
    provider character varying(50) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    is_current boolean DEFAULT false NOT NULL,
    builtin boolean DEFAULT false NOT NULL,
    status character varying(32) DEFAULT 'active'::character varying NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by bigint,
    updated_by bigint,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp(3) with time zone,
    CONSTRAINT ck_storage_profiles_config_object CHECK ((jsonb_typeof(config) = 'object'::text)),
    CONSTRAINT ck_storage_profiles_current_active CHECK (((NOT is_current) OR (enabled AND ((status)::text = 'active'::text) AND (deleted_at IS NULL)))),
    CONSTRAINT ck_storage_profiles_deleted_at_state CHECK (((deleted_at IS NULL) OR ((status)::text = 'deleted'::text))),
    CONSTRAINT ck_storage_profiles_deleted_state CHECK ((((status)::text <> 'deleted'::text) OR ((NOT enabled) AND (NOT is_current) AND (deleted_at IS NOT NULL)))),
    CONSTRAINT ck_storage_profiles_provider CHECK (((provider)::text = ANY ((ARRAY['local'::character varying, 's3-compatible'::character varying])::text[]))),
    CONSTRAINT ck_storage_profiles_status CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'disabled'::character varying, 'deleted'::character varying])::text[])))
);


--
-- Name: system_configs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE system_configs (
    id bigint NOT NULL,
    config_key character varying(100) NOT NULL,
    value text NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL
);


--
-- Name: system_configs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE system_configs ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME system_configs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: system_settings_admin_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE system_settings_admin_actions (
    id bigint NOT NULL,
    action_type character varying(64) NOT NULL,
    actor_id bigint NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT ck_system_settings_admin_actions_metadata CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT ck_system_settings_admin_actions_type CHECK (((action_type)::text = ANY ((ARRAY['update_operational_settings'::character varying, 'update_outbound_policy'::character varying])::text[]))),
    CONSTRAINT system_settings_admin_actions_actor_id_check CHECK ((actor_id > 0))
);


--
-- Name: system_settings_admin_actions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE system_settings_admin_actions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME system_settings_admin_actions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: task_attempts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_attempts (
    id bigint NOT NULL,
    task_id character varying(64) NOT NULL,
    lease_id bigint NOT NULL,
    attempt_no integer NOT NULL,
    river_job_id bigint DEFAULT 0 NOT NULL,
    status character varying(20) DEFAULT 'running'::character varying NOT NULL,
    failure_stage character varying(32) DEFAULT ''::character varying NOT NULL,
    failure_category character varying(64) DEFAULT ''::character varying NOT NULL,
    error_msg text DEFAULT ''::text NOT NULL,
    provider_started_at timestamp(3) with time zone,
    provider_finished_at timestamp(3) with time zone,
    finalize_started_at timestamp(3) with time zone,
    finalize_finished_at timestamp(3) with time zone,
    provider_latency_ms bigint DEFAULT 0 NOT NULL,
    finalize_latency_ms bigint DEFAULT 0 NOT NULL,
    total_latency_ms bigint DEFAULT 0 NOT NULL,
    completed_at timestamp(3) with time zone,
    cancelled_at timestamp(3) with time zone,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_task_attempts_attempt_no_positive CHECK ((attempt_no > 0)),
    CONSTRAINT ck_task_attempts_ids_valid CHECK ((river_job_id >= 0)),
    CONSTRAINT ck_task_attempts_latency_non_negative CHECK (((provider_latency_ms >= 0) AND (finalize_latency_ms >= 0) AND (total_latency_ms >= 0))),
    CONSTRAINT ck_task_attempts_status CHECK (((status)::text = ANY ((ARRAY['running'::character varying, 'completed'::character varying, 'failed'::character varying, 'requeued'::character varying, 'cancelled'::character varying])::text[]))),
    CONSTRAINT ck_task_attempts_terminal_consistency CHECK (((((status)::text = 'running'::text) AND (completed_at IS NULL) AND (cancelled_at IS NULL) AND ((failure_stage)::text = ''::text) AND ((failure_category)::text = ''::text) AND (error_msg = ''::text)) OR (((status)::text = 'completed'::text) AND (completed_at IS NOT NULL) AND (cancelled_at IS NULL) AND ((failure_stage)::text = ''::text) AND ((failure_category)::text = ''::text) AND (error_msg = ''::text)) OR (((status)::text = ANY ((ARRAY['failed'::character varying, 'requeued'::character varying])::text[])) AND (completed_at IS NOT NULL) AND (cancelled_at IS NULL) AND (btrim((failure_stage)::text) <> ''::text) AND (btrim((failure_category)::text) <> ''::text) AND (btrim(error_msg) <> ''::text)) OR (((status)::text = 'cancelled'::text) AND (completed_at IS NULL) AND (cancelled_at IS NOT NULL) AND (btrim((failure_stage)::text) <> ''::text) AND ((failure_category)::text = 'cancelled'::text) AND (btrim(error_msg) <> ''::text))))
);


--
-- Name: task_attempts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE task_attempts ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME task_attempts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: task_billing_settlements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_billing_settlements (
    task_id character varying(64) NOT NULL,
    state character varying(16) NOT NULL,
    settlement_transaction_id text,
    settled_at timestamp(3) with time zone,
    updated_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT task_billing_settlement_state_check CHECK (((((state)::text = 'held'::text) AND (settlement_transaction_id IS NULL) AND (settled_at IS NULL)) OR (((state)::text = 'free'::text) AND (settlement_transaction_id IS NULL) AND (settled_at IS NOT NULL)) OR (((state)::text = ANY ((ARRAY['captured'::character varying, 'released'::character varying])::text[])) AND (settlement_transaction_id IS NOT NULL) AND (btrim(settlement_transaction_id) <> ''::text) AND (settled_at IS NOT NULL)))),
    CONSTRAINT task_billing_settlements_state_check CHECK (((state)::text = ANY ((ARRAY['held'::character varying, 'free'::character varying, 'captured'::character varying, 'released'::character varying])::text[])))
);


--
-- Name: task_billing_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_billing_snapshots (
    task_id character varying(64) NOT NULL,
    user_id bigint NOT NULL,
    feature_slug character varying(64) NOT NULL,
    sku character varying(128) NOT NULL,
    price_version_id bigint NOT NULL,
    price_version bigint NOT NULL,
    unit_credits bigint NOT NULL,
    quantity bigint NOT NULL,
    total_credits bigint NOT NULL,
    quoted_at timestamp(3) with time zone NOT NULL,
    hold_transaction_id text,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    display_name character varying(120) NOT NULL,
    CONSTRAINT task_billing_snapshot_display_name_check CHECK (((char_length(btrim((display_name)::text)) >= 1) AND (char_length(btrim((display_name)::text)) <= 120))),
    CONSTRAINT task_billing_snapshot_hold_check CHECK ((((total_credits = 0) AND (hold_transaction_id IS NULL)) OR ((total_credits > 0) AND (hold_transaction_id IS NOT NULL) AND (btrim(hold_transaction_id) <> ''::text)))),
    CONSTRAINT task_billing_snapshot_total_check CHECK ((total_credits = (unit_credits * quantity))),
    CONSTRAINT task_billing_snapshots_price_version_check CHECK ((price_version > 0)),
    CONSTRAINT task_billing_snapshots_quantity_check CHECK ((quantity > 0)),
    CONSTRAINT task_billing_snapshots_total_credits_check CHECK (((total_credits >= 0) AND (total_credits <= '9000000000000000'::bigint))),
    CONSTRAINT task_billing_snapshots_unit_credits_check CHECK (((unit_credits >= 0) AND (unit_credits <= '9000000000000000'::bigint))),
    CONSTRAINT task_billing_snapshots_user_id_check CHECK ((user_id > 0))
);


--
-- Name: task_cancellations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_cancellations (
    task_id character varying(64) NOT NULL,
    expires_at timestamp(3) with time zone NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL
);


--
-- Name: task_daily_stat_backfills; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_daily_stat_backfills (
    id bigint NOT NULL,
    stat_date date NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL
);


--
-- Name: task_daily_stat_backfills_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE task_daily_stat_backfills ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME task_daily_stat_backfills_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: task_daily_stats; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_daily_stats (
    id bigint NOT NULL,
    stat_date date NOT NULL,
    user_id bigint DEFAULT 0 NOT NULL,
    account_id bigint DEFAULT 0 NOT NULL,
    provider_endpoint_id bigint DEFAULT 0 NOT NULL,
    credential_id bigint DEFAULT 0 NOT NULL,
    feature_slug character varying(64) DEFAULT ''::character varying NOT NULL,
    api_type character varying(50) DEFAULT ''::character varying NOT NULL,
    model_name character varying(128) DEFAULT ''::character varying NOT NULL,
    user_key character varying(128) DEFAULT ''::character varying NOT NULL,
    total bigint DEFAULT 0 NOT NULL,
    completed bigint DEFAULT 0 NOT NULL,
    failed bigint DEFAULT 0 NOT NULL,
    cancelled bigint DEFAULT 0 NOT NULL,
    latency_seconds_sum double precision DEFAULT 0 NOT NULL,
    latency_count bigint DEFAULT 0 NOT NULL,
    cost_micros_sum bigint DEFAULT 0 NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_task_daily_stats_counts CHECK (((total >= 0) AND (completed >= 0) AND (failed >= 0) AND (cancelled >= 0) AND (total = ((completed + failed) + cancelled)) AND (latency_seconds_sum >= (0)::double precision) AND (latency_count >= 0) AND (latency_count <= completed) AND (cost_micros_sum >= 0))),
    CONSTRAINT chk_task_daily_stats_dimension_ids CHECK (((user_id >= 0) AND (account_id >= 0) AND (provider_endpoint_id >= 0) AND (credential_id >= 0)))
);


--
-- Name: task_daily_stats_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE task_daily_stats ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME task_daily_stats_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: task_execution_metrics; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_execution_metrics (
    task_id character varying(64) NOT NULL,
    current_stage character varying(32) DEFAULT 'queued'::character varying NOT NULL,
    bottleneck_source character varying(32) DEFAULT 'unknown'::character varying NOT NULL,
    failure_stage character varying(32) DEFAULT ''::character varying NOT NULL,
    failure_category character varying(64) DEFAULT ''::character varying NOT NULL,
    assigned_at timestamp(3) with time zone,
    provider_started_at timestamp(3) with time zone,
    provider_finished_at timestamp(3) with time zone,
    finalize_started_at timestamp(3) with time zone,
    finalize_finished_at timestamp(3) with time zone,
    queue_wait_ms bigint DEFAULT 0 NOT NULL,
    worker_overhead_ms bigint DEFAULT 0 NOT NULL,
    provider_latency_ms bigint DEFAULT 0 NOT NULL,
    finalize_latency_ms bigint DEFAULT 0 NOT NULL,
    total_latency_ms bigint DEFAULT 0 NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_task_execution_metrics_bottleneck_source CHECK (((bottleneck_source)::text = ANY ((ARRAY['queue'::character varying, 'worker'::character varying, 'provider'::character varying, 'storage'::character varying, 'finalize'::character varying, 'input'::character varying, 'unknown'::character varying])::text[]))),
    CONSTRAINT ck_task_execution_metrics_current_stage CHECK (((current_stage)::text = ANY ((ARRAY['queued'::character varying, 'assigned'::character varying, 'provider'::character varying, 'finalizing'::character varying, 'completed'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[]))),
    CONSTRAINT ck_task_execution_metrics_latency_non_negative CHECK (((queue_wait_ms >= 0) AND (worker_overhead_ms >= 0) AND (provider_latency_ms >= 0) AND (finalize_latency_ms >= 0) AND (total_latency_ms >= 0))),
    CONSTRAINT ck_task_execution_metrics_terminal_consistency CHECK (((((current_stage)::text = 'completed'::text) AND ((failure_stage)::text = ''::text) AND ((failure_category)::text = ''::text)) OR (((current_stage)::text = 'failed'::text) AND (btrim((failure_stage)::text) <> ''::text) AND (btrim((failure_category)::text) <> ''::text)) OR (((current_stage)::text = 'cancelled'::text) AND (btrim((failure_stage)::text) <> ''::text) AND ((failure_category)::text = 'cancelled'::text)) OR ((current_stage)::text = ANY ((ARRAY['queued'::character varying, 'assigned'::character varying, 'provider'::character varying, 'finalizing'::character varying])::text[]))))
);


--
-- Name: task_leases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_leases (
    id bigint NOT NULL,
    task_id character varying(64) NOT NULL,
    lease_token character varying(128) NOT NULL,
    river_job_id bigint DEFAULT 0 NOT NULL,
    account_id bigint NOT NULL,
    provider_endpoint_id bigint NOT NULL,
    credential_id bigint NOT NULL,
    feature_slug character varying(64) DEFAULT ''::character varying NOT NULL,
    resolved_model character varying(128) DEFAULT ''::character varying NOT NULL,
    api_type character varying(50) DEFAULT ''::character varying NOT NULL,
    provider_endpoint_name character varying(100) DEFAULT ''::character varying NOT NULL,
    provider_endpoint_base_url character varying(500) DEFAULT ''::character varying NOT NULL,
    credential_fingerprint character varying(128) DEFAULT ''::character varying NOT NULL,
    credential_key_version character varying(64) DEFAULT ''::character varying NOT NULL,
    acquired_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    released_at timestamp(3) with time zone,
    release_reason character varying(32) DEFAULT ''::character varying NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_task_leases_ids_valid CHECK (((river_job_id >= 0) AND (account_id > 0) AND (provider_endpoint_id > 0) AND (credential_id > 0))),
    CONSTRAINT ck_task_leases_release_reason CHECK ((((released_at IS NULL) AND ((release_reason)::text = ''::text)) OR ((released_at IS NOT NULL) AND ((release_reason)::text = ANY ((ARRAY['completed'::character varying, 'failed'::character varying, 'requeued'::character varying, 'cancelled'::character varying, 'reset'::character varying, 'account_failed'::character varying])::text[]))))),
    CONSTRAINT ck_task_leases_snapshot CHECK (((btrim((lease_token)::text) <> ''::text) AND (btrim((resolved_model)::text) <> ''::text) AND (btrim((api_type)::text) <> ''::text) AND (btrim((provider_endpoint_base_url)::text) <> ''::text)))
);


--
-- Name: task_leases_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE task_leases ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME task_leases_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: task_outputs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_outputs (
    id bigint NOT NULL,
    task_id character varying(64) NOT NULL,
    media_asset_id bigint,
    output_kind character varying(32) DEFAULT 'asset'::character varying NOT NULL,
    role character varying(32) DEFAULT 'image'::character varying NOT NULL,
    url text DEFAULT ''::text NOT NULL,
    source_url text DEFAULT ''::text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    sort_index integer DEFAULT 0 NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ck_task_outputs_metadata_object CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT ck_task_outputs_output_kind CHECK (((output_kind)::text = ANY ((ARRAY['asset'::character varying, 'remote'::character varying])::text[]))),
    CONSTRAINT ck_task_outputs_output_kind_shape CHECK (((((output_kind)::text = 'asset'::text) AND (media_asset_id IS NOT NULL) AND (btrim(url) = ''::text)) OR (((output_kind)::text = 'remote'::text) AND (media_asset_id IS NULL) AND (btrim(url) <> ''::text)))),
    CONSTRAINT ck_task_outputs_role_not_blank CHECK ((btrim((role)::text) <> ''::text)),
    CONSTRAINT ck_task_outputs_sort_index_non_negative CHECK ((sort_index >= 0))
);


--
-- Name: task_outputs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE task_outputs ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME task_outputs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: task_stat_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_stat_events (
    id bigint NOT NULL,
    task_id character varying(64) NOT NULL,
    occurred_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    stat_date date NOT NULL,
    user_id bigint DEFAULT 0 NOT NULL,
    account_id bigint DEFAULT 0 NOT NULL,
    provider_endpoint_id bigint DEFAULT 0 NOT NULL,
    credential_id bigint DEFAULT 0 NOT NULL,
    feature_slug character varying(64) DEFAULT ''::character varying NOT NULL,
    api_type character varying(50) DEFAULT ''::character varying NOT NULL,
    model_name character varying(128) DEFAULT ''::character varying NOT NULL,
    user_key character varying(128) DEFAULT ''::character varying NOT NULL,
    status character varying(20) NOT NULL,
    latency_seconds double precision DEFAULT 0 NOT NULL,
    cost_micros bigint DEFAULT 0 NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_task_stat_events_cost CHECK ((cost_micros >= 0)),
    CONSTRAINT chk_task_stat_events_dimension_ids CHECK (((user_id >= 0) AND (account_id >= 0) AND (provider_endpoint_id >= 0) AND (credential_id >= 0))),
    CONSTRAINT chk_task_stat_events_latency CHECK ((latency_seconds >= (0)::double precision)),
    CONSTRAINT chk_task_stat_events_status CHECK (((status)::text = ANY ((ARRAY['completed'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[])))
);


--
-- Name: task_stat_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE task_stat_events ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME task_stat_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: task_submission_claims; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_submission_claims (
    user_id bigint NOT NULL,
    client_action_id character varying(128) NOT NULL,
    task_id character varying(64) NOT NULL,
    request_hash character varying(64) NOT NULL,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT task_submission_claims_client_action_id_check CHECK ((btrim((client_action_id)::text) <> ''::text)),
    CONSTRAINT task_submission_claims_request_hash_check CHECK (((request_hash)::text ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT task_submission_claims_user_id_check CHECK ((user_id > 0))
);


--
-- Name: tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE tasks (
    id bigint NOT NULL,
    task_id character varying(64) NOT NULL,
    user_id bigint NOT NULL,
    client_action_id character varying(128) DEFAULT ''::character varying NOT NULL,
    assigned_provider_endpoint_id bigint,
    assigned_credential_id bigint,
    account_id bigint,
    prompt text NOT NULL,
    user_prompt text NOT NULL,
    params text NOT NULL,
    feature_slug character varying(64) DEFAULT ''::character varying NOT NULL,
    priority integer DEFAULT 2 NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    lease_token character varying(128) DEFAULT ''::character varying NOT NULL,
    retry_count integer DEFAULT 0 NOT NULL,
    attempt_seq integer DEFAULT 0 NOT NULL,
    error_msg text NOT NULL,
    result text NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    completed_at timestamp(3) with time zone,
    cancelled_at timestamp(3) with time zone,
    cancelled_by bigint,
    cancel_reason text DEFAULT ''::text NOT NULL,
    CONSTRAINT ck_tasks_account_id_positive CHECK (((account_id IS NULL) OR (account_id > 0))),
    CONSTRAINT ck_tasks_assigned_credential_id_positive CHECK (((assigned_credential_id IS NULL) OR (assigned_credential_id > 0))),
    CONSTRAINT ck_tasks_assigned_provider_endpoint_id_positive CHECK (((assigned_provider_endpoint_id IS NULL) OR (assigned_provider_endpoint_id > 0))),
    CONSTRAINT ck_tasks_assignment_pair CHECK ((((account_id IS NULL) AND (assigned_provider_endpoint_id IS NULL) AND (assigned_credential_id IS NULL)) OR ((account_id IS NOT NULL) AND (assigned_provider_endpoint_id IS NOT NULL) AND (assigned_credential_id IS NOT NULL)))),
    CONSTRAINT ck_tasks_assignment_status CHECK (((((status)::text = 'pending'::text) AND (account_id IS NULL) AND (assigned_provider_endpoint_id IS NULL) AND (assigned_credential_id IS NULL)) OR (((status)::text = 'running'::text) AND (account_id IS NOT NULL) AND (assigned_provider_endpoint_id IS NOT NULL) AND (assigned_credential_id IS NOT NULL)) OR ((status)::text = ANY ((ARRAY['completed'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[])))),
    CONSTRAINT ck_tasks_attempt_seq_non_negative CHECK ((attempt_seq >= 0)),
    CONSTRAINT ck_tasks_cancelled_by_positive CHECK (((cancelled_by IS NULL) OR (cancelled_by > 0))),
    CONSTRAINT ck_tasks_client_action_id_trimmed CHECK (((client_action_id)::text = btrim((client_action_id)::text))),
    CONSTRAINT ck_tasks_lease_status CHECK ((((status)::text = 'running'::text) OR ((lease_token)::text = ''::text))),
    CONSTRAINT ck_tasks_priority CHECK (((priority >= 1) AND (priority <= 4))),
    CONSTRAINT ck_tasks_retry_count_non_negative CHECK ((retry_count >= 0)),
    CONSTRAINT ck_tasks_status CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'running'::character varying, 'completed'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[]))),
    CONSTRAINT ck_tasks_terminal_consistency CHECK (((((status)::text = ANY ((ARRAY['pending'::character varying, 'running'::character varying])::text[])) AND (completed_at IS NULL) AND (cancelled_at IS NULL) AND (cancelled_by IS NULL) AND (cancel_reason = ''::text)) OR (((status)::text = 'completed'::text) AND (completed_at IS NOT NULL) AND (cancelled_at IS NULL) AND (cancelled_by IS NULL) AND (cancel_reason = ''::text)) OR (((status)::text = 'failed'::text) AND (completed_at IS NOT NULL) AND (cancelled_at IS NULL) AND (cancelled_by IS NULL) AND (cancel_reason = ''::text) AND (btrim(error_msg) <> ''::text)) OR (((status)::text = 'cancelled'::text) AND (completed_at IS NOT NULL) AND (cancelled_at IS NOT NULL) AND (btrim(cancel_reason) <> ''::text))))
);


--
-- Name: tasks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE tasks ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME tasks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: user_deletion_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE user_deletion_requests (
    user_id bigint NOT NULL,
    auth_user_id character varying(191),
    storage_uid character varying(6),
    phase character varying(32) DEFAULT 'pending_auth'::character varying NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    completed_at timestamp(3) with time zone,
    CONSTRAINT ck_user_deletion_requests_phase CHECK (((phase)::text = ANY ((ARRAY['pending_auth'::character varying, 'pending_storage'::character varying, 'pending_tombstone'::character varying, 'completed'::character varying])::text[])))
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE users (
    id bigint NOT NULL,
    public_id uuid DEFAULT gen_random_uuid() NOT NULL,
    display_name character varying(100) DEFAULT 'IdeaBoard User'::character varying NOT NULL,
    auth_user_id character varying(128),
    storage_uid character(6) NOT NULL,
    avatar character varying(1024) DEFAULT ''::character varying NOT NULL,
    role character varying(20) DEFAULT 'user'::character varying NOT NULL,
    status smallint DEFAULT 1 NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_users_display_name CHECK (((length(btrim((display_name)::text)) >= 1) AND (length(btrim((display_name)::text)) <= 100))),
    CONSTRAINT chk_users_role CHECK (((role)::text = ANY ((ARRAY['admin'::character varying, 'user'::character varying])::text[]))),
    CONSTRAINT chk_users_status CHECK ((status = ANY (ARRAY[0, 1]))),
    CONSTRAINT chk_users_storage_uid CHECK ((storage_uid ~ '^[A-Z0-9]{6}$'::text))
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE users ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: users_used_uids; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE users_used_uids (
    id bigint NOT NULL,
    storage_uid character(6) NOT NULL,
    created_at timestamp(3) with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_users_used_uids_storage_uid CHECK ((storage_uid ~ '^[A-Z0-9]{6}$'::text))
);


--
-- Name: users_used_uids_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE users_used_uids ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME users_used_uids_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: payment_admin_actions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_admin_actions ALTER COLUMN id SET DEFAULT nextval('payment_admin_actions_id_seq'::regclass);


--
-- Name: api_account_models api_account_models_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY api_account_models
    ADD CONSTRAINT api_account_models_pkey PRIMARY KEY (id);


--
-- Name: api_account_secrets api_account_secrets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY api_account_secrets
    ADD CONSTRAINT api_account_secrets_pkey PRIMARY KEY (id);


--
-- Name: api_accounts api_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY api_accounts
    ADD CONSTRAINT api_accounts_pkey PRIMARY KEY (id);


--
-- Name: billing_admin_actions billing_admin_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_admin_actions
    ADD CONSTRAINT billing_admin_actions_pkey PRIMARY KEY (id);


--
-- Name: billing_price_versions billing_price_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_price_versions
    ADD CONSTRAINT billing_price_versions_pkey PRIMARY KEY (price_version_id);


--
-- Name: billing_price_versions billing_price_versions_sku_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_price_versions
    ADD CONSTRAINT billing_price_versions_sku_id_key UNIQUE (sku, price_version_id);


--
-- Name: billing_price_versions billing_price_versions_sku_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_price_versions
    ADD CONSTRAINT billing_price_versions_sku_version_key UNIQUE (sku, version);


--
-- Name: billing_price_versions billing_price_versions_snapshot_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_price_versions
    ADD CONSTRAINT billing_price_versions_snapshot_key UNIQUE (sku, price_version_id, version, unit_credits);


--
-- Name: billing_skus billing_skus_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_skus
    ADD CONSTRAINT billing_skus_pkey PRIMARY KEY (sku);


--
-- Name: board_members board_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY board_members
    ADD CONSTRAINT board_members_pkey PRIMARY KEY (id);


--
-- Name: board_share_tokens board_share_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY board_share_tokens
    ADD CONSTRAINT board_share_tokens_pkey PRIMARY KEY (id);


--
-- Name: boards boards_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY boards
    ADD CONSTRAINT boards_pkey PRIMARY KEY (id);


--
-- Name: credit_accounts credit_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_accounts
    ADD CONSTRAINT credit_accounts_pkey PRIMARY KEY (id);


--
-- Name: credit_charges credit_charges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_charges
    ADD CONSTRAINT credit_charges_pkey PRIMARY KEY (capture_transaction_id);


--
-- Name: credit_holds credit_holds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_holds
    ADD CONSTRAINT credit_holds_pkey PRIMARY KEY (hold_transaction_id);


--
-- Name: credit_postings credit_postings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_postings
    ADD CONSTRAINT credit_postings_pkey PRIMARY KEY (id);


--
-- Name: credit_reconciliation_findings credit_reconciliation_findings_fingerprint_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_reconciliation_findings
    ADD CONSTRAINT credit_reconciliation_findings_fingerprint_key UNIQUE (fingerprint);


--
-- Name: credit_reconciliation_findings credit_reconciliation_findings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_reconciliation_findings
    ADD CONSTRAINT credit_reconciliation_findings_pkey PRIMARY KEY (id);


--
-- Name: credit_reconciliation_run_findings credit_reconciliation_run_findings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_reconciliation_run_findings
    ADD CONSTRAINT credit_reconciliation_run_findings_pkey PRIMARY KEY (run_id, finding_id);


--
-- Name: credit_reconciliation_runs credit_reconciliation_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_reconciliation_runs
    ADD CONSTRAINT credit_reconciliation_runs_pkey PRIMARY KEY (id);


--
-- Name: credit_transactions credit_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_transactions
    ADD CONSTRAINT credit_transactions_pkey PRIMARY KEY (id);


--
-- Name: credit_wallet_restrictions credit_wallet_restrictions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_wallet_restrictions
    ADD CONSTRAINT credit_wallet_restrictions_pkey PRIMARY KEY (restriction_id);


--
-- Name: credit_wallets credit_wallets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_wallets
    ADD CONSTRAINT credit_wallets_pkey PRIMARY KEY (user_id);


--
-- Name: credit_welcome_claims credit_welcome_claims_claimed_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_welcome_claims
    ADD CONSTRAINT credit_welcome_claims_claimed_user_id_key UNIQUE (claimed_user_id);


--
-- Name: credit_welcome_claims credit_welcome_claims_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_welcome_claims
    ADD CONSTRAINT credit_welcome_claims_pkey PRIMARY KEY (identity_fingerprint);


--
-- Name: credit_welcome_identity_key_guard credit_welcome_identity_key_guard_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_welcome_identity_key_guard
    ADD CONSTRAINT credit_welcome_identity_key_guard_pkey PRIMARY KEY (singleton);


--
-- Name: feature_prompts feature_prompts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY feature_prompts
    ADD CONSTRAINT feature_prompts_pkey PRIMARY KEY (feature_slug);


--
-- Name: media_assets media_assets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY media_assets
    ADD CONSTRAINT media_assets_pkey PRIMARY KEY (id);


--
-- Name: media_refs media_refs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY media_refs
    ADD CONSTRAINT media_refs_pkey PRIMARY KEY (id);


--
-- Name: payment_admin_actions payment_admin_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_admin_actions
    ADD CONSTRAINT payment_admin_actions_pkey PRIMARY KEY (id);


--
-- Name: payment_checkout_shutdowns payment_checkout_shutdowns_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_checkout_shutdowns
    ADD CONSTRAINT payment_checkout_shutdowns_pkey PRIMARY KEY (id);


--
-- Name: payment_customers payment_customers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_customers
    ADD CONSTRAINT payment_customers_pkey PRIMARY KEY (user_id);


--
-- Name: payment_dispute_debit_requests payment_dispute_debit_requests_credit_transaction_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_dispute_debit_requests
    ADD CONSTRAINT payment_dispute_debit_requests_credit_transaction_id_key UNIQUE (credit_transaction_id);


--
-- Name: payment_dispute_debit_requests payment_dispute_debit_requests_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_dispute_debit_requests
    ADD CONSTRAINT payment_dispute_debit_requests_idempotency_key_key UNIQUE (idempotency_key);


--
-- Name: payment_dispute_debit_requests payment_dispute_debit_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_dispute_debit_requests
    ADD CONSTRAINT payment_dispute_debit_requests_pkey PRIMARY KEY (id);


--
-- Name: payment_disputes payment_disputes_debit_transaction_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_disputes
    ADD CONSTRAINT payment_disputes_debit_transaction_id_key UNIQUE (debit_transaction_id);


--
-- Name: payment_disputes payment_disputes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_disputes
    ADD CONSTRAINT payment_disputes_pkey PRIMARY KEY (id);


--
-- Name: payment_disputes payment_disputes_restore_transaction_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_disputes
    ADD CONSTRAINT payment_disputes_restore_transaction_id_key UNIQUE (restore_transaction_id);


--
-- Name: payment_disputes payment_disputes_stripe_dispute_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_disputes
    ADD CONSTRAINT payment_disputes_stripe_dispute_id_key UNIQUE (stripe_dispute_id);


--
-- Name: payment_order_reconciliations payment_order_reconciliations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_order_reconciliations
    ADD CONSTRAINT payment_order_reconciliations_pkey PRIMARY KEY (payment_order_id);


--
-- Name: payment_orders payment_orders_credit_transaction_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_orders
    ADD CONSTRAINT payment_orders_credit_transaction_id_key UNIQUE (credit_transaction_id);


--
-- Name: payment_orders payment_orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_orders
    ADD CONSTRAINT payment_orders_pkey PRIMARY KEY (id);


--
-- Name: payment_orders payment_orders_public_order_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_orders
    ADD CONSTRAINT payment_orders_public_order_number_key UNIQUE (public_order_number);


--
-- Name: payment_orders payment_orders_public_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_orders
    ADD CONSTRAINT payment_orders_public_token_key UNIQUE (public_token);


--
-- Name: payment_refunds payment_refunds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_refunds
    ADD CONSTRAINT payment_refunds_pkey PRIMARY KEY (id);


--
-- Name: payment_stripe_configurations payment_stripe_configurations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_stripe_configurations
    ADD CONSTRAINT payment_stripe_configurations_pkey PRIMARY KEY (singleton_id);


--
-- Name: payment_webhook_events payment_webhook_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_webhook_events
    ADD CONSTRAINT payment_webhook_events_pkey PRIMARY KEY (stripe_event_id);


--
-- Name: reward_code_digest_aliases pk_reward_code_digest_aliases; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_code_digest_aliases
    ADD CONSTRAINT pk_reward_code_digest_aliases PRIMARY KEY (key_version, code_digest);


--
-- Name: provider_endpoints provider_endpoints_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY provider_endpoints
    ADD CONSTRAINT provider_endpoints_pkey PRIMARY KEY (id);


--
-- Name: reward_admin_actions reward_admin_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_admin_actions
    ADD CONSTRAINT reward_admin_actions_pkey PRIMARY KEY (id);


--
-- Name: reward_codes reward_codes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_codes
    ADD CONSTRAINT reward_codes_pkey PRIMARY KEY (id);


--
-- Name: reward_redemptions reward_redemptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_redemptions
    ADD CONSTRAINT reward_redemptions_pkey PRIMARY KEY (id);


--
-- Name: storage_profile_secret_versions storage_profile_secret_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY storage_profile_secret_versions
    ADD CONSTRAINT storage_profile_secret_versions_pkey PRIMARY KEY (id);


--
-- Name: storage_profiles storage_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY storage_profiles
    ADD CONSTRAINT storage_profiles_pkey PRIMARY KEY (profile_id);


--
-- Name: system_configs system_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY system_configs
    ADD CONSTRAINT system_configs_pkey PRIMARY KEY (id);


--
-- Name: system_settings_admin_actions system_settings_admin_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY system_settings_admin_actions
    ADD CONSTRAINT system_settings_admin_actions_pkey PRIMARY KEY (id);


--
-- Name: task_attempts task_attempts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_attempts
    ADD CONSTRAINT task_attempts_pkey PRIMARY KEY (id);


--
-- Name: task_billing_settlements task_billing_settlements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_settlements
    ADD CONSTRAINT task_billing_settlements_pkey PRIMARY KEY (task_id);


--
-- Name: task_billing_snapshots task_billing_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_snapshots
    ADD CONSTRAINT task_billing_snapshots_pkey PRIMARY KEY (task_id);


--
-- Name: task_cancellations task_cancellations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_cancellations
    ADD CONSTRAINT task_cancellations_pkey PRIMARY KEY (task_id);


--
-- Name: task_daily_stat_backfills task_daily_stat_backfills_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_daily_stat_backfills
    ADD CONSTRAINT task_daily_stat_backfills_pkey PRIMARY KEY (id);


--
-- Name: task_daily_stats task_daily_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_daily_stats
    ADD CONSTRAINT task_daily_stats_pkey PRIMARY KEY (id);


--
-- Name: task_execution_metrics task_execution_metrics_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_execution_metrics
    ADD CONSTRAINT task_execution_metrics_pkey PRIMARY KEY (task_id);


--
-- Name: task_leases task_leases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_leases
    ADD CONSTRAINT task_leases_pkey PRIMARY KEY (id);


--
-- Name: task_outputs task_outputs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_outputs
    ADD CONSTRAINT task_outputs_pkey PRIMARY KEY (id);


--
-- Name: task_stat_events task_stat_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_stat_events
    ADD CONSTRAINT task_stat_events_pkey PRIMARY KEY (id);


--
-- Name: task_submission_claims task_submission_claims_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_submission_claims
    ADD CONSTRAINT task_submission_claims_pkey PRIMARY KEY (user_id, client_action_id);


--
-- Name: task_submission_claims task_submission_claims_task_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_submission_claims
    ADD CONSTRAINT task_submission_claims_task_id_key UNIQUE (task_id);


--
-- Name: tasks tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);


--
-- Name: api_accounts uq_api_accounts_id_provider_endpoint_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY api_accounts
    ADD CONSTRAINT uq_api_accounts_id_provider_endpoint_id UNIQUE (id, provider_endpoint_id);


--
-- Name: board_members uq_board_member; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY board_members
    ADD CONSTRAINT uq_board_member UNIQUE (board_id, user_id);


--
-- Name: board_share_tokens uq_board_share_tokens_token_hash; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY board_share_tokens
    ADD CONSTRAINT uq_board_share_tokens_token_hash UNIQUE (token_hash);


--
-- Name: boards uq_boards_board_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY boards
    ADD CONSTRAINT uq_boards_board_id UNIQUE (board_id);


--
-- Name: credit_accounts uq_credit_accounts_id_asset; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_accounts
    ADD CONSTRAINT uq_credit_accounts_id_asset UNIQUE (id, asset_code);


--
-- Name: credit_holds uq_credit_holds_terminal_transaction_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_holds
    ADD CONSTRAINT uq_credit_holds_terminal_transaction_id UNIQUE (terminal_transaction_id);


--
-- Name: credit_postings uq_credit_postings_transaction_account; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_postings
    ADD CONSTRAINT uq_credit_postings_transaction_account UNIQUE (transaction_id, account_id);


--
-- Name: credit_transactions uq_credit_transactions_idempotency_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_transactions
    ADD CONSTRAINT uq_credit_transactions_idempotency_key UNIQUE (idempotency_key);


--
-- Name: credit_transactions uq_credit_transactions_transaction_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_transactions
    ADD CONSTRAINT uq_credit_transactions_transaction_id UNIQUE (transaction_id);


--
-- Name: media_assets uq_media_assets_asset_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY media_assets
    ADD CONSTRAINT uq_media_assets_asset_id UNIQUE (asset_id);


--
-- Name: media_assets uq_media_assets_id_owner; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY media_assets
    ADD CONSTRAINT uq_media_assets_id_owner UNIQUE (id, owner_user_id);


--
-- Name: media_assets uq_media_assets_storage_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY media_assets
    ADD CONSTRAINT uq_media_assets_storage_key UNIQUE (storage_profile_id, storage_key);


--
-- Name: media_refs uq_media_refs_ref; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY media_refs
    ADD CONSTRAINT uq_media_refs_ref UNIQUE (media_asset_id, ref_type, ref_id, board_id, role);


--
-- Name: payment_customers uq_payment_customer_account; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_customers
    ADD CONSTRAINT uq_payment_customer_account UNIQUE (stripe_account_id, stripe_customer_id);


--
-- Name: payment_orders uq_payment_order_checkout_session; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_orders
    ADD CONSTRAINT uq_payment_order_checkout_session UNIQUE (stripe_account_id, stripe_checkout_session_id);


--
-- Name: payment_orders uq_payment_order_idempotency; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_orders
    ADD CONSTRAINT uq_payment_order_idempotency UNIQUE (user_id, client_idempotency_key);


--
-- Name: payment_refunds uq_payment_refund_idempotency; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_refunds
    ADD CONSTRAINT uq_payment_refund_idempotency UNIQUE (payment_order_id, idempotency_key);


--
-- Name: reward_codes uq_reward_codes_creator_idempotency; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_codes
    ADD CONSTRAINT uq_reward_codes_creator_idempotency UNIQUE (created_by, idempotency_key);


--
-- Name: reward_codes uq_reward_codes_digest; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_codes
    ADD CONSTRAINT uq_reward_codes_digest UNIQUE (key_version, code_digest);


--
-- Name: reward_redemptions uq_reward_redemptions_credit_transaction; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_redemptions
    ADD CONSTRAINT uq_reward_redemptions_credit_transaction UNIQUE (credit_transaction_id);


--
-- Name: reward_redemptions uq_reward_redemptions_user_code; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_redemptions
    ADD CONSTRAINT uq_reward_redemptions_user_code UNIQUE (user_id, reward_code_id);


--
-- Name: storage_profile_secret_versions uq_storage_profile_secret_versions_number; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY storage_profile_secret_versions
    ADD CONSTRAINT uq_storage_profile_secret_versions_number UNIQUE (profile_id, secret_name, version_number);


--
-- Name: system_configs uq_system_configs_config_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY system_configs
    ADD CONSTRAINT uq_system_configs_config_key UNIQUE (config_key);


--
-- Name: task_attempts uq_task_attempts_lease_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_attempts
    ADD CONSTRAINT uq_task_attempts_lease_id UNIQUE (lease_id);


--
-- Name: task_attempts uq_task_attempts_task_attempt_no; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_attempts
    ADD CONSTRAINT uq_task_attempts_task_attempt_no UNIQUE (task_id, attempt_no);


--
-- Name: task_daily_stats uq_task_daily_stat; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_daily_stats
    ADD CONSTRAINT uq_task_daily_stat UNIQUE (stat_date, user_id, account_id, provider_endpoint_id, credential_id, feature_slug, api_type, model_name, user_key);


--
-- Name: task_daily_stat_backfills uq_task_daily_stat_backfill_date; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_daily_stat_backfills
    ADD CONSTRAINT uq_task_daily_stat_backfill_date UNIQUE (stat_date);


--
-- Name: task_stat_events uq_task_stat_event_task_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_stat_events
    ADD CONSTRAINT uq_task_stat_event_task_id UNIQUE (task_id);


--
-- Name: tasks uq_tasks_task_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tasks
    ADD CONSTRAINT uq_tasks_task_id UNIQUE (task_id);


--
-- Name: users uq_users_auth_user_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY users
    ADD CONSTRAINT uq_users_auth_user_id UNIQUE (auth_user_id);


--
-- Name: users uq_users_public_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY users
    ADD CONSTRAINT uq_users_public_id UNIQUE (public_id);


--
-- Name: users uq_users_storage_uid; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY users
    ADD CONSTRAINT uq_users_storage_uid UNIQUE (storage_uid);


--
-- Name: users_used_uids uq_users_used_uids_storage_uid; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY users_used_uids
    ADD CONSTRAINT uq_users_used_uids_storage_uid UNIQUE (storage_uid);


--
-- Name: user_deletion_requests user_deletion_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY user_deletion_requests
    ADD CONSTRAINT user_deletion_requests_pkey PRIMARY KEY (user_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users_used_uids users_used_uids_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY users_used_uids
    ADD CONSTRAINT users_used_uids_pkey PRIMARY KEY (id);


--
-- Name: idx_api_account_models_account_enabled_sort; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_account_models_account_enabled_sort ON api_account_models USING btree (account_id, enabled, sort_order, id);


--
-- Name: idx_api_account_models_model_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_account_models_model_name ON api_account_models USING btree (model_name);


--
-- Name: idx_api_account_secrets_account_rotated; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_account_secrets_account_rotated ON api_account_secrets USING btree (account_id, rotated_at DESC);


--
-- Name: idx_api_accounts_provider_endpoint_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_accounts_provider_endpoint_id ON api_accounts USING btree (provider_endpoint_id);


--
-- Name: idx_api_accounts_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_accounts_status ON api_accounts USING btree (status);


--
-- Name: idx_billing_admin_actions_actor_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_admin_actions_actor_id ON billing_admin_actions USING btree (actor_id, id DESC);


--
-- Name: idx_billing_admin_actions_sku_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_admin_actions_sku_id ON billing_admin_actions USING btree (sku, id DESC);


--
-- Name: idx_board_member_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_board_member_user ON board_members USING btree (user_id, board_id);


--
-- Name: idx_board_share_token_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_board_share_token_active ON board_share_tokens USING btree (token_hash) WHERE (NOT revoked);


--
-- Name: idx_board_share_token_board; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_board_share_token_board ON board_share_tokens USING btree (board_id, created_at DESC);


--
-- Name: idx_boards_owner_updated; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_boards_owner_updated ON boards USING btree (owner_user_id, updated_at);


--
-- Name: idx_credit_accounts_system_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_accounts_system_role ON credit_accounts USING btree (account_role, shard_key) WHERE (wallet_user_id IS NULL);


--
-- Name: idx_credit_holds_wallet_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_holds_wallet_status ON credit_holds USING btree (wallet_user_id, status, created_at DESC);


--
-- Name: idx_credit_postings_account_id_desc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_postings_account_id_desc ON credit_postings USING btree (account_id, id DESC);


--
-- Name: idx_credit_postings_transaction_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_postings_transaction_id ON credit_postings USING btree (transaction_id);


--
-- Name: idx_credit_reconciliation_findings_list; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_reconciliation_findings_list ON credit_reconciliation_findings USING btree (status, id DESC);


--
-- Name: idx_credit_reconciliation_findings_wallet; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_reconciliation_findings_wallet ON credit_reconciliation_findings USING btree (wallet_user_id, id DESC) WHERE (wallet_user_id IS NOT NULL);


--
-- Name: idx_credit_reconciliation_runs_requested_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_reconciliation_runs_requested_at ON credit_reconciliation_runs USING btree (id DESC);


--
-- Name: idx_credit_transactions_business_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_transactions_business_reference ON credit_transactions USING btree (business_reference_type, business_reference_id, id DESC);


--
-- Name: idx_credit_transactions_capture_user_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_transactions_capture_user_created ON credit_transactions USING btree (wallet_user_id, created_at) INCLUDE (command_amount) WHERE ((command_type)::text = 'capture'::text);


--
-- Name: idx_credit_transactions_related; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_transactions_related ON credit_transactions USING btree (related_transaction_id) WHERE (related_transaction_id IS NOT NULL);


--
-- Name: idx_credit_transactions_wallet_id_desc; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_transactions_wallet_id_desc ON credit_transactions USING btree (wallet_user_id, id DESC);


--
-- Name: idx_credit_wallet_restrictions_wallet_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_credit_wallet_restrictions_wallet_active ON credit_wallet_restrictions USING btree (wallet_user_id, created_at DESC, restriction_id) WHERE (released_at IS NULL);


--
-- Name: idx_media_assets_owner_active_updated; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_assets_owner_active_updated ON media_assets USING btree (owner_user_id, updated_at DESC, id DESC) WHERE (deleted_at IS NULL);


--
-- Name: idx_media_assets_owner_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_assets_owner_status ON media_assets USING btree (owner_user_id, status, updated_at DESC);


--
-- Name: idx_media_assets_reclamation_due; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_assets_reclamation_due ON media_assets USING btree (reclaim_after, id) WHERE ((deleted_at IS NULL) AND (reclaim_after IS NOT NULL));


--
-- Name: idx_media_assets_storage_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_assets_storage_key ON media_assets USING btree (storage_profile_id, storage_key);


--
-- Name: idx_media_refs_owner_board_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_refs_owner_board_created ON media_refs USING btree (owner_user_id, board_id, created_at, id);


--
-- Name: idx_media_refs_owner_ref; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_refs_owner_ref ON media_refs USING btree (owner_user_id, ref_type, ref_id);


--
-- Name: idx_media_refs_task; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_media_refs_task ON media_refs USING btree (task_id, ref_type);


--
-- Name: idx_payment_checkout_shutdowns_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_checkout_shutdowns_active ON payment_checkout_shutdowns USING btree (created_at, id) WHERE (completed_at IS NULL);


--
-- Name: idx_payment_dispute_debit_requests_wallet; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_dispute_debit_requests_wallet ON payment_dispute_debit_requests USING btree (wallet_user_id, created_at DESC, id DESC);


--
-- Name: idx_payment_disputes_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_disputes_order ON payment_disputes USING btree (payment_order_id, opened_at, id);


--
-- Name: idx_payment_disputes_reconciliation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_disputes_reconciliation ON payment_disputes USING btree (next_reconciliation_at, id) WHERE (status = ANY (ARRAY['open'::text, 'lost'::text]));


--
-- Name: idx_payment_order_reconciliations_anomaly; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_order_reconciliations_anomaly ON payment_order_reconciliations USING btree (anomalous_at DESC, payment_order_id DESC) WHERE (anomalous_at IS NOT NULL);


--
-- Name: idx_payment_order_reconciliations_due; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_order_reconciliations_due ON payment_order_reconciliations USING btree (next_attempt_at, claimed_until, payment_order_id);


--
-- Name: idx_payment_orders_admin_history; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_orders_admin_history ON payment_orders USING btree (created_at DESC, id DESC);


--
-- Name: idx_payment_orders_admin_status_history; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_orders_admin_status_history ON payment_orders USING btree (status, created_at DESC, id DESC);


--
-- Name: idx_payment_orders_open; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_orders_open ON payment_orders USING btree (status, expires_at) WHERE (status = ANY (ARRAY['creating'::text, 'checkout_open'::text, 'async_payment_pending'::text]));


--
-- Name: idx_payment_orders_reconciliation_candidates; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_orders_reconciliation_candidates ON payment_orders USING btree (updated_at, id) WHERE (status = ANY (ARRAY['creating'::text, 'checkout_open'::text, 'async_payment_pending'::text, 'paid_pending_credit'::text]));


--
-- Name: idx_payment_orders_user_history; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_orders_user_history ON payment_orders USING btree (user_id, created_at DESC, id DESC);


--
-- Name: idx_payment_refunds_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_refunds_order ON payment_refunds USING btree (payment_order_id, created_at DESC, id DESC);


--
-- Name: idx_payment_refunds_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_refunds_pending ON payment_refunds USING btree (status, updated_at, id) WHERE (status = 'pending'::text);


--
-- Name: idx_payment_webhook_events_order_history; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_webhook_events_order_history ON payment_webhook_events USING btree (payment_order_id, stripe_created_at DESC, stripe_event_id DESC) WHERE (payment_order_id IS NOT NULL);


--
-- Name: idx_payment_webhook_events_received; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_payment_webhook_events_received ON payment_webhook_events USING btree (received_at DESC, stripe_event_id DESC);


--
-- Name: idx_reward_admin_actions_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reward_admin_actions_code ON reward_admin_actions USING btree (reward_code_id, id DESC);


--
-- Name: idx_reward_code_digest_aliases_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reward_code_digest_aliases_code ON reward_code_digest_aliases USING btree (reward_code_id);


--
-- Name: idx_reward_codes_list; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reward_codes_list ON reward_codes USING btree (id DESC);


--
-- Name: idx_reward_redemptions_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reward_redemptions_code ON reward_redemptions USING btree (reward_code_id, id DESC);


--
-- Name: idx_reward_redemptions_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reward_redemptions_user ON reward_redemptions USING btree (user_id, id DESC);


--
-- Name: idx_storage_profile_secret_versions_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_storage_profile_secret_versions_status ON storage_profile_secret_versions USING btree (status, expires_at);


--
-- Name: idx_storage_profiles_provider_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_storage_profiles_provider_status ON storage_profiles USING btree (provider, status);


--
-- Name: idx_system_settings_admin_actions_actor_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_system_settings_admin_actions_actor_id ON system_settings_admin_actions USING btree (actor_id, id DESC);


--
-- Name: idx_task_attempts_failed_category_completed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_attempts_failed_category_completed_at ON task_attempts USING btree (failure_category, completed_at DESC) WHERE (((status)::text = 'failed'::text) AND (completed_at IS NOT NULL) AND ((failure_category)::text <> ''::text));


--
-- Name: idx_task_attempts_failure_category_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_attempts_failure_category_created_at ON task_attempts USING btree (failure_category, created_at DESC) WHERE ((failure_category)::text <> ''::text);


--
-- Name: idx_task_attempts_running_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_attempts_running_created_at ON task_attempts USING btree (created_at) WHERE ((status)::text = 'running'::text);


--
-- Name: idx_task_attempts_status_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_attempts_status_created_at ON task_attempts USING btree (status, created_at DESC);


--
-- Name: idx_task_attempts_task_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_attempts_task_created_at ON task_attempts USING btree (task_id, created_at DESC);


--
-- Name: idx_task_attempts_terminal_completed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_attempts_terminal_completed_at ON task_attempts USING btree (completed_at DESC, id) WHERE ((completed_at IS NOT NULL) AND ((status)::text = ANY ((ARRAY['completed'::character varying, 'failed'::character varying, 'requeued'::character varying, 'cancelled'::character varying])::text[])));


--
-- Name: idx_task_billing_snapshots_user_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_billing_snapshots_user_created ON task_billing_snapshots USING btree (user_id, created_at);


--
-- Name: idx_task_cancellations_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_cancellations_expires_at ON task_cancellations USING btree (expires_at);


--
-- Name: idx_task_execution_metrics_bottleneck_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_execution_metrics_bottleneck_source ON task_execution_metrics USING btree (bottleneck_source);


--
-- Name: idx_task_execution_metrics_current_stage; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_execution_metrics_current_stage ON task_execution_metrics USING btree (current_stage);


--
-- Name: idx_task_execution_metrics_failure_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_execution_metrics_failure_category ON task_execution_metrics USING btree (failure_category);


--
-- Name: idx_task_leases_active_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_leases_active_account ON task_leases USING btree (account_id, task_id) WHERE (released_at IS NULL);


--
-- Name: idx_task_leases_active_model; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_leases_active_model ON task_leases USING btree (account_id, lower((resolved_model)::text)) WHERE (released_at IS NULL);


--
-- Name: idx_task_leases_credential_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_leases_credential_created_at ON task_leases USING btree (credential_id, created_at DESC);


--
-- Name: idx_task_leases_released_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_leases_released_at ON task_leases USING btree (released_at DESC) WHERE (released_at IS NOT NULL);


--
-- Name: idx_task_outputs_media_asset_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_outputs_media_asset_id ON task_outputs USING btree (media_asset_id);


--
-- Name: idx_task_outputs_task_sort; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_outputs_task_sort ON task_outputs USING btree (task_id, sort_index);


--
-- Name: idx_task_stat_events_account_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_stat_events_account_date ON task_stat_events USING btree (account_id, stat_date) WHERE (account_id > 0);


--
-- Name: idx_task_stat_events_credential_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_stat_events_credential_date ON task_stat_events USING btree (credential_id, stat_date) WHERE (credential_id > 0);


--
-- Name: idx_task_stat_events_feature_api_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_stat_events_feature_api_date ON task_stat_events USING btree (stat_date, feature_slug, api_type);


--
-- Name: idx_task_stat_events_model_latency; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_stat_events_model_latency ON task_stat_events USING btree (stat_date, model_name, latency_seconds) WHERE (latency_seconds > (0)::double precision);


--
-- Name: idx_task_stat_events_occurred_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_stat_events_occurred_at ON task_stat_events USING btree (occurred_at);


--
-- Name: idx_task_stat_events_provider_endpoint_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_stat_events_provider_endpoint_date ON task_stat_events USING btree (provider_endpoint_id, stat_date) WHERE (provider_endpoint_id > 0);


--
-- Name: idx_task_stat_events_stat_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_stat_events_stat_date ON task_stat_events USING btree (stat_date);


--
-- Name: idx_tasks_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_account_id ON tasks USING btree (account_id) WHERE (account_id IS NOT NULL);


--
-- Name: idx_tasks_account_status_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_account_status_created_at ON tasks USING btree (account_id, status, created_at DESC) WHERE (account_id IS NOT NULL);


--
-- Name: idx_tasks_assigned_credential_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_assigned_credential_id ON tasks USING btree (assigned_credential_id) WHERE (assigned_credential_id IS NOT NULL);


--
-- Name: idx_tasks_assigned_provider_endpoint_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_assigned_provider_endpoint_created_at ON tasks USING btree (assigned_provider_endpoint_id, created_at DESC) WHERE (assigned_provider_endpoint_id IS NOT NULL);


--
-- Name: idx_tasks_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_created_at ON tasks USING btree (created_at);


--
-- Name: idx_tasks_feature_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_feature_created_at ON tasks USING btree (feature_slug, created_at DESC) WHERE ((feature_slug)::text <> ''::text);


--
-- Name: idx_tasks_lease_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_lease_token ON tasks USING btree (lease_token);


--
-- Name: idx_tasks_pending_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_pending_updated_at ON tasks USING btree (updated_at) WHERE ((status)::text = 'pending'::text);


--
-- Name: idx_tasks_running_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_running_account ON tasks USING btree (account_id) WHERE (((status)::text = 'running'::text) AND (account_id IS NOT NULL));


--
-- Name: idx_tasks_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_status ON tasks USING btree (status);


--
-- Name: idx_tasks_status_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_status_created_at ON tasks USING btree (status, created_at DESC);


--
-- Name: idx_tasks_terminal_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_terminal_created_at ON tasks USING btree (created_at) WHERE ((status)::text = ANY ((ARRAY['completed'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[]));


--
-- Name: idx_tasks_user_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_user_created_at ON tasks USING btree (user_id, created_at DESC);


--
-- Name: idx_tasks_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_tasks_user_id ON tasks USING btree (user_id);


--
-- Name: uq_api_account_models_account_model_lower; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_api_account_models_account_model_lower ON api_account_models USING btree (account_id, lower((model_name)::text));


--
-- Name: uq_api_account_secrets_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_api_account_secrets_active ON api_account_secrets USING btree (account_id) WHERE ((status)::text = 'active'::text);


--
-- Name: uq_api_account_secrets_id_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_api_account_secrets_id_account_id ON api_account_secrets USING btree (id, account_id);


--
-- Name: uq_api_accounts_provider_endpoint_name_lower; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_api_accounts_provider_endpoint_name_lower ON api_accounts USING btree (provider_endpoint_id, lower((name)::text)) WHERE (deleted_at IS NULL);


--
-- Name: uq_boards_owner_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_boards_owner_name ON boards USING btree (owner_user_id, lower((name)::text));


--
-- Name: uq_credit_accounts_system_shard_role; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_credit_accounts_system_shard_role ON credit_accounts USING btree (asset_code, account_role, shard_key) WHERE (wallet_user_id IS NULL);


--
-- Name: uq_credit_accounts_user_role; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_credit_accounts_user_role ON credit_accounts USING btree (wallet_user_id, asset_code, account_role) WHERE (wallet_user_id IS NOT NULL);


--
-- Name: uq_credit_reconciliation_active_run; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_credit_reconciliation_active_run ON credit_reconciliation_runs USING btree ((true)) WHERE ((status)::text = ANY ((ARRAY['queued'::character varying, 'running'::character varying])::text[]));


--
-- Name: uq_credit_reconciliation_scheduled_for; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_credit_reconciliation_scheduled_for ON credit_reconciliation_runs USING btree (scheduled_for) WHERE ((trigger_type)::text = 'scheduled'::text);


--
-- Name: uq_credit_wallet_restrictions_active_source; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_credit_wallet_restrictions_active_source ON credit_wallet_restrictions USING btree (wallet_user_id, restriction_type, source, related_object_type, related_object_id) WHERE (released_at IS NULL);


--
-- Name: uq_payment_orders_stripe_charge; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_payment_orders_stripe_charge ON payment_orders USING btree (stripe_charge_id) WHERE (stripe_charge_id <> ''::text);


--
-- Name: uq_payment_orders_stripe_payment_intent; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_payment_orders_stripe_payment_intent ON payment_orders USING btree (stripe_payment_intent_id) WHERE (stripe_payment_intent_id <> ''::text);


--
-- Name: uq_payment_orders_user_active_quantity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_payment_orders_user_active_quantity ON payment_orders USING btree (user_id, idea_quantity) WHERE (status = ANY (ARRAY['creating'::text, 'checkout_open'::text, 'async_payment_pending'::text]));


--
-- Name: uq_payment_refund_stripe_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_payment_refund_stripe_id ON payment_refunds USING btree (stripe_refund_id) WHERE (stripe_refund_id <> ''::text);


--
-- Name: uq_provider_endpoints_api_type_base_url; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_provider_endpoints_api_type_base_url ON provider_endpoints USING btree (api_type, base_url);


--
-- Name: uq_provider_endpoints_name_lower; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_provider_endpoints_name_lower ON provider_endpoints USING btree (lower((name)::text));


--
-- Name: uq_storage_profile_secret_versions_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_storage_profile_secret_versions_active ON storage_profile_secret_versions USING btree (profile_id, secret_name) WHERE ((status)::text = 'active'::text);


--
-- Name: uq_storage_profiles_current; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_storage_profiles_current ON storage_profiles USING btree (is_current) WHERE is_current;


--
-- Name: uq_task_leases_lease_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_task_leases_lease_token ON task_leases USING btree (lease_token);


--
-- Name: uq_task_leases_task_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_task_leases_task_active ON task_leases USING btree (task_id) WHERE (released_at IS NULL);


--
-- Name: uq_tasks_user_client_action_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_tasks_user_client_action_id ON tasks USING btree (user_id, client_action_id) WHERE ((client_action_id)::text <> ''::text);


--
-- Name: credit_postings apply_credit_posting_balances; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER apply_credit_posting_balances AFTER INSERT ON credit_postings REFERENCING NEW TABLE AS inserted_credit_postings FOR EACH STATEMENT EXECUTE FUNCTION apply_credit_posting_balances();


--
-- Name: billing_admin_actions billing_admin_actions_append_only; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER billing_admin_actions_append_only BEFORE DELETE OR UPDATE ON billing_admin_actions FOR EACH ROW EXECUTE FUNCTION prevent_billing_admin_action_mutation();


--
-- Name: billing_price_versions billing_price_versions_append_only; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER billing_price_versions_append_only BEFORE DELETE OR UPDATE ON billing_price_versions FOR EACH ROW EXECUTE FUNCTION prevent_billing_price_version_mutation();


--
-- Name: billing_skus billing_skus_immutable_identity; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER billing_skus_immutable_identity BEFORE DELETE OR UPDATE ON billing_skus FOR EACH ROW EXECUTE FUNCTION prevent_billing_sku_identity_mutation();


--
-- Name: payment_refunds enforce_payment_refunds_transition; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER enforce_payment_refunds_transition BEFORE UPDATE ON payment_refunds FOR EACH ROW EXECUTE FUNCTION enforce_payment_refund_transition();


--
-- Name: task_outputs enforce_task_output_media_asset_owner; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER enforce_task_output_media_asset_owner BEFORE INSERT OR UPDATE OF task_id, media_asset_id ON task_outputs FOR EACH ROW EXECUTE FUNCTION enforce_task_output_media_asset_owner();


--
-- Name: credit_accounts guard_credit_account_projection; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER guard_credit_account_projection BEFORE UPDATE ON credit_accounts FOR EACH ROW EXECUTE FUNCTION guard_credit_account_projection();


--
-- Name: credit_postings guard_credit_posting_transaction_scope; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER guard_credit_posting_transaction_scope BEFORE INSERT ON credit_postings FOR EACH ROW EXECUTE FUNCTION guard_credit_posting_transaction_scope();


--
-- Name: payment_disputes guard_payment_disputes; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER guard_payment_disputes BEFORE UPDATE ON payment_disputes FOR EACH ROW EXECUTE FUNCTION guard_payment_dispute_lifecycle();


--
-- Name: payment_orders guard_payment_orders; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER guard_payment_orders BEFORE UPDATE ON payment_orders FOR EACH ROW EXECUTE FUNCTION guard_payment_order_lifecycle();


--
-- Name: payment_orders initialize_payment_order_reconciliation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER initialize_payment_order_reconciliation AFTER INSERT ON payment_orders FOR EACH ROW EXECUTE FUNCTION initialize_payment_order_reconciliation();


--
-- Name: payment_admin_actions payment_admin_actions_append_only; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER payment_admin_actions_append_only BEFORE DELETE OR UPDATE ON payment_admin_actions FOR EACH ROW EXECUTE FUNCTION prevent_payment_admin_action_mutation();


--
-- Name: api_accounts prevent_api_account_provider_endpoint_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER prevent_api_account_provider_endpoint_change BEFORE UPDATE OF provider_endpoint_id ON api_accounts FOR EACH ROW EXECUTE FUNCTION prevent_api_account_provider_endpoint_change();


--
-- Name: credit_postings prevent_credit_postings_mutation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER prevent_credit_postings_mutation BEFORE DELETE OR UPDATE ON credit_postings FOR EACH ROW EXECUTE FUNCTION prevent_credit_ledger_mutation();


--
-- Name: credit_transactions prevent_credit_transactions_mutation; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER prevent_credit_transactions_mutation BEFORE DELETE OR UPDATE ON credit_transactions FOR EACH ROW EXECUTE FUNCTION prevent_credit_ledger_mutation();


--
-- Name: storage_profiles prevent_referenced_storage_profile_locator_change; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER prevent_referenced_storage_profile_locator_change BEFORE UPDATE OF provider, config ON storage_profiles FOR EACH ROW EXECUTE FUNCTION prevent_referenced_storage_profile_locator_change();


--
-- Name: reward_admin_actions reward_admin_actions_append_only; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER reward_admin_actions_append_only BEFORE DELETE OR UPDATE ON reward_admin_actions FOR EACH ROW EXECUTE FUNCTION prevent_reward_admin_action_mutation();


--
-- Name: reward_codes reward_codes_identity_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER reward_codes_identity_immutable BEFORE UPDATE ON reward_codes FOR EACH ROW EXECUTE FUNCTION guard_reward_code_identity();


--
-- Name: api_account_models set_api_account_models_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_api_account_models_updated_at BEFORE UPDATE ON api_account_models FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: api_account_secrets set_api_account_secrets_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_api_account_secrets_updated_at BEFORE UPDATE ON api_account_secrets FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: api_accounts set_api_accounts_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_api_accounts_updated_at BEFORE UPDATE ON api_accounts FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: board_members set_board_members_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_board_members_updated_at BEFORE UPDATE ON board_members FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: board_share_tokens set_board_share_tokens_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_board_share_tokens_updated_at BEFORE UPDATE ON board_share_tokens FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: boards set_boards_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_boards_updated_at BEFORE UPDATE ON boards FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: feature_prompts set_feature_prompts_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_feature_prompts_updated_at BEFORE UPDATE ON feature_prompts FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: media_assets set_media_assets_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_media_assets_updated_at BEFORE UPDATE ON media_assets FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: media_refs set_media_refs_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_media_refs_updated_at BEFORE UPDATE ON media_refs FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: payment_customers set_payment_customers_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_payment_customers_updated_at BEFORE UPDATE ON payment_customers FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: payment_disputes set_payment_disputes_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_payment_disputes_updated_at BEFORE UPDATE ON payment_disputes FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: payment_order_reconciliations set_payment_order_reconciliations_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_payment_order_reconciliations_updated_at BEFORE UPDATE ON payment_order_reconciliations FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: payment_orders set_payment_orders_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_payment_orders_updated_at BEFORE UPDATE ON payment_orders FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: payment_refunds set_payment_refunds_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_payment_refunds_updated_at BEFORE UPDATE ON payment_refunds FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: payment_stripe_configurations set_payment_stripe_configurations_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_payment_stripe_configurations_updated_at BEFORE UPDATE ON payment_stripe_configurations FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: provider_endpoints set_provider_endpoints_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_provider_endpoints_updated_at BEFORE UPDATE ON provider_endpoints FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: reward_codes set_reward_codes_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_reward_codes_updated_at BEFORE UPDATE ON reward_codes FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: storage_profile_secret_versions set_storage_profile_secret_versions_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_storage_profile_secret_versions_updated_at BEFORE UPDATE ON storage_profile_secret_versions FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: storage_profiles set_storage_profiles_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_storage_profiles_updated_at BEFORE UPDATE ON storage_profiles FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: system_configs set_system_configs_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_system_configs_updated_at BEFORE UPDATE ON system_configs FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: task_attempts set_task_attempts_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_task_attempts_updated_at BEFORE UPDATE ON task_attempts FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: task_cancellations set_task_cancellations_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_task_cancellations_updated_at BEFORE UPDATE ON task_cancellations FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: task_daily_stat_backfills set_task_daily_stat_backfills_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_task_daily_stat_backfills_updated_at BEFORE UPDATE ON task_daily_stat_backfills FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: task_daily_stats set_task_daily_stats_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_task_daily_stats_updated_at BEFORE UPDATE ON task_daily_stats FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: task_execution_metrics set_task_execution_metrics_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_task_execution_metrics_updated_at BEFORE UPDATE ON task_execution_metrics FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: task_outputs set_task_outputs_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_task_outputs_updated_at BEFORE UPDATE ON task_outputs FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: task_stat_events set_task_stat_events_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_task_stat_events_updated_at BEFORE UPDATE ON task_stat_events FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: tasks set_tasks_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_tasks_updated_at BEFORE UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: users set_users_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER set_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();


--
-- Name: system_settings_admin_actions system_settings_admin_actions_append_only; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER system_settings_admin_actions_append_only BEFORE DELETE OR UPDATE ON system_settings_admin_actions FOR EACH ROW EXECUTE FUNCTION prevent_system_settings_admin_action_mutation();


--
-- Name: task_billing_settlements task_billing_settlements_state_machine; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER task_billing_settlements_state_machine BEFORE INSERT OR DELETE OR UPDATE ON task_billing_settlements FOR EACH ROW EXECUTE FUNCTION guard_task_billing_settlement_transition();


--
-- Name: task_billing_snapshots task_billing_snapshots_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER task_billing_snapshots_immutable BEFORE DELETE OR UPDATE ON task_billing_snapshots FOR EACH ROW EXECUTE FUNCTION prevent_task_billing_snapshot_mutation();


--
-- Name: task_billing_snapshots task_billing_snapshots_validate_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER task_billing_snapshots_validate_insert BEFORE INSERT ON task_billing_snapshots FOR EACH ROW EXECUTE FUNCTION validate_task_billing_snapshot_insert();


--
-- Name: task_submission_claims task_submission_claims_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER task_submission_claims_guard BEFORE INSERT OR DELETE OR UPDATE ON task_submission_claims FOR EACH ROW EXECUTE FUNCTION guard_task_submission_claim();


--
-- Name: credit_transactions validate_credit_transaction_on_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER validate_credit_transaction_on_insert AFTER INSERT ON credit_transactions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_credit_transaction_balance();


--
-- Name: billing_price_versions billing_price_versions_sku_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_price_versions
    ADD CONSTRAINT billing_price_versions_sku_fkey FOREIGN KEY (sku) REFERENCES billing_skus(sku) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: billing_skus billing_skus_current_price_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_skus
    ADD CONSTRAINT billing_skus_current_price_fk FOREIGN KEY (sku, current_price_version_id) REFERENCES billing_price_versions(sku, price_version_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: credit_reconciliation_findings credit_reconciliation_findings_first_seen_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_reconciliation_findings
    ADD CONSTRAINT credit_reconciliation_findings_first_seen_run_id_fkey FOREIGN KEY (first_seen_run_id) REFERENCES credit_reconciliation_runs(id) ON DELETE RESTRICT;


--
-- Name: credit_reconciliation_findings credit_reconciliation_findings_last_seen_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_reconciliation_findings
    ADD CONSTRAINT credit_reconciliation_findings_last_seen_run_id_fkey FOREIGN KEY (last_seen_run_id) REFERENCES credit_reconciliation_runs(id) ON DELETE RESTRICT;


--
-- Name: credit_reconciliation_findings credit_reconciliation_findings_wallet_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_reconciliation_findings
    ADD CONSTRAINT credit_reconciliation_findings_wallet_user_id_fkey FOREIGN KEY (wallet_user_id) REFERENCES credit_wallets(user_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: credit_reconciliation_run_findings credit_reconciliation_run_findings_finding_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_reconciliation_run_findings
    ADD CONSTRAINT credit_reconciliation_run_findings_finding_id_fkey FOREIGN KEY (finding_id) REFERENCES credit_reconciliation_findings(id) ON DELETE RESTRICT;


--
-- Name: credit_reconciliation_run_findings credit_reconciliation_run_findings_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_reconciliation_run_findings
    ADD CONSTRAINT credit_reconciliation_run_findings_run_id_fkey FOREIGN KEY (run_id) REFERENCES credit_reconciliation_runs(id) ON DELETE RESTRICT;


--
-- Name: api_account_models fk_api_account_models_account_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY api_account_models
    ADD CONSTRAINT fk_api_account_models_account_id FOREIGN KEY (account_id) REFERENCES api_accounts(id) ON DELETE CASCADE;


--
-- Name: api_account_secrets fk_api_account_secrets_account_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY api_account_secrets
    ADD CONSTRAINT fk_api_account_secrets_account_id FOREIGN KEY (account_id) REFERENCES api_accounts(id) ON DELETE CASCADE;


--
-- Name: api_accounts fk_api_accounts_provider_endpoint_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY api_accounts
    ADD CONSTRAINT fk_api_accounts_provider_endpoint_id FOREIGN KEY (provider_endpoint_id) REFERENCES provider_endpoints(id) ON DELETE RESTRICT;


--
-- Name: board_members fk_board_members_board; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY board_members
    ADD CONSTRAINT fk_board_members_board FOREIGN KEY (board_id) REFERENCES boards(board_id) ON DELETE CASCADE;


--
-- Name: board_members fk_board_members_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY board_members
    ADD CONSTRAINT fk_board_members_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;


--
-- Name: board_share_tokens fk_board_share_tokens_board; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY board_share_tokens
    ADD CONSTRAINT fk_board_share_tokens_board FOREIGN KEY (board_id) REFERENCES boards(board_id) ON DELETE CASCADE;


--
-- Name: board_share_tokens fk_board_share_tokens_created_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY board_share_tokens
    ADD CONSTRAINT fk_board_share_tokens_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: board_share_tokens fk_board_share_tokens_revoked_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY board_share_tokens
    ADD CONSTRAINT fk_board_share_tokens_revoked_by FOREIGN KEY (revoked_by) REFERENCES users(id) ON DELETE SET NULL;


--
-- Name: boards fk_boards_created_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY boards
    ADD CONSTRAINT fk_boards_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: boards fk_boards_last_saved_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY boards
    ADD CONSTRAINT fk_boards_last_saved_by FOREIGN KEY (last_saved_by) REFERENCES users(id) ON DELETE SET NULL;


--
-- Name: boards fk_boards_owner_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY boards
    ADD CONSTRAINT fk_boards_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE CASCADE;


--
-- Name: credit_accounts fk_credit_accounts_wallet_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_accounts
    ADD CONSTRAINT fk_credit_accounts_wallet_user_id FOREIGN KEY (wallet_user_id) REFERENCES credit_wallets(user_id) ON DELETE RESTRICT;


--
-- Name: credit_charges fk_credit_charges_capture_transaction_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_charges
    ADD CONSTRAINT fk_credit_charges_capture_transaction_id FOREIGN KEY (capture_transaction_id) REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT;


--
-- Name: credit_charges fk_credit_charges_wallet_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_charges
    ADD CONSTRAINT fk_credit_charges_wallet_user_id FOREIGN KEY (wallet_user_id) REFERENCES credit_wallets(user_id) ON DELETE RESTRICT;


--
-- Name: credit_holds fk_credit_holds_hold_transaction_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_holds
    ADD CONSTRAINT fk_credit_holds_hold_transaction_id FOREIGN KEY (hold_transaction_id) REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT;


--
-- Name: credit_holds fk_credit_holds_terminal_transaction_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_holds
    ADD CONSTRAINT fk_credit_holds_terminal_transaction_id FOREIGN KEY (terminal_transaction_id) REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT;


--
-- Name: credit_holds fk_credit_holds_wallet_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_holds
    ADD CONSTRAINT fk_credit_holds_wallet_user_id FOREIGN KEY (wallet_user_id) REFERENCES credit_wallets(user_id) ON DELETE RESTRICT;


--
-- Name: credit_postings fk_credit_postings_account_asset; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_postings
    ADD CONSTRAINT fk_credit_postings_account_asset FOREIGN KEY (account_id, asset_code) REFERENCES credit_accounts(id, asset_code) ON DELETE RESTRICT;


--
-- Name: credit_postings fk_credit_postings_transaction_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_postings
    ADD CONSTRAINT fk_credit_postings_transaction_id FOREIGN KEY (transaction_id) REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT;


--
-- Name: credit_transactions fk_credit_transactions_related_transaction_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_transactions
    ADD CONSTRAINT fk_credit_transactions_related_transaction_id FOREIGN KEY (related_transaction_id) REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT;


--
-- Name: credit_transactions fk_credit_transactions_wallet_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_transactions
    ADD CONSTRAINT fk_credit_transactions_wallet_user_id FOREIGN KEY (wallet_user_id) REFERENCES credit_wallets(user_id) ON DELETE RESTRICT;


--
-- Name: credit_wallet_restrictions fk_credit_wallet_restrictions_wallet_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_wallet_restrictions
    ADD CONSTRAINT fk_credit_wallet_restrictions_wallet_user_id FOREIGN KEY (wallet_user_id) REFERENCES credit_wallets(user_id) ON DELETE RESTRICT;


--
-- Name: credit_wallets fk_credit_wallets_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_wallets
    ADD CONSTRAINT fk_credit_wallets_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: credit_welcome_claims fk_credit_welcome_claims_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY credit_welcome_claims
    ADD CONSTRAINT fk_credit_welcome_claims_user_id FOREIGN KEY (claimed_user_id) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: feature_prompts fk_feature_prompts_updated_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY feature_prompts
    ADD CONSTRAINT fk_feature_prompts_updated_by FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL;


--
-- Name: media_assets fk_media_assets_owner_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY media_assets
    ADD CONSTRAINT fk_media_assets_owner_user_id FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: media_assets fk_media_assets_storage_profile_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY media_assets
    ADD CONSTRAINT fk_media_assets_storage_profile_id FOREIGN KEY (storage_profile_id) REFERENCES storage_profiles(profile_id) ON DELETE RESTRICT;


--
-- Name: media_refs fk_media_refs_asset_owner; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY media_refs
    ADD CONSTRAINT fk_media_refs_asset_owner FOREIGN KEY (media_asset_id, owner_user_id) REFERENCES media_assets(id, owner_user_id) ON DELETE CASCADE;


--
-- Name: reward_admin_actions fk_reward_admin_actions_actor; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_admin_actions
    ADD CONSTRAINT fk_reward_admin_actions_actor FOREIGN KEY (actor_user_id) REFERENCES users(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: reward_admin_actions fk_reward_admin_actions_code; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_admin_actions
    ADD CONSTRAINT fk_reward_admin_actions_code FOREIGN KEY (reward_code_id) REFERENCES reward_codes(id) ON UPDATE RESTRICT ON DELETE SET NULL;


--
-- Name: reward_code_digest_aliases fk_reward_code_digest_aliases_code; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_code_digest_aliases
    ADD CONSTRAINT fk_reward_code_digest_aliases_code FOREIGN KEY (reward_code_id) REFERENCES reward_codes(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: reward_codes fk_reward_codes_created_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_codes
    ADD CONSTRAINT fk_reward_codes_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: reward_redemptions fk_reward_redemptions_code; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_redemptions
    ADD CONSTRAINT fk_reward_redemptions_code FOREIGN KEY (reward_code_id) REFERENCES reward_codes(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: reward_redemptions fk_reward_redemptions_credit_transaction; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_redemptions
    ADD CONSTRAINT fk_reward_redemptions_credit_transaction FOREIGN KEY (credit_transaction_id) REFERENCES credit_transactions(transaction_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: reward_redemptions fk_reward_redemptions_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reward_redemptions
    ADD CONSTRAINT fk_reward_redemptions_user FOREIGN KEY (user_id) REFERENCES users(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: storage_profile_secret_versions fk_storage_profile_secret_versions_profile_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY storage_profile_secret_versions
    ADD CONSTRAINT fk_storage_profile_secret_versions_profile_id FOREIGN KEY (profile_id) REFERENCES storage_profiles(profile_id) ON DELETE CASCADE;


--
-- Name: storage_profiles fk_storage_profiles_created_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY storage_profiles
    ADD CONSTRAINT fk_storage_profiles_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;


--
-- Name: storage_profiles fk_storage_profiles_updated_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY storage_profiles
    ADD CONSTRAINT fk_storage_profiles_updated_by FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL;


--
-- Name: task_attempts fk_task_attempts_lease_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_attempts
    ADD CONSTRAINT fk_task_attempts_lease_id FOREIGN KEY (lease_id) REFERENCES task_leases(id) ON DELETE CASCADE;


--
-- Name: task_attempts fk_task_attempts_task_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_attempts
    ADD CONSTRAINT fk_task_attempts_task_id FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE;


--
-- Name: task_execution_metrics fk_task_execution_metrics_task_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_execution_metrics
    ADD CONSTRAINT fk_task_execution_metrics_task_id FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE;


--
-- Name: task_leases fk_task_leases_account_credential; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_leases
    ADD CONSTRAINT fk_task_leases_account_credential FOREIGN KEY (credential_id, account_id) REFERENCES api_account_secrets(id, account_id) ON DELETE RESTRICT;


--
-- Name: task_leases fk_task_leases_account_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_leases
    ADD CONSTRAINT fk_task_leases_account_id FOREIGN KEY (account_id) REFERENCES api_accounts(id) ON DELETE RESTRICT;


--
-- Name: task_leases fk_task_leases_account_provider_endpoint; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_leases
    ADD CONSTRAINT fk_task_leases_account_provider_endpoint FOREIGN KEY (account_id, provider_endpoint_id) REFERENCES api_accounts(id, provider_endpoint_id) ON DELETE RESTRICT;


--
-- Name: task_leases fk_task_leases_credential_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_leases
    ADD CONSTRAINT fk_task_leases_credential_id FOREIGN KEY (credential_id) REFERENCES api_account_secrets(id) ON DELETE RESTRICT;


--
-- Name: task_leases fk_task_leases_provider_endpoint_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_leases
    ADD CONSTRAINT fk_task_leases_provider_endpoint_id FOREIGN KEY (provider_endpoint_id) REFERENCES provider_endpoints(id) ON DELETE RESTRICT;


--
-- Name: task_leases fk_task_leases_task_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_leases
    ADD CONSTRAINT fk_task_leases_task_id FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE;


--
-- Name: task_outputs fk_task_outputs_media_asset_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_outputs
    ADD CONSTRAINT fk_task_outputs_media_asset_id FOREIGN KEY (media_asset_id) REFERENCES media_assets(id) ON DELETE RESTRICT;


--
-- Name: task_outputs fk_task_outputs_task_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_outputs
    ADD CONSTRAINT fk_task_outputs_task_id FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE;


--
-- Name: tasks fk_tasks_account_credential; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tasks
    ADD CONSTRAINT fk_tasks_account_credential FOREIGN KEY (assigned_credential_id, account_id) REFERENCES api_account_secrets(id, account_id) ON DELETE RESTRICT;


--
-- Name: tasks fk_tasks_account_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tasks
    ADD CONSTRAINT fk_tasks_account_id FOREIGN KEY (account_id) REFERENCES api_accounts(id) ON DELETE RESTRICT;


--
-- Name: tasks fk_tasks_account_provider_endpoint; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tasks
    ADD CONSTRAINT fk_tasks_account_provider_endpoint FOREIGN KEY (account_id, assigned_provider_endpoint_id) REFERENCES api_accounts(id, provider_endpoint_id) ON DELETE RESTRICT;


--
-- Name: tasks fk_tasks_assigned_credential_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tasks
    ADD CONSTRAINT fk_tasks_assigned_credential_id FOREIGN KEY (assigned_credential_id) REFERENCES api_account_secrets(id) ON DELETE RESTRICT;


--
-- Name: tasks fk_tasks_assigned_provider_endpoint_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tasks
    ADD CONSTRAINT fk_tasks_assigned_provider_endpoint_id FOREIGN KEY (assigned_provider_endpoint_id) REFERENCES provider_endpoints(id) ON DELETE RESTRICT;


--
-- Name: tasks fk_tasks_cancelled_by; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tasks
    ADD CONSTRAINT fk_tasks_cancelled_by FOREIGN KEY (cancelled_by) REFERENCES users(id) ON DELETE SET NULL;


--
-- Name: tasks fk_tasks_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY tasks
    ADD CONSTRAINT fk_tasks_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;


--
-- Name: payment_admin_actions payment_admin_actions_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_admin_actions
    ADD CONSTRAINT payment_admin_actions_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: payment_customers payment_customers_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_customers
    ADD CONSTRAINT payment_customers_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: payment_dispute_debit_requests payment_dispute_debit_requests_credit_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_dispute_debit_requests
    ADD CONSTRAINT payment_dispute_debit_requests_credit_transaction_id_fkey FOREIGN KEY (credit_transaction_id) REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT;


--
-- Name: payment_dispute_debit_requests payment_dispute_debit_requests_wallet_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_dispute_debit_requests
    ADD CONSTRAINT payment_dispute_debit_requests_wallet_user_id_fkey FOREIGN KEY (wallet_user_id) REFERENCES credit_wallets(user_id) ON DELETE RESTRICT;


--
-- Name: payment_disputes payment_disputes_debit_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_disputes
    ADD CONSTRAINT payment_disputes_debit_transaction_id_fkey FOREIGN KEY (debit_transaction_id) REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT;


--
-- Name: payment_disputes payment_disputes_payment_order_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_disputes
    ADD CONSTRAINT payment_disputes_payment_order_id_fkey FOREIGN KEY (payment_order_id) REFERENCES payment_orders(id) ON DELETE RESTRICT;


--
-- Name: payment_disputes payment_disputes_restore_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_disputes
    ADD CONSTRAINT payment_disputes_restore_transaction_id_fkey FOREIGN KEY (restore_transaction_id) REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT;


--
-- Name: payment_disputes payment_disputes_restriction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_disputes
    ADD CONSTRAINT payment_disputes_restriction_id_fkey FOREIGN KEY (restriction_id) REFERENCES credit_wallet_restrictions(restriction_id) ON DELETE RESTRICT;


--
-- Name: payment_disputes payment_disputes_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_disputes
    ADD CONSTRAINT payment_disputes_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: payment_order_reconciliations payment_order_reconciliations_payment_order_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_order_reconciliations
    ADD CONSTRAINT payment_order_reconciliations_payment_order_id_fkey FOREIGN KEY (payment_order_id) REFERENCES payment_orders(id) ON DELETE RESTRICT;


--
-- Name: payment_orders payment_orders_credit_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_orders
    ADD CONSTRAINT payment_orders_credit_transaction_id_fkey FOREIGN KEY (credit_transaction_id) REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT;


--
-- Name: payment_orders payment_orders_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_orders
    ADD CONSTRAINT payment_orders_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: payment_refunds payment_refunds_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_refunds
    ADD CONSTRAINT payment_refunds_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: payment_refunds payment_refunds_payment_order_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_refunds
    ADD CONSTRAINT payment_refunds_payment_order_id_fkey FOREIGN KEY (payment_order_id) REFERENCES payment_orders(id) ON DELETE RESTRICT;


--
-- Name: payment_refunds payment_refunds_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_refunds
    ADD CONSTRAINT payment_refunds_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: payment_stripe_configurations payment_stripe_configurations_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_stripe_configurations
    ADD CONSTRAINT payment_stripe_configurations_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: payment_stripe_configurations payment_stripe_configurations_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_stripe_configurations
    ADD CONSTRAINT payment_stripe_configurations_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE RESTRICT;


--
-- Name: payment_webhook_events payment_webhook_events_payment_order_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY payment_webhook_events
    ADD CONSTRAINT payment_webhook_events_payment_order_id_fkey FOREIGN KEY (payment_order_id) REFERENCES payment_orders(id) ON DELETE RESTRICT;


--
-- Name: task_billing_settlements task_billing_settlements_settlement_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_settlements
    ADD CONSTRAINT task_billing_settlements_settlement_transaction_id_fkey FOREIGN KEY (settlement_transaction_id) REFERENCES credit_transactions(transaction_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_billing_settlements task_billing_settlements_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_settlements
    ADD CONSTRAINT task_billing_settlements_task_id_fkey FOREIGN KEY (task_id) REFERENCES task_billing_snapshots(task_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_billing_snapshots task_billing_snapshot_hold_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_snapshots
    ADD CONSTRAINT task_billing_snapshot_hold_fk FOREIGN KEY (hold_transaction_id) REFERENCES credit_transactions(transaction_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_billing_snapshots task_billing_snapshot_price_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_snapshots
    ADD CONSTRAINT task_billing_snapshot_price_fk FOREIGN KEY (sku, price_version_id, price_version, unit_credits) REFERENCES billing_price_versions(sku, price_version_id, version, unit_credits) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_billing_snapshots task_billing_snapshots_sku_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_snapshots
    ADD CONSTRAINT task_billing_snapshots_sku_fkey FOREIGN KEY (sku) REFERENCES billing_skus(sku) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_submission_claims task_submission_claims_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_submission_claims
    ADD CONSTRAINT task_submission_claims_task_id_fkey FOREIGN KEY (task_id) REFERENCES task_billing_snapshots(task_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: user_deletion_requests user_deletion_requests_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY user_deletion_requests
    ADD CONSTRAINT user_deletion_requests_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT;


--
--

-- Static platform defaults intentionally use column lists and database defaults
-- so this baseline remains deterministic across fresh installations.
INSERT INTO storage_profiles (profile_id, name, provider, enabled, is_current, builtin, status, config)
VALUES ('builtin-local', '本地存储', 'local', TRUE, TRUE, TRUE, 'active', '{}'::jsonb)
ON CONFLICT (profile_id) DO NOTHING;

INSERT INTO system_configs (config_key, value)
VALUES ('api_outbound_allow_hosts', '[]')
ON CONFLICT (config_key) DO NOTHING;

-- pg_dump serializes function configuration with the source schema name.
-- The baseline must also work in isolated schemas, so bind every privileged
-- routine to the schema receiving this migration and revoke PUBLIC execution.
DO $$
DECLARE
  routine RECORD;
  target_schema TEXT := current_schema();
BEGIN
  FOR routine IN
    SELECT
      schemas.nspname AS schema_name,
      functions.proname AS function_name,
      pg_catalog.pg_get_function_identity_arguments(functions.oid) AS arguments
    FROM pg_catalog.pg_proc AS functions
    JOIN pg_catalog.pg_namespace AS schemas ON schemas.oid = functions.pronamespace
    WHERE schemas.nspname = target_schema
      AND functions.prosecdef
  LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%I(%s) SET search_path TO %I, pg_catalog, pg_temp',
      routine.schema_name,
      routine.function_name,
      routine.arguments,
      target_schema
    );
    EXECUTE format(
      'REVOKE ALL ON FUNCTION %I.%I(%s) FROM PUBLIC',
      routine.schema_name,
      routine.function_name,
      routine.arguments
    );
  END LOOP;
END;
$$;
