-- Board sharing uses one common role and explicit selected members.
-- destructive: approved
ALTER TABLE boards ADD COLUMN workspace_access text NOT NULL DEFAULT 'none';
ALTER TABLE boards ADD COLUMN access_version bigint NOT NULL DEFAULT 1;
ALTER TABLE boards ADD CONSTRAINT chk_boards_workspace_access CHECK (workspace_access IN ('none', 'viewer', 'editor'));
ALTER TABLE boards ADD CONSTRAINT chk_boards_access_version CHECK (access_version > 0);
DROP TRIGGER set_board_share_tokens_updated_at ON board_share_tokens;
DROP TABLE board_share_tokens;

DELETE FROM board_memberships;
ALTER TABLE board_memberships DROP COLUMN role;
