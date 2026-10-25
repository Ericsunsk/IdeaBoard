-- Workspace tenancy foundation (#801).
--
-- A Workspace is owned by a User (personal Workspace, no Organization) or by
-- an Organization. Every user additionally owns exactly one Default
-- personal workspace (is_default): it is never deleted or renamed,
-- and is auto-provisioned when missing. Workspace membership and
-- Board membership are separate permission layers. Public references must use
-- public_id; the internal_id bigint stays database-internal per the contract
-- kernel.

CREATE TABLE workspaces (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  organization_id BIGINT NULL REFERENCES organizations(internal_id),
  owner_user_id BIGINT NULL REFERENCES users(id),
  name TEXT NOT NULL,
  -- Default workspace marker (Bug2 root fix): every user owns exactly one
  -- Default personal workspace. Non-Default personal workspaces are
  -- unlimited; organization workspaces never carry the marker (see
  -- chk_workspaces_default_scope).
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_workspaces_public_id UNIQUE (public_id),
  -- Kernel prefix pin: mirrors contract.PrefixWorkspace (see also
  -- contract.IsWorkspaceID).
  CONSTRAINT chk_workspaces_public_id CHECK (public_id LIKE 'ws\_%'),
  CONSTRAINT chk_workspaces_owner_xor CHECK (
    (organization_id IS NOT NULL AND owner_user_id IS NULL)
    OR (organization_id IS NULL AND owner_user_id IS NOT NULL)
  ),
  CONSTRAINT chk_workspaces_default_scope CHECK (
    NOT is_default OR (organization_id IS NULL AND owner_user_id IS NOT NULL)
  )
);

-- Default workspace idempotency: at most one Default workspace per user
-- (deletion is hard, so no residual rows can collide with reprovisioning).
-- Plain personal workspaces are intentionally unlimited, so the legacy
-- ux_workspaces_personal_owner cap is gone.
CREATE UNIQUE INDEX ux_workspaces_default_owner ON workspaces(owner_user_id) WHERE organization_id IS NULL AND is_default;

CREATE TABLE workspace_memberships (
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  -- Role pin: mirrors tenancy.RoleOwner/RoleAdmin/RoleMember; keep the
  -- spellings identical to the tenancy kernel.
  role TEXT NOT NULL CHECK (role IN ('owner','admin','member')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, user_id)
);

CREATE INDEX idx_workspace_memberships_user ON workspace_memberships (user_id, workspace_id);

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
