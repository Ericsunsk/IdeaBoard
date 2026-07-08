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
INSERT INTO storage_profiles (
  profile_id, name, provider, enabled, is_current,
  builtin, status, config, created_by, updated_by
)
VALUES (
  sqlc.arg(profile_id), sqlc.arg(name), sqlc.arg(provider), sqlc.arg(enabled), sqlc.arg(is_current),
  sqlc.arg(builtin), sqlc.arg(status), sqlc.arg(config), sqlc.narg(created_by), sqlc.narg(updated_by)
)
ON CONFLICT (profile_id) DO UPDATE
SET name = EXCLUDED.name,
    enabled = EXCLUDED.enabled,
    is_current = EXCLUDED.is_current,
    builtin = EXCLUDED.builtin,
    status = EXCLUDED.status,
    config = EXCLUDED.config,
    updated_by = EXCLUDED.updated_by,
    deleted_at = CASE WHEN EXCLUDED.status = 'deleted' THEN COALESCE(storage_profiles.deleted_at, now()) ELSE NULL END;

-- name: GetStorageProfile :one
SELECT
  profile_id, name, provider, enabled, is_current, builtin,
  status, config, created_by, updated_by, created_at, updated_at, deleted_at
FROM storage_profiles
WHERE profile_id = sqlc.arg(profile_id);

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
INSERT INTO storage_profile_secret_versions (
  profile_id, secret_name, version_number, secret_material_kind, secret_ref, secret_payload,
  fingerprint, key_version, status, rotated_at, expires_at, retired_at, revoked_at
)
VALUES (
  sqlc.arg(profile_id),
  sqlc.arg(secret_name),
  COALESCE((
    SELECT MAX(version_number) + 1
    FROM storage_profile_secret_versions
    WHERE profile_id = sqlc.arg(profile_id)
      AND secret_name = sqlc.arg(secret_name)
  ), 1),
  sqlc.arg(secret_material_kind),
  sqlc.arg(secret_ref),
  sqlc.arg(secret_payload),
  sqlc.arg(fingerprint),
  sqlc.arg(key_version),
  'active',
  COALESCE(sqlc.narg(rotated_at), now()),
  sqlc.narg(expires_at),
  NULL,
  NULL
)
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

-- name: RevokeActiveStorageProfileSecretVersion :execrows
UPDATE storage_profile_secret_versions
SET status = 'revoked',
    revoked_at = now(),
    retired_at = NULL
WHERE profile_id = sqlc.arg(profile_id)
  AND secret_name = sqlc.arg(secret_name)
  AND status = 'active';

-- name: RestoreLatestRetiredStorageProfileSecretVersion :execrows
UPDATE storage_profile_secret_versions AS target_version
SET status = 'active',
    retired_at = NULL,
    revoked_at = NULL
WHERE id = (
    SELECT retired_version.id
    FROM storage_profile_secret_versions AS retired_version
    WHERE retired_version.profile_id = sqlc.arg(profile_id)
      AND retired_version.secret_name = sqlc.arg(secret_name)
      AND retired_version.status = 'retired'
    ORDER BY retired_version.version_number DESC
    LIMIT 1
  )
  AND NOT EXISTS (
    SELECT 1
    FROM storage_profile_secret_versions AS active_version
    WHERE active_version.profile_id = sqlc.arg(profile_id)
      AND active_version.secret_name = sqlc.arg(secret_name)
      AND active_version.status = 'active'
  );

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
INSERT INTO feature_prompt_revisions (
  feature_slug, prompt, enabled, variables, model_defaults,
  user_prompt_merge_policy, schema_version, updated_by, published_at
)
VALUES (
  sqlc.arg(feature_slug), sqlc.arg(prompt), sqlc.arg(enabled), sqlc.arg(variables),
  sqlc.arg(model_defaults), sqlc.arg(user_prompt_merge_policy),
  COALESCE((
    SELECT MAX(schema_version) + 1
    FROM feature_prompt_revisions
    WHERE feature_slug = sqlc.arg(feature_slug)
  ), 1),
  sqlc.narg(updated_by), sqlc.narg(published_at)
)
RETURNING
  id, feature_slug, prompt, enabled, variables, model_defaults,
  user_prompt_merge_policy, schema_version, updated_by, published_at, created_at;

-- name: SetFeaturePromptCurrentRevision :one
UPDATE feature_prompts
SET current_revision_id = sqlc.arg(revision_id)
WHERE feature_slug = sqlc.arg(feature_slug)
RETURNING feature_slug, current_revision_id, created_at, updated_at;
