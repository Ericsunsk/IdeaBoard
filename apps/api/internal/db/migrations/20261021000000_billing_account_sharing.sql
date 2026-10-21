-- Billing Account sharing foundation (#915).
--
-- A Billing Account holds the unified balance; many Workspaces may share one
-- account (organization workspaces share the organization account, no-org
-- studio spaces may explicitly link to the same studio account). Workspace
-- budgets and member quotas are consumption limits, not balances (945/946).
--
-- billing_account_members separates financial management (Owner/Admin) from
-- Workspace content membership. One User may manage many independent
-- accounts; ownership is transferable, never lifetime-bound.
-- workspace_billing_links is the explicit stable association: one Workspace
-- to one account, many Workspaces to one account allowed. Joining a
-- Workspace never merges personal funds.

CREATE TABLE billing_account_members (
  billing_account_id BIGINT NOT NULL REFERENCES billing_accounts(internal_id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('owner','admin')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (billing_account_id, user_id)
);

CREATE INDEX idx_billing_account_members_user ON billing_account_members (user_id, billing_account_id);

CREATE TABLE workspace_billing_links (
  workspace_id BIGINT PRIMARY KEY REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  billing_account_id BIGINT NOT NULL REFERENCES billing_accounts(internal_id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_workspace_billing_links_account ON workspace_billing_links (billing_account_id, workspace_id);
