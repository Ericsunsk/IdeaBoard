-- Account default new-member consumption quota (#947).
--
-- One row per Billing Account; absent means NOT CONFIGURED (fail closed:
-- invites are blocked until a Billing Owner/Admin sets the default, never
-- an implicit unlimited). The default seeds the per-member quota row on
-- first join of the account consumption scope (same five periods as #946:
-- daily / weekly / monthly UTC windows, fixed lifetime cap, or unlimited).
-- Changing the default only affects later joins; existing rows keep their
-- cap and accrued usage (per-member SetQuota). Rows and immutable
-- settlement history survive membership changes; only account deletion
-- cascades.

CREATE TABLE billing_account_default_member_quotas (
  billing_account_id BIGINT PRIMARY KEY REFERENCES billing_accounts(internal_id) ON DELETE CASCADE,
  period TEXT NOT NULL DEFAULT 'unlimited' CHECK (period IN ('daily', 'weekly', 'monthly', 'fixed', 'unlimited')),
  limit_credits BIGINT CHECK (limit_credits IS NULL OR (limit_credits > 0 AND limit_credits <= 9000000000000000)),
  updated_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT chk_default_member_quotas_period_limit CHECK (
    (period = 'unlimited' AND limit_credits IS NULL)
    OR (period IN ('daily', 'weekly', 'monthly', 'fixed') AND limit_credits IS NOT NULL AND limit_credits > 0 AND limit_credits <= 9000000000000000)
  )
);
