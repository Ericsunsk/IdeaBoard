-- migrate:up
ALTER TABLE feature_prompt_revisions
  DROP COLUMN enabled,
  DROP COLUMN published_at;

ALTER TABLE billing_admin_actions
  DROP CONSTRAINT ck_billing_admin_actions_type,
  DROP CONSTRAINT ck_billing_admin_actions_price_scope;

ALTER TABLE billing_admin_actions
  ADD CONSTRAINT ck_billing_admin_actions_type CHECK (
    action_type IN ('create_sku', 'update_sku', 'create_price', 'set_price_enabled', 'set_availability')
  ),
  ADD CONSTRAINT ck_billing_admin_actions_price_scope CHECK (
    (action_type IN ('create_sku', 'update_sku') AND price_version_id IS NULL)
    OR (action_type IN ('create_price', 'set_price_enabled', 'set_availability') AND price_version_id IS NOT NULL)
  );

-- statement:begin
CREATE FUNCTION set_billing_item_availability(
  p_sku TEXT,
  p_enabled BOOLEAN,
  p_actor_id BIGINT
)
RETURNS TABLE (
  sku TEXT,
  enabled BOOLEAN,
  updated_at TIMESTAMPTZ(3)
) AS $$
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
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.set_billing_item_availability(TEXT, BOOLEAN, BIGINT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END $$;
-- statement:end

REVOKE ALL ON FUNCTION set_billing_item_availability(TEXT, BOOLEAN, BIGINT) FROM PUBLIC;

-- migrate:down
-- Schema rollback only: append-only availability audit facts are retained.
-- Restore from a pre-migration database snapshot when exact legacy state is required.
DROP FUNCTION IF EXISTS set_billing_item_availability(TEXT, BOOLEAN, BIGINT);

ALTER TABLE feature_prompt_revisions
  ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE,
  ADD COLUMN published_at TIMESTAMPTZ(3);
ALTER TABLE feature_prompt_revisions ALTER COLUMN enabled DROP DEFAULT;
UPDATE feature_prompt_revisions SET published_at = created_at;
