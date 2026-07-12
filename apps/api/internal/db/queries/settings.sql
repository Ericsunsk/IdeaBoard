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

-- name: UpsertStorageProfile :exec
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
    deleted_at = CASE WHEN EXCLUDED.status = 'deleted' THEN COALESCE(storage_profiles.deleted_at, now()) ELSE NULL END;

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

-- name: EnsureFeaturePrompt :exec
INSERT INTO feature_prompts (feature_slug)
VALUES (sqlc.arg(feature_slug))
ON CONFLICT (feature_slug) DO NOTHING;

-- name: GetFeaturePromptForUpdate :one
SELECT feature_slug, current_revision_id, created_at, updated_at
FROM feature_prompts
WHERE feature_slug = sqlc.arg(feature_slug)
FOR UPDATE;

-- name: GetFeaturePrompt :one
SELECT
  p.feature_slug, p.current_revision_id,
  r.id AS revision_id, r.prompt, r.enabled, r.variables,
  r.model_defaults, r.user_prompt_merge_policy,
  r.schema_version, r.updated_by, r.published_at,
  p.created_at, p.updated_at, r.created_at AS revision_created_at
FROM feature_prompts AS p
JOIN feature_prompt_revisions AS r ON r.id = p.current_revision_id
WHERE p.feature_slug = sqlc.arg(feature_slug);

-- name: ListFeaturePrompts :many
SELECT
  p.feature_slug, p.current_revision_id,
  r.id AS revision_id, r.prompt, r.enabled, r.variables,
  r.model_defaults, r.user_prompt_merge_policy,
  r.schema_version, r.updated_by, r.published_at,
  p.created_at, p.updated_at, r.created_at AS revision_created_at
FROM feature_prompts AS p
JOIN feature_prompt_revisions AS r ON r.id = p.current_revision_id
ORDER BY p.feature_slug ASC;

-- name: CreateFeaturePromptRevision :one
WITH input AS (
  SELECT
    sqlc.arg(feature_slug)::varchar(64) AS feature_slug,
    sqlc.arg(prompt)::text AS prompt,
    sqlc.arg(enabled)::boolean AS enabled,
    sqlc.arg(variables)::jsonb AS variables,
    sqlc.arg(model_defaults)::jsonb AS model_defaults,
    sqlc.arg(user_prompt_merge_policy)::jsonb AS user_prompt_merge_policy,
    sqlc.narg(updated_by)::bigint AS updated_by,
    sqlc.narg(published_at)::timestamptz AS published_at
)
INSERT INTO feature_prompt_revisions (
  feature_slug, prompt, enabled, variables, model_defaults,
  user_prompt_merge_policy, schema_version, updated_by, published_at
)
SELECT
  input.feature_slug,
  input.prompt,
  input.enabled,
  input.variables,
  input.model_defaults,
  input.user_prompt_merge_policy,
  COALESCE((
    SELECT MAX(schema_version) + 1
    FROM feature_prompt_revisions
    WHERE feature_slug = input.feature_slug
  ), 1),
  input.updated_by,
  input.published_at
FROM input
RETURNING
  id, feature_slug, prompt, enabled, variables, model_defaults,
  user_prompt_merge_policy, schema_version, updated_by, published_at, created_at;

-- name: SetFeaturePromptCurrentRevision :one
UPDATE feature_prompts
SET current_revision_id = sqlc.arg(revision_id)
WHERE feature_slug = sqlc.arg(feature_slug)
RETURNING feature_slug, current_revision_id, created_at, updated_at;
