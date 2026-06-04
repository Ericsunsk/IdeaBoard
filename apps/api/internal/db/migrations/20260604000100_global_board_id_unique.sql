-- migrate:up
ALTER TABLE user_boards ADD CONSTRAINT uq_user_boards_board_id UNIQUE (board_id);

-- migrate:down
ALTER TABLE user_boards DROP CONSTRAINT IF EXISTS uq_user_boards_board_id;
