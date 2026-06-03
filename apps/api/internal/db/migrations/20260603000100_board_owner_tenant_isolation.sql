-- migrate:up
ALTER TABLE user_boards DROP CONSTRAINT IF EXISTS uk_user_boards_board_id;
ALTER TABLE user_boards ADD CONSTRAINT uk_user_boards_user_board_id UNIQUE (user_id, board_id);

ALTER TABLE board_members ADD COLUMN IF NOT EXISTS owner_user_id BIGINT;

DELETE FROM board_members AS m
WHERE NOT EXISTS (
  SELECT 1
  FROM user_boards AS b
  WHERE b.board_id = m.board_id
);

UPDATE board_members AS m
SET owner_user_id = b.user_id
FROM user_boards AS b
WHERE m.board_id = b.board_id
  AND m.owner_user_id IS NULL;

ALTER TABLE board_members ALTER COLUMN owner_user_id SET NOT NULL;
ALTER TABLE board_members DROP CONSTRAINT IF EXISTS idx_board_member;
ALTER TABLE board_members ADD CONSTRAINT idx_board_member UNIQUE (owner_user_id, board_id, user_id);

DROP INDEX IF EXISTS idx_board_member_user;
CREATE INDEX idx_board_member_user ON board_members (user_id, owner_user_id, board_id);

-- migrate:down
DROP INDEX IF EXISTS idx_board_member_user;
ALTER TABLE board_members DROP CONSTRAINT IF EXISTS idx_board_member;
ALTER TABLE board_members ADD CONSTRAINT idx_board_member UNIQUE (board_id, user_id);
ALTER TABLE board_members DROP COLUMN IF EXISTS owner_user_id;

ALTER TABLE user_boards DROP CONSTRAINT IF EXISTS uk_user_boards_user_board_id;
ALTER TABLE user_boards ADD CONSTRAINT uk_user_boards_board_id UNIQUE (board_id);
CREATE INDEX idx_board_member_user ON board_members (user_id, board_id);
