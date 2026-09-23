-- Organization tenancy foundation (#801).
--
-- A personal Workspace exists without an Organization. An Organization owns
-- memberships and Workspaces. Public references must use public_id; the
-- internal_id bigint stays database-internal per the contract kernel.

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
