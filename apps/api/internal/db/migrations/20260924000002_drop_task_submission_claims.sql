-- destructive: approved
-- #824 (#809 Phase 4b-R6): drop dormant task_submission_claims fact table and
-- its guard. guard_task_submission_claim() JOINs tasks ON tasks.task_id, a
-- column that does not exist on the workspace-scoped tasks table
-- (internal_id/public_id), so any insert into the table fails; repo-wide
-- grep shows zero queries and zero Go writers referencing the table. The
-- SubmissionTaskStore seam (PersistSubmissionClaim) stays for Phase 4a to
-- reintroduce a correct claim fact if needed.
DROP TRIGGER IF EXISTS task_submission_claims_guard ON task_submission_claims;
DROP TABLE IF EXISTS task_submission_claims;
DROP FUNCTION IF EXISTS guard_task_submission_claim();
