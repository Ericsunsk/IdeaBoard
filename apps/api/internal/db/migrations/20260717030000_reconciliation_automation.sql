-- migrate:up
DROP FUNCTION start_credit_reconciliation(BIGINT, BIGINT, TEXT);

ALTER TABLE credit_reconciliation_runs
  ADD COLUMN trigger_type VARCHAR(16),
  ADD COLUMN scheduled_for TIMESTAMPTZ(3);

UPDATE credit_reconciliation_runs
SET trigger_type = 'manual',
    stale_hold_after = INTERVAL '24 hours';

ALTER TABLE credit_reconciliation_runs
  ALTER COLUMN trigger_type SET NOT NULL,
  ALTER COLUMN requested_by DROP NOT NULL,
  ALTER COLUMN reason DROP NOT NULL,
  DROP CONSTRAINT credit_reconciliation_runs_requested_by_check,
  DROP CONSTRAINT ck_credit_reconciliation_run_reason,
  ADD CONSTRAINT ck_credit_reconciliation_run_trigger
    CHECK (trigger_type IN ('manual', 'scheduled')),
  ADD CONSTRAINT ck_credit_reconciliation_run_audit CHECK (
    (
      trigger_type = 'manual'
      AND requested_by IS NOT NULL
      AND requested_by > 0
      AND reason IS NOT NULL
      AND reason = btrim(reason, E' \t\n\r\f')
      AND char_length(reason) BETWEEN 1 AND 1000
      AND scheduled_for IS NULL
    )
    OR
    (
      trigger_type = 'scheduled'
      AND requested_by IS NULL
      AND reason IS NULL
      AND scheduled_for IS NOT NULL
    )
  ),
  ADD CONSTRAINT ck_credit_reconciliation_run_stale_hold_fixed
    CHECK (stale_hold_after = INTERVAL '24 hours');

CREATE UNIQUE INDEX uq_credit_reconciliation_scheduled_for
  ON credit_reconciliation_runs (scheduled_for)
  WHERE trigger_type = 'scheduled';

-- statement:begin
CREATE FUNCTION start_credit_reconciliation(
  p_trigger_type TEXT,
  p_requested_by BIGINT,
  p_reason TEXT,
  p_scheduled_for TIMESTAMPTZ,
  p_only_if_overdue BOOLEAN
)
RETURNS TABLE (run_id BIGINT, enqueue_required BOOLEAN) AS $$
DECLARE
  v_run_id BIGINT;
  v_run_status VARCHAR(16);
BEGIN
  p_trigger_type := btrim(p_trigger_type);
  p_reason := NULLIF(btrim(p_reason, E' \t\n\r\f'), '');
  IF p_trigger_type IS NULL
     OR p_only_if_overdue IS NULL
     OR (
       p_trigger_type = 'manual'
       AND (
         p_requested_by IS NULL OR p_requested_by <= 0
         OR p_reason IS NULL OR char_length(p_reason) > 1000
         OR p_scheduled_for IS NOT NULL
         OR p_only_if_overdue
       )
     )
     OR (
       p_trigger_type = 'scheduled'
       AND (
         p_requested_by IS NOT NULL
         OR p_reason IS NOT NULL
         OR p_scheduled_for IS NULL
       )
     )
     OR p_trigger_type NOT IN ('manual', 'scheduled') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB301', MESSAGE = 'invalid credit reconciliation request';
  END IF;

  PERFORM pg_advisory_xact_lock(5283921922134398277);

  IF p_only_if_overdue AND EXISTS (
    SELECT 1
    FROM credit_reconciliation_runs
    WHERE status = 'completed'
      AND completed_at >= clock_timestamp() - INTERVAL '24 hours'
  ) THEN
    RETURN QUERY SELECT NULL::BIGINT, FALSE;
    RETURN;
  END IF;

  SELECT runs.id INTO v_run_id
  FROM credit_reconciliation_runs runs
  WHERE runs.status IN ('queued', 'running')
  ORDER BY runs.id DESC
  LIMIT 1;
  IF FOUND THEN
    RETURN QUERY SELECT v_run_id, FALSE;
    RETURN;
  END IF;

  IF p_trigger_type = 'scheduled' THEN
    SELECT runs.id, runs.status INTO v_run_id, v_run_status
    FROM credit_reconciliation_runs runs
    WHERE runs.trigger_type = 'scheduled'
      AND runs.scheduled_for = p_scheduled_for;
    IF FOUND THEN
      IF p_only_if_overdue AND v_run_status = 'failed' THEN
        UPDATE credit_reconciliation_runs
        SET status = 'queued',
            error_message = '',
            started_at = NULL,
            completed_at = NULL
        WHERE id = v_run_id;
        RETURN QUERY SELECT v_run_id, TRUE;
        RETURN;
      END IF;
      RETURN QUERY SELECT v_run_id, FALSE;
      RETURN;
    END IF;
  END IF;

  INSERT INTO credit_reconciliation_runs (
    trigger_type,
    requested_by,
    reason,
    scheduled_for
  )
  VALUES (
    p_trigger_type,
    p_requested_by,
    p_reason,
    p_scheduled_for
  )
  RETURNING id INTO v_run_id;

  RETURN QUERY SELECT v_run_id, TRUE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format('ALTER FUNCTION %I.start_credit_reconciliation(TEXT, BIGINT, TEXT, TIMESTAMPTZ, BOOLEAN) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION start_credit_reconciliation(TEXT, BIGINT, TEXT, TIMESTAMPTZ, BOOLEAN) FROM PUBLIC;

-- migrate:down
DROP FUNCTION IF EXISTS start_credit_reconciliation(TEXT, BIGINT, TEXT, TIMESTAMPTZ, BOOLEAN);

-- statement:begin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM credit_reconciliation_runs WHERE trigger_type = 'scheduled'
  ) THEN
    RAISE EXCEPTION 'cannot roll back automated Reconciliation while scheduled Runs exist';
  END IF;
END;
$$;
-- statement:end

DROP INDEX IF EXISTS uq_credit_reconciliation_scheduled_for;

ALTER TABLE credit_reconciliation_runs
  DROP CONSTRAINT ck_credit_reconciliation_run_stale_hold_fixed,
  DROP CONSTRAINT ck_credit_reconciliation_run_audit,
  DROP CONSTRAINT ck_credit_reconciliation_run_trigger,
  DROP COLUMN scheduled_for,
  DROP COLUMN trigger_type,
  ALTER COLUMN requested_by SET NOT NULL,
  ALTER COLUMN reason SET NOT NULL,
  ADD CONSTRAINT credit_reconciliation_runs_requested_by_check CHECK (requested_by > 0),
  ADD CONSTRAINT ck_credit_reconciliation_run_reason CHECK (
    reason = btrim(reason, E' \t\n\r\f') AND char_length(reason) BETWEEN 1 AND 1000
  );

-- statement:begin
CREATE FUNCTION start_credit_reconciliation(
  p_requested_by BIGINT, p_stale_hold_seconds BIGINT, p_reason TEXT
)
RETURNS BIGINT AS $$
DECLARE v_run_id BIGINT;
BEGIN
  p_reason := btrim(p_reason, E' \t\n\r\f');
  IF p_requested_by IS NULL OR p_requested_by <= 0
     OR p_stale_hold_seconds IS NULL OR p_stale_hold_seconds < 60 OR p_stale_hold_seconds > 31536000
     OR p_reason IS NULL OR char_length(p_reason) NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB301', MESSAGE = 'invalid credit reconciliation request';
  END IF;
  PERFORM pg_advisory_xact_lock(5283921922134398277);
  SELECT id INTO v_run_id FROM credit_reconciliation_runs
   WHERE status IN ('queued', 'running') ORDER BY id DESC LIMIT 1;
  IF FOUND THEN RETURN v_run_id; END IF;
  INSERT INTO credit_reconciliation_runs (requested_by, stale_hold_after, reason)
  VALUES (p_requested_by, make_interval(secs => p_stale_hold_seconds::INTEGER), p_reason)
  RETURNING id INTO v_run_id;
  RETURN v_run_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format('ALTER FUNCTION %I.start_credit_reconciliation(BIGINT, BIGINT, TEXT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION start_credit_reconciliation(BIGINT, BIGINT, TEXT) FROM PUBLIC;
