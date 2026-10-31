# ADR-0033: Workspace Creation with Explicit Billing Account
> Superseded（Resolution 部分）：被 [ADR-0038](./0038-personal-organization-billing-model.md) 取代，当前产品合同以 0038 为准。

## Context

Issue #944 requires creation-time clarity for payers: new users own a usable
Default space, business workspaces visibly use one account, and independent
businesses can hold isolated funds. #915 landed stable resolution (explicit
`workspace_billing_links` → Organization pool → per-Workspace account,
fail closed) with financial Owner/Admin roles, and #916 moved Wallets under
Billing Accounts (funds identity on the account, the real operator on
`Metadata.Actor`), but creation stayed implicit: `CreateWorkspace` minted
only the workspace row, the payer was resolved lazily on first billing read
(minting a per-workspace account and wallet as a side effect), organizations
provisioned no pool until first use, and there was no explicit independent
vs shared choice. A member creating a second business space silently forked
a new wallet; an org workspace could be misread as personal; unauthorized
link attempts could leave half-created workspaces. Product model (unchanged):
Billing Account holds the unified balance, many Workspaces share one
account, Workspace records the consumption location, User records the actual
operator. Ledger and Task/River atomic-transaction contracts stay intact;
quota math (945/946), Workspace wallets, internal grants, cross-account
transfers, auto top-up, subscriptions, expiries, and payer switches stay out
of scope.

## Decision

1. New-user provision is one idempotent unit: `EnsureDefaultWorkspace`
   mints the Default workspace plus its deterministic personal Billing
   Account (creator as billing owner, Wallet via the account-insert
   trigger) in the same transaction. Concurrent logins collapse on
   `ux_workspaces_default_owner` plus the per-workspace account unique
   index and read back the winner; repeated logins never fork accounts.
   Welcome-grant eligibility still follows the existing verified-identity
   rule.
2. Personal workspace creation links its payer atomically in the same
   transaction: omitted billing defaults to the caller's current (personal)
   account via an explicit link (no new Wallet); `billing_account_id`
   shares an authorized non-org account (caller must be Billing
   Owner/Admin, `billing_account_forbidden` otherwise, unknown
   `billing_account_not_found`); `new_billing_account` mints a fresh
   isolated per-workspace account (creator as billing owner, balances never
   merge across same-Owner siblings, no link row). Empty billing opts keep
   the legacy lazy mode for direct Store callers; the Public API always
   passes an explicit choice. The idempotency hash covers the billing
   choice, so the same key with a different account conflicts instead of
   replaying another account's funds.
3. Organization creation and first config share one pool:
   `CreateOrganization` provisions the org account with the creator as
   billing owner in the same transaction; `CreateOrganizationWorkspace`
   ensures the pool and never mints a per-workspace account or a personal
   link. Billing fields with an organization scope fail closed with
   `billing_account_conflict`, never the creator personal pool.
4. Authorization failures roll back with no half workspace: the workspace
   row, membership, account mint, owner grant, and link commit in one
   transaction. Only authorized accounts accept new spaces.
5. Responses and UI name the payer: creation responses carry
   `billing_account_id` when the billing store resolves it; the create
   dialog defaults to shared-current with the account shown and offers an
   explicit independent option with the isolation notice. Existing
   workspace payer switches and balance transfers stay out of scope
   (`billing_account_conflict`).

## Consequences

- New spaces never implicitly fork wallets through the API; sharing is an
  explicit link, independence an explicit mint.
- Task settlement, payment orders, refunds/disputes, and Admin explicit
  accounts build on the same stable payer without change; history rows keep
  amounts and postings untouched.
- Follow-up batches (#945–#948) build quota/budget checks inside the
  settlement transaction this decision keeps pinned (after stable
  resolution, before Hold, on the same snapshot).

## Status

Accepted
