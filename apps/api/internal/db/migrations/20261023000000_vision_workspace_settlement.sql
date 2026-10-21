-- Vision Search workspace settlement (#918).
--
-- Vision Search bills the consuming Workspace's stable Billing Account
-- (#915 resolution: explicit workspace_billing_links -> organization pool
-- -> per-workspace account, fail closed), never the operator's personal
-- wallet. Each operation persists the settlement triple:
-- submitter (user_id, the real initiator recorded as Actor on every
-- Command), consumption location (workspace_id), and payer
-- (billing_account_id). Hold/Capture/Release and background reconciliation
-- all converge on the stored payer; they never re-resolve from the live
-- Owner or membership.
--
-- workspace_id is a plain BIGINT with no FK: the operation row is a money
-- fact and is retained when the scope is hard-deleted (same precedent as
-- task_billing_snapshots.task_id, which carries no task FK). History rows
-- keep workspace_id NULL (personal vision before #918); new workspace
-- operations always carry it (enforced in code, not in DDL, so no
-- backfill rewrites immutable facts).
--
-- Idempotency is bound to account + resource scope: the new partial unique
-- (workspace_id, idempotency_key) fences same-key reuse inside one
-- workspace across operators, while the pre-existing
-- (user_id, idempotency_key) unique plus the code-level
-- workspace/account/request-hash verification rejects cross-workspace and
-- cross-account replays with idempotency conflict instead of replaying
-- another scope's operation.

ALTER TABLE ai_search_operations ADD COLUMN workspace_id BIGINT;

CREATE INDEX idx_ai_search_operations_workspace
  ON ai_search_operations (workspace_id, created_at DESC);

CREATE UNIQUE INDEX uq_ai_search_operations_workspace_key
  ON ai_search_operations (workspace_id, idempotency_key)
  WHERE workspace_id IS NOT NULL;
