-- Board directed access-signal outbox (#940).
--
-- One row per affected user is recorded post-commit when an
-- access/membership change commits and replayed (hot log + NATS, never
-- webhooks) until the live emit completes. The evt_ id is deterministic
-- on (board, member, access_version) so live emits and replays converge
-- exactly. Pending rows are never pruned; delivered rows are audit trail
-- pruned past the NATS MaxAge window.
CREATE TABLE board_access_event_outbox (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  board_id TEXT NOT NULL,
  member_public_id TEXT NOT NULL,
  access_version BIGINT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  status_state TEXT NOT NULL DEFAULT 'pending',
  attempts INT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL,
  delivered_at TIMESTAMPTZ NULL,
  last_error TEXT NOT NULL DEFAULT '',
  CONSTRAINT uq_board_access_event_outbox_event_id UNIQUE (event_id),
  CONSTRAINT chk_board_access_event_outbox_event_id CHECK (event_id LIKE 'evt\_%'),
  CONSTRAINT chk_board_access_event_outbox_state CHECK (status_state IN ('pending','delivered')),
  CONSTRAINT chk_board_access_event_outbox_attempts CHECK (attempts >= 0),
  CONSTRAINT chk_board_access_event_outbox_workspace CHECK (workspace_id LIKE 'ws\_%'),
  CONSTRAINT chk_board_access_event_outbox_board CHECK (board_id LIKE 'board\_%'),
  CONSTRAINT chk_board_access_event_outbox_member CHECK (member_public_id LIKE 'usr\_%'),
  CONSTRAINT chk_board_access_event_outbox_version CHECK (access_version > 0)
);

CREATE INDEX idx_board_access_event_outbox_pending
  ON board_access_event_outbox (next_attempt_at, internal_id)
  WHERE status_state = 'pending';
CREATE INDEX idx_board_access_event_outbox_delivered
  ON board_access_event_outbox (delivered_at)
  WHERE status_state = 'delivered';
