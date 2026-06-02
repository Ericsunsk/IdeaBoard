-- name: CreateLineDraftMode :one
INSERT INTO line_draft_modes (name, cover_image, prompt, sort_order, is_enabled)
VALUES (sqlc.arg(name), sqlc.arg(cover_image), sqlc.arg(prompt), sqlc.arg(sort_order), sqlc.arg(is_enabled))
RETURNING id;

-- name: GetLineDraftModeByID :one
SELECT id, name, cover_image, prompt, sort_order, is_enabled, created_at, updated_at
FROM line_draft_modes
WHERE id = sqlc.arg(id);

-- name: ListLineDraftModes :many
SELECT id, name, cover_image, prompt, sort_order, is_enabled, created_at, updated_at
FROM line_draft_modes
ORDER BY sort_order ASC, id ASC;

-- name: ListEnabledLineDraftModes :many
SELECT id, name, cover_image, prompt, sort_order, is_enabled, created_at, updated_at
FROM line_draft_modes
WHERE is_enabled
ORDER BY sort_order ASC, id ASC;

-- name: UpdateLineDraftMode :exec
UPDATE line_draft_modes
SET name = sqlc.arg(name),
    cover_image = sqlc.arg(cover_image),
    prompt = sqlc.arg(prompt),
    sort_order = sqlc.arg(sort_order),
    is_enabled = sqlc.arg(is_enabled)
WHERE id = sqlc.arg(id);

-- name: DeleteLineDraftMode :exec
DELETE FROM line_draft_modes
WHERE id = sqlc.arg(id);
