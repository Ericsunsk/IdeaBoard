-- name: ListOutputsByTaskIDs :many
SELECT id, task_id, media_asset_id, role, url, storage_key, mime_type, width, height, size_bytes,
       source_url, checksum_sha256, storage_profile, metadata, sort_index, created_at, updated_at
FROM task_outputs
WHERE task_id = ANY(sqlc.arg(task_ids)::text[])
ORDER BY task_id ASC, sort_index ASC, id ASC;

-- name: DeleteOutputsByTaskID :exec
DELETE FROM task_outputs
WHERE task_id = sqlc.arg(task_id);

-- name: CreateTaskOutput :one
INSERT INTO task_outputs (
  task_id, media_asset_id, role, url, storage_key, mime_type, width, height, size_bytes,
  source_url, checksum_sha256, storage_profile, metadata, sort_index
) VALUES (
  sqlc.arg(task_id), sqlc.arg(media_asset_id), sqlc.arg(role), sqlc.arg(url), sqlc.arg(storage_key),
  sqlc.arg(mime_type), sqlc.arg(width), sqlc.arg(height), sqlc.arg(size_bytes),
  sqlc.arg(source_url), sqlc.arg(checksum_sha256), sqlc.arg(storage_profile), sqlc.arg(metadata),
  sqlc.arg(sort_index)
)
RETURNING id;
