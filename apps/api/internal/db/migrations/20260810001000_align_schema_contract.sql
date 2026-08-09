-- migrate:up

ALTER TABLE payment_orders
  ADD CONSTRAINT ck_payment_order_return_base_url
  CHECK (char_length(return_base_url) BETWEEN 1 AND 2048);

REVOKE ALL ON FUNCTION prevent_credit_ledger_mutation() FROM PUBLIC;
REVOKE ALL ON FUNCTION guard_credit_account_projection() FROM PUBLIC;
REVOKE ALL ON FUNCTION validate_credit_transaction_balance() FROM PUBLIC;
REVOKE ALL ON FUNCTION guard_credit_posting_transaction_scope() FROM PUBLIC;
REVOKE ALL ON FUNCTION prevent_reward_admin_action_mutation() FROM PUBLIC;
REVOKE ALL ON FUNCTION guard_reward_code_identity() FROM PUBLIC;

-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.prevent_reward_admin_action_mutation() SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END;
$$
-- statement:end
