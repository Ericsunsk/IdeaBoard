-- migrate:up
-- Billable Item Registry + Catalog commands are now the only Admin write seam.
-- Historical audit action values remain valid append-only facts; this migration
-- removes command functions without rewriting rows or tightening their CHECK.
DROP FUNCTION IF EXISTS create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT);
DROP FUNCTION IF EXISTS update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT);

-- migrate:down
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
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
  function_signature TEXT;
BEGIN
  FOREACH function_signature IN ARRAY ARRAY[
    'create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT)',
    'update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT)'
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
