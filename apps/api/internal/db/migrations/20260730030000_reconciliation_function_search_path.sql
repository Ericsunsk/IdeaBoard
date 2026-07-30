-- migrate:up
-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  IF trusted_schema IS NULL THEN
    RAISE EXCEPTION 'cannot resolve trusted schema for credit reconciliation';
  END IF;
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_reconciliation(BIGINT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END;
$$;
-- statement:end

-- migrate:down
-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  IF trusted_schema IS NULL THEN
    RAISE EXCEPTION 'cannot resolve trusted schema for credit reconciliation';
  END IF;
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_reconciliation(BIGINT) SET search_path TO pg_catalog',
    trusted_schema
  );
END;
$$;
-- statement:end
