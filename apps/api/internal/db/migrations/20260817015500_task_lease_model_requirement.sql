ALTER TABLE task_leases
ADD COLUMN model_requirement VARCHAR(16) NOT NULL DEFAULT 'required';

UPDATE task_leases
SET model_requirement = 'not-required'
WHERE btrim(resolved_model) = '';

ALTER TABLE task_leases
DROP CONSTRAINT ck_task_leases_snapshot;

ALTER TABLE task_leases
ADD CONSTRAINT ck_task_leases_snapshot CHECK (
  btrim(lease_token) <> ''
  AND btrim(api_type) <> ''
  AND btrim(provider_endpoint_protocol) <> ''
  AND btrim(provider_endpoint_base_url) <> ''
  AND (
    (model_requirement = 'required' AND btrim(resolved_model) <> '')
    OR (model_requirement = 'not-required' AND btrim(resolved_model) = '')
  )
);
