-- migrate:up

-- These SECURITY INVOKER trigger routines must resolve business tables through
-- the caller's schema. Hard-coding public breaks isolated schemas and routes
-- runtime-role reads to the wrong database objects.
ALTER FUNCTION guard_task_billing_settlement_transition() RESET search_path;
ALTER FUNCTION guard_task_submission_claim() RESET search_path;
ALTER FUNCTION validate_task_billing_snapshot_insert() RESET search_path;
