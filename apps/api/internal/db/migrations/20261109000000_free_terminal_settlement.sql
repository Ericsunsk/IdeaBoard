-- Free tasks converge their settlement read model at terminal states.
--
-- Before this migration, completeInTx/failInTx/cancelInTx skipped the
-- settlement write whenever no hold existed, so free tasks (total 0, no
-- hold) stayed on the initial 'free' row forever while every paid task
-- converged to captured/released. The service now writes free terminal
-- rows (captured on complete, released on fail/cancel) with zero amounts
-- and NULL transaction ids; the state machine below is the only gate that
-- stood in the way (OLD.state had to be 'held').
--
-- Terminal transitions converge held AND free origins. The amount
-- correspondence (captured = output_count * unit, released = total -
-- captured, tx-id presence matching positive amounts) already forces
-- zeros for free snapshots (total 0, unit 0), so no free-only guard is
-- added. Paid rows keep their exact previous behavior.

CREATE OR REPLACE FUNCTION guard_task_billing_settlement_transition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  snapshot_total BIGINT;
  snapshot_unit BIGINT;
  snapshot_quantity BIGINT;
  snapshot_user BIGINT;
  snapshot_account BIGINT;
  snapshot_hold TEXT;
  valid_terminal BOOLEAN;
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB202', MESSAGE = 'task billing settlements cannot be deleted';
  END IF;
  SELECT total_credits, unit_credits, quantity, user_id, billing_account_id, hold_transaction_id
  INTO snapshot_total, snapshot_unit, snapshot_quantity, snapshot_user, snapshot_account, snapshot_hold
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
  -- Terminal transitions converge held AND free origins (paid tasks
  -- capture/release their hold; free tasks converge with zero amounts and
  -- NULL transaction ids so the read model never forks on the initial
  -- state). The amount correspondence below forces zeros for free
  -- snapshots (total 0, unit 0), so no extra free-only guard is needed.
  IF NEW.task_id <> OLD.task_id
     OR OLD.state NOT IN ('held', 'free')
     OR NEW.state NOT IN ('captured', 'released')
     OR NEW.actual_output_count < 0
     OR NEW.actual_output_count > snapshot_quantity
     OR NEW.captured_credits <> NEW.actual_output_count * snapshot_unit
     OR NEW.released_credits <> snapshot_total - NEW.captured_credits
     OR (NEW.captured_credits > 0) <> (NEW.capture_transaction_id IS NOT NULL)
     OR (NEW.released_credits > 0) <> (NEW.release_transaction_id IS NOT NULL)
     OR NEW.settled_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = 'IB203', MESSAGE = 'invalid task billing settlement transition';
  END IF;
  SELECT (
    (NEW.capture_transaction_id IS NULL OR EXISTS (
      SELECT 1 FROM credit_transactions transactions
      WHERE transactions.transaction_id = NEW.capture_transaction_id
        AND transactions.command_type = 'capture'
        AND transactions.command_amount = NEW.captured_credits
        AND transactions.billing_account_id = snapshot_account
        AND transactions.related_transaction_id = snapshot_hold
        AND transactions.business_reference_type = 'task'
        AND transactions.business_reference_id = NEW.task_id
    )) AND
    (NEW.release_transaction_id IS NULL OR EXISTS (
      SELECT 1 FROM credit_transactions transactions
      WHERE transactions.transaction_id = NEW.release_transaction_id
        AND transactions.command_type = 'release'
        AND transactions.command_amount = NEW.released_credits
        AND transactions.billing_account_id = snapshot_account
        AND transactions.related_transaction_id = snapshot_hold
        AND transactions.business_reference_type = 'task'
        AND transactions.business_reference_id = NEW.task_id
    ))
  ) INTO valid_terminal;
  IF NOT valid_terminal THEN
    RAISE EXCEPTION USING ERRCODE = 'IB207', MESSAGE = 'task billing terminal settlement does not match Credit Ledger';
  END IF;
  RETURN NEW;
END;
$$;
