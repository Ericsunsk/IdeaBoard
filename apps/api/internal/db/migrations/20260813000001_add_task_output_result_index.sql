DROP INDEX idx_task_outputs_task_sort;
ALTER TABLE task_outputs
  DROP CONSTRAINT ck_task_outputs_sort_index_non_negative,
  DROP COLUMN sort_index,
  ADD COLUMN result_index INTEGER NOT NULL DEFAULT 0,
  ADD CONSTRAINT ck_task_outputs_result_index_non_negative CHECK (result_index >= 0);
CREATE INDEX idx_task_outputs_task_sort ON task_outputs (task_id, result_index);
CREATE UNIQUE INDEX uq_task_outputs_task_result_index ON task_outputs (task_id, result_index);
