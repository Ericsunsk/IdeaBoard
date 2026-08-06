-- migrate:up
ALTER TABLE users
  DROP CONSTRAINT IF EXISTS uq_users_username,
  DROP COLUMN IF EXISTS username;

-- migrate:down
ALTER TABLE users
  ADD COLUMN username VARCHAR(50);

UPDATE users
SET username = 'user-' || id
WHERE username IS NULL;

ALTER TABLE users
  ALTER COLUMN username SET NOT NULL,
  ADD CONSTRAINT uq_users_username UNIQUE (username);
