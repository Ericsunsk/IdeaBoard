# ADR-0027: Vision Search Workspace Settlement

## Context

Issue #918 requires paid Vision Search from a Workspace to bill the
shared account behind that Workspace. #915 landed the stable payer
resolution (explicit `workspace_billing_links` → Organization pool →
per-Workspace account, joinable to the caller transaction, fail closed)
and #916 moved Wallets under Billing Accounts, but Vision settlement
still resolved the operator's deterministic initial personal account:
a member's workspace search charged (and refunded) their own wallet no
matter which shared account the Workspace consumed. The operation row
persisted the operator and the payer but not the consuming Workspace, so
idempotency could replay across scopes and terminal settlement had no
resource anchor. Product model: Billing Account holds the unified
balance, many Workspaces share one account, Workspace records the
consumption location, User records the actual operator (desktop consensus
《IdeaBoard-团队账单池设计共识》, ADR-0024/0025). Ledger and Task/River
atomic-transaction contracts stay intact; quota math (945/946),
Workspace wallets, internal grants, cross-account transfers, auto top-up,
subscriptions, expiries, and payer switches are out of scope.

## Decision

1. Vision Search bills the consuming Workspace's stable Billing Account
   through the #915 chain inside the reservation transaction (membership
   gate first, then account; hidden workspaces and non-members fail with
   workspace_not_found before any catalog or ledger read). The submitter
   stays the real initiator (Actor on every Command). No personal-wallet
   fallback exists on this path.
2. Every operation persists the settlement triple: submitter (`user_id`),
   consumption location (`workspace_id`, plain BIGINT with no FK so the
   money fact survives scope hard deletes, same precedent as
   `task_billing_snapshots.task_id`), and payer (`billing_account_id`).
   History rows keep `workspace_id` NULL and never replay as workspace
   operations.
3. Hold, success Capture, failure Release, and background reconciliation
   all converge on the STORED payer. Capture/Release verify the stored
   resource scope without re-gating membership or re-resolving the
   account; reconciliation reads only the stored payer. Owner handovers
   and member departures mid-flight cannot move the money.
4. Idempotency binds account + resource scope: the request hash covers
   workspace + submitter + normalized input, replay additionally verifies
   stored workspace/payer/hash, and a partial unique
   `(workspace_id, idempotency_key)` fences same-workspace reuse across
   operators. Cross-workspace, cross-account, cross-operator, and
   cross-body key reuse conflicts instead of replaying.
5. Search permission, insufficient-funds feedback, and balance refresh all
   target the payer: membership gates before billing, Hold failures
   surface per-payer insufficiency without leaking other accounts, and
   wallet reads stay account-keyed. Financial management still grants no
   Board content access; no per-person canSpend switch is added.
6. Future consumption limits (945/946 budgets/quotas) join the
   reservation transaction at the marked boundary (identity resolved
   before Quote/Hold); this decision implements no limit math, only the
   boundary.

## Consequences

- Organization pools and explicit no-org links settle through the same
  chain; per-workspace accounts are its degenerate case.
- The durable `VisionBillingService` triple is the settlement fact
  source; the v1 `vision-queries` HTTP seam now carries the consuming
  workspace into its settler contract so its future production adapter
  resolves the same payer (ephemeral free-marker behavior for unpriced
  SKUs is unchanged).
- Reconciliation converges timed-out/uncertain operations on the
  original payer with a retryable timeout marker; it never re-executes
  the provider and never captures.
- Follow-up tickets (945/946 execution, final integration gate) build on
  this triple and boundary; intermediate steps are not released
  separately.

## Status

Accepted. Vision Search is temporarily unavailable: production wires the
vision seam as nil, so `Extract` fails closed with
`vision_search_unavailable`. The settlement triple and `vision` slug
reservation stay; re-enable by injecting a real `VisionGatewayClient`.
