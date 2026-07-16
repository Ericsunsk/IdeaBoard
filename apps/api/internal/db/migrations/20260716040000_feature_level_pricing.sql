-- migrate:up
DROP FUNCTION IF EXISTS set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT);
DROP FUNCTION IF EXISTS create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT);

DROP TRIGGER IF EXISTS billing_price_states_no_delete ON billing_price_version_states;
DROP TABLE billing_price_version_states;
DROP FUNCTION IF EXISTS prevent_billing_price_state_delete();

-- statement:begin
CREATE FUNCTION publish_billing_price(
  p_sku TEXT,
  p_display_name TEXT,
  p_description TEXT,
  p_unit_credits BIGINT,
  p_actor_id BIGINT
)
RETURNS TABLE (
  price_version_id BIGINT,
  version BIGINT,
  unit_credits BIGINT,
  created_by BIGINT,
  created_at TIMESTAMPTZ(3)
) AS $$
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
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.publish_billing_price(TEXT, TEXT, TEXT, BIGINT, BIGINT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END $$;
-- statement:end

REVOKE ALL ON FUNCTION publish_billing_price(TEXT, TEXT, TEXT, BIGINT, BIGINT) FROM PUBLIC;

-- migrate:down
-- Schema rollback only: Price Version enabled state no longer exists after Up.
-- Restore data from a pre-migration database snapshot when exact legacy states are required.
DROP FUNCTION IF EXISTS publish_billing_price(TEXT, TEXT, TEXT, BIGINT, BIGINT);

CREATE TABLE billing_price_version_states (
  price_version_id BIGINT PRIMARY KEY REFERENCES billing_price_versions(price_version_id) ON DELETE RESTRICT,
  enabled BOOLEAN NOT NULL,
  updated_by BIGINT NOT NULL CHECK (updated_by > 0),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp()
);

INSERT INTO billing_price_version_states (price_version_id, enabled, updated_by, updated_at)
SELECT price_version_id, TRUE, created_by, created_at FROM billing_price_versions;

-- statement:begin
CREATE FUNCTION prevent_billing_price_state_delete()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB105', MESSAGE = 'billing price state cannot be deleted';
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER billing_price_states_no_delete
BEFORE DELETE ON billing_price_version_states
FOR EACH ROW EXECUTE FUNCTION prevent_billing_price_state_delete();

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
  IF p_version IS NULL OR p_version <= 0 OR p_enabled IS NULL
     OR p_actor_id IS NULL OR p_actor_id <= 0
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
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
  function_signature TEXT;
BEGIN
  FOREACH function_signature IN ARRAY ARRAY[
    'create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT)',
    'set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT)',
    'prevent_billing_price_state_delete()'
  ] LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      trusted_schema, function_signature, trusted_schema
    );
  END LOOP;
END $$;
-- statement:end

REVOKE ALL ON FUNCTION create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION prevent_billing_price_state_delete() FROM PUBLIC;
