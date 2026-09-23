ALTER TABLE board_share_tokens
  ADD COLUMN idempotency_key VARCHAR(128) NOT NULL DEFAULT '',
  ADD COLUMN request_hash VARCHAR(64) NOT NULL DEFAULT '',
  ADD COLUMN token_secret TEXT NOT NULL DEFAULT '';

ALTER TABLE board_share_tokens
  ADD CONSTRAINT ck_board_share_tokens_idempotency
  CHECK ((idempotency_key = '' AND request_hash = '') OR (idempotency_key <> '' AND request_hash <> ''));

CREATE UNIQUE INDEX uq_board_share_tokens_creator_idempotency
  ON board_share_tokens (created_by, idempotency_key)
  WHERE idempotency_key <> '';
