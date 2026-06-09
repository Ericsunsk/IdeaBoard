-- name: GetSystemConfig :one
SELECT id, config_key, value, created_at, updated_at
FROM system_configs
WHERE config_key = sqlc.arg(config_key);

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

-- name: UpsertStorageProfileSecret :exec
INSERT INTO storage_profile_secrets (
  profile_id, secret_name, secret_material_kind, secret_ref, secret_payload, fingerprint,
  key_version, status, rotated_at, expires_at, retired_at
)
VALUES (
  sqlc.arg(profile_id), sqlc.arg(secret_name), sqlc.arg(secret_material_kind), sqlc.arg(secret_ref), sqlc.arg(secret_payload), sqlc.arg(fingerprint),
  sqlc.arg(key_version), sqlc.arg(status), COALESCE(sqlc.narg(rotated_at), now()), sqlc.narg(expires_at), sqlc.narg(retired_at)
)
ON CONFLICT (profile_id, secret_name) WHERE status = 'active' DO UPDATE
SET secret_material_kind = EXCLUDED.secret_material_kind,
    secret_ref = EXCLUDED.secret_ref,
    secret_payload = EXCLUDED.secret_payload,
    fingerprint = EXCLUDED.fingerprint,
    key_version = EXCLUDED.key_version,
    status = EXCLUDED.status,
    rotated_at = CASE
      WHEN storage_profile_secrets.fingerprint IS DISTINCT FROM EXCLUDED.fingerprint THEN EXCLUDED.rotated_at
      ELSE storage_profile_secrets.rotated_at
    END,
    expires_at = EXCLUDED.expires_at,
    retired_at = EXCLUDED.retired_at;

-- name: GetStorageProfileSecret :one
SELECT
  id, profile_id, secret_name, secret_material_kind, secret_ref, secret_payload, fingerprint,
  key_version, status, rotated_at, expires_at, retired_at, created_at, updated_at
FROM storage_profile_secrets
WHERE profile_id = sqlc.arg(profile_id)
  AND secret_name = sqlc.arg(secret_name)
  AND status = 'active';

-- name: HasStorageProfileSecret :one
SELECT EXISTS (
  SELECT 1
  FROM storage_profile_secrets
  WHERE profile_id = sqlc.arg(profile_id)
    AND secret_name = sqlc.arg(secret_name)
    AND status = 'active'
    AND (btrim(secret_payload) <> '' OR btrim(secret_ref) <> '')
)::boolean;

-- name: UpsertFeaturePrompt :one
INSERT INTO feature_prompts (
  feature_slug, prompt, enabled, variables,
  schema_version, updated_by, published_at
)
VALUES (
  sqlc.arg(feature_slug), sqlc.arg(prompt), sqlc.arg(enabled), sqlc.arg(variables),
  1, sqlc.narg(updated_by), sqlc.narg(published_at)
)
ON CONFLICT (feature_slug) DO UPDATE
SET prompt = EXCLUDED.prompt,
    enabled = EXCLUDED.enabled,
    variables = EXCLUDED.variables,
    schema_version = feature_prompts.schema_version + 1,
    updated_by = EXCLUDED.updated_by,
    published_at = EXCLUDED.published_at
RETURNING schema_version;

-- name: GetFeaturePrompt :one
SELECT
  feature_slug, prompt, enabled, variables,
  schema_version, updated_by, published_at, created_at, updated_at
FROM feature_prompts
WHERE feature_slug = sqlc.arg(feature_slug);

-- name: ListFeaturePrompts :many
SELECT
  feature_slug, prompt, enabled, variables,
  schema_version, updated_by, published_at, created_at, updated_at
FROM feature_prompts
ORDER BY feature_slug ASC;

-- name: CreateFeaturePromptRevision :one
INSERT INTO feature_prompt_revisions (
  feature_slug, prompt, enabled, variables, schema_version, updated_by, published_at
)
VALUES (
  sqlc.arg(feature_slug), sqlc.arg(prompt), sqlc.arg(enabled), sqlc.arg(variables),
  sqlc.arg(schema_version), sqlc.narg(updated_by), sqlc.narg(published_at)
)
RETURNING id;

-- name: ListFeaturePromptRevisions :many
SELECT
  id, feature_slug, prompt, enabled, variables, schema_version, updated_by, published_at, created_at
FROM feature_prompt_revisions
WHERE feature_slug = sqlc.arg(feature_slug)
ORDER BY schema_version DESC, id DESC;
