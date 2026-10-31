# ADR-0029: Billing Account Payment Settlement
> Superseded（Resolution 部分）：被 [ADR-0038](./0038-personal-organization-billing-model.md) 取代，当前产品合同以 0038 为准。

## Context

Issue #919 requires personal and organization top-up and payment
confirmation to settle on the payer Billing Account. #915 landed stable
payer resolution (explicit `workspace_billing_links` → Organization pool
→ per-Workspace account, joinable to the caller transaction, fail closed)
with financial Owner/Admin roles, and #916 moved Wallets under Billing
Accounts (funds identity on the account, the real operator on
`Metadata.Actor`), but the Payments top-up path still keyed everything by
the operator: `payment_orders` and `payment_customers` carried only
`user_id`, the `(user_id, client key)` pair scoped idempotency, unpaid
slots counted per operator, and settlement re-derived the personal
account from the operator at credit time. A member topping up a shared
Workspace paid from (and read) a personal wallet no matter which shared
account the Workspace consumed, and any Billing manager other than the
creating operator could neither observe nor continue the order. Product
model: Billing Account holds the unified balance, many Workspaces share
one account, Workspace records the consumption location, User records the
actual operator (desktop consensus 《IdeaBoard-团队账单池设计共识》,
ADR-0024/0025). Ledger and Task/River atomic-transaction contracts stay
intact; quota math (945/946), Workspace wallets, internal grants,
cross-account transfers, auto top-up, subscriptions, expiries, and payer
switches are out of scope.

## Decision

1. The Payment Order persists the settlement pair: payer
   (`billing_account_id`, plain `BIGINT` with no FK so the money fact
   survives scope hard deletes, same precedent as
   `task_billing_snapshots.task_id`) and operator (`user_id`, the real
   User). Existing orders keep their money facts untouched and gain
   their determined original payer through
   `ensure_personal_billing_account_for_user(user_id)`; no legacy
   wallet branch is kept.
2. Stripe Customers are reused per payer account
   (`billing_account_id, stripe_account_id, stripe_mode`); test and live
   stay isolated and key rotation never duplicates. Unpaid-slot
   accounting (at most 3 unpaid active Checkouts of different quantities)
   and the `(account, client key)` idempotency scope address the payer
   account; obsolete per-operator unique constraints are removed so one
   manager can recharge independent accounts. The request hash binds the
   account plus the purchase body; the same client key in another account
   creates an independent order and never replays the first account's order.
3. Checkout, continue/abandon, Return, Webhook, and background recovery
   converge on the persisted account under a per-account advisory lock
   (Stripe I/O never holds a local transaction). Checkout create,
   continue, abandon, pending lists, return status, purchase options,
   and recharge history all require Billing Owner/Admin on the resolved
   account; space-only Workspace admins observe no financial facts.
   Member exits and management changes never move the obligation:
   settlement re-reads the order row, never the live membership.
4. The verified Stripe paid fact is first made durable alone as
   `paid_pending_credit`; `purchase_credit` with
   `paid_pending_credit → credited` then commits atomically in one
   PostgreSQL transaction against the stored payer (the credited guard
   requires the purchase transaction on the order's own account). The
   browser back-jump is never a payment fact: Return re-reads the
   Stripe Checkout Session server-side.
5. Order queries, recharge history, and SSE hints scope to the account
   and its authorized audience: reads address the payer account, and
   settlement hints fan out only to the account's current financial
   managers (operator-only fallback when the audience is unavailable),
   carrying the payer account so consumers invalidate only the matching
   account's queries.
6. First release excludes Workspace wallets, internal grants,
   cross-account balance moves, auto top-up, subscriptions, expiries,
   and payer switches; keeps the existing payment/refund business
   policy; and publishes no intermediate release.

## Consequences

- Top-up reads and writes name one payer: the recharge page carries an
  explicit `billing_account_id`, and any Billing Owner/Admin of that
  account manages its orders, regardless of who created them.
- Non-manager Workspace members (including space-only admins) receive
  `billing_account_forbidden` on every finance route; outsiders still
  converge to `workspace_not_found` before any financial lookup.
- Follow-up tickets (945/946 execution, final integration gate) build
  on this pair and boundary; intermediate steps are not released
  separately.

## Status

Accepted
