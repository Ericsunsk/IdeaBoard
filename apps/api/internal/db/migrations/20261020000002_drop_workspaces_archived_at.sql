-- destructive: approved
-- Drop workspaces.archived_at (ADR-0023 hard delete follow-up).
--
-- Deletion is an immediate irreversible hard delete: there is no
-- archive/restore lifecycle, no query selects this column and no write sets
-- it. Reads no longer filter on it (queries/workspaces.sql, webhook store
-- and tests updated in the same change). Pre-launch database: no backfill,
-- the column and the stale partial-index predicate leave together.
--
-- Order matters: the stale partial index predicates on archived_at, so it
-- is dropped first; dropping the column afterwards cannot cascade-drop
-- anything else.
DROP INDEX ux_workspaces_default_owner;

ALTER TABLE workspaces DROP COLUMN archived_at;

CREATE UNIQUE INDEX ux_workspaces_default_owner ON workspaces(owner_user_id) WHERE organization_id IS NULL AND is_default;
