-- Y.Doc collaboration update log (ADR-0020). Appended by the realtime sidecar
-- via POST /internal/realtime/y-updates; trimmed by the compaction endpoint
-- after the snapshot lands in board_documents + board_revisions. See
-- internal/db/schema/fragments/boards.sql for the contract comment.
CREATE TABLE board_y_updates (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  board_id BIGINT NOT NULL REFERENCES boards(internal_id) ON DELETE CASCADE,
  author_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  update_data BYTEA NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT chk_board_y_updates_data CHECK (length(update_data) BETWEEN 1 AND 1048576)
);

CREATE INDEX idx_board_y_updates_board ON board_y_updates (board_id, internal_id);
