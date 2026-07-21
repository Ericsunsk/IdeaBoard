-- migrate:up
DROP TRIGGER reward_admin_actions_append_only ON reward_admin_actions;
DROP FUNCTION prevent_reward_admin_action_mutation();

ALTER TABLE reward_admin_actions
  DROP CONSTRAINT fk_reward_admin_actions_code,
  DROP CONSTRAINT ck_reward_admin_actions_type,
  ALTER COLUMN reward_code_id DROP NOT NULL,
  ADD COLUMN reward_code_id_snapshot BIGINT,
  ADD COLUMN target_snapshot JSONB;

UPDATE reward_admin_actions actions
SET reward_code_id_snapshot = actions.reward_code_id,
    target_snapshot = jsonb_build_object(
      'id', codes.id,
      'display_name', codes.display_name,
      'masked_code_hint', '•••• ' || codes.code_hint,
      'credit_amount', codes.credit_amount,
      'max_redemptions', codes.max_redemptions,
      -- Pre-management Journal rows only record creation. These defaults are
      -- derivable from the create operation, current state is not historical state.
      'redemption_count', 0,
      'expires_at', codes.expires_at,
      'enabled', TRUE
    )
FROM reward_codes codes
WHERE codes.id = actions.reward_code_id;

ALTER TABLE reward_admin_actions
  ALTER COLUMN reward_code_id_snapshot SET NOT NULL,
  ALTER COLUMN target_snapshot SET NOT NULL,
  ADD CONSTRAINT ck_reward_admin_actions_type CHECK (
    action_type IN (
      'create_reward_code',
      'update_reward_code',
      'rotate_reward_code',
      'set_reward_code_enabled',
      'delete_reward_code'
    )
  ),
  ADD CONSTRAINT ck_reward_admin_actions_code_snapshot CHECK (reward_code_id_snapshot > 0),
  ADD CONSTRAINT ck_reward_admin_actions_target_snapshot CHECK (jsonb_typeof(target_snapshot) = 'object'),
  ADD CONSTRAINT fk_reward_admin_actions_code FOREIGN KEY (reward_code_id)
    REFERENCES reward_codes(id) ON UPDATE RESTRICT ON DELETE SET NULL;

-- statement:begin
CREATE FUNCTION prevent_reward_admin_action_mutation()
RETURNS TRIGGER AS $$
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
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format('ALTER FUNCTION %I.prevent_reward_admin_action_mutation() SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
END;
$$;
-- statement:end

CREATE TRIGGER reward_admin_actions_append_only
BEFORE UPDATE OR DELETE ON reward_admin_actions
FOR EACH ROW EXECUTE FUNCTION prevent_reward_admin_action_mutation();

REVOKE ALL ON FUNCTION prevent_reward_admin_action_mutation() FROM PUBLIC;

ALTER TABLE reward_redemptions
  ADD COLUMN reward_code_display_name VARCHAR(120),
  ADD COLUMN masked_code_hint VARCHAR(9);

UPDATE reward_redemptions redemptions
SET reward_code_display_name = codes.display_name,
    masked_code_hint = '•••• ' || codes.code_hint
FROM reward_codes codes
WHERE codes.id = redemptions.reward_code_id;

ALTER TABLE reward_redemptions
  ALTER COLUMN reward_code_display_name SET NOT NULL,
  ALTER COLUMN masked_code_hint SET NOT NULL,
  ADD CONSTRAINT ck_reward_redemptions_display_name CHECK (
    char_length(reward_code_display_name) BETWEEN 1 AND 120
  ),
  ADD CONSTRAINT ck_reward_redemptions_masked_code_hint CHECK (
    masked_code_hint ~ '^•••• [A-Z0-9]{4}$'
  );

DROP TRIGGER reward_codes_identity_immutable ON reward_codes;
DROP FUNCTION guard_reward_code_identity();

-- statement:begin
CREATE FUNCTION guard_reward_code_identity()
RETURNS TRIGGER AS $$
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
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER reward_codes_identity_immutable
BEFORE UPDATE ON reward_codes
FOR EACH ROW EXECUTE FUNCTION guard_reward_code_identity();

REVOKE ALL ON FUNCTION guard_reward_code_identity() FROM PUBLIC;

-- statement:begin
CREATE FUNCTION update_reward_code_definition_v1(
  p_reward_code_id BIGINT,
  p_display_name TEXT,
  p_credit_amount BIGINT,
  p_max_redemptions BIGINT,
  p_expires_at TIMESTAMPTZ,
  p_actor_user_id BIGINT
)
RETURNS BOOLEAN AS $$
DECLARE
  before_code RECORD;
  after_code RECORD;
BEGIN
  SELECT * INTO before_code FROM reward_codes WHERE id = p_reward_code_id FOR UPDATE;
  IF NOT FOUND THEN
    RETURN FALSE;
  END IF;
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
  SET display_name = p_display_name,
      credit_amount = p_credit_amount,
      max_redemptions = p_max_redemptions,
      expires_at = p_expires_at
  WHERE id = p_reward_code_id
  RETURNING * INTO after_code;

  INSERT INTO reward_admin_actions (
    reward_code_id, reward_code_id_snapshot, action_type, actor_user_id, metadata, target_snapshot
  ) VALUES (
    after_code.id,
    after_code.id,
    'update_reward_code',
    p_actor_user_id,
    jsonb_build_object(
      'previous_display_name', before_code.display_name,
      'previous_credit_amount', before_code.credit_amount,
      'previous_max_redemptions', before_code.max_redemptions,
      'previous_expires_at', before_code.expires_at
    ),
    jsonb_build_object(
      'id', after_code.id,
      'display_name', after_code.display_name,
      'masked_code_hint', '•••• ' || after_code.code_hint,
      'credit_amount', after_code.credit_amount,
      'max_redemptions', after_code.max_redemptions,
      'redemption_count', after_code.redemption_count,
      'expires_at', after_code.expires_at,
      'enabled', after_code.enabled
    )
  );
  RETURN TRUE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE FUNCTION set_reward_code_enabled_v1(
  p_reward_code_id BIGINT,
  p_enabled BOOLEAN,
  p_actor_user_id BIGINT
)
RETURNS BOOLEAN AS $$
DECLARE
  before_code RECORD;
  after_code RECORD;
BEGIN
  SELECT * INTO before_code FROM reward_codes WHERE id = p_reward_code_id FOR UPDATE;
  IF NOT FOUND THEN
    RETURN FALSE;
  END IF;
  IF before_code.enabled = p_enabled THEN
    RETURN TRUE;
  END IF;

  UPDATE reward_codes SET enabled = p_enabled WHERE id = p_reward_code_id RETURNING * INTO after_code;
  INSERT INTO reward_admin_actions (
    reward_code_id, reward_code_id_snapshot, action_type, actor_user_id, metadata, target_snapshot
  ) VALUES (
    after_code.id,
    after_code.id,
    'set_reward_code_enabled',
    p_actor_user_id,
    jsonb_build_object('previous_enabled', before_code.enabled),
    jsonb_build_object(
      'id', after_code.id,
      'display_name', after_code.display_name,
      'masked_code_hint', '•••• ' || after_code.code_hint,
      'credit_amount', after_code.credit_amount,
      'max_redemptions', after_code.max_redemptions,
      'redemption_count', after_code.redemption_count,
      'expires_at', after_code.expires_at,
      'enabled', after_code.enabled
    )
  );
  RETURN TRUE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE FUNCTION rotate_reward_code_v1(
  p_reward_code_id BIGINT,
  p_key_version TEXT,
  p_code_digest BYTEA,
  p_code_hint TEXT,
  p_previous_key_version TEXT,
  p_previous_code_digest BYTEA,
  p_actor_user_id BIGINT
)
RETURNS BOOLEAN AS $$
DECLARE
  before_code RECORD;
  after_code RECORD;
BEGIN
  SELECT * INTO before_code FROM reward_codes WHERE id = p_reward_code_id FOR UPDATE;
  IF NOT FOUND THEN
    RETURN FALSE;
  END IF;

  UPDATE reward_codes
  SET key_version = p_key_version,
      code_digest = p_code_digest,
      code_hint = p_code_hint
  WHERE id = p_reward_code_id
  RETURNING * INTO after_code;

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
    after_code.id,
    after_code.id,
    'rotate_reward_code',
    p_actor_user_id,
    jsonb_build_object('previous_masked_code_hint', '•••• ' || before_code.code_hint),
    jsonb_build_object(
      'id', after_code.id,
      'display_name', after_code.display_name,
      'masked_code_hint', '•••• ' || after_code.code_hint,
      'credit_amount', after_code.credit_amount,
      'max_redemptions', after_code.max_redemptions,
      'redemption_count', after_code.redemption_count,
      'expires_at', after_code.expires_at,
      'enabled', after_code.enabled
    )
  );
  RETURN TRUE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE FUNCTION delete_reward_code_v1(
  p_reward_code_id BIGINT,
  p_actor_user_id BIGINT
)
RETURNS BOOLEAN AS $$
DECLARE
  target_code RECORD;
BEGIN
  SELECT * INTO target_code FROM reward_codes WHERE id = p_reward_code_id FOR UPDATE;
  IF NOT FOUND THEN
    RETURN FALSE;
  END IF;
  IF target_code.redemption_count <> 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB516', MESSAGE = 'redeemed reward Code cannot be deleted';
  END IF;

  INSERT INTO reward_admin_actions (
    reward_code_id, reward_code_id_snapshot, action_type, actor_user_id, metadata, target_snapshot
  ) VALUES (
    target_code.id,
    target_code.id,
    'delete_reward_code',
    p_actor_user_id,
    '{}'::JSONB,
    jsonb_build_object(
      'id', target_code.id,
      'display_name', target_code.display_name,
      'masked_code_hint', '•••• ' || target_code.code_hint,
      'credit_amount', target_code.credit_amount,
      'max_redemptions', target_code.max_redemptions,
      'redemption_count', target_code.redemption_count,
      'expires_at', target_code.expires_at,
      'enabled', target_code.enabled
    )
  );
  DELETE FROM reward_code_digest_aliases WHERE reward_code_id = p_reward_code_id;
  DELETE FROM reward_codes WHERE id = p_reward_code_id;
  RETURN TRUE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE FUNCTION reserve_reward_code_redemption_v1(p_reward_code_id BIGINT)
RETURNS BOOLEAN AS $$
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
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- Bind each SECURITY DEFINER function to the schema that owns the Rewards tables.
-- statement:begin
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format('ALTER FUNCTION %I.update_reward_code_definition_v1(BIGINT, TEXT, BIGINT, BIGINT, TIMESTAMPTZ, BIGINT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.set_reward_code_enabled_v1(BIGINT, BOOLEAN, BIGINT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.rotate_reward_code_v1(BIGINT, TEXT, BYTEA, TEXT, TEXT, BYTEA, BIGINT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.delete_reward_code_v1(BIGINT, BIGINT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.reserve_reward_code_redemption_v1(BIGINT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION update_reward_code_definition_v1(BIGINT, TEXT, BIGINT, BIGINT, TIMESTAMPTZ, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION set_reward_code_enabled_v1(BIGINT, BOOLEAN, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION rotate_reward_code_v1(BIGINT, TEXT, BYTEA, TEXT, TEXT, BYTEA, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION delete_reward_code_v1(BIGINT, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION reserve_reward_code_redemption_v1(BIGINT) FROM PUBLIC;

-- migrate:down
DROP FUNCTION IF EXISTS update_reward_code_definition_v1(BIGINT, TEXT, BIGINT, BIGINT, TIMESTAMPTZ, BIGINT);
DROP FUNCTION IF EXISTS set_reward_code_enabled_v1(BIGINT, BOOLEAN, BIGINT);
DROP FUNCTION IF EXISTS rotate_reward_code_v1(BIGINT, TEXT, BYTEA, TEXT, TEXT, BYTEA, BIGINT);
DROP FUNCTION IF EXISTS delete_reward_code_v1(BIGINT, BIGINT);
DROP FUNCTION IF EXISTS reserve_reward_code_redemption_v1(BIGINT);

DROP TRIGGER reward_admin_actions_append_only ON reward_admin_actions;
DROP FUNCTION prevent_reward_admin_action_mutation();

DROP TRIGGER reward_codes_identity_immutable ON reward_codes;
DROP FUNCTION guard_reward_code_identity();

-- statement:begin
CREATE FUNCTION guard_reward_code_identity()
RETURNS TRIGGER AS $$
BEGIN
  IF NEW.display_name IS DISTINCT FROM OLD.display_name
     OR NEW.key_version IS DISTINCT FROM OLD.key_version
     OR NEW.code_digest IS DISTINCT FROM OLD.code_digest
     OR NEW.code_hint IS DISTINCT FROM OLD.code_hint
     OR NEW.credit_amount IS DISTINCT FROM OLD.credit_amount
     OR NEW.max_redemptions IS DISTINCT FROM OLD.max_redemptions
     OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
     OR NEW.redemption_count < OLD.redemption_count
     OR NEW.redemption_count > OLD.redemption_count + 1
     OR NEW.created_by IS DISTINCT FROM OLD.created_by
     OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
     OR NEW.request_fingerprint IS DISTINCT FROM OLD.request_fingerprint
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION USING ERRCODE = 'IB512', MESSAGE = 'reward Code definition is immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER reward_codes_identity_immutable
BEFORE UPDATE ON reward_codes
FOR EACH ROW EXECUTE FUNCTION guard_reward_code_identity();

REVOKE ALL ON FUNCTION guard_reward_code_identity() FROM PUBLIC;

ALTER TABLE reward_admin_actions DROP CONSTRAINT fk_reward_admin_actions_code;
DELETE FROM reward_admin_actions WHERE reward_code_id IS NULL;
DELETE FROM reward_admin_actions WHERE action_type <> 'create_reward_code';
ALTER TABLE reward_admin_actions
  DROP CONSTRAINT ck_reward_admin_actions_type,
  DROP CONSTRAINT ck_reward_admin_actions_code_snapshot,
  DROP CONSTRAINT ck_reward_admin_actions_target_snapshot,
  DROP COLUMN reward_code_id_snapshot,
  DROP COLUMN target_snapshot,
  ALTER COLUMN reward_code_id SET NOT NULL,
  ADD CONSTRAINT ck_reward_admin_actions_type CHECK (action_type IN ('create_reward_code')),
  ADD CONSTRAINT fk_reward_admin_actions_code FOREIGN KEY (reward_code_id)
    REFERENCES reward_codes(id) ON UPDATE RESTRICT ON DELETE RESTRICT;

-- statement:begin
CREATE FUNCTION prevent_reward_admin_action_mutation()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB511', MESSAGE = 'reward admin actions are append-only';
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER reward_admin_actions_append_only
BEFORE UPDATE OR DELETE ON reward_admin_actions
FOR EACH ROW EXECUTE FUNCTION prevent_reward_admin_action_mutation();

REVOKE ALL ON FUNCTION prevent_reward_admin_action_mutation() FROM PUBLIC;

ALTER TABLE reward_redemptions
  DROP CONSTRAINT ck_reward_redemptions_display_name,
  DROP CONSTRAINT ck_reward_redemptions_masked_code_hint,
  DROP COLUMN reward_code_display_name,
  DROP COLUMN masked_code_hint;
