-- migrate:up
-- statement:begin
CREATE OR REPLACE FUNCTION execute_credit_reconciliation(p_run_id BIGINT)
RETURNS VOID AS $$
DECLARE
  v_stale_after INTERVAL;
  v_finding RECORD;
  v_restriction_id TEXT;
BEGIN
  IF NOT pg_try_advisory_xact_lock(5283921922134398278) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB302', MESSAGE = 'credit reconciliation already running';
  END IF;
  UPDATE credit_reconciliation_runs SET status = 'running', started_at = clock_timestamp(), completed_at = NULL,
    error_message = '', attempt_count = attempt_count + 1
  WHERE id = p_run_id AND status IN ('queued', 'failed')
  RETURNING stale_hold_after INTO v_stale_after;
  IF NOT FOUND THEN RETURN; END IF;

  CREATE TEMP TABLE reconciliation_scan (
    finding_type TEXT, severity TEXT, risk TEXT, wallet_user_id BIGINT,
    subject_type TEXT, subject_id TEXT, details JSONB
  ) ON COMMIT DROP;

  INSERT INTO reconciliation_scan
  SELECT 'ledger_postings_invalid', 'critical', 'high', transactions.wallet_user_id,
    'transaction', transactions.transaction_id,
    jsonb_build_object('declared_posting_count', transactions.posting_count, 'actual_posting_count', COUNT(postings.id),
      'asset_count', COUNT(DISTINCT postings.asset_code), 'posting_sum', COALESCE(SUM(postings.amount), 0),
      'debit_amount', COALESCE(-SUM(postings.amount) FILTER (WHERE postings.amount < 0), 0),
      'credit_amount', COALESCE(SUM(postings.amount) FILTER (WHERE postings.amount > 0), 0),
      'command_amount', transactions.command_amount)
  FROM credit_transactions transactions
  LEFT JOIN credit_postings postings ON postings.transaction_id = transactions.transaction_id
  GROUP BY transactions.transaction_id, transactions.wallet_user_id, transactions.posting_count, transactions.command_amount
  HAVING COUNT(postings.id) < 2 OR COUNT(postings.id) <> transactions.posting_count
      OR COUNT(DISTINCT postings.asset_code) <> 1
      OR BOOL_OR(postings.asset_code IS DISTINCT FROM transactions.asset_code)
      OR COALESCE(SUM(postings.amount), 0) <> 0
      OR COALESCE(-SUM(postings.amount) FILTER (WHERE postings.amount < 0), 0) <> transactions.command_amount
      OR COALESCE(SUM(postings.amount) FILTER (WHERE postings.amount > 0), 0) <> transactions.command_amount;

  INSERT INTO reconciliation_scan
  SELECT 'command_shape_invalid', 'critical', 'high', transactions.wallet_user_id, 'transaction', transactions.transaction_id,
    jsonb_build_object('command_type', transactions.command_type, 'actor_type', transactions.actor_type,
      'related_transaction_id', transactions.related_transaction_id, 'command_amount', transactions.command_amount)
  FROM credit_transactions transactions
  WHERE transactions.command_type NOT IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction', 'purchase_credit')
    OR (transactions.command_type IN ('deduction', 'correction') AND transactions.actor_type <> 'admin')
    OR (transactions.command_type = 'purchase_credit' AND (
      transactions.actor_type <> 'service' OR transactions.actor_id <> 'payments'
      OR transactions.source <> 'stripe_purchase'
      OR transactions.business_reference_type <> 'payment_order'
    ))
    OR transactions.command_amount <= 0;

  INSERT INTO reconciliation_scan
  SELECT 'wallet_projection_mismatch', 'critical', 'high', accounts.wallet_user_id,
    'account', accounts.id::TEXT,
    jsonb_build_object('account_role', accounts.account_role, 'projected_balance', accounts.balance,
      'ledger_balance', COALESCE(SUM(postings.amount), 0))
  FROM credit_accounts accounts
  LEFT JOIN credit_postings postings ON postings.account_id = accounts.id
  WHERE accounts.wallet_user_id IS NOT NULL
  GROUP BY accounts.id, accounts.wallet_user_id, accounts.account_role, accounts.balance
  HAVING accounts.balance <> COALESCE(SUM(postings.amount), 0);

  INSERT INTO reconciliation_scan
  SELECT 'stale_hold', 'warning', 'low', holds.wallet_user_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('created_at', holds.created_at, 'age_seconds', EXTRACT(EPOCH FROM clock_timestamp() - holds.created_at)::BIGINT)
  FROM credit_holds holds WHERE holds.status = 'held' AND holds.created_at < clock_timestamp() - v_stale_after;

  INSERT INTO reconciliation_scan
  SELECT 'task_hold_missing_billing_snapshot', 'critical', 'high', transactions.wallet_user_id,
    'hold', transactions.transaction_id,
    jsonb_build_object('task_id', transactions.business_reference_id, 'billing_snapshot_exists', FALSE)
  FROM credit_transactions transactions
  LEFT JOIN task_billing_snapshots snapshots ON snapshots.hold_transaction_id = transactions.transaction_id
  WHERE transactions.command_type = 'hold' AND transactions.business_reference_type = 'task'
    AND snapshots.task_id IS NULL;

  INSERT INTO reconciliation_scan
  SELECT 'task_trace_missing', 'warning', 'low', snapshots.user_id,
    'task', snapshots.task_id,
    jsonb_build_object('hold_transaction_id', snapshots.hold_transaction_id, 'settlement_state', settlements.state)
  FROM task_billing_snapshots snapshots
  JOIN credit_transactions transactions ON transactions.transaction_id = snapshots.hold_transaction_id
    AND transactions.command_type = 'hold' AND transactions.command_amount = snapshots.total_credits
    AND transactions.wallet_user_id = snapshots.user_id
    AND transactions.business_reference_type = 'task' AND transactions.business_reference_id = snapshots.task_id
  JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  JOIN credit_holds holds ON holds.hold_transaction_id = snapshots.hold_transaction_id
    AND holds.wallet_user_id = snapshots.user_id AND holds.amount = snapshots.total_credits
    AND holds.status = settlements.state AND holds.terminal_transaction_id IS NOT DISTINCT FROM settlements.settlement_transaction_id
  LEFT JOIN tasks ON tasks.task_id = snapshots.task_id
  WHERE snapshots.total_credits > 0 AND tasks.task_id IS NULL
    AND settlements.state IN ('held', 'captured', 'released')
    AND (settlements.state = 'held' OR EXISTS (
      SELECT 1 FROM credit_transactions terminal
      WHERE terminal.transaction_id = settlements.settlement_transaction_id
        AND terminal.command_type = CASE settlements.state WHEN 'captured' THEN 'capture' ELSE 'release' END
        AND terminal.command_amount = snapshots.total_credits
        AND terminal.wallet_user_id = snapshots.user_id
        AND terminal.related_transaction_id = snapshots.hold_transaction_id
        AND terminal.business_reference_type = 'task' AND terminal.business_reference_id = snapshots.task_id
    ))
    AND NOT EXISTS (
      SELECT 1
      FROM credit_transactions ledger_transactions
      LEFT JOIN LATERAL (
        SELECT COUNT(*) AS posting_count, COUNT(DISTINCT postings.asset_code) AS asset_count,
          BOOL_OR(postings.asset_code IS DISTINCT FROM ledger_transactions.asset_code) AS asset_mismatch,
          COALESCE(SUM(postings.amount), 0) AS posting_sum,
          COALESCE(-SUM(postings.amount) FILTER (WHERE postings.amount < 0), 0) AS debit_amount,
          COALESCE(SUM(postings.amount) FILTER (WHERE postings.amount > 0), 0) AS credit_amount
        FROM credit_postings postings
        WHERE postings.transaction_id = ledger_transactions.transaction_id
      ) ledger ON TRUE
      WHERE ledger_transactions.transaction_id IN (transactions.transaction_id, settlements.settlement_transaction_id)
        AND (ledger.posting_count < 2 OR ledger.posting_count <> ledger_transactions.posting_count
          OR ledger.asset_count <> 1 OR ledger.asset_mismatch OR ledger.posting_sum <> 0
          OR ledger.debit_amount <> ledger_transactions.command_amount
          OR ledger.credit_amount <> ledger_transactions.command_amount)
    );

  INSERT INTO reconciliation_scan
  SELECT 'task_billing_fact_missing', 'critical', 'high', snapshots.user_id, 'task', snapshots.task_id,
    jsonb_build_object('hold_transaction_id', snapshots.hold_transaction_id, 'hold_exists', holds.hold_transaction_id IS NOT NULL,
      'settlement_exists', settlements.task_id IS NOT NULL, 'hold_command', hold_transactions.command_type,
      'hold_command_amount', hold_transactions.command_amount, 'expected_amount', snapshots.total_credits,
      'hold_wallet_matches', hold_transactions.wallet_user_id = snapshots.user_id,
      'hold_reference_matches', hold_transactions.business_reference_type = 'task' AND hold_transactions.business_reference_id = snapshots.task_id,
      'hold_projection_matches', holds.wallet_user_id = snapshots.user_id AND holds.amount = snapshots.total_credits,
      'settlement_state', settlements.state, 'terminal_command', terminal.command_type,
      'terminal_command_amount', terminal.command_amount)
  FROM task_billing_snapshots snapshots
  LEFT JOIN credit_transactions hold_transactions ON hold_transactions.transaction_id = snapshots.hold_transaction_id
  LEFT JOIN credit_holds holds ON holds.hold_transaction_id = snapshots.hold_transaction_id
  LEFT JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_transactions terminal ON terminal.transaction_id = settlements.settlement_transaction_id
  WHERE snapshots.total_credits > 0 AND (
    holds.hold_transaction_id IS NULL OR settlements.task_id IS NULL
    OR hold_transactions.command_type IS DISTINCT FROM 'hold'
    OR hold_transactions.command_amount IS DISTINCT FROM snapshots.total_credits
    OR hold_transactions.wallet_user_id IS DISTINCT FROM snapshots.user_id
    OR hold_transactions.business_reference_type IS DISTINCT FROM 'task'
    OR hold_transactions.business_reference_id IS DISTINCT FROM snapshots.task_id
    OR holds.wallet_user_id IS DISTINCT FROM snapshots.user_id
    OR holds.amount IS DISTINCT FROM snapshots.total_credits
    OR settlements.state NOT IN ('held', 'captured', 'released')
    OR holds.status IS DISTINCT FROM settlements.state
    OR holds.terminal_transaction_id IS DISTINCT FROM settlements.settlement_transaction_id
    OR (settlements.state IN ('captured', 'released') AND (
      terminal.transaction_id IS NULL
      OR terminal.command_type IS DISTINCT FROM CASE settlements.state WHEN 'captured' THEN 'capture' ELSE 'release' END
      OR terminal.command_amount IS DISTINCT FROM snapshots.total_credits
      OR terminal.wallet_user_id IS DISTINCT FROM snapshots.user_id
      OR terminal.related_transaction_id IS DISTINCT FROM snapshots.hold_transaction_id
      OR terminal.business_reference_type IS DISTINCT FROM 'task'
      OR terminal.business_reference_id IS DISTINCT FROM snapshots.task_id
    ))
  );

  INSERT INTO reconciliation_scan
  SELECT CASE
      WHEN tasks.status IN ('pending', 'running') THEN 'active_task_not_held'
      WHEN tasks.status = 'completed' AND (settlements.state IS DISTINCT FROM 'captured' OR holds.status IS DISTINCT FROM 'captured') THEN 'completed_task_not_captured'
      WHEN tasks.status IN ('failed', 'cancelled') AND (settlements.state IS DISTINCT FROM 'released' OR holds.status IS DISTINCT FROM 'released') THEN 'failed_task_not_released'
    END,
    'critical', 'high', snapshots.user_id, 'task', tasks.task_id,
    jsonb_build_object('task_status', tasks.status, 'settlement_state', settlements.state,
      'hold_status', holds.status, 'settlement_transaction_id', settlements.settlement_transaction_id)
  FROM task_billing_snapshots snapshots
  JOIN tasks ON tasks.task_id = snapshots.task_id
  LEFT JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_holds holds ON holds.hold_transaction_id = snapshots.hold_transaction_id
  WHERE snapshots.total_credits > 0 AND (
    (tasks.status IN ('pending', 'running') AND (settlements.state IS DISTINCT FROM 'held' OR holds.status IS DISTINCT FROM 'held'))
    OR (tasks.status = 'completed' AND (settlements.state IS DISTINCT FROM 'captured' OR holds.status IS DISTINCT FROM 'captured'))
    OR (tasks.status IN ('failed', 'cancelled') AND (settlements.state IS DISTINCT FROM 'released' OR holds.status IS DISTINCT FROM 'released')));

  INSERT INTO reconciliation_scan
  SELECT 'task_terminal_transaction_count', 'critical', 'high', snapshots.user_id, 'task', tasks.task_id,
    jsonb_build_object('task_status', tasks.status, 'expected_command',
      CASE WHEN tasks.status = 'completed' THEN 'capture' ELSE 'release' END, 'actual_count', COUNT(terminals.transaction_id))
  FROM task_billing_snapshots snapshots
  JOIN tasks ON tasks.task_id = snapshots.task_id
  LEFT JOIN credit_transactions terminals ON terminals.related_transaction_id = snapshots.hold_transaction_id
    AND terminals.command_type = CASE WHEN tasks.status = 'completed' THEN 'capture' ELSE 'release' END
    AND terminals.wallet_user_id = snapshots.user_id
    AND terminals.business_reference_type = 'task' AND terminals.business_reference_id = tasks.task_id
  WHERE snapshots.total_credits > 0 AND tasks.status IN ('completed', 'failed', 'cancelled')
  GROUP BY snapshots.user_id, tasks.task_id, tasks.status
  HAVING COUNT(terminals.transaction_id) <> 1;

  INSERT INTO reconciliation_scan
  WITH conflicts AS (
    SELECT idempotency_key, COUNT(*) AS transaction_count, COUNT(DISTINCT request_hash) AS request_hash_count
    FROM credit_transactions GROUP BY idempotency_key HAVING COUNT(DISTINCT request_hash) > 1
  )
  SELECT 'idempotency_request_hash_conflict', 'critical', 'high', transactions.wallet_user_id,
    'idempotency_key', conflicts.idempotency_key,
    jsonb_build_object('transaction_count', conflicts.transaction_count, 'request_hash_count', conflicts.request_hash_count)
  FROM conflicts JOIN credit_transactions transactions USING (idempotency_key)
  GROUP BY conflicts.idempotency_key, conflicts.transaction_count, conflicts.request_hash_count, transactions.wallet_user_id;

  INSERT INTO reconciliation_scan
  SELECT 'task_settlement_reference_invalid', 'critical', 'high', snapshots.user_id, 'task', snapshots.task_id,
    jsonb_build_object('settlement_state', settlements.state, 'settlement_transaction_id', settlements.settlement_transaction_id,
      'transaction_command', terminal.command_type, 'transaction_wallet_user_id', terminal.wallet_user_id,
      'business_reference_type', terminal.business_reference_type, 'business_reference_id', terminal.business_reference_id)
  FROM task_billing_snapshots snapshots
  JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_transactions terminal ON terminal.transaction_id = settlements.settlement_transaction_id
  WHERE settlements.state IN ('captured', 'released') AND (
    terminal.transaction_id IS NULL
    OR terminal.command_type IS DISTINCT FROM CASE settlements.state WHEN 'captured' THEN 'capture' ELSE 'release' END
    OR terminal.wallet_user_id IS DISTINCT FROM snapshots.user_id
    OR terminal.related_transaction_id IS DISTINCT FROM snapshots.hold_transaction_id
    OR terminal.business_reference_type IS DISTINCT FROM 'task'
    OR terminal.business_reference_id IS DISTINCT FROM snapshots.task_id
  );

  INSERT INTO reconciliation_scan
  SELECT 'duplicate_business_reference', 'error', 'high', wallet_user_id, 'business_reference',
    business_reference_type || ':' || business_reference_id || ':' || command_type,
    jsonb_build_object('command_type', command_type, 'transaction_count', COUNT(*))
  FROM credit_transactions
  GROUP BY wallet_user_id, business_reference_type, business_reference_id, command_type HAVING COUNT(*) > 1;

  INSERT INTO reconciliation_scan
  SELECT 'correction_amount_exceeds_original', 'critical', 'high', original.wallet_user_id,
    'transaction', original.transaction_id,
    jsonb_build_object('original_command_type', original.command_type, 'original_amount', original.command_amount,
      'corrected_amount', SUM(corrections.command_amount))
  FROM credit_transactions original
  JOIN credit_transactions corrections ON corrections.command_type = 'correction'
    AND corrections.related_transaction_id = original.transaction_id
  GROUP BY original.transaction_id, original.wallet_user_id, original.command_type, original.command_amount, original.actor_type
  HAVING original.command_type NOT IN ('grant', 'deduction', 'refund')
    OR original.actor_type <> 'admin' OR SUM(corrections.command_amount) > original.command_amount;

  INSERT INTO reconciliation_scan
  SELECT 'refund_amount_invalid', 'critical', 'high', charges.wallet_user_id, 'charge', charges.capture_transaction_id,
    jsonb_build_object('charge_amount', charges.amount, 'projected_refunded_amount', charges.refunded_amount,
      'ledger_refunded_amount', COALESCE(refunds.amount, 0) - COALESCE(corrections.amount, 0))
  FROM credit_charges charges
  LEFT JOIN LATERAL (
    SELECT SUM(command_amount) AS amount FROM credit_transactions
    WHERE command_type = 'refund' AND related_transaction_id = charges.capture_transaction_id
  ) refunds ON TRUE
  LEFT JOIN LATERAL (
    SELECT SUM(correction.command_amount) AS amount
    FROM credit_transactions correction
    JOIN credit_transactions refund ON refund.transaction_id = correction.related_transaction_id
    WHERE correction.command_type = 'correction' AND refund.command_type = 'refund'
      AND refund.related_transaction_id = charges.capture_transaction_id
  ) corrections ON TRUE
  WHERE charges.refunded_amount < 0 OR charges.refunded_amount > charges.amount
    OR charges.refunded_amount <> COALESCE(refunds.amount, 0) - COALESCE(corrections.amount, 0);

  INSERT INTO reconciliation_scan
  SELECT 'illegal_reference_chain', 'critical', 'high', transactions.wallet_user_id, 'transaction', transactions.transaction_id,
    jsonb_build_object('command_type', transactions.command_type, 'related_transaction_id', transactions.related_transaction_id,
      'related_command_type', related.command_type, 'related_wallet_user_id', related.wallet_user_id)
  FROM credit_transactions transactions
  LEFT JOIN credit_transactions related ON related.transaction_id = transactions.related_transaction_id
  WHERE (transactions.command_type IN ('grant', 'deduction', 'hold', 'purchase_credit') AND transactions.related_transaction_id IS NOT NULL)
    OR (transactions.command_type IN ('capture', 'release') AND
      (related.transaction_id IS NULL OR related.command_type <> 'hold' OR related.wallet_user_id <> transactions.wallet_user_id))
    OR (transactions.command_type = 'refund' AND
      (related.transaction_id IS NULL OR related.command_type <> 'capture' OR related.wallet_user_id <> transactions.wallet_user_id))
    OR (transactions.command_type = 'correction' AND
      (related.transaction_id IS NULL OR related.command_type NOT IN ('grant', 'deduction', 'refund')
       OR related.actor_type <> 'admin' OR related.wallet_user_id <> transactions.wallet_user_id));

  INSERT INTO reconciliation_scan
  SELECT 'hold_state_invalid', 'critical', 'high', holds.wallet_user_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('hold_status', holds.status, 'hold_amount', holds.amount, 'hold_command', hold_tx.command_type,
      'terminal_command', terminal.command_type)
  FROM credit_holds holds
  LEFT JOIN credit_transactions hold_tx ON hold_tx.transaction_id = holds.hold_transaction_id
  LEFT JOIN credit_transactions terminal ON terminal.transaction_id = holds.terminal_transaction_id
  WHERE hold_tx.command_type IS DISTINCT FROM 'hold' OR hold_tx.wallet_user_id IS DISTINCT FROM holds.wallet_user_id
    OR hold_tx.command_amount IS DISTINCT FROM holds.amount
    OR (holds.status = 'held' AND holds.terminal_transaction_id IS NOT NULL)
    OR (holds.status = 'captured' AND (terminal.command_type IS DISTINCT FROM 'capture' OR terminal.related_transaction_id IS DISTINCT FROM holds.hold_transaction_id))
    OR (holds.status = 'released' AND (terminal.command_type IS DISTINCT FROM 'release' OR terminal.related_transaction_id IS DISTINCT FROM holds.hold_transaction_id));

  INSERT INTO reconciliation_scan
  SELECT 'hold_terminal_transaction_count', 'critical', 'high', holds.wallet_user_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('hold_status', holds.status, 'terminal_transaction_count', COUNT(terminals.transaction_id))
  FROM credit_holds holds
  LEFT JOIN credit_transactions terminals ON terminals.related_transaction_id = holds.hold_transaction_id
    AND terminals.command_type IN ('capture', 'release')
  GROUP BY holds.hold_transaction_id, holds.wallet_user_id, holds.status
  HAVING COUNT(terminals.transaction_id) <> CASE WHEN holds.status = 'held' THEN 0 ELSE 1 END;

  INSERT INTO reconciliation_scan
  SELECT 'charge_state_invalid', 'critical', 'high', charges.wallet_user_id, 'charge', charges.capture_transaction_id,
    jsonb_build_object('charge_amount', charges.amount, 'capture_command', capture.command_type, 'capture_amount', capture.command_amount)
  FROM credit_charges charges
  LEFT JOIN credit_transactions capture ON capture.transaction_id = charges.capture_transaction_id
  WHERE capture.command_type IS DISTINCT FROM 'capture' OR capture.wallet_user_id IS DISTINCT FROM charges.wallet_user_id
    OR capture.command_amount IS DISTINCT FROM charges.amount;

  WITH normalized AS (
    SELECT scan.*, encode(sha256(convert_to(concat_ws(E'\x1f', finding_type, COALESCE(wallet_user_id::TEXT, ''), subject_type, subject_id), 'UTF8')), 'hex') AS fingerprint
    FROM reconciliation_scan scan WHERE finding_type IS NOT NULL
  ), upserted AS (
    INSERT INTO credit_reconciliation_findings (
      fingerprint, finding_type, severity, risk, wallet_user_id, subject_type, subject_id, details,
      first_seen_run_id, last_seen_run_id
    ) SELECT fingerprint, finding_type, severity, risk, wallet_user_id, subject_type, subject_id, details, p_run_id, p_run_id
      FROM normalized
    ON CONFLICT (fingerprint) DO UPDATE SET
      severity = EXCLUDED.severity, risk = EXCLUDED.risk, wallet_user_id = EXCLUDED.wallet_user_id,
      details = EXCLUDED.details, status = 'open', isolated = FALSE, last_seen_run_id = p_run_id,
      last_seen_at = clock_timestamp(), occurrence_count = credit_reconciliation_findings.occurrence_count + 1,
      resolved_at = NULL
    RETURNING id
  ) INSERT INTO credit_reconciliation_run_findings (run_id, finding_id)
    SELECT p_run_id, id FROM upserted ON CONFLICT DO NOTHING;

  UPDATE credit_reconciliation_findings
  SET status = 'resolved', resolved_at = clock_timestamp()
  WHERE status = 'open' AND last_seen_run_id <> p_run_id;

  FOR v_finding IN
    SELECT restrictions.restriction_id::TEXT AS restriction_id,
           restrictions.wallet_user_id,
           findings.id AS finding_id
    FROM credit_wallet_restrictions restrictions
    JOIN credit_reconciliation_findings findings
      ON restrictions.related_object_type = 'finding'
     AND restrictions.related_object_id = findings.id::TEXT
    WHERE restrictions.restriction_type = 'ledger_integrity'
      AND restrictions.source = 'reconciliation'
      AND restrictions.released_at IS NULL
      AND (
        findings.status <> 'open'
        OR findings.risk <> 'high'
        OR findings.wallet_user_id IS NULL
        OR findings.wallet_user_id <> restrictions.wallet_user_id
        OR findings.last_seen_run_id <> p_run_id
      )
    ORDER BY restrictions.wallet_user_id, findings.id
  LOOP
    PERFORM release_credit_wallet_restriction(
      v_finding.restriction_id,
      'system',
      'credits-reconciliation',
      'finding no longer requires ledger integrity isolation'
    );
  END LOOP;

  FOR v_finding IN
    SELECT findings.id, findings.wallet_user_id, findings.finding_type
    FROM credit_reconciliation_findings findings
    WHERE findings.last_seen_run_id = p_run_id
      AND findings.status = 'open'
      AND findings.risk = 'high'
      AND findings.wallet_user_id IS NOT NULL
    ORDER BY findings.wallet_user_id, findings.id
  LOOP
    SELECT restriction_id INTO v_restriction_id
    FROM create_credit_wallet_restriction(
      v_finding.wallet_user_id,
      'ledger_integrity',
      'reconciliation',
      'finding',
      v_finding.id::TEXT,
      'high-risk reconciliation finding: ' || v_finding.finding_type,
      'system',
      'credits-reconciliation'
    );
  END LOOP;

  UPDATE credit_reconciliation_findings findings
  SET isolated = EXISTS (
    SELECT 1
    FROM credit_wallet_restrictions restrictions
    WHERE restrictions.wallet_user_id = findings.wallet_user_id
      AND restrictions.restriction_type = 'ledger_integrity'
      AND restrictions.source = 'reconciliation'
      AND restrictions.related_object_type = 'finding'
      AND restrictions.related_object_id = findings.id::TEXT
      AND restrictions.released_at IS NULL
  );

  UPDATE credit_reconciliation_run_findings links SET
    observed_severity = findings.severity,
    observed_risk = findings.risk,
    observed_details = findings.details,
    observed_status = findings.status,
    observed_isolated = findings.isolated,
    observed_occurrence_count = findings.occurrence_count,
    observed_first_seen_at = findings.first_seen_at,
    observed_last_seen_at = findings.last_seen_at,
    observed_resolved_at = findings.resolved_at
  FROM credit_reconciliation_findings findings
  WHERE links.run_id = p_run_id AND links.finding_id = findings.id;

  UPDATE credit_reconciliation_runs runs SET status = 'completed', completed_at = clock_timestamp(),
    finding_count = (SELECT COUNT(*) FROM credit_reconciliation_run_findings WHERE run_id = p_run_id),
    critical_count = (SELECT COUNT(*) FROM credit_reconciliation_findings WHERE last_seen_run_id = p_run_id AND severity = 'critical'),
    isolated_wallet_count = (SELECT COUNT(DISTINCT wallet_user_id) FROM credit_reconciliation_findings
      WHERE last_seen_run_id = p_run_id AND isolated)
  WHERE runs.id = p_run_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

-- statement:end

-- statement:begin
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_reconciliation(BIGINT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION execute_credit_reconciliation(BIGINT) FROM PUBLIC;

-- migrate:down
-- statement:begin
CREATE OR REPLACE FUNCTION execute_credit_reconciliation(p_run_id BIGINT)
RETURNS VOID AS $$
DECLARE
  v_stale_after INTERVAL;
  v_finding RECORD;
  v_restriction_id TEXT;
BEGIN
  IF NOT pg_try_advisory_xact_lock(5283921922134398278) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB302', MESSAGE = 'credit reconciliation already running';
  END IF;
  UPDATE credit_reconciliation_runs SET status = 'running', started_at = clock_timestamp(), completed_at = NULL,
    error_message = '', attempt_count = attempt_count + 1
  WHERE id = p_run_id AND status IN ('queued', 'failed')
  RETURNING stale_hold_after INTO v_stale_after;
  IF NOT FOUND THEN RETURN; END IF;

  CREATE TEMP TABLE reconciliation_scan (
    finding_type TEXT, severity TEXT, risk TEXT, wallet_user_id BIGINT,
    subject_type TEXT, subject_id TEXT, details JSONB
  ) ON COMMIT DROP;

  INSERT INTO reconciliation_scan
  SELECT 'ledger_postings_invalid', 'critical', 'high', transactions.wallet_user_id,
    'transaction', transactions.transaction_id,
    jsonb_build_object('declared_posting_count', transactions.posting_count, 'actual_posting_count', COUNT(postings.id),
      'asset_count', COUNT(DISTINCT postings.asset_code), 'posting_sum', COALESCE(SUM(postings.amount), 0))
  FROM credit_transactions transactions
  LEFT JOIN credit_postings postings ON postings.transaction_id = transactions.transaction_id
  GROUP BY transactions.transaction_id, transactions.wallet_user_id, transactions.posting_count
  HAVING COUNT(postings.id) < 2 OR COUNT(postings.id) <> transactions.posting_count
      OR COUNT(DISTINCT postings.asset_code) <> 1
      OR BOOL_OR(postings.asset_code IS DISTINCT FROM transactions.asset_code)
      OR COALESCE(SUM(postings.amount), 0) <> 0;

  INSERT INTO reconciliation_scan
  SELECT 'command_shape_invalid', 'critical', 'high', transactions.wallet_user_id, 'transaction', transactions.transaction_id,
    jsonb_build_object('command_type', transactions.command_type, 'actor_type', transactions.actor_type,
      'related_transaction_id', transactions.related_transaction_id, 'command_amount', transactions.command_amount)
  FROM credit_transactions transactions
  WHERE transactions.command_type NOT IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction', 'purchase_credit')
    OR (transactions.command_type IN ('deduction', 'correction') AND transactions.actor_type <> 'admin')
    OR (transactions.command_type = 'purchase_credit' AND (
      transactions.actor_type <> 'service' OR transactions.actor_id <> 'payments'
      OR transactions.source <> 'stripe_purchase'
      OR transactions.business_reference_type <> 'payment_order'
    ))
    OR transactions.command_amount <= 0;

  INSERT INTO reconciliation_scan
  SELECT 'wallet_projection_mismatch', 'critical', 'high', accounts.wallet_user_id,
    'account', accounts.id::TEXT,
    jsonb_build_object('account_role', accounts.account_role, 'projected_balance', accounts.balance,
      'ledger_balance', COALESCE(SUM(postings.amount), 0))
  FROM credit_accounts accounts
  LEFT JOIN credit_postings postings ON postings.account_id = accounts.id
  WHERE accounts.wallet_user_id IS NOT NULL
  GROUP BY accounts.id, accounts.wallet_user_id, accounts.account_role, accounts.balance
  HAVING accounts.balance <> COALESCE(SUM(postings.amount), 0);

  INSERT INTO reconciliation_scan
  SELECT 'stale_hold', 'warning', 'low', holds.wallet_user_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('created_at', holds.created_at, 'age_seconds', EXTRACT(EPOCH FROM clock_timestamp() - holds.created_at)::BIGINT)
  FROM credit_holds holds WHERE holds.status = 'held' AND holds.created_at < clock_timestamp() - v_stale_after;

  INSERT INTO reconciliation_scan
  SELECT 'task_hold_missing_task', 'critical', 'high', transactions.wallet_user_id,
    'hold', transactions.transaction_id,
    jsonb_build_object('task_id', transactions.business_reference_id)
  FROM credit_transactions transactions
  LEFT JOIN tasks ON tasks.task_id = transactions.business_reference_id
  LEFT JOIN task_billing_snapshots snapshots ON snapshots.hold_transaction_id = transactions.transaction_id
  WHERE transactions.command_type = 'hold' AND transactions.business_reference_type = 'task'
    AND (tasks.task_id IS NULL OR snapshots.task_id IS NULL);

  INSERT INTO reconciliation_scan
  SELECT 'task_billing_fact_missing', 'critical', 'high', snapshots.user_id, 'task', snapshots.task_id,
    jsonb_build_object('hold_transaction_id', snapshots.hold_transaction_id, 'hold_exists', holds.hold_transaction_id IS NOT NULL,
      'settlement_exists', settlements.task_id IS NOT NULL)
  FROM task_billing_snapshots snapshots
  LEFT JOIN credit_holds holds ON holds.hold_transaction_id = snapshots.hold_transaction_id
  LEFT JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  WHERE snapshots.total_credits > 0 AND (holds.hold_transaction_id IS NULL OR settlements.task_id IS NULL);

  INSERT INTO reconciliation_scan
  SELECT CASE
      WHEN tasks.status IN ('pending', 'running') THEN 'active_task_not_held'
      WHEN tasks.status = 'completed' AND (settlements.state IS DISTINCT FROM 'captured' OR holds.status IS DISTINCT FROM 'captured') THEN 'completed_task_not_captured'
      WHEN tasks.status IN ('failed', 'cancelled') AND (settlements.state IS DISTINCT FROM 'released' OR holds.status IS DISTINCT FROM 'released') THEN 'failed_task_not_released'
    END,
    'critical', 'high', snapshots.user_id, 'task', tasks.task_id,
    jsonb_build_object('task_status', tasks.status, 'settlement_state', settlements.state,
      'hold_status', holds.status, 'settlement_transaction_id', settlements.settlement_transaction_id)
  FROM task_billing_snapshots snapshots
  JOIN tasks ON tasks.task_id = snapshots.task_id
  LEFT JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_holds holds ON holds.hold_transaction_id = snapshots.hold_transaction_id
  WHERE snapshots.total_credits > 0 AND (
    (tasks.status IN ('pending', 'running') AND (settlements.state IS DISTINCT FROM 'held' OR holds.status IS DISTINCT FROM 'held'))
    OR (tasks.status = 'completed' AND (settlements.state IS DISTINCT FROM 'captured' OR holds.status IS DISTINCT FROM 'captured'))
    OR (tasks.status IN ('failed', 'cancelled') AND (settlements.state IS DISTINCT FROM 'released' OR holds.status IS DISTINCT FROM 'released')));

  INSERT INTO reconciliation_scan
  SELECT 'task_terminal_transaction_count', 'critical', 'high', snapshots.user_id, 'task', tasks.task_id,
    jsonb_build_object('task_status', tasks.status, 'expected_command',
      CASE WHEN tasks.status = 'completed' THEN 'capture' ELSE 'release' END, 'actual_count', COUNT(terminals.transaction_id))
  FROM task_billing_snapshots snapshots
  JOIN tasks ON tasks.task_id = snapshots.task_id
  LEFT JOIN credit_transactions terminals ON terminals.related_transaction_id = snapshots.hold_transaction_id
    AND terminals.command_type = CASE WHEN tasks.status = 'completed' THEN 'capture' ELSE 'release' END
    AND terminals.wallet_user_id = snapshots.user_id
    AND terminals.business_reference_type = 'task' AND terminals.business_reference_id = tasks.task_id
  WHERE snapshots.total_credits > 0 AND tasks.status IN ('completed', 'failed', 'cancelled')
  GROUP BY snapshots.user_id, tasks.task_id, tasks.status
  HAVING COUNT(terminals.transaction_id) <> 1;

  INSERT INTO reconciliation_scan
  WITH conflicts AS (
    SELECT idempotency_key, COUNT(*) AS transaction_count, COUNT(DISTINCT request_hash) AS request_hash_count
    FROM credit_transactions GROUP BY idempotency_key HAVING COUNT(DISTINCT request_hash) > 1
  )
  SELECT 'idempotency_request_hash_conflict', 'critical', 'high', transactions.wallet_user_id,
    'idempotency_key', conflicts.idempotency_key,
    jsonb_build_object('transaction_count', conflicts.transaction_count, 'request_hash_count', conflicts.request_hash_count)
  FROM conflicts JOIN credit_transactions transactions USING (idempotency_key)
  GROUP BY conflicts.idempotency_key, conflicts.transaction_count, conflicts.request_hash_count, transactions.wallet_user_id;

  INSERT INTO reconciliation_scan
  SELECT 'task_settlement_reference_invalid', 'critical', 'high', snapshots.user_id, 'task', snapshots.task_id,
    jsonb_build_object('settlement_state', settlements.state, 'settlement_transaction_id', settlements.settlement_transaction_id,
      'transaction_command', terminal.command_type, 'transaction_wallet_user_id', terminal.wallet_user_id,
      'business_reference_type', terminal.business_reference_type, 'business_reference_id', terminal.business_reference_id)
  FROM task_billing_snapshots snapshots
  JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_transactions terminal ON terminal.transaction_id = settlements.settlement_transaction_id
  WHERE settlements.state IN ('captured', 'released') AND (
    terminal.transaction_id IS NULL
    OR terminal.command_type IS DISTINCT FROM CASE settlements.state WHEN 'captured' THEN 'capture' ELSE 'release' END
    OR terminal.wallet_user_id IS DISTINCT FROM snapshots.user_id
    OR terminal.related_transaction_id IS DISTINCT FROM snapshots.hold_transaction_id
    OR terminal.business_reference_type IS DISTINCT FROM 'task'
    OR terminal.business_reference_id IS DISTINCT FROM snapshots.task_id
  );

  INSERT INTO reconciliation_scan
  SELECT 'duplicate_business_reference', 'error', 'high', wallet_user_id, 'business_reference',
    business_reference_type || ':' || business_reference_id || ':' || command_type,
    jsonb_build_object('command_type', command_type, 'transaction_count', COUNT(*))
  FROM credit_transactions
  GROUP BY wallet_user_id, business_reference_type, business_reference_id, command_type HAVING COUNT(*) > 1;

  INSERT INTO reconciliation_scan
  SELECT 'correction_amount_exceeds_original', 'critical', 'high', original.wallet_user_id,
    'transaction', original.transaction_id,
    jsonb_build_object('original_command_type', original.command_type, 'original_amount', original.command_amount,
      'corrected_amount', SUM(corrections.command_amount))
  FROM credit_transactions original
  JOIN credit_transactions corrections ON corrections.command_type = 'correction'
    AND corrections.related_transaction_id = original.transaction_id
  GROUP BY original.transaction_id, original.wallet_user_id, original.command_type, original.command_amount, original.actor_type
  HAVING original.command_type NOT IN ('grant', 'deduction', 'refund')
    OR original.actor_type <> 'admin' OR SUM(corrections.command_amount) > original.command_amount;

  INSERT INTO reconciliation_scan
  SELECT 'refund_amount_invalid', 'critical', 'high', charges.wallet_user_id, 'charge', charges.capture_transaction_id,
    jsonb_build_object('charge_amount', charges.amount, 'projected_refunded_amount', charges.refunded_amount,
      'ledger_refunded_amount', COALESCE(refunds.amount, 0) - COALESCE(corrections.amount, 0))
  FROM credit_charges charges
  LEFT JOIN LATERAL (
    SELECT SUM(command_amount) AS amount FROM credit_transactions
    WHERE command_type = 'refund' AND related_transaction_id = charges.capture_transaction_id
  ) refunds ON TRUE
  LEFT JOIN LATERAL (
    SELECT SUM(correction.command_amount) AS amount
    FROM credit_transactions correction
    JOIN credit_transactions refund ON refund.transaction_id = correction.related_transaction_id
    WHERE correction.command_type = 'correction' AND refund.command_type = 'refund'
      AND refund.related_transaction_id = charges.capture_transaction_id
  ) corrections ON TRUE
  WHERE charges.refunded_amount < 0 OR charges.refunded_amount > charges.amount
    OR charges.refunded_amount <> COALESCE(refunds.amount, 0) - COALESCE(corrections.amount, 0);

  INSERT INTO reconciliation_scan
  SELECT 'illegal_reference_chain', 'critical', 'high', transactions.wallet_user_id, 'transaction', transactions.transaction_id,
    jsonb_build_object('command_type', transactions.command_type, 'related_transaction_id', transactions.related_transaction_id,
      'related_command_type', related.command_type, 'related_wallet_user_id', related.wallet_user_id)
  FROM credit_transactions transactions
  LEFT JOIN credit_transactions related ON related.transaction_id = transactions.related_transaction_id
  WHERE (transactions.command_type IN ('grant', 'deduction', 'hold', 'purchase_credit') AND transactions.related_transaction_id IS NOT NULL)
    OR (transactions.command_type IN ('capture', 'release') AND
      (related.transaction_id IS NULL OR related.command_type <> 'hold' OR related.wallet_user_id <> transactions.wallet_user_id))
    OR (transactions.command_type = 'refund' AND
      (related.transaction_id IS NULL OR related.command_type <> 'capture' OR related.wallet_user_id <> transactions.wallet_user_id))
    OR (transactions.command_type = 'correction' AND
      (related.transaction_id IS NULL OR related.command_type NOT IN ('grant', 'deduction', 'refund')
       OR related.actor_type <> 'admin' OR related.wallet_user_id <> transactions.wallet_user_id));

  INSERT INTO reconciliation_scan
  SELECT 'hold_state_invalid', 'critical', 'high', holds.wallet_user_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('hold_status', holds.status, 'hold_amount', holds.amount, 'hold_command', hold_tx.command_type,
      'terminal_command', terminal.command_type)
  FROM credit_holds holds
  LEFT JOIN credit_transactions hold_tx ON hold_tx.transaction_id = holds.hold_transaction_id
  LEFT JOIN credit_transactions terminal ON terminal.transaction_id = holds.terminal_transaction_id
  WHERE hold_tx.command_type IS DISTINCT FROM 'hold' OR hold_tx.wallet_user_id IS DISTINCT FROM holds.wallet_user_id
    OR hold_tx.command_amount IS DISTINCT FROM holds.amount
    OR (holds.status = 'held' AND holds.terminal_transaction_id IS NOT NULL)
    OR (holds.status = 'captured' AND (terminal.command_type IS DISTINCT FROM 'capture' OR terminal.related_transaction_id IS DISTINCT FROM holds.hold_transaction_id))
    OR (holds.status = 'released' AND (terminal.command_type IS DISTINCT FROM 'release' OR terminal.related_transaction_id IS DISTINCT FROM holds.hold_transaction_id));

  INSERT INTO reconciliation_scan
  SELECT 'hold_terminal_transaction_count', 'critical', 'high', holds.wallet_user_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('hold_status', holds.status, 'terminal_transaction_count', COUNT(terminals.transaction_id))
  FROM credit_holds holds
  LEFT JOIN credit_transactions terminals ON terminals.related_transaction_id = holds.hold_transaction_id
    AND terminals.command_type IN ('capture', 'release')
  GROUP BY holds.hold_transaction_id, holds.wallet_user_id, holds.status
  HAVING COUNT(terminals.transaction_id) <> CASE WHEN holds.status = 'held' THEN 0 ELSE 1 END;

  INSERT INTO reconciliation_scan
  SELECT 'charge_state_invalid', 'critical', 'high', charges.wallet_user_id, 'charge', charges.capture_transaction_id,
    jsonb_build_object('charge_amount', charges.amount, 'capture_command', capture.command_type, 'capture_amount', capture.command_amount)
  FROM credit_charges charges
  LEFT JOIN credit_transactions capture ON capture.transaction_id = charges.capture_transaction_id
  WHERE capture.command_type IS DISTINCT FROM 'capture' OR capture.wallet_user_id IS DISTINCT FROM charges.wallet_user_id
    OR capture.command_amount IS DISTINCT FROM charges.amount;

  WITH normalized AS (
    SELECT scan.*, encode(sha256(convert_to(concat_ws(E'\x1f', finding_type, COALESCE(wallet_user_id::TEXT, ''), subject_type, subject_id), 'UTF8')), 'hex') AS fingerprint
    FROM reconciliation_scan scan WHERE finding_type IS NOT NULL
  ), upserted AS (
    INSERT INTO credit_reconciliation_findings (
      fingerprint, finding_type, severity, risk, wallet_user_id, subject_type, subject_id, details,
      first_seen_run_id, last_seen_run_id
    ) SELECT fingerprint, finding_type, severity, risk, wallet_user_id, subject_type, subject_id, details, p_run_id, p_run_id
      FROM normalized
    ON CONFLICT (fingerprint) DO UPDATE SET
      severity = EXCLUDED.severity, risk = EXCLUDED.risk, wallet_user_id = EXCLUDED.wallet_user_id,
      details = EXCLUDED.details, status = 'open', isolated = FALSE, last_seen_run_id = p_run_id,
      last_seen_at = clock_timestamp(), occurrence_count = credit_reconciliation_findings.occurrence_count + 1,
      resolved_at = NULL
    RETURNING id
  ) INSERT INTO credit_reconciliation_run_findings (run_id, finding_id)
    SELECT p_run_id, id FROM upserted ON CONFLICT DO NOTHING;

  UPDATE credit_reconciliation_findings
  SET status = 'resolved', resolved_at = clock_timestamp()
  WHERE status = 'open' AND last_seen_run_id <> p_run_id;

  FOR v_finding IN
    SELECT restrictions.restriction_id::TEXT AS restriction_id,
           restrictions.wallet_user_id,
           findings.id AS finding_id
    FROM credit_wallet_restrictions restrictions
    JOIN credit_reconciliation_findings findings
      ON restrictions.related_object_type = 'finding'
     AND restrictions.related_object_id = findings.id::TEXT
    WHERE restrictions.restriction_type = 'ledger_integrity'
      AND restrictions.source = 'reconciliation'
      AND restrictions.released_at IS NULL
      AND (
        findings.status <> 'open'
        OR findings.risk <> 'high'
        OR findings.wallet_user_id IS NULL
        OR findings.wallet_user_id <> restrictions.wallet_user_id
        OR findings.last_seen_run_id <> p_run_id
      )
    ORDER BY restrictions.wallet_user_id, findings.id
  LOOP
    PERFORM release_credit_wallet_restriction(
      v_finding.restriction_id,
      'system',
      'credits-reconciliation',
      'finding no longer requires ledger integrity isolation'
    );
  END LOOP;

  FOR v_finding IN
    SELECT findings.id, findings.wallet_user_id, findings.finding_type
    FROM credit_reconciliation_findings findings
    WHERE findings.last_seen_run_id = p_run_id
      AND findings.status = 'open'
      AND findings.risk = 'high'
      AND findings.wallet_user_id IS NOT NULL
    ORDER BY findings.wallet_user_id, findings.id
  LOOP
    SELECT restriction_id INTO v_restriction_id
    FROM create_credit_wallet_restriction(
      v_finding.wallet_user_id,
      'ledger_integrity',
      'reconciliation',
      'finding',
      v_finding.id::TEXT,
      'high-risk reconciliation finding: ' || v_finding.finding_type,
      'system',
      'credits-reconciliation'
    );
  END LOOP;

  UPDATE credit_reconciliation_findings findings
  SET isolated = EXISTS (
    SELECT 1
    FROM credit_wallet_restrictions restrictions
    WHERE restrictions.wallet_user_id = findings.wallet_user_id
      AND restrictions.restriction_type = 'ledger_integrity'
      AND restrictions.source = 'reconciliation'
      AND restrictions.related_object_type = 'finding'
      AND restrictions.related_object_id = findings.id::TEXT
      AND restrictions.released_at IS NULL
  );

  UPDATE credit_reconciliation_run_findings links SET
    observed_severity = findings.severity,
    observed_risk = findings.risk,
    observed_details = findings.details,
    observed_status = findings.status,
    observed_isolated = findings.isolated,
    observed_occurrence_count = findings.occurrence_count,
    observed_first_seen_at = findings.first_seen_at,
    observed_last_seen_at = findings.last_seen_at,
    observed_resolved_at = findings.resolved_at
  FROM credit_reconciliation_findings findings
  WHERE links.run_id = p_run_id AND links.finding_id = findings.id;

  UPDATE credit_reconciliation_runs runs SET status = 'completed', completed_at = clock_timestamp(),
    finding_count = (SELECT COUNT(*) FROM credit_reconciliation_run_findings WHERE run_id = p_run_id),
    critical_count = (SELECT COUNT(*) FROM credit_reconciliation_findings WHERE last_seen_run_id = p_run_id AND severity = 'critical'),
    isolated_wallet_count = (SELECT COUNT(DISTINCT wallet_user_id) FROM credit_reconciliation_findings
      WHERE last_seen_run_id = p_run_id AND isolated)
  WHERE runs.id = p_run_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

-- statement:end

-- statement:begin
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_reconciliation(BIGINT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION execute_credit_reconciliation(BIGINT) FROM PUBLIC;
