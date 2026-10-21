-- Consumption attribution survives deletion of the mutable task/workspace.
-- Historical snapshots with no remaining task retain unknown attribution.
ALTER TABLE task_billing_snapshots ADD COLUMN workspace_id BIGINT CHECK (workspace_id > 0);
ALTER TABLE task_billing_snapshots DISABLE TRIGGER task_billing_snapshots_immutable;
UPDATE task_billing_snapshots snapshots
SET workspace_id = tasks.workspace_id
FROM tasks
WHERE tasks.public_id = snapshots.task_id
  AND tasks.billing_account_id = snapshots.billing_account_id;
ALTER TABLE task_billing_snapshots ENABLE TRIGGER task_billing_snapshots_immutable;
CREATE INDEX idx_task_billing_snapshots_account_workspace_created
ON task_billing_snapshots (billing_account_id, workspace_id, created_at);
