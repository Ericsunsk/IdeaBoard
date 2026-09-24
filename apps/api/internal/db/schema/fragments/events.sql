-- Workspace event streams and webhook delivery (#807). Events share one envelope
-- for SSE and signed webhooks. NATS JetStream is the transport, PostgreSQL tracks
-- delivery state for webhooks (retry, dead-letter, status).
CREATE TABLE workspace_events (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  event_version INT NOT NULL DEFAULT 1,
  actor_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  event_data JSONB NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_workspace_events_public_id UNIQUE (public_id),
  CONSTRAINT chk_workspace_events_public_id CHECK (public_id LIKE 'evt\_%'),
  CONSTRAINT chk_workspace_events_type CHECK (length(btrim(event_type)) > 0),
  CONSTRAINT chk_workspace_events_version CHECK (event_version > 0),
  CONSTRAINT chk_workspace_events_resource CHECK (length(btrim(resource_type)) > 0 AND length(btrim(resource_id)) > 0),
  CONSTRAINT chk_workspace_events_data CHECK (jsonb_typeof(event_data) = 'object')
);

CREATE INDEX idx_workspace_events_workspace ON workspace_events (workspace_id, occurred_at DESC);
CREATE INDEX idx_workspace_events_type ON workspace_events (event_type, occurred_at DESC);
CREATE INDEX idx_workspace_events_resource ON workspace_events (resource_type, resource_id, occurred_at DESC);

-- Webhook subscriptions with filtered event types and endpoint signing secrets.
CREATE TABLE event_subscriptions (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  endpoint_url TEXT NOT NULL,
  signing_secret TEXT NOT NULL,
  event_types TEXT[] NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_event_subscriptions_public_id UNIQUE (public_id),
  CONSTRAINT chk_event_subscriptions_public_id CHECK (public_id LIKE 'sub\_%'),
  CONSTRAINT chk_event_subscriptions_status CHECK (status IN ('active','paused','disabled')),
  CONSTRAINT chk_event_subscriptions_event_types CHECK (array_length(event_types, 1) > 0)
);

CREATE INDEX idx_event_subscriptions_workspace ON event_subscriptions (workspace_id, created_at DESC);
CREATE INDEX idx_event_subscriptions_status ON event_subscriptions (status) WHERE status = 'active';

-- Webhook delivery state with retry tracking and dead-letter parking.
CREATE TABLE event_deliveries (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_id BIGINT NOT NULL REFERENCES workspace_events(internal_id) ON DELETE CASCADE,
  subscription_id BIGINT NOT NULL REFERENCES event_subscriptions(internal_id) ON DELETE CASCADE,
  attempt_count INT NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',
  next_retry_at TIMESTAMPTZ NULL,
  last_attempted_at TIMESTAMPTZ NULL,
  completed_at TIMESTAMPTZ NULL,
  response_status INT NULL,
  response_body TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT chk_event_deliveries_attempt_count CHECK (attempt_count >= 0),
  CONSTRAINT chk_event_deliveries_status CHECK (status IN ('pending','retrying','delivered','failed','dead_letter')),
  CONSTRAINT chk_event_deliveries_completion CHECK (
    (status IN ('pending','retrying') AND completed_at IS NULL)
    OR (status IN ('delivered','failed','dead_letter') AND completed_at IS NOT NULL)
  )
);

CREATE INDEX idx_event_deliveries_subscription ON event_deliveries (subscription_id, created_at DESC);
CREATE INDEX idx_event_deliveries_retry ON event_deliveries (next_retry_at) WHERE status = 'retrying' AND next_retry_at IS NOT NULL;
CREATE INDEX idx_event_deliveries_status ON event_deliveries (status, created_at);
