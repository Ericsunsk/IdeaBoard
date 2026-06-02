-- migrate:up
CREATE INDEX IF NOT EXISTS idx_tasks_running_account ON tasks (account_id) WHERE status = 'running';

CREATE TABLE IF NOT EXISTS task_cancellations (
  task_id VARCHAR(64) PRIMARY KEY,
  expires_at TIMESTAMPTZ(3) NOT NULL,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_task_cancellations_expires_at ON task_cancellations (expires_at);

DROP TRIGGER IF EXISTS set_task_cancellations_updated_at ON task_cancellations;
CREATE TRIGGER set_task_cancellations_updated_at BEFORE UPDATE ON task_cancellations FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- migrate:down
DROP TABLE IF EXISTS task_cancellations;
DROP INDEX IF EXISTS idx_tasks_running_account;
