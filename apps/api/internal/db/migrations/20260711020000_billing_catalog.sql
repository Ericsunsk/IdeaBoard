-- migrate:up
CREATE TABLE billing_skus (
  sku VARCHAR(128) PRIMARY KEY,
  display_name VARCHAR(120) NOT NULL,
  description VARCHAR(1000) NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  current_price_version_id BIGINT,
  created_by BIGINT NOT NULL CHECK (created_by > 0),
  updated_by BIGINT NOT NULL CHECK (updated_by > 0),
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT billing_skus_canonical_check CHECK (
    sku = lower(btrim(sku))
    AND char_length(sku) BETWEEN 3 AND 128
    AND sku ~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
  ),
  CONSTRAINT billing_skus_display_name_check CHECK (char_length(btrim(display_name)) BETWEEN 1 AND 120)
);

CREATE TABLE billing_price_versions (
  price_version_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  sku VARCHAR(128) NOT NULL REFERENCES billing_skus(sku) ON UPDATE RESTRICT ON DELETE RESTRICT,
  version BIGINT NOT NULL CHECK (version > 0),
  unit_credits BIGINT NOT NULL CHECK (unit_credits BETWEEN 0 AND 9000000000000000),
  created_by BIGINT NOT NULL CHECK (created_by > 0),
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT billing_price_versions_sku_version_key UNIQUE (sku, version),
  CONSTRAINT billing_price_versions_sku_id_key UNIQUE (sku, price_version_id)
);

CREATE TABLE billing_price_version_states (
  price_version_id BIGINT PRIMARY KEY REFERENCES billing_price_versions(price_version_id) ON DELETE RESTRICT,
  enabled BOOLEAN NOT NULL,
  updated_by BIGINT NOT NULL CHECK (updated_by > 0),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp()
);

ALTER TABLE billing_skus
  ADD CONSTRAINT billing_skus_current_price_fk
  FOREIGN KEY (sku, current_price_version_id)
  REFERENCES billing_price_versions(sku, price_version_id)
  ON UPDATE RESTRICT ON DELETE RESTRICT;

-- statement:begin
CREATE OR REPLACE FUNCTION prevent_billing_sku_identity_mutation()
RETURNS TRIGGER AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB104', MESSAGE = 'billing SKU cannot be deleted or reused';
  END IF;
  IF NEW.sku <> OLD.sku THEN
    RAISE EXCEPTION USING ERRCODE = 'IB104', MESSAGE = 'billing SKU cannot be renamed';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER billing_skus_immutable_identity
BEFORE UPDATE OR DELETE ON billing_skus
FOR EACH ROW EXECUTE FUNCTION prevent_billing_sku_identity_mutation();

-- statement:begin
CREATE OR REPLACE FUNCTION prevent_billing_price_version_mutation()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB105', MESSAGE = 'billing price versions are immutable';
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER billing_price_versions_append_only
BEFORE UPDATE OR DELETE ON billing_price_versions
FOR EACH ROW EXECUTE FUNCTION prevent_billing_price_version_mutation();

-- statement:begin
CREATE OR REPLACE FUNCTION prevent_billing_price_state_delete()
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
    'prevent_billing_sku_identity_mutation()',
    'prevent_billing_price_version_mutation()',
    'prevent_billing_price_state_delete()'
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
REVOKE ALL ON FUNCTION prevent_billing_sku_identity_mutation() FROM PUBLIC;
REVOKE ALL ON FUNCTION prevent_billing_price_version_mutation() FROM PUBLIC;
REVOKE ALL ON FUNCTION prevent_billing_price_state_delete() FROM PUBLIC;

-- migrate:down
DROP FUNCTION IF EXISTS set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT);
DROP FUNCTION IF EXISTS create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT);
DROP FUNCTION IF EXISTS update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT);
DROP FUNCTION IF EXISTS create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT);
DROP TABLE IF EXISTS billing_price_version_states;
ALTER TABLE billing_skus DROP CONSTRAINT IF EXISTS billing_skus_current_price_fk;
DROP TABLE IF EXISTS billing_price_versions;
DROP TABLE IF EXISTS billing_skus;
DROP FUNCTION IF EXISTS prevent_billing_price_state_delete();
DROP FUNCTION IF EXISTS prevent_billing_price_version_mutation();
DROP FUNCTION IF EXISTS prevent_billing_sku_identity_mutation();
