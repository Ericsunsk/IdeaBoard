-- name: GetSystemConfig :one
SELECT id, config_key, value, created_at, updated_at
FROM system_configs
WHERE config_key = sqlc.arg(config_key);

-- name: ListSystemConfigsByKeys :many
SELECT id, config_key, value, created_at, updated_at
FROM system_configs
WHERE config_key = ANY(sqlc.arg(config_keys)::text[])
ORDER BY config_key ASC;

-- name: ListSystemConfigsByPrefix :many
SELECT id, config_key, value, created_at, updated_at
FROM system_configs
WHERE config_key LIKE sqlc.arg(config_key_like)
ORDER BY config_key ASC;

-- name: UpsertSystemConfig :exec
INSERT INTO system_configs (config_key, value)
VALUES (sqlc.arg(config_key), sqlc.arg(value))
ON CONFLICT (config_key) DO UPDATE
SET value = EXCLUDED.value;

-- name: DeleteSystemConfigsByKeys :exec
DELETE FROM system_configs
WHERE config_key = ANY(sqlc.arg(config_keys)::text[]);

-- name: LockAIGatewayCatalogWrite :exec
SELECT pg_advisory_xact_lock(
  hashtextextended('ai_gateway_catalog_write', 0)
);

-- name: UpsertStorageProfile :execrows
WITH input AS (
  SELECT
    sqlc.arg(profile_id)::varchar(64) AS profile_id,
    sqlc.arg(name)::varchar(100) AS name,
    sqlc.arg(provider)::varchar(50) AS provider,
    sqlc.arg(enabled)::boolean AS enabled,
    sqlc.arg(is_current)::boolean AS is_current,
    sqlc.arg(builtin)::boolean AS builtin,
    sqlc.arg(status)::varchar(32) AS status,
    sqlc.arg(config)::jsonb AS config,
    sqlc.narg(created_by)::bigint AS created_by,
    sqlc.narg(updated_by)::bigint AS updated_by
)
INSERT INTO storage_profiles (
  profile_id, name, provider, enabled, is_current,
  builtin, status, config, created_by, updated_by
)
SELECT
  input.profile_id,
  input.name,
  input.provider,
  input.enabled,
  input.is_current,
  input.builtin,
  input.status,
  input.config,
  input.created_by,
  input.updated_by
FROM input
ON CONFLICT (profile_id) DO UPDATE
SET name = EXCLUDED.name,
    enabled = EXCLUDED.enabled,
    is_current = EXCLUDED.is_current,
    builtin = EXCLUDED.builtin,
    status = EXCLUDED.status,
    config = EXCLUDED.config,
    updated_by = EXCLUDED.updated_by,
    deleted_at = CASE WHEN EXCLUDED.status = 'deleted' THEN COALESCE(storage_profiles.deleted_at, now()) ELSE NULL END
-- #809 hard cut: the locator-immutability guard used to consult
-- media_assets.storage_profile_id, which no longer exists in the
-- workspace model (the prevent_referenced_storage_profile_locator_change
-- trigger was removed with it). Only same-locator updates may proceed.
WHERE
    storage_profiles.provider = EXCLUDED.provider
    AND COALESCE(storage_profiles.config->>'endpoint', '') = COALESCE(EXCLUDED.config->>'endpoint', '')
    AND COALESCE(storage_profiles.config->>'bucket_name', '') = COALESCE(EXCLUDED.config->>'bucket_name', '')
    AND COALESCE(storage_profiles.config->>'region_id', '') = COALESCE(EXCLUDED.config->>'region_id', '')
    AND COALESCE(storage_profiles.config->>'force_path_style', 'false') = COALESCE(EXCLUDED.config->>'force_path_style', 'false');

-- name: ListStorageProfiles :many
SELECT
  profile_id, name, provider, enabled, is_current, builtin,
  status, config, created_by, updated_by, created_at, updated_at, deleted_at
FROM storage_profiles
WHERE (sqlc.arg(include_deleted)::boolean OR status <> 'deleted')
ORDER BY builtin DESC, created_at ASC, profile_id ASC;

-- name: ClearCurrentStorageProfiles :exec
UPDATE storage_profiles
SET is_current = FALSE
WHERE is_current;

-- name: MarkStorageProfileDeleted :execrows
UPDATE storage_profiles
SET enabled = FALSE,
    is_current = FALSE,
    status = 'deleted',
    deleted_at = COALESCE(deleted_at, now())
WHERE profile_id = sqlc.arg(profile_id)
  AND NOT builtin
  AND NOT is_current
  AND status <> 'deleted';

-- name: SetCurrentStorageProfile :execrows
UPDATE storage_profiles
SET is_current = TRUE,
    updated_by = sqlc.narg(updated_by)
WHERE profile_id = sqlc.arg(profile_id)
  AND enabled
  AND status = 'active';

-- name: LockStorageProfileSecretVersionRotation :one
SELECT TRUE::boolean AS locked
FROM (
  SELECT pg_advisory_xact_lock(
    hashtextextended(
      'storage_profile_secret_versions:' ||
        sqlc.arg(profile_id)::text ||
        ':' ||
        sqlc.arg(secret_name)::text,
      0
    )
  )
) AS storage_profile_secret_rotation_lock;

-- name: RetireActiveStorageProfileSecretVersion :execrows
UPDATE storage_profile_secret_versions
SET status = 'retired',
    retired_at = now()
WHERE profile_id = sqlc.arg(profile_id)
  AND secret_name = sqlc.arg(secret_name)
  AND status = 'active';

-- name: InsertStorageProfileSecretVersion :one
WITH input AS (
  SELECT
    sqlc.arg(profile_id)::varchar(64) AS profile_id,
    sqlc.arg(secret_name)::varchar(64) AS secret_name,
    sqlc.arg(secret_material_kind)::varchar(20) AS secret_material_kind,
    sqlc.arg(secret_ref)::text AS secret_ref,
    sqlc.arg(secret_payload)::text AS secret_payload,
    sqlc.arg(fingerprint)::varchar(128) AS fingerprint,
    sqlc.arg(key_version)::varchar(64) AS key_version,
    sqlc.narg(rotated_at)::timestamptz AS rotated_at,
    sqlc.narg(expires_at)::timestamptz AS expires_at
)
INSERT INTO storage_profile_secret_versions (
  profile_id, secret_name, version_number, secret_material_kind, secret_ref, secret_payload,
  fingerprint, key_version, status, rotated_at, expires_at, retired_at, revoked_at
)
SELECT
  input.profile_id,
  input.secret_name,
  COALESCE((
    SELECT MAX(version_number) + 1
    FROM storage_profile_secret_versions
    WHERE profile_id = input.profile_id
      AND secret_name = input.secret_name
  ), 1),
  input.secret_material_kind,
  input.secret_ref,
  input.secret_payload,
  input.fingerprint,
  input.key_version,
  'active',
  COALESCE(input.rotated_at, now()),
  input.expires_at,
  NULL,
  NULL
FROM input
RETURNING
  id, profile_id, secret_name, version_number, secret_material_kind, secret_ref, secret_payload,
  fingerprint, key_version, status, rotated_at, expires_at, retired_at, revoked_at, created_at, updated_at;

-- name: GetActiveStorageProfileSecretVersion :one
SELECT
  id, profile_id, secret_name, version_number, secret_material_kind, secret_ref, secret_payload,
  fingerprint, key_version, status, rotated_at, expires_at, retired_at, revoked_at, created_at, updated_at
FROM storage_profile_secret_versions
WHERE profile_id = sqlc.arg(profile_id)
  AND secret_name = sqlc.arg(secret_name)
  AND status = 'active';

-- name: HasActiveStorageProfileSecretVersion :one
SELECT EXISTS (
  SELECT 1
  FROM storage_profile_secret_versions
  WHERE profile_id = sqlc.arg(profile_id)
    AND secret_name = sqlc.arg(secret_name)
    AND status = 'active'
    AND (btrim(secret_payload) <> '' OR btrim(secret_ref) <> '')
)::boolean;

-- name: ListStorageProfileSecretVersions :many
SELECT
  id, profile_id, secret_name, version_number, secret_material_kind, secret_ref, secret_payload,
  fingerprint, key_version, status, rotated_at, expires_at, retired_at, revoked_at, created_at, updated_at
FROM storage_profile_secret_versions
WHERE profile_id = sqlc.arg(profile_id)
  AND secret_name = sqlc.arg(secret_name)
ORDER BY version_number DESC;

-- name: GetFeaturePrompt :one
SELECT feature_slug, prompt, updated_by, created_at, updated_at
FROM feature_prompts
WHERE feature_slug = sqlc.arg(feature_slug);

-- name: ListFeaturePrompts :many
SELECT feature_slug, prompt, updated_by, created_at, updated_at
FROM feature_prompts
ORDER BY feature_slug ASC;

-- name: UpsertFeaturePrompt :one
INSERT INTO feature_prompts (feature_slug, prompt, updated_by)
VALUES (sqlc.arg(feature_slug), sqlc.arg(prompt), sqlc.narg(updated_by))
ON CONFLICT (feature_slug) DO UPDATE SET
  prompt = EXCLUDED.prompt,
  updated_by = EXCLUDED.updated_by
RETURNING feature_slug, prompt, updated_by, created_at, updated_at;
