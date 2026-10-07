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

-- Task status transition outbox (#903). The row is inserted in the same
-- transaction as the task state flip and replayed until every downstream
-- channel accepts the immutable event identity.
CREATE TABLE task_status_event_outbox (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  status TEXT NOT NULL,
  event_type TEXT NULL,
  event_version INT NULL,
  actor_type TEXT NOT NULL DEFAULT 'service',
  actor_id TEXT NOT NULL DEFAULT 'tasking',
  resource_type TEXT NOT NULL DEFAULT 'task',
  resource_id TEXT NOT NULL,
  event_data JSONB NOT NULL DEFAULT '{}'::jsonb,
  occurred_at TIMESTAMPTZ NOT NULL,
  status_state TEXT NOT NULL DEFAULT 'pending',
  attempts INT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL,
  delivered_at TIMESTAMPTZ NULL,
  last_error TEXT NOT NULL DEFAULT '',
  CONSTRAINT uq_task_status_event_outbox_event_id UNIQUE (event_id),
  CONSTRAINT chk_task_status_event_outbox_event_id CHECK (event_id LIKE 'evt\_%'),
  CONSTRAINT chk_task_status_event_outbox_status CHECK (status IN ('pending','running','completed','failed','cancelled')),
  CONSTRAINT chk_task_status_event_outbox_state CHECK (status_state IN ('pending','delivered')),
  CONSTRAINT chk_task_status_event_outbox_attempts CHECK (attempts >= 0),
  CONSTRAINT chk_task_status_event_outbox_workspace CHECK (length(btrim(workspace_id)) > 0),
  CONSTRAINT chk_task_status_event_outbox_task CHECK (length(btrim(task_id)) > 0),
  CONSTRAINT chk_task_status_event_outbox_data CHECK (jsonb_typeof(event_data) = 'object')
);

CREATE INDEX idx_task_status_event_outbox_pending
  ON task_status_event_outbox (next_attempt_at, internal_id)
  WHERE status_state = 'pending';

-- Webhook subscriptions with filtered event types and endpoint signing secrets.
-- signing_secret is encrypted at rest (AES-256-GCM via APP_SECRETS_ENCRYPTION_KEY):
-- material_kind='encrypted' rows hold ciphertext over the whsec_ transport string;
-- material_kind='plaintext' rows predate the encryption cutover and are accepted on
-- read only (converged to 'encrypted' by best-effort re-encrypt-on-read).
-- key_version fingerprints the encrypting master key generation for rotation detection.
CREATE TABLE event_subscriptions (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  endpoint_url TEXT NOT NULL,
  signing_secret TEXT NOT NULL,
  signing_secret_material_kind TEXT NOT NULL DEFAULT 'plaintext',
  signing_secret_key_version TEXT NOT NULL DEFAULT '',
  event_types TEXT[] NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_event_subscriptions_public_id UNIQUE (public_id),
  CONSTRAINT chk_event_subscriptions_public_id CHECK (public_id LIKE 'sub\_%'),
  CONSTRAINT chk_event_subscriptions_status CHECK (status IN ('active','paused','disabled')),
  CONSTRAINT chk_event_subscriptions_event_types CHECK (array_length(event_types, 1) > 0),
  CONSTRAINT chk_event_subscriptions_secret_material CHECK (signing_secret_material_kind IN ('plaintext', 'encrypted')),
  CONSTRAINT chk_event_subscriptions_secret_payload CHECK (
    (signing_secret_material_kind = 'encrypted' AND btrim(signing_secret) <> '')
    OR (signing_secret_material_kind = 'plaintext' AND signing_secret LIKE 'whsec\_%')
  )
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

-- A single event/subscription pair has one durable delivery aggregate.
CREATE UNIQUE INDEX ux_event_deliveries_event_subscription
  ON event_deliveries (event_id, subscription_id);

-- Per-attempt history remains readable after River retries, restart, or discard.
CREATE TABLE event_delivery_attempts (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  delivery_id BIGINT NOT NULL REFERENCES event_deliveries(internal_id) ON DELETE CASCADE,
  attempt_number INT NOT NULL,
  status_code INT NOT NULL DEFAULT 0,
  outcome TEXT NOT NULL,
  error_message TEXT NOT NULL DEFAULT '',
  attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_event_delivery_attempts_delivery_attempt UNIQUE (delivery_id, attempt_number),
  CONSTRAINT chk_event_delivery_attempts_number CHECK (attempt_number > 0),
  CONSTRAINT chk_event_delivery_attempts_outcome CHECK (outcome IN ('delivered', 'failed'))
);

CREATE INDEX idx_event_delivery_attempts_delivery
  ON event_delivery_attempts (delivery_id, attempt_number DESC);
