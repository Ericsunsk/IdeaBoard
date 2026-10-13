-- destructive: approved
-- Modify "billing_admin_actions" table
ALTER TABLE "billing_admin_actions" DROP CONSTRAINT "ck_billing_admin_actions_reason", DROP COLUMN "reason";
-- Modify "control_audit_records" table
ALTER TABLE "control_audit_records" DROP COLUMN "reason";
-- Modify "credit_reconciliation_runs" table
ALTER TABLE "credit_reconciliation_runs" DROP CONSTRAINT "ck_credit_reconciliation_run_audit", ADD CONSTRAINT "ck_credit_reconciliation_run_audit" CHECK ((((trigger_type)::text = 'manual'::text) AND (requested_by IS NOT NULL) AND (requested_by > 0) AND (scheduled_for IS NULL)) OR (((trigger_type)::text = 'scheduled'::text) AND (requested_by IS NULL) AND (scheduled_for IS NOT NULL))), DROP COLUMN "reason";
-- Modify "identity_admin_reconciliation_runs" table
ALTER TABLE "identity_admin_reconciliation_runs" DROP CONSTRAINT "ck_identity_admin_recon_run_audit", ADD CONSTRAINT "ck_identity_admin_recon_run_audit" CHECK ((((trigger_type)::text = 'manual'::text) AND (requested_by IS NOT NULL) AND (requested_by > 0) AND (scheduled_for IS NULL)) OR (((trigger_type)::text = 'scheduled'::text) AND (requested_by IS NULL) AND (scheduled_for IS NOT NULL))), DROP COLUMN "reason";
-- Modify "identity_auth_sync_intents" table
ALTER TABLE "identity_auth_sync_intents" DROP COLUMN "reason";

DROP FUNCTION start_identity_admin_reconciliation(TEXT, BIGINT, TEXT, TIMESTAMPTZ, BOOLEAN);
CREATE FUNCTION start_identity_admin_reconciliation(
  p_trigger_type TEXT,
  p_requested_by BIGINT,
  p_scheduled_for TIMESTAMPTZ,
  p_only_if_overdue BOOLEAN
)
RETURNS TABLE (run_id BIGINT, enqueue_required BOOLEAN) AS $$
DECLARE
  v_run_id BIGINT;
  v_run_status VARCHAR(16);
BEGIN
  p_trigger_type := btrim(p_trigger_type);
  IF p_trigger_type IS NULL
     OR p_only_if_overdue IS NULL
     OR (
       p_trigger_type = 'manual'
       AND (
         p_requested_by IS NULL OR p_requested_by <= 0
         OR p_scheduled_for IS NOT NULL
         OR p_only_if_overdue
       )
     )
     OR (
       p_trigger_type = 'scheduled'
       AND (
         p_requested_by IS NOT NULL
         OR p_scheduled_for IS NULL
       )
     )
     OR p_trigger_type NOT IN ('manual', 'scheduled') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB311', MESSAGE = 'invalid identity admin reconciliation request';
  END IF;

  PERFORM pg_advisory_xact_lock(5283921922134398291);

  IF p_only_if_overdue AND EXISTS (
    SELECT 1
    FROM identity_admin_reconciliation_runs
    WHERE status = 'completed'
      AND completed_at >= clock_timestamp() - INTERVAL '24 hours'
  ) THEN
    RETURN QUERY SELECT NULL::BIGINT, FALSE;
    RETURN;
  END IF;

  SELECT runs.id INTO v_run_id
  FROM identity_admin_reconciliation_runs runs
  WHERE runs.status IN ('queued', 'running')
  ORDER BY runs.id DESC
  LIMIT 1;
  IF FOUND THEN
    RETURN QUERY SELECT v_run_id, FALSE;
    RETURN;
  END IF;

  IF p_trigger_type = 'scheduled' THEN
    SELECT runs.id, runs.status INTO v_run_id, v_run_status
    FROM identity_admin_reconciliation_runs runs
    WHERE runs.trigger_type = 'scheduled'
      AND runs.scheduled_for = p_scheduled_for;
    IF FOUND THEN
      IF p_only_if_overdue AND v_run_status = 'failed' THEN
        UPDATE identity_admin_reconciliation_runs
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

  INSERT INTO identity_admin_reconciliation_runs (
    trigger_type,
    requested_by,
    scheduled_for
  )
  VALUES (
    p_trigger_type,
    p_requested_by,
    p_scheduled_for
  )
  RETURNING id INTO v_run_id;

  RETURN QUERY SELECT v_run_id, TRUE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
REVOKE ALL ON FUNCTION start_identity_admin_reconciliation(TEXT, BIGINT, TIMESTAMPTZ, BOOLEAN) FROM PUBLIC;
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format('ALTER FUNCTION %I.start_identity_admin_reconciliation(TEXT, BIGINT, TIMESTAMPTZ, BOOLEAN) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
END;
$$;

DROP FUNCTION start_credit_reconciliation(TEXT, BIGINT, TEXT, TIMESTAMPTZ, BOOLEAN);
CREATE FUNCTION start_credit_reconciliation(
  p_trigger_type TEXT,
  p_requested_by BIGINT,
  p_scheduled_for TIMESTAMPTZ,
  p_only_if_overdue BOOLEAN
)
RETURNS TABLE (run_id BIGINT, enqueue_required BOOLEAN) AS $$
DECLARE
  v_run_id BIGINT;
  v_run_status VARCHAR(16);
BEGIN
  p_trigger_type := btrim(p_trigger_type);
  IF p_trigger_type IS NULL
     OR p_only_if_overdue IS NULL
     OR (
       p_trigger_type = 'manual'
       AND (
         p_requested_by IS NULL OR p_requested_by <= 0
         OR p_scheduled_for IS NOT NULL
         OR p_only_if_overdue
       )
     )
     OR (
       p_trigger_type = 'scheduled'
       AND (
         p_requested_by IS NOT NULL
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
    scheduled_for
  )
  VALUES (
    p_trigger_type,
    p_requested_by,
    p_scheduled_for
  )
  RETURNING id INTO v_run_id;

  RETURN QUERY SELECT v_run_id, TRUE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
REVOKE ALL ON FUNCTION start_credit_reconciliation(TEXT, BIGINT, TIMESTAMPTZ, BOOLEAN) FROM PUBLIC;
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format('ALTER FUNCTION %I.start_credit_reconciliation(TEXT, BIGINT, TIMESTAMPTZ, BOOLEAN) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
END;
$$;

CREATE OR REPLACE FUNCTION delete_archived_billing_item(p_sku text, p_actor_id bigint) RETURNS text
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
    action_type, sku, price_version_id, actor_id, metadata
  ) VALUES (
    'delete_archived', p_sku, NULL, p_actor_id,
    jsonb_build_object(
      'display_name', v_display_name,
      'description', v_description,
      'price_version_count', v_price_count,
      'outcome', 'deleted'
    )
  );

  PERFORM set_config('ideaboard.billing_archive_delete', p_sku, TRUE);
  DELETE FROM billing_current_prices WHERE sku = p_sku;
  UPDATE billing_skus SET current_price_version_id = NULL WHERE sku = p_sku;
  DELETE FROM billing_price_versions WHERE sku = p_sku;
  DELETE FROM billing_skus WHERE sku = p_sku;
  RETURN p_sku;
END;
$_$;

CREATE OR REPLACE FUNCTION set_billing_item_availability(p_sku text, p_enabled boolean, p_actor_id bigint) RETURNS TABLE(sku text, enabled boolean, updated_at timestamp with time zone)
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
    action_type, sku, price_version_id, actor_id, metadata
  ) VALUES (
    'set_availability', p_sku, v_price_version_id, p_actor_id,
    jsonb_build_object(
      'old', CASE WHEN v_old_enabled THEN 'active' ELSE 'disabled' END,
      'new', CASE WHEN p_enabled THEN 'active' ELSE 'disabled' END,
      'outcome', v_outcome
    )
  );

  RETURN QUERY SELECT p_sku, p_enabled, v_updated_at;
END;
$_$;

CREATE OR REPLACE FUNCTION publish_billing_price(p_sku text, p_display_name text, p_description text, p_unit_credits bigint, p_actor_id bigint) RETURNS TABLE(price_version_id bigint, version bigint, unit_credits bigint, created_by bigint, created_at timestamp with time zone)
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
    action_type, sku, price_version_id, actor_id, metadata
  ) VALUES (
    'create_price', p_sku, v_price_version_id, p_actor_id,
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

CREATE OR REPLACE FUNCTION publish_billing_prices(
  p_sku text,
  p_display_name text,
  p_description text,
  p_prices jsonb,
  p_actor_id bigint
)
RETURNS SETOF billing_price_versions
LANGUAGE plpgsql SECURITY DEFINER
SET search_path FROM CURRENT
AS $fn$
DECLARE
  v_resolution TEXT;
  v_value JSONB;
  v_unit BIGINT;
  v_old_id BIGINT;
  v_old_version BIGINT;
  v_old_unit BIGINT;
  v_version BIGINT;
  v_new_id BIGINT;
  v_materialized BOOLEAN;
  v_inserted BIGINT;
  v_result billing_price_versions%ROWTYPE;
BEGIN
  p_sku := lower(btrim(p_sku));
  p_display_name := btrim(p_display_name);
  p_description := btrim(COALESCE(p_description, ''));
  IF p_sku IS NULL OR p_sku !~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
     OR char_length(p_sku) > 128 OR p_display_name IS NULL
     OR char_length(p_display_name) NOT BETWEEN 1 AND 120
     OR char_length(p_description) > 1000 OR p_prices IS NULL
     OR jsonb_typeof(p_prices) <> 'object' OR p_actor_id IS NULL OR p_actor_id <= 0
     OR NOT (p_prices ? '1k') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing prices publish command';
  END IF;

  IF EXISTS (
    SELECT 1 FROM jsonb_object_keys(p_prices) AS keys(key)
    WHERE key NOT IN ('1k', '2k', '4k')
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing prices resolution';
  END IF;

  INSERT INTO billing_skus (sku, display_name, description, enabled, created_by, updated_by)
  VALUES (p_sku, p_display_name, p_description, TRUE, p_actor_id, p_actor_id)
  ON CONFLICT (sku) DO NOTHING;
  GET DIAGNOSTICS v_inserted = ROW_COUNT;
  v_materialized := v_inserted > 0;

  PERFORM 1 FROM billing_skus WHERE sku = p_sku FOR UPDATE;
  UPDATE billing_skus
  SET display_name = p_display_name, description = p_description,
      enabled = CASE WHEN NOT EXISTS (
        SELECT 1 FROM billing_current_prices AS current_prices
        WHERE current_prices.sku = p_sku
          AND current_prices.output_resolution IN ('1k')
      ) THEN TRUE ELSE enabled END,
      updated_by = p_actor_id, updated_at = clock_timestamp()
  WHERE sku = p_sku;

  FOREACH v_resolution IN ARRAY ARRAY['1k', '2k', '4k'] LOOP
    v_value := p_prices -> v_resolution;
    IF v_value IS NULL THEN
      DELETE FROM billing_current_prices
      WHERE sku = p_sku AND billing_current_prices.output_resolution = v_resolution;
      CONTINUE;
    END IF;
    IF jsonb_typeof(v_value) <> 'number' THEN
      RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'resolution price must be an integer';
    END IF;
    v_unit := (v_value #>> '{}')::BIGINT;
    IF v_unit < 0 OR v_unit > 9000000000000000 OR (v_value #>> '{}') !~ '^[0-9]+$' THEN
      RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'resolution price is out of range';
    END IF;

    SELECT current.price_version_id, prices.version, prices.unit_credits
    INTO v_old_id, v_old_version, v_old_unit
    FROM billing_current_prices current
    JOIN billing_price_versions prices
      ON prices.sku = current.sku AND prices.output_resolution = current.output_resolution
     AND prices.price_version_id = current.price_version_id
    WHERE current.sku = p_sku AND current.output_resolution = v_resolution;

    SELECT COALESCE(max(prices.version), 0) + 1 INTO v_version
    FROM billing_price_versions prices
    WHERE prices.sku = p_sku AND prices.output_resolution = v_resolution;
    INSERT INTO billing_price_versions (sku, output_resolution, version, unit_credits, created_by)
    VALUES (p_sku, v_resolution, v_version, v_unit, p_actor_id)
    RETURNING billing_price_versions.* INTO v_result;
    v_new_id := v_result.price_version_id;
    INSERT INTO billing_current_prices (sku, output_resolution, price_version_id)
    VALUES (p_sku, v_resolution, v_new_id)
    ON CONFLICT ON CONSTRAINT billing_current_prices_pkey DO UPDATE
      SET price_version_id = EXCLUDED.price_version_id, updated_at = clock_timestamp();
    IF v_resolution IN ('1k') THEN
      UPDATE billing_skus SET current_price_version_id = v_new_id WHERE sku = p_sku;
    END IF;
    INSERT INTO billing_admin_actions (action_type, sku, price_version_id, actor_id, metadata)
    VALUES ('create_price', p_sku, v_new_id, p_actor_id,
      jsonb_build_object('output_resolution', v_resolution, 'materialized', v_materialized,
        'old', CASE WHEN v_old_id IS NULL THEN NULL ELSE jsonb_build_object('price_version_id', v_old_id, 'version', v_old_version, 'unit_credits', v_old_unit) END,
        'new', jsonb_build_object('price_version_id', v_new_id, 'version', v_version, 'unit_credits', v_unit), 'outcome', 'published'));
    RETURN NEXT v_result;
  END LOOP;
END;
$fn$;

DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format('ALTER FUNCTION %I.delete_archived_billing_item(TEXT, BIGINT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.publish_billing_price(TEXT, TEXT, TEXT, BIGINT, BIGINT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.set_billing_item_availability(TEXT, BOOLEAN, BIGINT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.publish_billing_prices(TEXT, TEXT, TEXT, JSONB, BIGINT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
END;
$$;

-- Modify "credit_transactions" table
ALTER TABLE "credit_transactions" DROP CONSTRAINT "ck_credit_transactions_reason", ADD CONSTRAINT "ck_credit_transactions_reason" CHECK (((actor_type)::text = 'admin'::text) OR (btrim(reason) <> ''::text));
-- Modify "credit_wallet_restrictions" table
ALTER TABLE "credit_wallet_restrictions" DROP CONSTRAINT "ck_credit_wallet_restrictions_reason", ADD CONSTRAINT "ck_credit_wallet_restrictions_reason" CHECK (((created_by_actor_type)::text = 'admin'::text) OR ((length(btrim(reason)) >= 1) AND (length(btrim(reason)) <= 1000))), DROP CONSTRAINT "ck_credit_wallet_restrictions_release", ADD CONSTRAINT "ck_credit_wallet_restrictions_release" CHECK (((released_at IS NULL) AND (released_by_actor_type IS NULL) AND (released_by_actor_id IS NULL) AND (release_reason IS NULL)) OR ((released_at IS NOT NULL) AND (released_at >= created_at) AND ((released_by_actor_type)::text = ANY ((ARRAY['admin'::character varying, 'system'::character varying, 'service'::character varying])::text[])) AND (btrim((released_by_actor_id)::text) <> ''::text) AND (((released_by_actor_type)::text = 'admin'::text) OR ((length(btrim(release_reason)) >= 1) AND (length(btrim(release_reason)) <= 1000)))));

ALTER TABLE credit_transactions DISABLE TRIGGER prevent_credit_transactions_mutation;
WITH transaction_reasons AS (
  SELECT id, ''::TEXT AS reason
  FROM credit_transactions
  WHERE actor_type = 'admin' AND reason <> ''
  UNION ALL
  SELECT transactions.id, 'Stripe refund hold'
  FROM credit_transactions transactions
  JOIN payment_refunds refunds ON transactions.business_reference_id = refunds.id::TEXT
  WHERE transactions.actor_type = 'service'
    AND transactions.actor_id = 'payments'
    AND transactions.source = 'stripe_refund_hold'
    AND transactions.business_reference_type = 'payment_refund'
    AND transactions.reason <> 'Stripe refund hold'
    AND refunds.actor_id IS NOT NULL
)
UPDATE credit_transactions transactions
SET reason = transaction_reasons.reason,
    request_hash = encode(sha256(convert_to(jsonb_build_object(
      'command_type', transactions.command_type, 'user_id', transactions.wallet_user_id,
      'amount', transactions.command_amount,
      'related_transaction_id', COALESCE(transactions.related_transaction_id, ''),
      'actor', jsonb_build_object('type', transactions.actor_type, 'id', transactions.actor_id),
      'source', transactions.source, 'reason', transaction_reasons.reason,
      'business_reference', jsonb_build_object('type', transactions.business_reference_type, 'id', transactions.business_reference_id)
    )::TEXT, 'UTF8')), 'hex')
FROM transaction_reasons
WHERE transactions.id = transaction_reasons.id;
ALTER TABLE credit_transactions ENABLE TRIGGER prevent_credit_transactions_mutation;
UPDATE credit_wallet_restrictions SET reason = '' WHERE created_by_actor_type = 'admin' AND reason <> '';
UPDATE credit_wallet_restrictions SET release_reason = '' WHERE released_by_actor_type = 'admin' AND release_reason <> '';
ALTER TABLE payment_refunds DISABLE TRIGGER set_payment_refunds_updated_at;
UPDATE payment_refunds SET reason = '' WHERE actor_id IS NOT NULL AND reason <> '';
ALTER TABLE payment_refunds ENABLE TRIGGER set_payment_refunds_updated_at;

CREATE OR REPLACE FUNCTION create_credit_wallet_restriction(
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
  IF p_actor_type = 'admin' THEN
    p_reason := '';
  END IF;

  IF p_user_id <= 0
     OR p_restriction_type = '' OR length(p_restriction_type) > 64
     OR p_source = '' OR length(p_source) > 64
     OR p_related_object_type = '' OR length(p_related_object_type) > 64
     OR p_related_object_id = '' OR length(p_related_object_id) > 160
     OR (p_actor_type <> 'admin' AND p_reason = '') OR length(p_reason) > 1000
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

CREATE OR REPLACE FUNCTION release_credit_wallet_restriction(
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
  IF p_actor_type = 'admin' THEN
    p_reason := '';
  END IF;
  p_reason := btrim(p_reason);

  IF p_restriction_id !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
     OR p_actor_type NOT IN ('admin', 'system', 'service')
     OR p_actor_id = '' OR length(p_actor_id) > 128
     OR (p_actor_type <> 'admin' AND p_reason = '') OR length(p_reason) > 1000 THEN
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

CREATE OR REPLACE FUNCTION execute_credit_command_v2(
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
  v_hold_amount BIGINT; v_captured_amount BIGINT; v_released_amount BIGINT;
  v_capture_transaction_id VARCHAR(64); v_release_transaction_id VARCHAR(64);
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
  IF p_actor_type = 'admin' THEN
    p_reason := '';
  END IF;
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_user_id <= 0 OR p_command_type NOT IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction')
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160 OR p_actor_type NOT IN ('admin', 'system', 'service', 'user')
     OR p_actor_id = '' OR length(p_actor_id) > 128 OR p_source = '' OR length(p_source) > 64 OR (p_actor_type <> 'admin' AND p_reason = '')
     OR p_business_reference_type = '' OR length(p_business_reference_type) > 64
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160
     OR (p_command_type IN ('deduction', 'correction') AND p_actor_type <> 'admin')
     OR (p_command_type IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction') AND (p_amount <= 0 OR p_amount > 9000000000000000))
     OR (p_command_type IN ('capture', 'release') AND p_related_transaction_id = '')
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
    SELECT amount, status, captured_amount, released_amount, capture_transaction_id, release_transaction_id
      INTO v_hold_amount, v_hold_status, v_captured_amount, v_released_amount, v_capture_transaction_id, v_release_transaction_id
      FROM credit_holds holds
      WHERE holds.hold_transaction_id = p_related_transaction_id AND holds.wallet_user_id = p_user_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB005', MESSAGE = 'credit hold not found'; END IF;
    IF v_hold_status <> 'held' THEN RAISE EXCEPTION USING ERRCODE = 'IB006', MESSAGE = 'credit hold already settled'; END IF;
    IF (p_command_type = 'capture' AND v_capture_transaction_id IS NOT NULL)
       OR (p_command_type = 'release' AND v_release_transaction_id IS NOT NULL) THEN
      RAISE EXCEPTION USING ERRCODE = 'IB006', MESSAGE = 'credit hold settlement command already recorded';
    END IF;
    IF v_amount > v_hold_amount - v_captured_amount - v_released_amount THEN
      RAISE EXCEPTION USING ERRCODE = 'IB013', MESSAGE = 'credit hold settlement exceeds remaining amount';
    END IF;
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
    WHEN 'capture' THEN v_from_id := v_reserved_id; v_to_id := v_revenue_id;
    WHEN 'release' THEN v_from_id := v_reserved_id; v_to_id := v_available_id;
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
    IF p_command_type = 'capture' THEN
      v_captured_amount := v_captured_amount + v_amount;
      v_capture_transaction_id := v_transaction_id;
    ELSE
      v_released_amount := v_released_amount + v_amount;
      v_release_transaction_id := v_transaction_id;
    END IF;
    IF v_captured_amount + v_released_amount = v_hold_amount THEN
      v_terminal_status := CASE WHEN v_captured_amount > 0 THEN 'captured' ELSE 'released' END;
    ELSE
      v_terminal_status := 'held';
    END IF;
    UPDATE credit_holds
    SET captured_amount = v_captured_amount,
        released_amount = v_released_amount,
        status = v_terminal_status,
        capture_transaction_id = v_capture_transaction_id,
        release_transaction_id = v_release_transaction_id,
        settled_at = CASE WHEN v_terminal_status = 'held' THEN NULL ELSE now() END
    WHERE hold_transaction_id = p_related_transaction_id;
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

DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format('ALTER FUNCTION %I.create_credit_wallet_restriction(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.release_credit_wallet_restriction(TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.execute_credit_command_v2(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
END;
$$;

DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = trusted_schema
      AND table_name = 'user'
      AND column_name = 'banReason'
  ) THEN
    EXECUTE format('UPDATE %I."user" SET "banReason" = NULL WHERE "banReason" IS NOT NULL', trusted_schema);
  END IF;
END;
$$;

UPDATE identity_admin_reconciliation_findings
SET details = details - 'auth_ban_reason'
WHERE details ? 'auth_ban_reason';
UPDATE identity_admin_reconciliation_run_findings
SET observed_details = observed_details - 'auth_ban_reason'
WHERE observed_details ? 'auth_ban_reason';
