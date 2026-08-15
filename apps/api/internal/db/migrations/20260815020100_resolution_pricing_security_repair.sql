-- Repair the security ACL of the resolution-aware billing commands.
-- The preceding migration may already be recorded before final routine verification runs.

REVOKE ALL ON FUNCTION delete_archived_billing_item(TEXT, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION publish_billing_prices(TEXT, TEXT, TEXT, JSONB, BIGINT) FROM PUBLIC;
