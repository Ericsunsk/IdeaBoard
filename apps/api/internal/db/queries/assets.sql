-- name: CreateLineDraftMode :execlastid
INSERT INTO line_draft_modes (name, cover_image, prompt, sort_order, is_enabled)
VALUES (?, ?, ?, ?, ?);

-- name: GetLineDraftModeByID :one
SELECT id, name, cover_image, prompt, sort_order, is_enabled, created_at, updated_at
FROM line_draft_modes
WHERE id = ?;

-- name: ListLineDraftModes :many
SELECT id, name, cover_image, prompt, sort_order, is_enabled, created_at, updated_at
FROM line_draft_modes
ORDER BY sort_order ASC, id ASC;

-- name: ListEnabledLineDraftModes :many
SELECT id, name, cover_image, prompt, sort_order, is_enabled, created_at, updated_at
FROM line_draft_modes
WHERE is_enabled = 1
ORDER BY sort_order ASC, id ASC;

-- name: UpdateLineDraftMode :exec
UPDATE line_draft_modes
SET name = ?,
    cover_image = ?,
    prompt = ?,
    sort_order = ?,
    is_enabled = ?
WHERE id = ?;

-- name: DeleteLineDraftMode :exec
DELETE FROM line_draft_modes
WHERE id = ?;
