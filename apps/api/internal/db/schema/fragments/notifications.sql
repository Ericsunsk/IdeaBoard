-- User notifications center (#958, ADR-0040).
--
-- One row is one user notification. Notifications carry in-app alerts for
-- workspace invitations, studio/billing handover, billing credit low,
-- recharge success, and workspace member joined.
--
-- Status transitions:
--   unread -> read | archived

CREATE TABLE user_notifications (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  action_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  deduplication_key TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'unread',
  read_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_user_notifications_public_id UNIQUE (public_id),
  CONSTRAINT chk_user_notifications_public_id CHECK (public_id LIKE 'notif\_%'),
  CONSTRAINT chk_user_notifications_status CHECK (status IN ('unread', 'read', 'archived')),
  CONSTRAINT chk_user_notifications_type CHECK (length(btrim(type)) > 0),
  CONSTRAINT chk_user_notifications_title CHECK (length(btrim(title)) > 0),
  CONSTRAINT chk_user_notifications_action_payload CHECK (jsonb_typeof(action_payload) = 'object')
);

CREATE INDEX idx_user_notifications_user_status ON user_notifications (user_id, status, created_at DESC);
CREATE INDEX idx_user_notifications_user_created ON user_notifications (user_id, created_at DESC);

-- Durable event identity survives HTTP idempotency cache/process restarts.
CREATE UNIQUE INDEX uq_user_notifications_deduplication ON user_notifications (user_id, deduplication_key) WHERE deduplication_key <> '';
