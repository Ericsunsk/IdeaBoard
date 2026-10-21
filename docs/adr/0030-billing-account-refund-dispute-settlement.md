# ADR-0030: Billing Account Refund and Dispute Settlement

## Context

Issue #920 requires external top-up refunds and payment disputes to converge
on the original order's payer Billing Account. #919 persisted
`payment_orders/payment_customers.billing_account_id`, backfilled history to
the original payer, and converged paid settlement (`purchase_credit` with
`paid_pending_credit → credited`) on the row-inline account, but refund and
dispute settlement still re-derived the personal account from the operator
(`ResolvePersonalAccount(order.UserID / refund.UserID / dispute.UserID)`).
A member topping up a shared account, then leaving, would have refunds and
dispute debits land on the operator's personal wallet instead of the shared
payer; hints fanned out only to the operator; the Admin refundable quantity
read the operator's wallet. Product model: Billing Account holds the unified
balance, many Workspaces share one account, Workspace records the consumption
location, User records the actual operator (desktop consensus, ADR-0024/0025,
ADR-0029). Ledger and Task/River atomic-transaction contracts stay intact;
linked consumption refunds and quota math (945/946, 924) are out of scope.

## Decision

1. Every refund/dispute funds command addresses the order row's fixed
   `billing_account_id`: `PurchaseRefundHold` at pending reservation
   (Admin and Dashboard paths), `Release` on provider failure,
   `PurchaseRefund` on success, `payment_dispute_debit` + reasoned
   Restriction on open/lost, `payment_dispute_restore` + precise Restriction
   release on won. Missing payers fail closed; there is no per-operator
   fallback and no cross-account deduction. The refund/dispute rows keep
   `user_id` as operator attribution only.
2. Cumulative-cap, idempotency, and shortage policy are unchanged in shape and
   now evaluated on the payer account: pending + succeeded + anomaly refunds
   plus open/lost disputes share the order's paid Idea/amount facts;
   `(payment_order_id, idempotency_key)` replay, Stripe-ID dedup, terminal
   immutability, and `purchase_refund` hold-account binding prevent double
   settlement; insufficient unconsumed Ideas surface `ErrInsufficientCredits`
   as a durable anomaly for manual recovery.
3. Stripe I/O never holds a local transaction. The Admin reservation commits
   on its own connection before the provider call (`BeginCommittedRefundTx`);
   provider-result marking and each settlement phase commit funds commands,
   status flips, and `payment_admin_actions` audit in one transaction.
   Dispute intake retrieves the canonical Stripe fact before opening the
   settlement transaction; dispute checkout expiry runs Stripe expires inside
   the per-account advisory lock with short transactions only.
4. Records, feedback, and hints name the payer account with the real
   operator: Admin detail refundable reads the payer Wallet; refund/dispute
   hints carry `billing_account_id` and fan out to the payer's current
   financial managers (operator-only fallback); Admin re-reads the order
   detail after refund and user clients re-read account-scoped Wallet, orders,
   and recharge history from the hint. Dispute expiry targets the payer
   account's active Checkouts.
5. First release excludes Workspace wallets, internal grants, cross-account
   moves, auto top-up, subscriptions, expiries, payer switches, and linked
   consumption-refund quota corrections; keeps the existing refund/dispute
   business policy; publishes no intermediate release.

## Consequences

- Shared-account top-ups refund and dispute where they were paid, even after
  the operator leaves or management changes; converted history orders replay
  because their backfilled payer is read from the same order row.
- Follow-up tickets (945/946 execution, 924 linked refunds, final integration
  gate) build on this pair and boundary; intermediate steps are not released
  separately.

## Status

Accepted
