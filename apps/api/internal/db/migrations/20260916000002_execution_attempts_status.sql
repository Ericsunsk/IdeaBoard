-- ai-gateway T5: execution attempts keep the full terminal vocabulary so
-- requeue/cancel history stays distinguishable for triage.
ALTER TABLE task_execution_attempts DROP CONSTRAINT ck_task_execution_attempts_status;
ALTER TABLE task_execution_attempts ADD CONSTRAINT ck_task_execution_attempts_status CHECK (status IN ('running', 'completed', 'failed', 'requeued', 'cancelled'));
