-- name: ListAdminBrandLogoOverrides :many
SELECT scope, target_key, logo_key, created_at, updated_at
FROM admin_brand_logo_overrides
ORDER BY scope ASC, target_key ASC
LIMIT 500;

-- name: UpsertAdminBrandLogoOverride :one
INSERT INTO admin_brand_logo_overrides (scope, target_key, logo_key)
VALUES (sqlc.arg(scope), sqlc.arg(target_key), sqlc.arg(logo_key))
ON CONFLICT (scope, target_key)
DO UPDATE SET logo_key = EXCLUDED.logo_key
RETURNING scope, target_key, logo_key, created_at, updated_at;

-- name: DeleteAdminBrandLogoOverride :execrows
DELETE FROM admin_brand_logo_overrides
WHERE scope = sqlc.arg(scope)
  AND target_key = sqlc.arg(target_key);
