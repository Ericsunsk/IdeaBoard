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
  -- Kernel prefix pin: mirrors contract.PrefixOrganization.
  CONSTRAINT chk_organizations_public_id CHECK (public_id LIKE 'org\_%')
);

CREATE TABLE organization_memberships (
  organization_id BIGINT NOT NULL REFERENCES organizations(internal_id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  -- Role pin: mirrors tenancy.RoleOwner/RoleAdmin/RoleMember; keep the
  -- spellings identical to the tenancy kernel.
  role TEXT NOT NULL CHECK (role IN ('owner','admin','member')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, user_id)
);

CREATE INDEX idx_organization_memberships_user ON organization_memberships (user_id, organization_id);

-- Organization closure is permanent. Financial and content history remains
-- for audit; remaining credits cannot be spent, restored, or moved to Personal.
ALTER TABLE organizations ADD COLUMN closed_at TIMESTAMPTZ NULL;
ALTER TABLE organizations ADD COLUMN closed_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;

-- Immutable Organization closure audit. No restore lifecycle exists.
CREATE TABLE organization_closures (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  organization_id BIGINT NOT NULL REFERENCES organizations(internal_id) ON DELETE RESTRICT,
  billing_account_id BIGINT NOT NULL,
  closed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  closed_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
  reason TEXT NOT NULL DEFAULT '',
  membership_snapshot JSONB NOT NULL DEFAULT '[]'::JSONB,
  workspace_snapshot JSONB NOT NULL DEFAULT '[]'::JSONB,
  available_balance BIGINT NOT NULL DEFAULT 0,
  reserved_balance BIGINT NOT NULL DEFAULT 0,
  credited_order_refs JSONB NOT NULL DEFAULT '[]'::JSONB,
  disposition TEXT NOT NULL DEFAULT 'expired',
  disposition_detail JSONB NOT NULL DEFAULT '{}'::JSONB,
  CONSTRAINT chk_organization_closures_reason CHECK (char_length(reason) <= 280),
  CONSTRAINT chk_organization_closures_balances CHECK (available_balance >= 0 AND reserved_balance >= 0),
  CONSTRAINT chk_organization_closures_snapshots CHECK (
    jsonb_typeof(membership_snapshot) = 'array'
    AND jsonb_typeof(workspace_snapshot) = 'array'
    AND jsonb_typeof(credited_order_refs) = 'array'
    AND jsonb_typeof(disposition_detail) = 'object'
  ),
  CONSTRAINT chk_organization_closures_disposition CHECK (disposition = 'expired')
);

-- Each Organization can close only once.
CREATE UNIQUE INDEX uq_organization_closures_org
  ON organization_closures (organization_id);
