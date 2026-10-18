CREATE TABLE tasks (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  submitter_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  billing_account_id BIGINT NOT NULL REFERENCES billing_accounts(internal_id) ON DELETE RESTRICT,
  service TEXT NOT NULL,
  workflow_input JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  priority INT NOT NULL DEFAULT 2,
  retry_count INT NOT NULL DEFAULT 0,
  lease_token TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at TIMESTAMPTZ NULL,
  completed_at TIMESTAMPTZ NULL,
  cancelled_at TIMESTAMPTZ NULL,
  cancelled_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT uq_tasks_public_id UNIQUE (public_id),
  -- Kernel prefix pin: mirrors contract.PrefixTask; the SQL CHECK and
  -- contract.IsTaskID must accept the same shape.
  CONSTRAINT chk_tasks_public_id CHECK (public_id LIKE 'task\_%'),
  CONSTRAINT chk_tasks_status CHECK (status IN ('pending','running','completed','failed','cancelled')),
  CONSTRAINT chk_tasks_priority CHECK (priority BETWEEN 1 AND 4),
  CONSTRAINT chk_tasks_retry_count CHECK (retry_count >= 0),
  CONSTRAINT chk_tasks_service CHECK (length(btrim(service)) BETWEEN 1 AND 64),
  CONSTRAINT chk_tasks_lease_status CHECK (status = 'running' OR lease_token = ''),
  CONSTRAINT chk_tasks_terminal_consistency CHECK (
    (status IN ('pending','running') AND completed_at IS NULL AND cancelled_at IS NULL)
    OR (status = 'completed' AND completed_at IS NOT NULL AND cancelled_at IS NULL)
    OR (status = 'failed' AND completed_at IS NOT NULL AND cancelled_at IS NULL)
    OR (status = 'cancelled' AND cancelled_at IS NOT NULL)
  ),
  CONSTRAINT chk_tasks_workflow_input CHECK (jsonb_typeof(workflow_input) = 'object')
);

CREATE INDEX idx_tasks_workspace ON tasks (workspace_id, created_at DESC);
CREATE INDEX idx_tasks_status ON tasks (status, created_at) WHERE status IN ('pending','running');
CREATE INDEX idx_tasks_submitter ON tasks (submitter_user_id, created_at DESC);
CREATE INDEX idx_tasks_billing ON tasks (billing_account_id, created_at DESC);

-- Durable submit idempotency fact for task:create. Written inside the
-- settlement transaction alongside Quote + Credit Hold + task insert + River
-- enqueue, so a committed claim always has its task row and a rolled-back
-- settlement leaves no claim behind.
--
-- Scope is (workspace_id, operation, idempotency_key): tasks are
-- workspace-scoped, and operation keeps "task:create" separable from future
-- submit-shaped operations sharing the client key namespace. Claims never
-- expire, matching the sibling durable facts
-- (uq_payment_order_idempotency, uq_reward_codes_creator_idempotency).
--
-- Not a revival of task_submission_claims (dropped in
-- 20260924000002): that table was user-scoped, carried no operation, and its
-- guard function JOINed tasks ON tasks.task_id, a column that does not exist
-- here, so every insert raised IB209. The deferrable FK below is the correct
-- form of that guard: the claim is written before the task row it names within
-- the same transaction, so the check must run at COMMIT.
CREATE TABLE task_submission_idempotency (
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  operation TEXT NOT NULL,
  idempotency_key VARCHAR(128) NOT NULL,
  request_hash VARCHAR(64) NOT NULL,
  task_public_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT task_submission_idempotency_pkey PRIMARY KEY (workspace_id, operation, idempotency_key),
  CONSTRAINT fk_task_submission_idempotency_task FOREIGN KEY (task_public_id)
    REFERENCES tasks(public_id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  CONSTRAINT ck_task_submission_idempotency_operation CHECK (char_length(btrim(operation)) BETWEEN 1 AND 64),
  CONSTRAINT ck_task_submission_idempotency_key CHECK (char_length(btrim(idempotency_key)) BETWEEN 1 AND 128),
  CONSTRAINT ck_task_submission_idempotency_request_hash CHECK (request_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT ck_task_submission_idempotency_task_public_id CHECK (char_length(btrim(task_public_id)) BETWEEN 1 AND 64)
);

-- Task outputs reference Media Assets or remote URLs without exposing storage keys.
CREATE TABLE task_outputs (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  task_id BIGINT NOT NULL REFERENCES tasks(internal_id) ON DELETE CASCADE,
  output_index INT NOT NULL,
  output_kind TEXT NOT NULL,
  media_asset_id BIGINT NULL REFERENCES media_assets(internal_id) ON DELETE RESTRICT,
  remote_url TEXT NULL,
  metadata JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_task_outputs_task_index UNIQUE (task_id, output_index),
  CONSTRAINT chk_task_outputs_index CHECK (output_index >= 0),
  CONSTRAINT chk_task_outputs_kind CHECK (output_kind IN ('media_asset','remote_url')),
  CONSTRAINT chk_task_outputs_kind_shape CHECK (
    (output_kind = 'media_asset' AND media_asset_id IS NOT NULL AND remote_url IS NULL)
    OR (output_kind = 'remote_url' AND media_asset_id IS NULL AND remote_url IS NOT NULL)
  ),
  CONSTRAINT chk_task_outputs_metadata CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_task_outputs_task ON task_outputs (task_id, output_index);
CREATE INDEX idx_task_outputs_media ON task_outputs (media_asset_id) WHERE media_asset_id IS NOT NULL;

-- Execution attribution records support the task monitor and worker query
-- surfaces. They are keyed by the workspace task identifiers and remain
-- separate from the task lifecycle fact stored in tasks.
CREATE TABLE task_execution_metrics (
  task_id VARCHAR(64) PRIMARY KEY,
  current_stage VARCHAR(32) NOT NULL DEFAULT 'queued',
  bottleneck_source VARCHAR(32) NOT NULL DEFAULT 'unknown',
  failure_stage VARCHAR(32) NOT NULL DEFAULT '',
  failure_category VARCHAR(64) NOT NULL DEFAULT '',
  assigned_at TIMESTAMPTZ(3),
  provider_started_at TIMESTAMPTZ(3),
  provider_finished_at TIMESTAMPTZ(3),
  finalize_started_at TIMESTAMPTZ(3),
  finalize_finished_at TIMESTAMPTZ(3),
  queue_wait_ms BIGINT NOT NULL DEFAULT 0,
  worker_overhead_ms BIGINT NOT NULL DEFAULT 0,
  provider_latency_ms BIGINT NOT NULL DEFAULT 0,
  finalize_latency_ms BIGINT NOT NULL DEFAULT 0,
  total_latency_ms BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  CONSTRAINT ck_task_execution_metrics_current_stage CHECK (current_stage IN ('queued', 'assigned', 'provider', 'finalizing', 'completed', 'failed', 'cancelled')),
  CONSTRAINT ck_task_execution_metrics_bottleneck_source CHECK (bottleneck_source IN ('queue', 'worker', 'provider', 'storage', 'finalize', 'input', 'unknown')),
  CONSTRAINT ck_task_execution_metrics_terminal_consistency CHECK (
    (current_stage = 'completed' AND failure_stage = '' AND failure_category = '')
    OR (current_stage = 'failed' AND btrim(failure_stage) <> '' AND btrim(failure_category) <> '')
    OR (current_stage = 'cancelled' AND btrim(failure_stage) <> '' AND failure_category = 'cancelled')
    OR current_stage IN ('queued', 'assigned', 'provider', 'finalizing')
  ),
  CONSTRAINT ck_task_execution_metrics_latency_non_negative CHECK (
    queue_wait_ms >= 0
    AND worker_overhead_ms >= 0
    AND provider_latency_ms >= 0
    AND finalize_latency_ms >= 0
    AND total_latency_ms >= 0
  )
);

CREATE INDEX idx_task_execution_metrics_current_stage ON task_execution_metrics (current_stage);
CREATE INDEX idx_task_execution_metrics_bottleneck_source ON task_execution_metrics (bottleneck_source);
CREATE INDEX idx_task_execution_metrics_failure_category ON task_execution_metrics (failure_category);

CREATE TABLE task_cancellations (
  task_id VARCHAR(64) PRIMARY KEY,
  expires_at TIMESTAMPTZ(3) NOT NULL,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now()
);

CREATE INDEX idx_task_cancellations_expires_at ON task_cancellations (expires_at);

CREATE TABLE task_execution_attempts (
  id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  task_id VARCHAR(64) NOT NULL,
  river_job_id BIGINT NOT NULL DEFAULT 0,
  attempt_no INTEGER NOT NULL,
  execution_token VARCHAR(128) NOT NULL,
  service VARCHAR(64) NOT NULL DEFAULT '',
  requested_model VARCHAR(128) NOT NULL DEFAULT '',
  resolved_model VARCHAR(128) NOT NULL DEFAULT '',
  provider VARCHAR(64) NOT NULL DEFAULT '',
  gateway_step VARCHAR(16) NOT NULL DEFAULT '',
  receipt_log_id VARCHAR(64) NOT NULL DEFAULT '',
  receipt_event_id VARCHAR(64) NOT NULL DEFAULT '',
  cache_hit BOOLEAN NOT NULL DEFAULT FALSE,
  status VARCHAR(20) NOT NULL DEFAULT 'running',
  failure_stage VARCHAR(32) NOT NULL DEFAULT '',
  failure_category VARCHAR(64) NOT NULL DEFAULT '',
  error_msg TEXT NOT NULL DEFAULT '',
  provider_started_at TIMESTAMPTZ(3),
  provider_finished_at TIMESTAMPTZ(3),
  completed_at TIMESTAMPTZ(3),
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  CONSTRAINT uq_task_execution_attempts_token UNIQUE (execution_token),
  CONSTRAINT uq_task_execution_attempts_task_attempt_no UNIQUE (task_id, attempt_no),
  CONSTRAINT ck_task_execution_attempts_token CHECK (btrim(execution_token) <> ''),
  CONSTRAINT ck_task_execution_attempts_attempt_no_positive CHECK (attempt_no > 0),
  CONSTRAINT ck_task_execution_attempts_ids_valid CHECK (river_job_id >= 0),
  CONSTRAINT ck_task_execution_attempts_status CHECK (status IN ('running', 'completed', 'failed', 'requeued', 'cancelled')),
  CONSTRAINT ck_task_execution_attempts_terminal_consistency CHECK (
    (
      status = 'running'
      AND completed_at IS NULL
      AND failure_stage = ''
      AND failure_category = ''
      AND error_msg = ''
    )
    OR (
      status IN ('completed', 'failed')
      AND completed_at IS NOT NULL
    )
    OR (
      status IN ('requeued', 'cancelled')
      AND completed_at IS NOT NULL
    )
  )
);

CREATE INDEX idx_task_execution_attempts_task_id ON task_execution_attempts (task_id);
CREATE INDEX idx_task_execution_attempts_status_created_at ON task_execution_attempts (status, created_at DESC);
CREATE INDEX idx_task_execution_attempts_running_created_at ON task_execution_attempts (created_at) WHERE status = 'running';
