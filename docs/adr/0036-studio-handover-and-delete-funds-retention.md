# ADR-0036: Studio Handover and Delete Funds Retention

## Context

Issue #948 requires studio-level handover plus space deletion that keeps
funds history: a Billing Owner hands the whole funds identity to a
recipient, and deleting a Workspace never deletes shared money or its
audit facts. #915 landed stable payer resolution with financial
Owner/Admin roles, #916 moved Wallets under Billing Accounts (funds
identity on the account), and #944 made creation-time payers explicit,
but ownership could only move through the immediate direct-transfer
endpoint (no recipient confirmation, no audit row, no workspace linkage),
and deletion had no handover-aware funds guard. Product rules confirmed
for this ticket: invite-style (Owner initiates, recipient accepts, may
reject, initiator may cancel; idempotent and auditable); the Default
space never transfers; deletion with in-flight facts converges first or
refuses; history keeps snapshots with no recovery window; a shared funds
account is never deleted or frozen, only detached; quotas never reset,
no new grants, no attribution rewrite; production never auto-executes.

## Decision

1. One invite row is the handover fact (`billing_account_transfer_invites`:
   `bti_` public id, `ba_` account snapshot, from/to users, `pending` /
   `accepted` / `rejected` / `cancelled`, idempotency key, granted-scope
   `ws_` ids, created/decided timestamps plus decider). Pending means
   undecided (decided columns null); terminal rows always carry both.
   At most one pending invite per account (partial unique index);
   `(billing_account_id, idempotency_key)` is unique, so the same key
   replays the first invite while a different recipient/initiator
   conflicts. `billing_account_id` is a plain `BIGINT` with no FK (same
   precedent as payment orders), so invite history survives scope hard
   deletes.
2. Only a Billing Owner initiates; only the recipient accepts or rejects;
   only the initiator cancels; non-pending decisions conflict. Every
   invite route rides the workspace prelude (member-only visibility,
   hidden scopes converge to 404). Financial managers see account invitation
   history; other Workspace members see only invitations addressed to them,
   so a recipient can discover and decide before receiving financial roles.
   All four mutations require an
   `Idempotency-Key` header under per-operation scopes
   (`billing.account.transfer-invite|accept|reject|cancel`).
3. Acceptance hands management over: the recipient becomes Billing Owner,
   the previous Owner steps down to Admin (recipient granted first, so
   the account never loses its last Owner mid-handover; the initiator
   must still hold Owner or the handover conflicts), and the recipient
   gains Workspace Owner on every non-Default business Workspace linked
   to the account (workspace-level only, never org roles; existing
   owners are never demoted). The Default Workspace never transfers.
   The granted `ws_` ids are stored on the decision as the audit scope.
   Reject/cancel leave membership unchanged.
4. The exit path detaches only (`DELETE .../billing-account/members/:id`):
   the management row leaves while Wallet, balance projection, Ledger,
   snapshots, and operation rows stay readable under their
   `billing_account_id`. There is no account delete or freeze endpoint.
   The last Owner cannot be removed.
5. Workspace deletion resolves the existing payer without provisioning
   and fails closed with `billing_account_conflict` while unsettled
   payment orders (`creating`, `checkout_open`, `async_payment_pending`,
   `paid_pending_credit`), pending handover invites, or shared-anchor
   references (other linked workspaces, other workspaces' tasks) still
   need the funds row. Pending/running Tasks and
   pending/reconciliation-pending Search operations settle first;
   terminal rows stay readable under their workspace/account snapshots.
   Deletion stays immediate hard delete with no recovery window
   (ADR-0023); quotas never reset, no welcome grant is minted, and
   row-pinned settlement keeps existing Task/Search/payment/refund
   attribution on the original account.
6. The pre-existing immediate direct-transfer endpoint stays wired for
   manager-to-manager swaps; #948 adds the invite path above as the
   audited studio handover. No automatic execution exists anywhere:
   every ownership move needs an explicit human decision.

## Consequences

- Handovers are confirmable, rejectable, replay-safe, and auditable;
  single-pending plus idempotency keys make concurrent initiates
  collapse instead of fork.
- Shared money outlives any single space: deletes refuse while funds
  facts are in flight and retain snapshots otherwise.
- Follow-up work builds on the same stable payer without touching this
  contract; history rows keep amounts and postings untouched.

## Status

Accepted
