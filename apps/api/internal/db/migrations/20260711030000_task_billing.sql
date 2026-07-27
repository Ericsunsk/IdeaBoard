-- migrate:up
ALTER TABLE billing_price_versions
  ADD CONSTRAINT billing_price_versions_snapshot_key
  UNIQUE (sku, price_version_id, version, unit_credits);

CREATE TABLE task_billing_snapshots (
  task_id VARCHAR(64) PRIMARY KEY,
  user_id BIGINT NOT NULL CHECK (user_id > 0),
  feature_slug VARCHAR(64) NOT NULL,
  sku VARCHAR(128) NOT NULL REFERENCES billing_skus(sku) ON UPDATE RESTRICT ON DELETE RESTRICT,
  price_version_id BIGINT NOT NULL,
  price_version BIGINT NOT NULL CHECK (price_version > 0),
  unit_credits BIGINT NOT NULL CHECK (unit_credits BETWEEN 0 AND 9000000000000000),
  quantity BIGINT NOT NULL CHECK (quantity > 0),
  total_credits BIGINT NOT NULL CHECK (total_credits BETWEEN 0 AND 9000000000000000),
  quoted_at TIMESTAMPTZ(3) NOT NULL,
  hold_transaction_id TEXT,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT task_billing_snapshot_total_check CHECK (total_credits = unit_credits * quantity),
  CONSTRAINT task_billing_snapshot_price_fk FOREIGN KEY (sku, price_version_id, price_version, unit_credits)
    REFERENCES billing_price_versions(sku, price_version_id, version, unit_credits)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT task_billing_snapshot_hold_fk FOREIGN KEY (hold_transaction_id)
    REFERENCES credit_transactions(transaction_id)
    ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT task_billing_snapshot_hold_check CHECK (
    (total_credits = 0 AND hold_transaction_id IS NULL) OR
    (total_credits > 0 AND hold_transaction_id IS NOT NULL AND btrim(hold_transaction_id) <> '')
  )
);

CREATE TABLE task_billing_settlements (
  task_id VARCHAR(64) PRIMARY KEY REFERENCES task_billing_snapshots(task_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
  state VARCHAR(16) NOT NULL CHECK (state IN ('held', 'free', 'captured', 'released')),
  settlement_transaction_id TEXT REFERENCES credit_transactions(transaction_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
  settled_at TIMESTAMPTZ(3),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT task_billing_settlement_state_check CHECK (
    (state = 'held' AND settlement_transaction_id IS NULL AND settled_at IS NULL) OR
    (state = 'free' AND settlement_transaction_id IS NULL AND settled_at IS NOT NULL) OR
    (state IN ('captured', 'released') AND settlement_transaction_id IS NOT NULL AND btrim(settlement_transaction_id) <> '' AND settled_at IS NOT NULL)
  )
);

-- statement:begin
CREATE OR REPLACE FUNCTION validate_task_billing_snapshot_insert()
RETURNS TRIGGER AS $$
DECLARE
  valid_hold BOOLEAN;
BEGIN
  IF NEW.total_credits = 0 THEN
    RETURN NEW;
  END IF;
  SELECT EXISTS (
    SELECT 1
    FROM credit_transactions transactions
    WHERE transactions.transaction_id = NEW.hold_transaction_id
      AND transactions.command_type = 'hold'
      AND transactions.wallet_user_id = NEW.user_id
      AND transactions.command_amount = NEW.total_credits
      AND transactions.business_reference_type = 'task'
      AND transactions.business_reference_id = NEW.task_id
  ) INTO valid_hold;
  IF NOT valid_hold THEN
    RAISE EXCEPTION USING ERRCODE = 'IB204', MESSAGE = 'task billing snapshot Hold does not match Credit Ledger';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER task_billing_snapshots_validate_insert
BEFORE INSERT ON task_billing_snapshots
FOR EACH ROW EXECUTE FUNCTION validate_task_billing_snapshot_insert();

-- statement:begin
CREATE OR REPLACE FUNCTION guard_task_billing_settlement_transition()
RETURNS TRIGGER AS $$
DECLARE
  snapshot_total BIGINT;
  snapshot_user BIGINT;
  snapshot_hold TEXT;
  valid_terminal BOOLEAN;
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB202', MESSAGE = 'task billing settlements cannot be deleted';
  END IF;
  SELECT total_credits, user_id, hold_transaction_id
  INTO snapshot_total, snapshot_user, snapshot_hold
  FROM task_billing_snapshots
  WHERE task_id = NEW.task_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB205', MESSAGE = 'task billing settlement snapshot is missing';
  END IF;
  IF TG_OP = 'INSERT' THEN
    IF (snapshot_total = 0 AND NEW.state <> 'free')
       OR (snapshot_total > 0 AND NEW.state <> 'held') THEN
      RAISE EXCEPTION USING ERRCODE = 'IB206', MESSAGE = 'task billing initial settlement state does not match snapshot';
    END IF;
    RETURN NEW;
  END IF;
  IF NEW.task_id <> OLD.task_id
     OR OLD.state <> 'held'
     OR NEW.state NOT IN ('captured', 'released')
     OR NEW.settlement_transaction_id IS NULL
     OR NEW.settled_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = 'IB203', MESSAGE = 'invalid task billing settlement transition';
  END IF;
  SELECT EXISTS (
    SELECT 1
    FROM credit_transactions transactions
    WHERE transactions.transaction_id = NEW.settlement_transaction_id
      AND transactions.command_type = CASE NEW.state WHEN 'captured' THEN 'capture' ELSE 'release' END
      AND transactions.wallet_user_id = snapshot_user
      AND transactions.related_transaction_id = snapshot_hold
      AND transactions.business_reference_type = 'task'
      AND transactions.business_reference_id = NEW.task_id
  ) INTO valid_terminal;
  IF NOT valid_terminal THEN
    RAISE EXCEPTION USING ERRCODE = 'IB207', MESSAGE = 'task billing terminal settlement does not match Credit Ledger';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER task_billing_settlements_state_machine
BEFORE INSERT OR UPDATE OR DELETE ON task_billing_settlements
FOR EACH ROW EXECUTE FUNCTION guard_task_billing_settlement_transition();

-- statement:begin
CREATE OR REPLACE FUNCTION prevent_task_billing_snapshot_mutation()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB201', MESSAGE = 'task billing snapshots are immutable';
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER task_billing_snapshots_immutable
BEFORE UPDATE OR DELETE ON task_billing_snapshots
FOR EACH ROW EXECUTE FUNCTION prevent_task_billing_snapshot_mutation();

CREATE TABLE task_submission_claims (
  user_id BIGINT NOT NULL CHECK (user_id > 0),
  client_action_id VARCHAR(128) NOT NULL CHECK (btrim(client_action_id) <> ''),
  task_id VARCHAR(64) NOT NULL UNIQUE REFERENCES task_billing_snapshots(task_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
  request_hash VARCHAR(64) NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (user_id, client_action_id)
);

-- statement:begin
CREATE OR REPLACE FUNCTION guard_task_submission_claim()
RETURNS TRIGGER AS $$
DECLARE
  valid_claim BOOLEAN;
BEGIN
  IF TG_OP <> 'INSERT' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB208', MESSAGE = 'task submission claims are immutable';
  END IF;
  SELECT EXISTS (
    SELECT 1
    FROM task_billing_snapshots snapshots
    JOIN tasks ON tasks.task_id = snapshots.task_id
    WHERE snapshots.task_id = NEW.task_id
      AND snapshots.user_id = NEW.user_id
      AND tasks.user_id = NEW.user_id
      AND tasks.client_action_id = NEW.client_action_id
  ) INTO valid_claim;
  IF NOT valid_claim THEN
    RAISE EXCEPTION USING ERRCODE = 'IB209', MESSAGE = 'task submission claim does not match Task Billing facts';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;
-- statement:end

CREATE TRIGGER task_submission_claims_guard
BEFORE INSERT OR UPDATE OR DELETE ON task_submission_claims
FOR EACH ROW EXECUTE FUNCTION guard_task_submission_claim();

-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.validate_task_billing_snapshot_insert() SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
  EXECUTE format(
    'ALTER FUNCTION %I.prevent_task_billing_snapshot_mutation() SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
  EXECUTE format(
    'ALTER FUNCTION %I.guard_task_billing_settlement_transition() SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
  EXECUTE format(
    'ALTER FUNCTION %I.guard_task_submission_claim() SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
END $$;
-- statement:end

REVOKE ALL ON FUNCTION prevent_task_billing_snapshot_mutation() FROM PUBLIC;
REVOKE ALL ON FUNCTION validate_task_billing_snapshot_insert() FROM PUBLIC;
REVOKE ALL ON FUNCTION guard_task_billing_settlement_transition() FROM PUBLIC;
REVOKE ALL ON FUNCTION guard_task_submission_claim() FROM PUBLIC;

-- migrate:down
DROP TABLE IF EXISTS task_billing_settlements;
DROP TABLE IF EXISTS task_submission_claims;
DROP TABLE IF EXISTS task_billing_snapshots;
ALTER TABLE billing_price_versions DROP CONSTRAINT IF EXISTS billing_price_versions_snapshot_key;
DROP FUNCTION IF EXISTS guard_task_billing_settlement_transition();
DROP FUNCTION IF EXISTS prevent_task_billing_snapshot_mutation();
DROP FUNCTION IF EXISTS validate_task_billing_snapshot_insert();
DROP FUNCTION IF EXISTS guard_task_submission_claim();
