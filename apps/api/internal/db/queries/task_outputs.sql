-- name: ListOutputsByTaskIDs :many
SELECT
  o.id,
  o.task_id,
  o.media_asset_id,
  o.output_kind,
  o.role,
  (CASE WHEN o.output_kind = 'remote' THEN o.url ELSE COALESCE(a.url, '') END)::text AS url,
  COALESCE(a.storage_key, '') AS storage_key,
  COALESCE(a.mime_type, '') AS mime_type,
  COALESCE(a.width, 0)::int AS width,
  COALESCE(a.height, 0)::int AS height,
  COALESCE(a.size_bytes, 0)::bigint AS size_bytes,
  o.source_url,
  COALESCE(a.checksum_sha256, '') AS checksum_sha256,
  COALESCE(a.storage_profile_id, '') AS storage_profile,
  o.metadata,
  o.result_index,
  o.created_at,
  o.updated_at
FROM task_outputs AS o
LEFT JOIN media_assets AS a ON a.id = o.media_asset_id
WHERE o.task_id = ANY(sqlc.arg(task_ids)::text[])
ORDER BY o.task_id ASC, o.result_index ASC, o.id ASC;

-- name: DeleteOutputsByTaskID :exec
DELETE FROM task_outputs
WHERE task_id = sqlc.arg(task_id);

-- name: CreateTaskOutput :one
INSERT INTO task_outputs (
  task_id, media_asset_id, output_kind, role, url,
  source_url, metadata, result_index
) VALUES (
  sqlc.arg(task_id), sqlc.arg(media_asset_id), sqlc.arg(output_kind), sqlc.arg(role), sqlc.arg(url),
  sqlc.arg(source_url), sqlc.arg(metadata), sqlc.arg(result_index)
)
RETURNING id;
