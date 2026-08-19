CREATE TRIGGER set_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_api_accounts_updated_at BEFORE UPDATE ON api_accounts FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_api_account_models_updated_at BEFORE UPDATE ON api_account_models FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_api_account_secrets_updated_at BEFORE UPDATE ON api_account_secrets FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_tasks_updated_at BEFORE UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_task_execution_metrics_updated_at BEFORE UPDATE ON task_execution_metrics FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_task_attempts_updated_at BEFORE UPDATE ON task_attempts FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_task_cancellations_updated_at BEFORE UPDATE ON task_cancellations FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_media_assets_updated_at BEFORE UPDATE ON media_assets FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_media_refs_updated_at BEFORE UPDATE ON media_refs FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_media_derivatives_updated_at BEFORE UPDATE ON media_derivatives FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_task_outputs_updated_at BEFORE UPDATE ON task_outputs FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_task_daily_stats_updated_at BEFORE UPDATE ON task_daily_stats FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_task_stat_events_updated_at BEFORE UPDATE ON task_stat_events FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_task_daily_stat_backfills_updated_at BEFORE UPDATE ON task_daily_stat_backfills FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_provider_endpoints_updated_at BEFORE UPDATE ON provider_endpoints FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_provider_endpoint_models_updated_at BEFORE UPDATE ON provider_endpoint_models FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_system_configs_updated_at BEFORE UPDATE ON system_configs FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_storage_profiles_updated_at BEFORE UPDATE ON storage_profiles FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_storage_profile_secret_versions_updated_at BEFORE UPDATE ON storage_profile_secret_versions FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_feature_prompts_updated_at BEFORE UPDATE ON feature_prompts FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_boards_updated_at BEFORE UPDATE ON boards FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_board_members_updated_at BEFORE UPDATE ON board_members FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER set_board_share_tokens_updated_at BEFORE UPDATE ON board_share_tokens FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE media_assets ADD CONSTRAINT fk_media_assets_storage_profile_id FOREIGN KEY (storage_profile_id) REFERENCES storage_profiles(profile_id) ON DELETE RESTRICT;
ALTER TABLE payment_webhook_events ADD CONSTRAINT payment_webhook_events_payment_order_id_fkey FOREIGN KEY (payment_order_id) REFERENCES payment_orders(id) ON DELETE RESTRICT;

ALTER TABLE credit_transactions
  DROP CONSTRAINT ck_credit_transactions_command_type,
  ADD CONSTRAINT ck_credit_transactions_command_type CHECK (
    command_type IN (
      'grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction',
      'purchase_credit', 'purchase_refund', 'payment_dispute_debit', 'payment_dispute_restore'
    )
  );

-- This records a signed Dispute debit request that could not create a Ledger
-- transaction because the Wallet had no available Ideas to recover.
CREATE TABLE payment_dispute_debit_requests (
  id UUID PRIMARY KEY,
  wallet_user_id BIGINT NOT NULL REFERENCES credit_wallets(user_id) ON DELETE RESTRICT,
  idempotency_key TEXT NOT NULL UNIQUE,
  request_hash CHAR(64) NOT NULL,
  requested_amount BIGINT NOT NULL,
  debited_amount BIGINT NOT NULL,
  available_balance_after BIGINT NOT NULL,
  reserved_balance_after BIGINT NOT NULL,
  credit_transaction_id TEXT UNIQUE REFERENCES credit_transactions(transaction_id) ON DELETE RESTRICT,
  business_reference_type TEXT NOT NULL,
  business_reference_id TEXT NOT NULL,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  CONSTRAINT ck_payment_dispute_debit_request_hash CHECK (request_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT ck_payment_dispute_debit_request_amounts CHECK (
    requested_amount > 0 AND debited_amount BETWEEN 0 AND requested_amount
    AND available_balance_after >= 0 AND reserved_balance_after >= 0
  ),
  CONSTRAINT ck_payment_dispute_debit_request_reference CHECK (
    business_reference_type = 'payment_dispute' AND btrim(business_reference_id) <> ''
  ),
  CONSTRAINT ck_payment_dispute_debit_request_transaction CHECK (
    (debited_amount = 0 AND credit_transaction_id IS NULL)
    OR (debited_amount > 0 AND credit_transaction_id IS NOT NULL)
  )
);

CREATE INDEX idx_payment_dispute_debit_requests_wallet
  ON payment_dispute_debit_requests (wallet_user_id, created_at DESC, id DESC);

-- statement:begin
CREATE FUNCTION execute_credit_payment_dispute_debit_v1(
  p_user_id BIGINT,
  p_requested_amount BIGINT,
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
  replayed BOOLEAN, created_at TIMESTAMPTZ(3), requested_amount BIGINT, unrecovered_amount BIGINT
)
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
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE FUNCTION execute_credit_payment_dispute_restore_v1(
  p_user_id BIGINT,
  p_original_transaction_id TEXT,
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
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_payment_dispute_debit_v1(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_payment_dispute_restore_v1(BIGINT, TEXT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION execute_credit_payment_dispute_debit_v1(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION execute_credit_payment_dispute_restore_v1(BIGINT, TEXT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;

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
$$ LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = pg_catalog;

REVOKE ALL ON FUNCTION has_unsettled_payment_state(BIGINT) FROM PUBLIC;

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
