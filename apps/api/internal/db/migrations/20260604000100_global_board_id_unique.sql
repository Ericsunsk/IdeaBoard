-- migrate:up
ALTER TABLE user_boards ADD CONSTRAINT uk_user_boards_board_id UNIQUE (board_id);

-- migrate:down
ALTER TABLE user_boards DROP CONSTRAINT IF EXISTS uk_user_boards_board_id;
