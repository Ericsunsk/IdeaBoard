-- Billing handover invite expiry (#948 follow-up, invite-style ownership transfer).
--
-- A pending invitation lapses 7 days after issue (Stripe/AWS/GitHub-Billing
-- mainstream: an unanswered invite never moves funds or membership, and the
-- owner may re-issue). Expiry is read from expires_at; rows stay 'pending'
-- while lapsed, so no status CHECK and no pending-index change is needed.
-- Accept/reject/cancel on a lapsed invite fail closed with
-- billing_account_conflict, and re-issue refreshes the pending row in place,
-- so the at-most-one-pending-per-account invariant is unchanged.
ALTER TABLE billing_account_transfer_invites
  ADD COLUMN expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '7 days');
