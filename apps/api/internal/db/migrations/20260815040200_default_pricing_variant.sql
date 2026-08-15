-- Add a non-resolution default pricing variant for fixed-price workflows.
ALTER TABLE billing_price_versions
  ALTER COLUMN output_resolution TYPE VARCHAR(8);
ALTER TABLE billing_price_versions
  DROP CONSTRAINT billing_price_versions_output_resolution_check,
  ADD CONSTRAINT billing_price_versions_output_resolution_check
    CHECK (output_resolution IN ('default', '1k', '2k', '4k'));

ALTER TABLE task_billing_snapshots
  ALTER COLUMN output_resolution TYPE VARCHAR(8);
ALTER TABLE task_billing_snapshots
  DROP CONSTRAINT task_billing_snapshots_output_resolution_check,
  ADD CONSTRAINT task_billing_snapshots_output_resolution_check
    CHECK (output_resolution IN ('default', '1k', '2k', '4k'));

ALTER TABLE billing_current_prices
  ALTER COLUMN output_resolution TYPE VARCHAR(8);
ALTER TABLE billing_current_prices
  DROP CONSTRAINT billing_current_prices_resolution_check,
  ADD CONSTRAINT billing_current_prices_resolution_check
    CHECK (output_resolution IN ('default', '1k', '2k', '4k'));

CREATE OR REPLACE FUNCTION publish_billing_prices(
  p_sku text,
  p_display_name text,
  p_description text,
  p_prices jsonb,
  p_actor_id bigint
)
RETURNS TABLE(output_resolution varchar, price_version_id bigint, version bigint,
              unit_credits bigint, created_by bigint, created_at timestamptz)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path TO 'public', 'pg_catalog', 'pg_temp'
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
BEGIN
  p_sku := lower(btrim(p_sku));
  p_display_name := btrim(p_display_name);
  p_description := btrim(COALESCE(p_description, ''));
  IF p_sku IS NULL OR p_sku !~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
     OR char_length(p_sku) > 128 OR p_display_name IS NULL
     OR char_length(p_display_name) NOT BETWEEN 1 AND 120
     OR char_length(p_description) > 1000 OR p_prices IS NULL
     OR jsonb_typeof(p_prices) <> 'object' OR p_actor_id IS NULL OR p_actor_id <= 0
     OR NOT (p_prices ? '1k' OR p_prices ? 'default') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing prices publish command';
  END IF;

  IF EXISTS (
    SELECT 1 FROM jsonb_object_keys(p_prices) AS keys(key)
    WHERE key NOT IN ('default', '1k', '2k', '4k')
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
        SELECT 1 FROM billing_current_prices
        WHERE sku = p_sku AND output_resolution IN ('default', '1k')
      ) THEN TRUE ELSE enabled END,
      updated_by = p_actor_id, updated_at = clock_timestamp()
  WHERE sku = p_sku;

  FOREACH v_resolution IN ARRAY ARRAY['default', '1k', '2k', '4k'] LOOP
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
    RETURNING billing_price_versions.price_version_id INTO v_new_id;
    INSERT INTO billing_current_prices (sku, output_resolution, price_version_id)
    VALUES (p_sku, v_resolution, v_new_id)
    ON CONFLICT (sku, output_resolution) DO UPDATE
      SET price_version_id = EXCLUDED.price_version_id, updated_at = clock_timestamp();
    IF v_resolution IN ('default', '1k') THEN
      UPDATE billing_skus SET current_price_version_id = v_new_id WHERE sku = p_sku;
    END IF;
    INSERT INTO billing_admin_actions (action_type, sku, price_version_id, actor_id, reason, metadata)
    VALUES ('create_price', p_sku, v_new_id, p_actor_id, 'price published',
      jsonb_build_object('output_resolution', v_resolution, 'materialized', v_materialized,
        'old', CASE WHEN v_old_id IS NULL THEN NULL ELSE jsonb_build_object('price_version_id', v_old_id, 'version', v_old_version, 'unit_credits', v_old_unit) END,
        'new', jsonb_build_object('price_version_id', v_new_id, 'version', v_version, 'unit_credits', v_unit), 'outcome', 'published'));
    output_resolution := v_resolution;
    price_version_id := v_new_id;
    version := v_version;
    unit_credits := v_unit;
    created_by := p_actor_id;
    created_at := clock_timestamp();
    RETURN NEXT;
  END LOOP;
END;
$fn$;

REVOKE ALL ON FUNCTION publish_billing_prices(TEXT, TEXT, TEXT, JSONB, BIGINT) FROM PUBLIC;
