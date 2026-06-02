-- name: ListOutputsByTaskIDs :many
SELECT id, task_id, role, url, storage_key, mime_type, width, height, size_bytes,
       sort_index, created_at, updated_at
FROM task_outputs
WHERE task_id IN (sqlc.slice('task_ids'))
ORDER BY task_id ASC, sort_index ASC, id ASC;

-- name: DeleteOutputsByTaskID :exec
DELETE FROM task_outputs
WHERE task_id = ?;

-- name: CreateTaskOutput :execlastid
INSERT INTO task_outputs (
  task_id, role, url, storage_key, mime_type, width, height, size_bytes, sort_index
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
