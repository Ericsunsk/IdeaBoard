-- migrate:up
CREATE TABLE IF NOT EXISTS task_execution_metrics (
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
  CONSTRAINT fk_task_execution_metrics_task_id FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_task_execution_metrics_current_stage ON task_execution_metrics (current_stage);
CREATE INDEX IF NOT EXISTS idx_task_execution_metrics_bottleneck_source ON task_execution_metrics (bottleneck_source);
CREATE INDEX IF NOT EXISTS idx_task_execution_metrics_failure_category ON task_execution_metrics (failure_category);

DROP TRIGGER IF EXISTS set_task_execution_metrics_updated_at ON task_execution_metrics;
CREATE TRIGGER set_task_execution_metrics_updated_at BEFORE UPDATE ON task_execution_metrics FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- migrate:down
DROP TRIGGER IF EXISTS set_task_execution_metrics_updated_at ON task_execution_metrics;
DROP TABLE IF EXISTS task_execution_metrics;
