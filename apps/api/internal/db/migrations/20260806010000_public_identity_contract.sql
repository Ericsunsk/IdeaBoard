-- migrate:up
ALTER TABLE users
  ADD COLUMN public_id UUID NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN display_name VARCHAR(100) DEFAULT 'IdeaBoard User';

UPDATE users
SET display_name = username;

ALTER TABLE users
  ALTER COLUMN display_name SET NOT NULL,
  ADD CONSTRAINT uq_users_public_id UNIQUE (public_id),
  ADD CONSTRAINT chk_users_display_name
    CHECK (length(btrim(display_name)) BETWEEN 1 AND 100);

-- migrate:down
ALTER TABLE users
  DROP CONSTRAINT IF EXISTS chk_users_display_name,
  DROP CONSTRAINT IF EXISTS uq_users_public_id,
  DROP COLUMN IF EXISTS display_name,
  DROP COLUMN IF EXISTS public_id;
