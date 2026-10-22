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

-- Billing Account foundation (#804). A Billing Account is owned by exactly
-- one Workspace (initial owner) or exactly one Organization. The public_id
-- is the API-facing identifier per contract kernel (#800).
CREATE TABLE billing_accounts (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  workspace_id BIGINT NULL REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  organization_id BIGINT NULL REFERENCES organizations(internal_id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_billing_accounts_public_id UNIQUE (public_id),
  -- Kernel prefix pin: mirrors contract.PrefixBillingAccount.
  CONSTRAINT chk_billing_accounts_public_id CHECK (public_id LIKE 'ba\_%'),
  CONSTRAINT chk_billing_accounts_owner_xor CHECK (
    (workspace_id IS NOT NULL AND organization_id IS NULL)
    OR (workspace_id IS NULL AND organization_id IS NOT NULL)
  )
);

-- 1:1 relationship: each workspace has exactly one billing account
CREATE UNIQUE INDEX ux_billing_accounts_workspace ON billing_accounts (workspace_id) WHERE workspace_id IS NOT NULL;
CREATE UNIQUE INDEX ux_billing_accounts_organization ON billing_accounts (organization_id) WHERE organization_id IS NOT NULL;

-- Billing Account membership (#915). Financial management is separate from
-- Workspace content membership: a Billing Account has Owner/Admin managers,
-- while consumption stays gated by Workspace membership + Board capability.
-- One User may manage many independent accounts; balances never merge across
-- accounts. Ownership is transferable and never lifetime-bound to the
-- provisioning login identity.
CREATE TABLE billing_account_members (
  billing_account_id BIGINT NOT NULL REFERENCES billing_accounts(internal_id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  -- Role pin: mirrors tenancy.BillingRoleOwner/BillingRoleAdmin; keep the
  -- spellings identical to the tenancy kernel.
  role TEXT NOT NULL CHECK (role IN ('owner','admin')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (billing_account_id, user_id)
);

CREATE INDEX idx_billing_account_members_user ON billing_account_members (user_id, billing_account_id);

-- Explicit Workspace to Billing Account association (#915). One Workspace
-- resolves to exactly one Billing Account; many Workspaces may share one
-- account (organization workspaces share the organization account, no-org
-- studio spaces may explicitly link to the same studio account).
-- Independent accounts stay isolated: no row here means no sharing.
-- The link is explicit and stable: creating a new business space never
-- auto-creates a wallet, and joining a Workspace never merges personal funds.
CREATE TABLE workspace_billing_links (
  workspace_id BIGINT PRIMARY KEY REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  billing_account_id BIGINT NOT NULL REFERENCES billing_accounts(internal_id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_workspace_billing_links_account ON workspace_billing_links (billing_account_id, workspace_id);

-- Member cross-workspace consumption quota (#946).
--
-- One row per (Billing Account, User); absent means unlimited (the default).
-- The quota caps one member's settled + pending consumption across every
-- Workspace sharing the account (paid Task generations + Vision Search),
-- attributed to the real operator (snapshot/operation user_id). It is a
-- limit over the shared balance, never a sub-wallet, grant, or reservation:
-- adjusting the limit never clears usage, unused periodic quota never rolls
-- over, and joining/rejoining never re-issues (rows and history survive
-- membership changes; only user deletion cascades).
-- Consumption attribution locks to the submit-time period (task snapshot
-- quoted_at / vision operation created_at, both UTC); linked consumption
-- refunds correct the original period, so a cross-period refund returns to
-- the account balance without widening the current-period quota.
CREATE TABLE billing_account_member_quotas (
  billing_account_id BIGINT NOT NULL REFERENCES billing_accounts(internal_id) ON DELETE CASCADE,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  period TEXT NOT NULL DEFAULT 'unlimited' CHECK (period IN ('daily', 'weekly', 'monthly', 'fixed', 'unlimited')),
  limit_credits BIGINT CHECK (limit_credits IS NULL OR (limit_credits > 0 AND limit_credits <= 9000000000000000)),
  updated_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (billing_account_id, user_id),
  CONSTRAINT chk_member_quotas_period_limit CHECK (
    (period = 'unlimited' AND limit_credits IS NULL)
    OR (period IN ('daily', 'weekly', 'monthly', 'fixed') AND limit_credits IS NOT NULL AND limit_credits > 0 AND limit_credits <= 9000000000000000)
  )
);

CREATE INDEX idx_member_quotas_user ON billing_account_member_quotas (user_id, billing_account_id);

-- Every Billing Account owns exactly one Credit Wallet (#916): provisioning
-- follows account creation, never the current Workspace Owner's personal
-- wallet. The function lives in the Credits fragment; the trigger stays with
-- the account tables it fires on.
CREATE FUNCTION provision_billing_account_wallet_on_account_insert() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  PERFORM provision_billing_account_wallet(NEW.internal_id);
  RETURN NEW;
END;
$$;

CREATE TRIGGER provision_billing_account_wallet_after_account_insert
AFTER INSERT ON billing_accounts
FOR EACH ROW
EXECUTE FUNCTION provision_billing_account_wallet_on_account_insert();

REVOKE ALL ON FUNCTION provision_billing_account_wallet_on_account_insert() FROM PUBLIC;

-- ensure_personal_billing_account_for_user returns the deterministic initial
-- personal Billing Account for one user (#916): the per-workspace account of
-- the user's Default personal workspace, provisioning the Default workspace,
-- its owner membership, the billing account, and the billing owner grant on
-- demand. Every pre-916 personal balance, history row, and open Hold is
-- attributed to exactly this account by the migration; joining a Workspace
-- or Organization never merges funds here. Organization accounts are never
-- returned: personal money is never injected into an organization.
-- SQL-minted ws_/ba_ suffixes are hex and satisfy the kernel alphabet.
CREATE FUNCTION ensure_personal_billing_account_for_user(p_user_id BIGINT)
RETURNS BIGINT
LANGUAGE plpgsql SECURITY DEFINER
SET search_path TO 'public', 'pg_catalog', 'pg_temp'
AS $$
DECLARE
  v_workspace_id BIGINT;
  v_billing_account_id BIGINT;
BEGIN
  IF p_user_id <= 0 OR NOT EXISTS (SELECT 1 FROM users WHERE id = p_user_id) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid personal billing account owner';
  END IF;

  SELECT internal_id INTO v_workspace_id
  FROM workspaces
  WHERE owner_user_id = p_user_id AND organization_id IS NULL AND is_default
  LIMIT 1;
  IF NOT FOUND THEN
    FOR _ IN 1..8 LOOP
      BEGIN
        INSERT INTO workspaces (public_id, owner_user_id, name, is_default)
        VALUES ('ws_' || substr(md5(gen_random_uuid()::TEXT), 1, 12), p_user_id, 'My Workspace', TRUE)
        RETURNING workspaces.internal_id INTO v_workspace_id;
        EXIT;
      EXCEPTION WHEN unique_violation THEN
        SELECT internal_id INTO v_workspace_id
        FROM workspaces
        WHERE owner_user_id = p_user_id AND organization_id IS NULL AND is_default
        LIMIT 1;
        IF FOUND THEN
          EXIT;
        END IF;
      END;
    END LOOP;
    IF v_workspace_id IS NULL THEN
      RAISE EXCEPTION 'personal default workspace could not be provisioned';
    END IF;
  END IF;

  INSERT INTO workspace_memberships (workspace_id, user_id, role)
  VALUES (v_workspace_id, p_user_id, 'owner')
  ON CONFLICT (workspace_id, user_id) DO NOTHING;

  SELECT internal_id INTO v_billing_account_id
  FROM billing_accounts
  WHERE workspace_id = v_workspace_id
  LIMIT 1;
  IF NOT FOUND THEN
    FOR _ IN 1..8 LOOP
      BEGIN
        INSERT INTO billing_accounts (public_id, workspace_id, organization_id)
        VALUES ('ba_' || substr(md5(gen_random_uuid()::TEXT), 1, 12), v_workspace_id, NULL)
        RETURNING billing_accounts.internal_id INTO v_billing_account_id;
        EXIT;
      EXCEPTION WHEN unique_violation THEN
        SELECT internal_id INTO v_billing_account_id
        FROM billing_accounts
        WHERE workspace_id = v_workspace_id
        LIMIT 1;
        IF FOUND THEN
          EXIT;
        END IF;
      END;
    END LOOP;
    IF v_billing_account_id IS NULL THEN
      RAISE EXCEPTION 'personal billing account could not be provisioned';
    END IF;
  END IF;

  INSERT INTO billing_account_members (billing_account_id, user_id, role)
  VALUES (v_billing_account_id, p_user_id, 'owner')
  ON CONFLICT (billing_account_id, user_id) DO NOTHING;

  RETURN v_billing_account_id;
END;
$$;

REVOKE ALL ON FUNCTION ensure_personal_billing_account_for_user(BIGINT) FROM PUBLIC;

-- Studio handover invitations (#948, invite-style ownership transfer).
--
-- One row is one invitation: a Billing Owner initiates, the recipient
-- accepts or rejects, and the initiator may cancel while pending. The row
-- is the audit fact (created/decided timestamps plus decider) and survives
-- scope hard deletes: billing_account_id is a plain BIGINT with no FK (same
-- precedent as payment_orders.billing_account_id), so deleting a Workspace
-- never deletes the handover history; the ba_ snapshot stays readable.
-- Funds never move: acceptance only rewrites billing_account_members plus
-- non-Default Workspace memberships, never Wallet/Ledger/snapshot rows.
-- A pending invitation lapses 7 days after issue (expires_at): a lapsed
-- invite never moves funds or membership, decisions on it fail closed, and
-- the owner may re-issue (the pending row refreshes in place).
CREATE TABLE billing_account_transfer_invites (
  internal_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  public_id TEXT NOT NULL,
  billing_account_id BIGINT NOT NULL,
  billing_account_public_id TEXT NOT NULL,
  from_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  to_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'pending',
  idempotency_key TEXT NOT NULL,
  scope_workspace_public_ids TEXT[] NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '7 days'),
  decided_at TIMESTAMPTZ,
  decided_by BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
  CONSTRAINT uq_transfer_invites_public_id UNIQUE (public_id),
  CONSTRAINT chk_transfer_invites_public_id CHECK (public_id LIKE 'bti\_%'),
  CONSTRAINT chk_transfer_invites_account CHECK (billing_account_id > 0),
  CONSTRAINT chk_transfer_invites_account_public CHECK (billing_account_public_id LIKE 'ba\_%'),
  CONSTRAINT chk_transfer_invites_parties CHECK (from_user_id > 0 AND to_user_id > 0 AND from_user_id <> to_user_id),
  CONSTRAINT chk_transfer_invites_status CHECK (status IN ('pending', 'accepted', 'rejected', 'cancelled')),
  CONSTRAINT chk_transfer_invites_key CHECK (btrim(idempotency_key) <> ''),
  CONSTRAINT chk_transfer_invites_decided_shape CHECK (
    (status = 'pending' AND decided_at IS NULL AND decided_by IS NULL)
    OR (status IN ('accepted', 'rejected', 'cancelled') AND decided_at IS NOT NULL AND decided_by IS NOT NULL)
  ),
  CONSTRAINT uq_transfer_invites_account_idem UNIQUE (billing_account_id, idempotency_key)
);

-- At most one pending invitation per Billing Account: concurrent initiates
-- collapse instead of forking conflicting handovers.
CREATE UNIQUE INDEX uq_transfer_invites_pending_per_account
  ON billing_account_transfer_invites (billing_account_id)
  WHERE status = 'pending';

CREATE INDEX idx_transfer_invites_account_status
  ON billing_account_transfer_invites (billing_account_id, status, created_at DESC);

CREATE INDEX idx_transfer_invites_recipient
  ON billing_account_transfer_invites (to_user_id, status, created_at DESC);
