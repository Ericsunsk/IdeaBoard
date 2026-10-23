-- destructive: approved
-- Retire publish_billing_price (single-resolution command).
--
-- All catalog publishes go through publish_billing_prices (JSONB
-- multi-resolution, 1k-required backfill); no caller references the
-- single-price form anymore. Pre-launch database: drop with no compat shim.
DROP FUNCTION publish_billing_price(TEXT, TEXT, TEXT, BIGINT, BIGINT);
