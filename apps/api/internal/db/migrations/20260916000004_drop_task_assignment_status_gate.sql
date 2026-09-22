-- destructive: approved
-- ai-gateway T11: drop the retired assignment-status gate. The gateway
-- cutover (T5) claims running tasks with an execution token and no
-- account identity; the old CHECK rejected every such claim. The
-- all-or-nothing pair check stays for history sanity; lease fencing
-- (running OR empty token) is untouched.
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS ck_tasks_assignment_status;
