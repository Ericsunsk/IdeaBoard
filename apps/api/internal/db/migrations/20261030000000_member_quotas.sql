-- Member cross-workspace consumption quota (#946).
--
-- One row per (Billing Account, User); absent means unlimited (the
-- default). The quota caps one member's settled + pending consumption
-- across every Workspace sharing the account (paid Task generations +
-- Vision Search), attributed to the real operator. It is a limit over the
-- shared balance, never a sub-wallet, grant, or balance reservation:
-- adjusting the limit never clears usage, unused periodic quota never
-- rolls over, and joining/rejoining never re-issues (rows and immutable
-- settlement history survive membership changes). Consumption attribution
-- locks to the submit-time period (task snapshot quoted_at / vision
-- operation created_at, both UTC); linked consumption refunds correct the
-- original period, so a cross-period refund returns to the account balance
-- without widening the current-period quota.

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
