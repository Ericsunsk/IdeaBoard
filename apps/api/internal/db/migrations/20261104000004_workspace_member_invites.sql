-- Workspace member invitation confirmation (#957).
--
-- One row is one invitation: an authorized workspace member (Owner/Admin)
-- initiates an invite for an email address, generating a random 32-byte crypto
-- token whose SHA-256 hash is stored in token_hash. The invitation expires
-- in 7 days.
--
-- Status transitions:
--   pending -> accepted | declined | cancelled | expired
--
-- Invariants:
--   (current_active_members + pending_invites) <= 5
--   At most one pending invitation per email per workspace.

CREATE TABLE workspace_member_invites (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  inviter_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  invitee_email TEXT NOT NULL,
  invitee_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
  role TEXT NOT NULL DEFAULT 'member',
  status TEXT NOT NULL DEFAULT 'pending',
  token_hash TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '7 days'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  responded_at TIMESTAMPTZ NULL,
  responded_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT uq_workspace_member_invites_public_id UNIQUE (public_id),
  CONSTRAINT chk_workspace_member_invites_public_id CHECK (public_id LIKE 'wsi\_%'),
  CONSTRAINT chk_workspace_member_invites_email CHECK (btrim(invitee_email) <> '' AND invitee_email = lower(invitee_email)),
  CONSTRAINT chk_workspace_member_invites_role CHECK (role IN ('admin', 'member')),
  CONSTRAINT chk_workspace_member_invites_status CHECK (status IN ('pending', 'accepted', 'declined', 'cancelled', 'expired')),
  CONSTRAINT uq_workspace_member_invites_token_hash UNIQUE (token_hash),
  CONSTRAINT chk_workspace_member_invites_token_hash CHECK (length(token_hash) = 64),
  CONSTRAINT chk_workspace_member_invites_response_shape CHECK (
    (status = 'pending' AND responded_at IS NULL AND responded_by_user_id IS NULL)
    OR (status IN ('accepted', 'declined', 'cancelled') AND responded_at IS NOT NULL)
    OR (status = 'expired')
  )
);

CREATE UNIQUE INDEX uq_workspace_member_invites_pending_email
  ON workspace_member_invites (workspace_id, invitee_email)
  WHERE status = 'pending';

CREATE INDEX idx_workspace_member_invites_ws_status
  ON workspace_member_invites (workspace_id, status, created_at DESC);

CREATE INDEX idx_workspace_member_invites_email_status
  ON workspace_member_invites (invitee_email, status, created_at DESC);

CREATE INDEX idx_workspace_member_invites_invitee_user
  ON workspace_member_invites (invitee_user_id, status, created_at DESC)
  WHERE invitee_user_id IS NOT NULL;
