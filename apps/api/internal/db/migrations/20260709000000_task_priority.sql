-- migrate:up
ALTER TABLE tasks
  ADD COLUMN IF NOT EXISTS priority INTEGER NOT NULL DEFAULT 2;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'ck_tasks_priority'
      AND conrelid = 'tasks'::regclass
  ) THEN
    ALTER TABLE tasks
      ADD CONSTRAINT ck_tasks_priority CHECK (priority BETWEEN 1 AND 4);
  END IF;
END;
$$;

-- migrate:down
ALTER TABLE tasks
  DROP CONSTRAINT IF EXISTS ck_tasks_priority,
  DROP COLUMN IF EXISTS priority;
