-- migrate:up
-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.apply_credit_posting_balances() SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END $$;
-- statement:end

-- migrate:down
-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.apply_credit_posting_balances() SET search_path TO pg_catalog',
    trusted_schema
  );
END $$;
-- statement:end
