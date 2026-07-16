-- migrate:up
-- Repair development databases that applied an early draft of migration
-- 20260715020000 before its complete lifecycle contract was committed.
ALTER TABLE reward_campaigns
  DROP CONSTRAINT IF EXISTS ck_reward_campaigns_status,
  ADD CONSTRAINT ck_reward_campaigns_status CHECK (status IN ('draft', 'active', 'paused', 'cancelled'));

ALTER TABLE reward_admin_actions
  DROP CONSTRAINT IF EXISTS fk_reward_admin_actions_campaign,
  DROP CONSTRAINT IF EXISTS ck_reward_admin_actions_type,
  ADD CONSTRAINT ck_reward_admin_actions_type CHECK (
    action_type IN ('create_campaign', 'update_campaign', 'issue_public_code', 'activate_campaign', 'pause_campaign', 'resume_campaign', 'cancel_campaign', 'delete_campaign')
  );

ALTER TABLE reward_codes
  ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ(3),
  ADD COLUMN IF NOT EXISTS revoked_by BIGINT,
  DROP CONSTRAINT IF EXISTS fk_reward_codes_revoked_by,
  DROP CONSTRAINT IF EXISTS ck_reward_codes_revocation,
  ADD CONSTRAINT fk_reward_codes_revoked_by FOREIGN KEY (revoked_by) REFERENCES users(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT ck_reward_codes_revocation CHECK (
    (revoked_at IS NULL AND revoked_by IS NULL) OR (revoked_at IS NOT NULL AND revoked_by IS NOT NULL)
  );

DROP TRIGGER IF EXISTS reward_campaign_rules_frozen ON reward_campaigns;

-- statement:begin
CREATE OR REPLACE FUNCTION guard_reward_campaign_rule_mutation()
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

DROP TRIGGER IF EXISTS reward_codes_immutable ON reward_codes;

-- statement:begin
CREATE OR REPLACE FUNCTION guard_reward_code_mutation()
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
-- This repair is intentionally irreversible: migration 20260715020000 owns
-- the lifecycle schema, and removing repaired objects would corrupt it.
SELECT 1;
