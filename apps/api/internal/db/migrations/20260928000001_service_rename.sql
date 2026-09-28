-- Service rename, breaking clean cut (#847, ADR-0012).
--
-- `workflow_kind` (request/response) and the `feature` request/param fields
-- are renamed to `service` with no compatibility alias. The slug values
-- themselves (generation, inpaint, render, variations, cutout, fusion,
-- upscale, outpaint) are unchanged; only the identity column names move.
-- Single-value `feature -> model` settings mappings are removed from the
-- routing path in the same change (#848 introduces the Service Model Pool
-- and the RoutingSnapshot); the pool versioning and snapshot tables land
-- with #848, not here.
--
--   tasks.workflow_type -> tasks.service (canonical service slug per task)
--   task_billing_snapshots.feature_slug -> task_billing_snapshots.service
--     (billing stays keyed by Service + resolution + quantity)
--   task_execution_attempts.feature_slug -> task_execution_attempts.service
--     (durable attempt fact; retry-layer columns land with #848)
--
-- Deliberately untouched: task_daily_stats / task_stat_events /
-- task_daily_stat_backfills feature_slug (observability facts owned by
-- #852), feature_prompts.feature_slug (per-service prompt configuration =
-- execution semantics, not routing identity).
--
-- This clean break requires a fresh task store. Existing task rows contain
-- schema-v5 workflow_input JSON, while the new decoder accepts only v6.
-- Stop before renaming any columns rather than leave unreadable task facts.
-- Drain drawing queues and reset the non-production task store first.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM tasks) THEN
    RAISE EXCEPTION 'service rename requires an empty task store; reset old task data before migration';
  END IF;
END
$$;

ALTER TABLE tasks
  RENAME COLUMN workflow_type TO service;

ALTER TABLE tasks
  RENAME CONSTRAINT chk_tasks_workflow_type TO chk_tasks_service;

ALTER TABLE task_billing_snapshots
  RENAME COLUMN feature_slug TO service;

ALTER TABLE task_execution_attempts
  RENAME COLUMN feature_slug TO service;
