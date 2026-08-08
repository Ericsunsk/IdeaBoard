-- migrate:up
DROP TABLE IF EXISTS line_draft_modes;
DROP SEQUENCE IF EXISTS line_draft_modes_id_seq;

-- migrate:down
-- The retired Line Draft Modes feature is intentionally not restored.
SELECT 1;
