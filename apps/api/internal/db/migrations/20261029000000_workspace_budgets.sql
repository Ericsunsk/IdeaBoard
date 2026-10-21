-- Workspace shared consumption budget (#945).
--
-- One row per workspace; absent means unlimited (the default). The budget
-- is a cumulative cap on settled + pending consumption in the current UTC
-- period, not a wallet, grant, or balance reservation. Adjusting the limit
-- never clears usage; a monthly reset starts a new UTC calendar month
-- without deleting history. Consumption attribution locks to the
-- submit-time period (task snapshot quoted_at / vision operation
-- created_at); linked consumption refunds correct the original period, so
-- a cross-period refund never grants current-period quota.

CREATE TABLE workspace_budgets (
  workspace_id BIGINT PRIMARY KEY REFERENCES workspaces(internal_id) ON DELETE CASCADE,
  period TEXT NOT NULL DEFAULT 'unlimited' CHECK (period IN ('monthly', 'unlimited')),
  limit_credits BIGINT CHECK (limit_credits IS NULL OR (limit_credits > 0 AND limit_credits <= 9000000000000000)),
  updated_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT chk_workspace_budgets_period_limit CHECK (
    (period = 'unlimited' AND limit_credits IS NULL)
    OR (period = 'monthly' AND limit_credits IS NOT NULL AND limit_credits > 0 AND limit_credits <= 9000000000000000)
  )
);
