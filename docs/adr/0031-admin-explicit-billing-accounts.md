# ADR-0031: Admin Operates Explicit Billing Accounts

## Context

Issue #923 puts the Admin/Reconciliation operations surface on real funds
accounts. After #915–#921, every funds command is account-keyed and the
domain holds per-account isolation (restriction create/release precision,
cross-account settlement rejection, sole-owned lifecycle freeze), but the
Control Plane still addressed everything through `user_id`: fund commands
resolved only the target user's deterministic initial personal account, the
account list resolved a single workspace, wallet/restriction views attached a
single-user linkage, and Findings hid the internal `billing_account_id` from
the wire. A platform Admin could neither locate/audit a shared or
organization account nor repair one, and Finding rows gave no machine-usable
repair scope. Product model (unchanged): Billing Account holds the unified
balance, many Workspaces share one account, Workspace records the
consumption location, User records the actual operator. Ledger and
Task/River atomic-transaction contracts stay intact; budgets/quotas
(945/946), internal grants, balance transfers, and auto-topup stay out of
scope.

## Decision

1. Every Admin funds call addresses exactly one explicit Billing Account:
   `user_id` targets the user's initial personal account, `billing_account_id`
   names any shared/organization account directly (XOR, fail closed). Wallet
   and Ledger gain ba_-addressed reads; commands keep the platform-Admin
   operator as Ledger Actor with fact references, idempotency, and audit.
   Account-addressed projections carry no synthesized user linkage.
2. Admin account locate/audit is a first-class read: paginated account list
   (newest-first, owner-scope filter), account detail with ownership
   linkage, explicitly linked workspaces, financial managers (usr_
   identities), and the wallet summary. Manual restrict/release ride the
   typed `WalletRestrictionCommands` seam on the explicit account; release
   addresses exactly one `restriction_id`.
3. Findings expose `billing_account_id` (ba_) as the repair scope:
   automatic restriction and release act only on that account plus its own
   cause, never on same-Owner sibling accounts. Linked Refund/Correction
   keep their repair duty on restricted wallets with cross-account
   references rejected; no new refund policy is introduced.
4. Platform Admin (admin business role on human/worker sessions, enforced by
   the Control guard) and customer-side Billing Owner/Admin (recharge/bills/
   quotas on its own accounts, never content access) are distinct
   permissions: the former audits any ba_ account here, the latter never
   implies this surface.

## Consequences

- Control mutations accept `billing_account_id` alongside `user_id`;
  wallet/restriction views make user linkage optional and always carry the
  ba_ identity. Generated Control clients are regenerated from the spec.
- Admin UI addresses accounts through the credits feature resource boundary
  (account list/detail/wallet/ledger/restriction resources, account-mode
  wallet dialog, correction tab, restriction panel, ba_-scoped Finding
  queue); platform-vs-account permission is stated in-product.
- Focused tests pin org-anomaly isolation, non-org sharing, personal
  lifecycle isolation, anti-cross-account references, duplicate-fix
  convergence, and linked-Refund repair.

## Status

Accepted
