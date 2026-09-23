-- destructive: approved
-- AccountPool retirement must erase credential material even on rows already
-- marked retired by an earlier migration.
UPDATE api_account_secrets
SET secret_payload = '',
    fingerprint = '',
    key_version = '',
    status = 'retired',
    updated_at = now()
WHERE secret_payload <> ''
   OR fingerprint <> ''
   OR key_version <> '';
