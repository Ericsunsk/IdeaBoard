-- Object bytes are copied and verified by the migrator before applying SQL.
UPDATE users
SET avatar = regexp_replace(avatar, '^/uploads/profile/([^/]+)/avatar/(.+)$', 'uploads/avatars/\1/\2')
WHERE avatar LIKE '/uploads/profile/%';
