ALTER TABLE task_billing_settlements
  DROP CONSTRAINT task_billing_settlement_state_check,
  DROP CONSTRAINT task_billing_settlements_settlement_transaction_id_fkey,
  DROP COLUMN settlement_transaction_id,
  ADD COLUMN actual_output_count BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN captured_credits BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN released_credits BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN capture_transaction_id TEXT,
  ADD COLUMN release_transaction_id TEXT,
  ADD CONSTRAINT task_billing_settlements_capture_transaction_id_fkey FOREIGN KEY (capture_transaction_id) REFERENCES credit_transactions(transaction_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT task_billing_settlements_release_transaction_id_fkey FOREIGN KEY (release_transaction_id) REFERENCES credit_transactions(transaction_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT task_billing_settlement_amounts_check CHECK (actual_output_count >= 0 AND captured_credits >= 0 AND released_credits >= 0),
  ADD CONSTRAINT task_billing_settlement_state_check CHECK (
    (state = 'held' AND capture_transaction_id IS NULL AND release_transaction_id IS NULL AND settled_at IS NULL)
    OR (state = 'free' AND capture_transaction_id IS NULL AND release_transaction_id IS NULL AND settled_at IS NOT NULL)
    OR (state IN ('captured', 'released') AND settled_at IS NOT NULL)
  );

CREATE OR REPLACE FUNCTION guard_task_billing_settlement_transition() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  snapshot_total BIGINT;
  snapshot_unit BIGINT;
  snapshot_quantity BIGINT;
  snapshot_user BIGINT;
  snapshot_hold TEXT;
  valid_terminal BOOLEAN;
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB202', MESSAGE = 'task billing settlements cannot be deleted';
  END IF;
  SELECT total_credits, unit_credits, quantity, user_id, hold_transaction_id
  INTO snapshot_total, snapshot_unit, snapshot_quantity, snapshot_user, snapshot_hold
  FROM task_billing_snapshots WHERE task_id = NEW.task_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB205', MESSAGE = 'task billing settlement snapshot is missing';
  END IF;
  IF TG_OP = 'INSERT' THEN
    IF (snapshot_total = 0 AND NEW.state <> 'free') OR (snapshot_total > 0 AND NEW.state <> 'held') THEN
      RAISE EXCEPTION USING ERRCODE = 'IB206', MESSAGE = 'task billing initial settlement state does not match snapshot';
    END IF;
    RETURN NEW;
  END IF;
  IF NEW.task_id <> OLD.task_id OR OLD.state <> 'held' OR NEW.state NOT IN ('captured', 'released')
     OR NEW.actual_output_count < 0 OR NEW.actual_output_count > snapshot_quantity
     OR NEW.captured_credits <> NEW.actual_output_count * snapshot_unit
     OR NEW.released_credits <> snapshot_total - NEW.captured_credits
     OR (NEW.captured_credits > 0) <> (NEW.capture_transaction_id IS NOT NULL)
     OR (NEW.released_credits > 0) <> (NEW.release_transaction_id IS NOT NULL)
     OR NEW.settled_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = 'IB203', MESSAGE = 'invalid task billing settlement transition';
  END IF;
  SELECT
    (NEW.capture_transaction_id IS NULL OR EXISTS (
      SELECT 1 FROM credit_transactions transactions
      WHERE transactions.transaction_id = NEW.capture_transaction_id AND transactions.command_type = 'capture'
        AND transactions.command_amount = NEW.captured_credits AND transactions.wallet_user_id = snapshot_user
        AND transactions.related_transaction_id = snapshot_hold AND transactions.business_reference_type = 'task'
        AND transactions.business_reference_id = NEW.task_id
    )) AND
    (NEW.release_transaction_id IS NULL OR EXISTS (
      SELECT 1 FROM credit_transactions transactions
      WHERE transactions.transaction_id = NEW.release_transaction_id AND transactions.command_type = 'release'
        AND transactions.command_amount = NEW.released_credits AND transactions.wallet_user_id = snapshot_user
        AND transactions.related_transaction_id = snapshot_hold AND transactions.business_reference_type = 'task'
        AND transactions.business_reference_id = NEW.task_id
    ))
  INTO valid_terminal;
  IF NOT valid_terminal THEN
    RAISE EXCEPTION USING ERRCODE = 'IB207', MESSAGE = 'task billing terminal settlement does not match Credit Ledger';
  END IF;
  RETURN NEW;
END;
$$;
