-- migrate:up
ALTER TABLE reward_campaigns
  ADD COLUMN starts_at TIMESTAMPTZ(3),
  ADD COLUMN expires_at TIMESTAMPTZ(3),
  DROP CONSTRAINT ck_reward_campaigns_status,
  ADD CONSTRAINT ck_reward_campaigns_status CHECK (status IN ('draft', 'active', 'paused', 'cancelled')),
  ADD CONSTRAINT ck_reward_campaigns_schedule CHECK (
    starts_at IS NULL OR expires_at IS NULL OR starts_at < expires_at
  );

ALTER TABLE reward_admin_actions
  DROP CONSTRAINT fk_reward_admin_actions_campaign,
  DROP CONSTRAINT ck_reward_admin_actions_type,
  ADD CONSTRAINT ck_reward_admin_actions_type CHECK (
    action_type IN ('create_campaign', 'update_campaign', 'issue_public_code', 'activate_campaign', 'pause_campaign', 'resume_campaign', 'cancel_campaign', 'delete_campaign')
  );

ALTER TABLE reward_codes
  ADD COLUMN revoked_at TIMESTAMPTZ(3),
  ADD COLUMN revoked_by BIGINT,
  ADD CONSTRAINT fk_reward_codes_revoked_by FOREIGN KEY (revoked_by) REFERENCES users(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT ck_reward_codes_revocation CHECK (
    (revoked_at IS NULL AND revoked_by IS NULL) OR (revoked_at IS NOT NULL AND revoked_by IS NOT NULL)
  );

-- statement:begin
CREATE FUNCTION guard_reward_campaign_rule_mutation()
RETURNS TRIGGER AS $$
DECLARE
  has_issued_code BOOLEAN;
BEGIN
  EXECUTE pg_catalog.format(
    'SELECT EXISTS (SELECT 1 FROM %I.reward_codes WHERE campaign_id = $1)',
    TG_TABLE_SCHEMA
  ) INTO has_issued_code USING OLD.id;
  IF TG_OP = 'DELETE' THEN
    IF OLD.status <> 'draft' OR has_issued_code THEN
      RAISE EXCEPTION USING ERRCODE = 'IB503', MESSAGE = 'only an empty reward Draft can be deleted';
    END IF;
    RETURN OLD;
  END IF;
  IF has_issued_code AND (
    NEW.display_name IS DISTINCT FROM OLD.display_name
    OR NEW.credit_amount IS DISTINCT FROM OLD.credit_amount
    OR NEW.mode IS DISTINCT FROM OLD.mode
    OR NEW.max_redemptions IS DISTINCT FROM OLD.max_redemptions
    OR NEW.starts_at IS DISTINCT FROM OLD.starts_at
    OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
    OR NEW.created_by IS DISTINCT FROM OLD.created_by
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB502', MESSAGE = 'reward campaign rules are frozen after Code issuance';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER reward_campaign_rules_frozen
BEFORE UPDATE OR DELETE ON reward_campaigns
FOR EACH ROW EXECUTE FUNCTION guard_reward_campaign_rule_mutation();

REVOKE ALL ON FUNCTION guard_reward_campaign_rule_mutation() FROM PUBLIC;

-- statement:begin
CREATE FUNCTION guard_reward_code_mutation()
RETURNS TRIGGER AS $$
BEGIN
  IF TG_OP = 'DELETE'
     OR NEW.campaign_id IS DISTINCT FROM OLD.campaign_id
     OR NEW.key_version IS DISTINCT FROM OLD.key_version
     OR NEW.code_digest IS DISTINCT FROM OLD.code_digest
     OR NEW.code_hint IS DISTINCT FROM OLD.code_hint
     OR NEW.issued_by IS DISTINCT FROM OLD.issued_by
     OR NEW.issued_at IS DISTINCT FROM OLD.issued_at
     OR OLD.revoked_at IS NOT NULL
     OR NEW.revoked_at IS NULL
     OR NEW.revoked_by IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = 'IB504', MESSAGE = 'reward Code identity and revocation are immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER reward_codes_immutable
BEFORE UPDATE OR DELETE ON reward_codes
FOR EACH ROW EXECUTE FUNCTION guard_reward_code_mutation();

REVOKE ALL ON FUNCTION guard_reward_code_mutation() FROM PUBLIC;

-- migrate:down
DROP TRIGGER IF EXISTS reward_codes_immutable ON reward_codes;
DROP FUNCTION IF EXISTS guard_reward_code_mutation();
DROP TRIGGER IF EXISTS reward_campaign_rules_frozen ON reward_campaigns;
DROP FUNCTION IF EXISTS guard_reward_campaign_rule_mutation();

ALTER TABLE reward_admin_actions
  DROP CONSTRAINT ck_reward_admin_actions_type,
  ADD CONSTRAINT ck_reward_admin_actions_type CHECK (
    action_type IN ('create_campaign', 'issue_public_code', 'activate_campaign')
  );

-- statement:begin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM reward_admin_actions actions
    LEFT JOIN reward_campaigns campaigns ON campaigns.id = actions.campaign_id
    WHERE campaigns.id IS NULL
  ) THEN
    RAISE EXCEPTION 'cannot restore Rewards Campaign FK while deleted Campaign audit facts exist';
  END IF;
  ALTER TABLE reward_admin_actions
    ADD CONSTRAINT fk_reward_admin_actions_campaign
    FOREIGN KEY (campaign_id) REFERENCES reward_campaigns(id) ON UPDATE RESTRICT ON DELETE RESTRICT;
END;
$$;
-- statement:end

UPDATE reward_campaigns SET status = 'active' WHERE status = 'paused';

-- statement:begin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM reward_campaigns WHERE status = 'cancelled')
     OR EXISTS (SELECT 1 FROM reward_codes WHERE revoked_at IS NOT NULL) THEN
    RAISE EXCEPTION 'cannot roll back Rewards lifecycle schema after Campaign cancellation';
  END IF;
END;
$$;
-- statement:end

ALTER TABLE reward_codes
  DROP CONSTRAINT ck_reward_codes_revocation,
  DROP CONSTRAINT fk_reward_codes_revoked_by,
  DROP COLUMN revoked_by,
  DROP COLUMN revoked_at;

ALTER TABLE reward_campaigns
  DROP CONSTRAINT ck_reward_campaigns_schedule,
  DROP CONSTRAINT ck_reward_campaigns_status,
  ADD CONSTRAINT ck_reward_campaigns_status CHECK (status IN ('draft', 'active')),
  DROP COLUMN expires_at,
  DROP COLUMN starts_at;
