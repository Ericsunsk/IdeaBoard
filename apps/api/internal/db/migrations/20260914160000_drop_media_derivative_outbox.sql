-- destructive: approved
-- ADR-0007: display tiers resolve on demand through imgproxy, so the durable
-- derivative outbox (and the River worker that drained it) is gone. Only
-- processing-input still materializes, and it does so synchronously inside
-- ingestion completion, so it needs no queue.
DROP TABLE IF EXISTS media_derivative_outbox;
