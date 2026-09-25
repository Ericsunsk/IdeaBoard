-- Workspace board store columns (#809 follow-up to #802/#817).
--
-- boards.workspace_id is already the workspace link, so no separate link
-- table is introduced; board_revisions is already INSERT-only immutable.
-- This migration only adds what the WorkspaceBoardStore postgres
-- implementation needs beyond the baseline:
--   boards.owner_user_id: implicit owner (creator). Nullable so the
--     migration applies where boards rows already exist; the store sets
--     it on every insert and reads owner-less rows as not found.
--   board_share_tokens.token_id: opaque sht_ bearer id for {token_id}
--     segments (public_id stays the bst_ row id).
--   board_share_tokens.token_prefix: list-safe bearer prefix.
--   board_share_tokens.usage_count: join counter (durable audit is #808).
-- Only the bearer hash is stored in token_secret (never the bearer), so
-- share secrets require no reversible encryption at rest.

ALTER TABLE boards
  ADD COLUMN owner_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT;

ALTER TABLE board_share_tokens
  ADD COLUMN token_id TEXT NULL,
  ADD COLUMN token_prefix TEXT NOT NULL DEFAULT '',
  ADD COLUMN usage_count BIGINT NOT NULL DEFAULT 0;

ALTER TABLE board_share_tokens
  ADD CONSTRAINT uq_board_share_tokens_token_id UNIQUE (token_id),
  ADD CONSTRAINT chk_board_share_tokens_token_id CHECK (token_id IS NULL OR token_id LIKE 'sht\_%'),
  ADD CONSTRAINT chk_board_share_tokens_usage_count CHECK (usage_count >= 0);

COMMENT ON COLUMN boards.owner_user_id IS 'Implicit board owner (creator); the creator never needs a board_memberships row.';
COMMENT ON COLUMN board_share_tokens.token_id IS 'Opaque sht_ bearer id exposed in {token_id} URL segments; public_id stays the bst_ row id.';
COMMENT ON COLUMN board_share_tokens.token_secret IS 'SHA-256 hex of the bearer; the bearer itself is never stored.';

-- The set_board_memberships_updated_at / set_board_share_tokens_updated_at
-- triggers (integrity fragment) assign NEW.updated_at, but the baseline
-- tables never had that column, so every UPDATE on these tables failed
-- with `record "new" has no field "updated_at"`. The legacy Member /
-- ShareToken shapes both carry UpdatedAt: restore the intended columns.
ALTER TABLE board_memberships
  ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE board_share_tokens
  ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
