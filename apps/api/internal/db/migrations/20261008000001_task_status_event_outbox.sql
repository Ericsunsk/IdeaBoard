-- Durable task status event outbox (#903). Every transition row is inserted
-- inside the same transaction as its tasks state change and replayed until
-- downstream delivery succeeds.
CREATE TABLE task_status_event_outbox (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  status TEXT NOT NULL,
  event_type TEXT NULL,
  event_version INT NULL,
  actor_type TEXT NOT NULL DEFAULT 'service',
  actor_id TEXT NOT NULL DEFAULT 'tasking',
  resource_type TEXT NOT NULL DEFAULT 'task',
  resource_id TEXT NOT NULL,
  event_data JSONB NOT NULL DEFAULT '{}'::jsonb,
  occurred_at TIMESTAMPTZ NOT NULL,
  status_state TEXT NOT NULL DEFAULT 'pending',
  attempts INT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL,
  delivered_at TIMESTAMPTZ NULL,
  last_error TEXT NOT NULL DEFAULT '',
  CONSTRAINT uq_task_status_event_outbox_event_id UNIQUE (event_id),
  CONSTRAINT chk_task_status_event_outbox_event_id CHECK (event_id LIKE 'evt\_%'),
  CONSTRAINT chk_task_status_event_outbox_status CHECK (status IN ('pending','running','completed','failed','cancelled')),
  CONSTRAINT chk_task_status_event_outbox_state CHECK (status_state IN ('pending','delivered')),
  CONSTRAINT chk_task_status_event_outbox_attempts CHECK (attempts >= 0),
  CONSTRAINT chk_task_status_event_outbox_workspace CHECK (length(btrim(workspace_id)) > 0),
  CONSTRAINT chk_task_status_event_outbox_task CHECK (length(btrim(task_id)) > 0),
  CONSTRAINT chk_task_status_event_outbox_data CHECK (jsonb_typeof(event_data) = 'object')
);

CREATE INDEX idx_task_status_event_outbox_pending
  ON task_status_event_outbox (next_attempt_at, internal_id)
  WHERE status_state = 'pending';
