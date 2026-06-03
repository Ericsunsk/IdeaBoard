-- migrate:up
ALTER TABLE task_outputs
DROP CONSTRAINT IF EXISTS fk_task_outputs_task_id;

ALTER TABLE task_outputs
ADD CONSTRAINT fk_task_outputs_task_id
FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE;

-- migrate:down
ALTER TABLE task_outputs
DROP CONSTRAINT IF EXISTS fk_task_outputs_task_id;

ALTER TABLE task_outputs
ADD CONSTRAINT fk_task_outputs_task_id
FOREIGN KEY (task_id) REFERENCES tasks(task_id);
