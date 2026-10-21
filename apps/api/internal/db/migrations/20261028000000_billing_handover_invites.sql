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

CREATE UNIQUE INDEX uq_transfer_invites_pending_per_account
  ON billing_account_transfer_invites (billing_account_id)
  WHERE status = 'pending';

CREATE INDEX idx_transfer_invites_account_status
  ON billing_account_transfer_invites (billing_account_id, status, created_at DESC);

CREATE INDEX idx_transfer_invites_recipient
  ON billing_account_transfer_invites (to_user_id, status, created_at DESC);
