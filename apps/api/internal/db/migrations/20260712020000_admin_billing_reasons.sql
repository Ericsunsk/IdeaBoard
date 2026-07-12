-- migrate:up
CREATE TABLE billing_admin_actions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  action_type VARCHAR(32) NOT NULL,
  sku VARCHAR(128) NOT NULL REFERENCES billing_skus(sku) ON UPDATE RESTRICT ON DELETE RESTRICT,
  price_version_id BIGINT,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  reason VARCHAR(1000) NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT ck_billing_admin_actions_type CHECK (
    action_type IN ('create_sku', 'update_sku', 'create_price', 'set_price_enabled')
  ),
  CONSTRAINT ck_billing_admin_actions_reason CHECK (
    reason = btrim(reason, E' \t\n\r\f') AND char_length(reason) BETWEEN 1 AND 1000
  ),
  CONSTRAINT ck_billing_admin_actions_price_scope CHECK (
    (action_type IN ('create_sku', 'update_sku') AND price_version_id IS NULL)
    OR (action_type IN ('create_price', 'set_price_enabled') AND price_version_id IS NOT NULL)
  ),
  CONSTRAINT ck_billing_admin_actions_metadata CHECK (jsonb_typeof(metadata) = 'object'),
  CONSTRAINT fk_billing_admin_actions_price
    FOREIGN KEY (sku, price_version_id)
    REFERENCES billing_price_versions(sku, price_version_id)
    ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE INDEX idx_billing_admin_actions_sku_id
  ON billing_admin_actions (sku, id DESC);
CREATE INDEX idx_billing_admin_actions_actor_id
  ON billing_admin_actions (actor_id, id DESC);

-- statement:begin
CREATE OR REPLACE FUNCTION prevent_billing_admin_action_mutation()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB107', MESSAGE = 'billing admin actions are append-only';
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER billing_admin_actions_append_only
BEFORE UPDATE OR DELETE ON billing_admin_actions
FOR EACH ROW EXECUTE FUNCTION prevent_billing_admin_action_mutation();

ALTER TABLE credit_reconciliation_runs
  ADD COLUMN reason VARCHAR(1000) NOT NULL DEFAULT 'pre-reason reconciliation request';
ALTER TABLE credit_reconciliation_runs ALTER COLUMN reason DROP DEFAULT;
ALTER TABLE credit_reconciliation_runs
  ADD CONSTRAINT ck_credit_reconciliation_run_reason CHECK (
    reason = btrim(reason, E' \t\n\r\f') AND char_length(reason) BETWEEN 1 AND 1000
  );

DROP FUNCTION create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT);
DROP FUNCTION update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT);
DROP FUNCTION create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT);
DROP FUNCTION set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT);
DROP FUNCTION start_credit_reconciliation(BIGINT, BIGINT);

-- statement:begin
CREATE FUNCTION create_billing_sku(
  p_sku TEXT, p_display_name TEXT, p_description TEXT, p_enabled BOOLEAN, p_actor_id BIGINT, p_reason TEXT
)
RETURNS VOID AS $$
BEGIN
  p_sku := lower(btrim(p_sku));
  p_display_name := btrim(p_display_name);
  p_description := btrim(COALESCE(p_description, ''));
  p_reason := btrim(p_reason, E' \t\n\r\f');
  IF p_sku !~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
     OR char_length(p_sku) > 128
     OR char_length(p_display_name) NOT BETWEEN 1 AND 120
     OR char_length(p_description) > 1000
     OR p_enabled IS NULL OR p_actor_id IS NULL OR p_actor_id <= 0
     OR p_reason IS NULL OR char_length(p_reason) NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing SKU command';
  END IF;
  INSERT INTO billing_skus (
    sku, display_name, description, enabled, created_by, updated_by
  ) VALUES (
    p_sku, p_display_name, p_description, p_enabled, p_actor_id, p_actor_id
  );
  INSERT INTO billing_admin_actions (action_type, sku, actor_id, reason, metadata)
  VALUES (
    'create_sku', p_sku, p_actor_id, p_reason,
    jsonb_build_object('display_name', p_display_name, 'description', p_description, 'enabled', p_enabled)
  );
EXCEPTION WHEN unique_violation THEN
  RAISE EXCEPTION USING ERRCODE = 'IB102', MESSAGE = 'billing SKU already exists';
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE FUNCTION update_billing_sku(
  p_sku TEXT, p_display_name TEXT, p_description TEXT, p_enabled BOOLEAN, p_actor_id BIGINT, p_reason TEXT
)
RETURNS VOID AS $$
BEGIN
  p_sku := lower(btrim(p_sku));
  p_reason := btrim(p_reason, E' \t\n\r\f');
  IF p_actor_id IS NULL OR p_actor_id <= 0
     OR (p_display_name IS NULL AND p_description IS NULL AND p_enabled IS NULL)
     OR (p_display_name IS NOT NULL AND char_length(btrim(p_display_name)) NOT BETWEEN 1 AND 120)
     OR (p_description IS NOT NULL AND char_length(btrim(p_description)) > 1000)
     OR p_reason IS NULL OR char_length(p_reason) NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing SKU update';
  END IF;
  UPDATE billing_skus
  SET display_name = COALESCE(btrim(p_display_name), display_name),
      description = COALESCE(btrim(p_description), description),
      enabled = COALESCE(p_enabled, enabled),
      updated_by = p_actor_id,
      updated_at = clock_timestamp()
  WHERE sku = p_sku;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB103', MESSAGE = 'billing SKU not found';
  END IF;
  INSERT INTO billing_admin_actions (action_type, sku, actor_id, reason, metadata)
  VALUES (
    'update_sku', p_sku, p_actor_id, p_reason,
    jsonb_strip_nulls(jsonb_build_object(
      'display_name', CASE WHEN p_display_name IS NULL THEN NULL ELSE btrim(p_display_name) END,
      'description', CASE WHEN p_description IS NULL THEN NULL ELSE btrim(p_description) END,
      'enabled', p_enabled
    ))
  );
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE FUNCTION create_billing_price_version(
  p_sku TEXT, p_unit_credits BIGINT, p_enabled BOOLEAN, p_actor_id BIGINT, p_reason TEXT
)
RETURNS TABLE (
  price_version_id BIGINT, version BIGINT, unit_credits BIGINT, enabled BOOLEAN,
  created_by BIGINT, created_at TIMESTAMPTZ(3), updated_by BIGINT, updated_at TIMESTAMPTZ(3)
) AS $$
DECLARE
  v_version BIGINT;
  v_price_version_id BIGINT;
BEGIN
  p_sku := lower(btrim(p_sku));
  p_reason := btrim(p_reason, E' \t\n\r\f');
  IF p_unit_credits < 0 OR p_unit_credits > 9000000000000000
     OR p_enabled IS NULL OR p_actor_id IS NULL OR p_actor_id <= 0
     OR p_reason IS NULL OR char_length(p_reason) NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing price command';
  END IF;
  PERFORM 1 FROM billing_skus WHERE sku = p_sku FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB103', MESSAGE = 'billing SKU not found';
  END IF;
  SELECT COALESCE(max(prices.version), 0) + 1
  INTO v_version
  FROM billing_price_versions prices
  WHERE prices.sku = p_sku;

  INSERT INTO billing_price_versions (sku, version, unit_credits, created_by)
  VALUES (p_sku, v_version, p_unit_credits, p_actor_id)
  RETURNING billing_price_versions.price_version_id INTO v_price_version_id;

  INSERT INTO billing_price_version_states (price_version_id, enabled, updated_by)
  VALUES (v_price_version_id, p_enabled, p_actor_id);

  UPDATE billing_skus
  SET current_price_version_id = v_price_version_id,
      updated_by = p_actor_id,
      updated_at = clock_timestamp()
  WHERE sku = p_sku;

  INSERT INTO billing_admin_actions (action_type, sku, price_version_id, actor_id, reason, metadata)
  VALUES (
    'create_price', p_sku, v_price_version_id, p_actor_id, p_reason,
    jsonb_build_object('version', v_version, 'unit_credits', p_unit_credits, 'enabled', p_enabled)
  );

  RETURN QUERY
  SELECT prices.price_version_id, prices.version, prices.unit_credits, states.enabled,
         prices.created_by, prices.created_at, states.updated_by, states.updated_at
  FROM billing_price_versions prices
  JOIN billing_price_version_states states USING (price_version_id)
  WHERE prices.price_version_id = v_price_version_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE FUNCTION set_billing_price_version_enabled(
  p_sku TEXT, p_version BIGINT, p_enabled BOOLEAN, p_actor_id BIGINT, p_reason TEXT
)
RETURNS TABLE (
  price_version_id BIGINT, version BIGINT, unit_credits BIGINT, enabled BOOLEAN,
  created_by BIGINT, created_at TIMESTAMPTZ(3), updated_by BIGINT, updated_at TIMESTAMPTZ(3)
) AS $$
DECLARE
  v_price_version_id BIGINT;
BEGIN
  p_sku := lower(btrim(p_sku));
  p_reason := btrim(p_reason, E' \t\n\r\f');
  IF p_version IS NULL OR p_version <= 0 OR p_enabled IS NULL OR p_actor_id IS NULL OR p_actor_id <= 0
     OR p_reason IS NULL OR char_length(p_reason) NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing price state command';
  END IF;
  SELECT prices.price_version_id INTO v_price_version_id
  FROM billing_price_versions prices
  WHERE prices.sku = p_sku AND prices.version = p_version;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB106', MESSAGE = 'billing price version not found';
  END IF;

  UPDATE billing_price_version_states states
  SET enabled = p_enabled, updated_by = p_actor_id, updated_at = clock_timestamp()
  WHERE states.price_version_id = v_price_version_id;

  INSERT INTO billing_admin_actions (action_type, sku, price_version_id, actor_id, reason, metadata)
  VALUES (
    'set_price_enabled', p_sku, v_price_version_id, p_actor_id, p_reason,
    jsonb_build_object('version', p_version, 'enabled', p_enabled)
  );

  RETURN QUERY
  SELECT prices.price_version_id, prices.version, prices.unit_credits, states.enabled,
         prices.created_by, prices.created_at, states.updated_by, states.updated_at
  FROM billing_price_versions prices
  JOIN billing_price_version_states states USING (price_version_id)
  WHERE prices.price_version_id = v_price_version_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE FUNCTION start_credit_reconciliation(
  p_requested_by BIGINT, p_stale_hold_seconds BIGINT, p_reason TEXT
)
RETURNS BIGINT AS $$
DECLARE v_run_id BIGINT;
BEGIN
  p_reason := btrim(p_reason, E' \t\n\r\f');
  IF p_requested_by IS NULL OR p_requested_by <= 0
     OR p_stale_hold_seconds IS NULL OR p_stale_hold_seconds < 60 OR p_stale_hold_seconds > 31536000
     OR p_reason IS NULL OR char_length(p_reason) NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB301', MESSAGE = 'invalid credit reconciliation request';
  END IF;
  PERFORM pg_advisory_xact_lock(5283921922134398277);
  SELECT id INTO v_run_id FROM credit_reconciliation_runs
   WHERE status IN ('queued', 'running') ORDER BY id DESC LIMIT 1;
  IF FOUND THEN RETURN v_run_id; END IF;
  INSERT INTO credit_reconciliation_runs (requested_by, stale_hold_after, reason)
  VALUES (p_requested_by, make_interval(secs => p_stale_hold_seconds::INTEGER), p_reason)
  RETURNING id INTO v_run_id;
  RETURN v_run_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
  function_signature TEXT;
BEGIN
  FOREACH function_signature IN ARRAY ARRAY[
    'create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT)',
    'update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT)',
    'create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT)',
    'set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT)',
    'start_credit_reconciliation(BIGINT, BIGINT, TEXT)',
    'prevent_billing_admin_action_mutation()'
  ] LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      trusted_schema, function_signature, trusted_schema
    );
  END LOOP;
END $$;
-- statement:end

REVOKE ALL ON FUNCTION create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION start_credit_reconciliation(BIGINT, BIGINT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION prevent_billing_admin_action_mutation() FROM PUBLIC;

-- migrate:down
DROP FUNCTION IF EXISTS start_credit_reconciliation(BIGINT, BIGINT, TEXT);
DROP FUNCTION IF EXISTS set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT);
DROP FUNCTION IF EXISTS create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT);
DROP FUNCTION IF EXISTS update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT);
DROP FUNCTION IF EXISTS create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT);
ALTER TABLE credit_reconciliation_runs DROP CONSTRAINT IF EXISTS ck_credit_reconciliation_run_reason;
ALTER TABLE credit_reconciliation_runs DROP COLUMN IF EXISTS reason;
DROP TABLE IF EXISTS billing_admin_actions;
DROP FUNCTION IF EXISTS prevent_billing_admin_action_mutation();

-- statement:begin
CREATE OR REPLACE FUNCTION create_billing_sku(
  p_sku TEXT, p_display_name TEXT, p_description TEXT, p_enabled BOOLEAN, p_actor_id BIGINT
)
RETURNS VOID AS $$
BEGIN
  p_sku := lower(btrim(p_sku));
  p_display_name := btrim(p_display_name);
  p_description := btrim(COALESCE(p_description, ''));
  IF p_sku !~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
     OR char_length(p_sku) > 128
     OR char_length(p_display_name) NOT BETWEEN 1 AND 120
     OR char_length(p_description) > 1000
     OR p_enabled IS NULL OR p_actor_id <= 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing SKU command';
  END IF;
  INSERT INTO billing_skus (
    sku, display_name, description, enabled, created_by, updated_by
  ) VALUES (
    p_sku, p_display_name, p_description, p_enabled, p_actor_id, p_actor_id
  );
EXCEPTION WHEN unique_violation THEN
  RAISE EXCEPTION USING ERRCODE = 'IB102', MESSAGE = 'billing SKU already exists';
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE OR REPLACE FUNCTION update_billing_sku(
  p_sku TEXT, p_display_name TEXT, p_description TEXT, p_enabled BOOLEAN, p_actor_id BIGINT
)
RETURNS VOID AS $$
BEGIN
  p_sku := lower(btrim(p_sku));
  IF p_actor_id <= 0
     OR (p_display_name IS NOT NULL AND char_length(btrim(p_display_name)) NOT BETWEEN 1 AND 120)
     OR (p_description IS NOT NULL AND char_length(btrim(p_description)) > 1000) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing SKU update';
  END IF;
  UPDATE billing_skus
  SET display_name = COALESCE(btrim(p_display_name), display_name),
      description = COALESCE(btrim(p_description), description),
      enabled = COALESCE(p_enabled, enabled),
      updated_by = p_actor_id,
      updated_at = clock_timestamp()
  WHERE sku = p_sku;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB103', MESSAGE = 'billing SKU not found';
  END IF;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE OR REPLACE FUNCTION create_billing_price_version(
  p_sku TEXT, p_unit_credits BIGINT, p_enabled BOOLEAN, p_actor_id BIGINT
)
RETURNS TABLE (
  price_version_id BIGINT, version BIGINT, unit_credits BIGINT, enabled BOOLEAN,
  created_by BIGINT, created_at TIMESTAMPTZ(3), updated_by BIGINT, updated_at TIMESTAMPTZ(3)
) AS $$
DECLARE
  v_version BIGINT;
  v_price_version_id BIGINT;
BEGIN
  p_sku := lower(btrim(p_sku));
  IF p_unit_credits < 0 OR p_unit_credits > 9000000000000000
     OR p_enabled IS NULL OR p_actor_id <= 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing price command';
  END IF;
  PERFORM 1 FROM billing_skus WHERE sku = p_sku FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB103', MESSAGE = 'billing SKU not found';
  END IF;
  SELECT COALESCE(max(prices.version), 0) + 1
  INTO v_version
  FROM billing_price_versions prices
  WHERE prices.sku = p_sku;

  INSERT INTO billing_price_versions (sku, version, unit_credits, created_by)
  VALUES (p_sku, v_version, p_unit_credits, p_actor_id)
  RETURNING billing_price_versions.price_version_id INTO v_price_version_id;

  INSERT INTO billing_price_version_states (price_version_id, enabled, updated_by)
  VALUES (v_price_version_id, p_enabled, p_actor_id);

  UPDATE billing_skus
  SET current_price_version_id = v_price_version_id,
      updated_by = p_actor_id,
      updated_at = clock_timestamp()
  WHERE sku = p_sku;

  RETURN QUERY
  SELECT prices.price_version_id, prices.version, prices.unit_credits, states.enabled,
         prices.created_by, prices.created_at, states.updated_by, states.updated_at
  FROM billing_price_versions prices
  JOIN billing_price_version_states states USING (price_version_id)
  WHERE prices.price_version_id = v_price_version_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE OR REPLACE FUNCTION set_billing_price_version_enabled(
  p_sku TEXT, p_version BIGINT, p_enabled BOOLEAN, p_actor_id BIGINT
)
RETURNS TABLE (
  price_version_id BIGINT, version BIGINT, unit_credits BIGINT, enabled BOOLEAN,
  created_by BIGINT, created_at TIMESTAMPTZ(3), updated_by BIGINT, updated_at TIMESTAMPTZ(3)
) AS $$
DECLARE
  v_price_version_id BIGINT;
BEGIN
  p_sku := lower(btrim(p_sku));
  IF p_version <= 0 OR p_enabled IS NULL OR p_actor_id <= 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing price state command';
  END IF;
  SELECT prices.price_version_id INTO v_price_version_id
  FROM billing_price_versions prices
  WHERE prices.sku = p_sku AND prices.version = p_version;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB106', MESSAGE = 'billing price version not found';
  END IF;

  UPDATE billing_price_version_states states
  SET enabled = p_enabled, updated_by = p_actor_id, updated_at = clock_timestamp()
  WHERE states.price_version_id = v_price_version_id;

  RETURN QUERY
  SELECT prices.price_version_id, prices.version, prices.unit_credits, states.enabled,
         prices.created_by, prices.created_at, states.updated_by, states.updated_at
  FROM billing_price_versions prices
  JOIN billing_price_version_states states USING (price_version_id)
  WHERE prices.price_version_id = v_price_version_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE OR REPLACE FUNCTION start_credit_reconciliation(p_requested_by BIGINT, p_stale_hold_seconds BIGINT)
RETURNS BIGINT AS $$
DECLARE v_run_id BIGINT;
BEGIN
  IF p_requested_by <= 0 OR p_stale_hold_seconds < 60 OR p_stale_hold_seconds > 31536000 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB301', MESSAGE = 'invalid credit reconciliation request';
  END IF;
  PERFORM pg_advisory_xact_lock(5283921922134398277);
  SELECT id INTO v_run_id FROM credit_reconciliation_runs
   WHERE status IN ('queued', 'running') ORDER BY id DESC LIMIT 1;
  IF FOUND THEN RETURN v_run_id; END IF;
  INSERT INTO credit_reconciliation_runs (requested_by, stale_hold_after)
  VALUES (p_requested_by, make_interval(secs => p_stale_hold_seconds::INTEGER)) RETURNING id INTO v_run_id;
  RETURN v_run_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
  function_signature TEXT;
BEGIN
  FOREACH function_signature IN ARRAY ARRAY[
    'create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT)',
    'update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT)',
    'create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT)',
    'set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT)',
    'start_credit_reconciliation(BIGINT, BIGINT)'
  ] LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      trusted_schema, function_signature, trusted_schema
    );
  END LOOP;
END $$;
-- statement:end

REVOKE ALL ON FUNCTION create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION start_credit_reconciliation(BIGINT, BIGINT) FROM PUBLIC;
