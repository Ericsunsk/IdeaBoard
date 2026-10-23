-- destructive: approved
-- Drop organizations.archived_at (dead column follow-up).
--
-- Closure (closed_at/closed_by) is the permanent lifecycle flag: no query
-- writes archived_at and every read treats it as always NULL (verified
-- SELECT count(*)=0 WHERE archived_at IS NOT NULL, and no index predicates
-- on the column). Pre-launch database: drop the column with no backfill.
ALTER TABLE organizations DROP COLUMN archived_at;
