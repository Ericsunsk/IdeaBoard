-- Workspace tenancy foundation (#801).
--
-- A Workspace is owned by a User (personal Workspace, no Organization) or by
-- an Organization. Workspace membership and Board membership are separate
-- permission layers. Public references must use public_id; the internal_id
-- bigint stays database-internal per the contract kernel.

CREATE TABLE workspaces (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  organization_id BIGINT NULL REFERENCES organizations(internal_id),
  owner_user_id BIGINT NULL REFERENCES users(id),
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at TIMESTAMPTZ NULL,
  CONSTRAINT uq_workspaces_public_id UNIQUE (public_id),
  CONSTRAINT chk_workspaces_public_id CHECK (public_id LIKE 'ws\_%'),
  CONSTRAINT chk_workspaces_owner CHECK (organization_id IS NOT NULL OR owner_user_id IS NOT NULL)
);

-- Personal workspace idempotency: at most one live personal Workspace per user.
-- Archived personal Workspaces stay out of the way so reprovisioning works.
CREATE UNIQUE INDEX ux_workspaces_personal_owner ON workspaces(owner_user_id) WHERE organization_id IS NULL AND archived_at IS NULL;

CREATE TABLE workspace_memberships (
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('owner','admin','member')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, user_id)
);

CREATE INDEX idx_workspace_memberships_user ON workspace_memberships (user_id, workspace_id);

-- Billing Account owner stub (#801). The full Billing Account lifecycle
-- (identity, wallet binding, provider state) belongs to #804. This stub only
-- reserves the owner columns so workspace/organization work converges without
-- inventing the billing contract: a Billing Account is owned by a Workspace
-- (initial owner) or by an Organization.
CREATE TABLE billing_accounts (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  workspace_id BIGINT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  organization_id BIGINT NULL REFERENCES organizations(internal_id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT chk_billing_accounts_owner CHECK (workspace_id IS NOT NULL OR organization_id IS NOT NULL)
);

CREATE INDEX idx_billing_accounts_workspace ON billing_accounts (workspace_id) WHERE workspace_id IS NOT NULL;
CREATE INDEX idx_billing_accounts_organization ON billing_accounts (organization_id) WHERE organization_id IS NOT NULL;
