-- Object bytes are copied and verified by the migrator before applying SQL.
UPDATE upload_sessions
SET storage_key = 'uploads/ws/' || substr(storage_key, length('ws-uploads/') + 1)
WHERE storage_key LIKE 'ws-uploads/%';

UPDATE media_assets
SET storage_key = 'uploads/ws/' || substr(storage_key, length('ws-uploads/') + 1)
WHERE storage_key LIKE 'ws-uploads/%';
