-- migrate:up
-- Admin audit is a self-contained target snapshot and survives Catalog cleanup.
ALTER TABLE billing_admin_actions
  DROP CONSTRAINT billing_admin_actions_sku_fkey,
  DROP CONSTRAINT fk_billing_admin_actions_price;

ALTER TABLE billing_admin_actions
  DROP CONSTRAINT ck_billing_admin_actions_type,
  DROP CONSTRAINT ck_billing_admin_actions_price_scope;

ALTER TABLE billing_admin_actions
  ADD CONSTRAINT ck_billing_admin_actions_type CHECK (
    action_type IN ('create_sku', 'update_sku', 'create_price', 'set_price_enabled', 'set_availability', 'delete_archived')
  ),
  ADD CONSTRAINT ck_billing_admin_actions_price_scope CHECK (
    (action_type IN ('create_sku', 'update_sku', 'delete_archived') AND price_version_id IS NULL)
    OR (action_type IN ('create_price', 'set_price_enabled', 'set_availability') AND price_version_id IS NOT NULL)
  );

-- Catalog aggregate deletion is allowed only while the typed command holds a
-- transaction-local guard for the exact SKU. Runtime roles retain no direct
-- DELETE privileges on these tables.
-- statement:begin
CREATE OR REPLACE FUNCTION prevent_billing_sku_identity_mutation()
RETURNS TRIGGER AS $$
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
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE OR REPLACE FUNCTION prevent_billing_price_version_mutation()
RETURNS TRIGGER AS $$
BEGIN
  IF TG_OP = 'DELETE'
     AND current_setting('ideaboard.billing_archive_delete', TRUE) = OLD.sku THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION USING ERRCODE = 'IB105', MESSAGE = 'billing price versions are immutable';
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE FUNCTION delete_archived_billing_item(
  p_sku TEXT,
  p_actor_id BIGINT
)
RETURNS TEXT AS $$
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
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.delete_archived_billing_item(TEXT, BIGINT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END $$;
-- statement:end

REVOKE ALL ON FUNCTION delete_archived_billing_item(TEXT, BIGINT) FROM PUBLIC;

-- migrate:down
-- This down migration is schema-only. Once a hard delete has committed, the
-- deleted Catalog rows and their detached audit targets can only be recovered
-- from a pre-migration database snapshot.
-- statement:begin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM billing_admin_actions WHERE action_type = 'delete_archived'
  ) THEN
    RAISE EXCEPTION
      'cannot roll back archived Billing lifecycle after committed hard deletes; restore the pre-migration database snapshot first';
  END IF;
END $$;
-- statement:end

DROP FUNCTION IF EXISTS delete_archived_billing_item(TEXT, BIGINT);

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
  ),
  ADD CONSTRAINT billing_admin_actions_sku_fkey
    FOREIGN KEY (sku) REFERENCES billing_skus(sku) ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT fk_billing_admin_actions_price
    FOREIGN KEY (sku, price_version_id)
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

-- statement:begin
CREATE OR REPLACE FUNCTION prevent_billing_price_version_mutation()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB105', MESSAGE = 'billing price versions are immutable';
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end
