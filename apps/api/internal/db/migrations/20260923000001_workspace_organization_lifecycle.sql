-- Workspace, Organization, and Principal lifecycle foundation (#801).
--
-- Fresh-replay safe: only CREATE TABLE / CREATE INDEX, no DROP of legacy
-- tables. Mirrors fragments/organization.sql and fragments/workspace.sql;
-- the schema-release catalog check enforces snapshot/migration equivalence.
CREATE TABLE organizations (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at TIMESTAMPTZ NULL,
  CONSTRAINT uq_organizations_public_id UNIQUE (public_id),
  CONSTRAINT chk_organizations_public_id CHECK (public_id LIKE 'org\_%')
);
CREATE TABLE organization_memberships (
  organization_id BIGINT NOT NULL REFERENCES organizations(internal_id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('owner','admin','member')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, user_id)
);
CREATE INDEX idx_organization_memberships_user ON organization_memberships (user_id, organization_id);
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
CREATE UNIQUE INDEX ux_workspaces_personal_owner ON workspaces(owner_user_id) WHERE organization_id IS NULL AND archived_at IS NULL;
CREATE TABLE workspace_memberships (
  workspace_id BIGINT NOT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('owner','admin','member')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX idx_workspace_memberships_user ON workspace_memberships (user_id, workspace_id);
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
