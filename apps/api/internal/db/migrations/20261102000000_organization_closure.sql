-- Organization closure and restoration (#955).
--
-- Closure is a state flag, not a delete: closed_at/closed_by suspend all
-- member access and new spending while membership, Workspace, permission,
-- and billing history rows are retained. Only the Owner restores; restore
-- clears the flag and the retained rows resume (no rewrite, no fund move,
-- never into Personal).
ALTER TABLE organizations ADD COLUMN closed_at TIMESTAMPTZ NULL;
ALTER TABLE organizations ADD COLUMN closed_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;

-- Organization closure events (#955, append-only audit with snapshots).
--
-- One row is one closure: the audit fact (closed/restored stamps plus
-- actors and reason), the retained membership/Workspace snapshots restore
-- reinstates, and the fail-closed balance disposition (balances, credited
-- order refs, terms hook outcome). Money facts (Wallets, Ledger,
-- snapshots, payment orders/refunds/disputes) are never named for deletion
-- and survive closure and restoration by construction. At most one open
-- (unrestored) closure exists per Organization.
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
  -- Fail-closed disposition: recharge terms are out of scope, so unknown
  -- terms never trigger an original-channel refund nor a Personal
  -- transfer. Only retained exists until recharge terms land.
  disposition TEXT NOT NULL DEFAULT 'retained',
  disposition_detail JSONB NOT NULL DEFAULT '{}'::JSONB,
  restored_at TIMESTAMPTZ NULL,
  restored_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT chk_organization_closures_reason CHECK (char_length(reason) <= 280),
  CONSTRAINT chk_organization_closures_balances CHECK (available_balance >= 0 AND reserved_balance >= 0),
  CONSTRAINT chk_organization_closures_snapshots CHECK (
    jsonb_typeof(membership_snapshot) = 'array'
    AND jsonb_typeof(workspace_snapshot) = 'array'
    AND jsonb_typeof(credited_order_refs) = 'array'
    AND jsonb_typeof(disposition_detail) = 'object'
  ),
  CONSTRAINT chk_organization_closures_disposition CHECK (disposition = 'retained'),
  CONSTRAINT chk_organization_closures_restored_shape CHECK (
    (restored_at IS NULL AND restored_by IS NULL)
    OR (restored_at IS NOT NULL AND restored_by IS NOT NULL AND restored_at >= closed_at)
  )
);

-- At most one open closure per Organization: concurrent closes collapse
-- instead of forking conflicting audits.
CREATE UNIQUE INDEX uq_organization_closures_open_per_org
  ON organization_closures (organization_id)
  WHERE restored_at IS NULL;

CREATE INDEX idx_organization_closures_org
  ON organization_closures (organization_id, internal_id DESC);
