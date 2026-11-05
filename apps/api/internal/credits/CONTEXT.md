# Credits Context

Credits is the platform-level closed-ecosystem currency of IdeaBoard.
User-facing copy shows `Idea` (`1 Idea` for exactly 1, `N Ideas` otherwise);
internal domain units, code, APIs, and schema keep using `Credit`/`credits`.
This context owns only currency accounts, the immutable ledger,
balance projections, and Credit Commands;
product identity, versioned pricing, and Quotes belong to the `billing` context.

## Package Map

- `domain`: pure value objects and policies
  (`CreditAmount`, ledger shape, wallet, restriction rules); no I/O.
- `application`: command handlers plus `wallet`, `readmodel`, `usage`,
  `health`, and `reconciliation` use cases.
- `ports`: inbound/outbound contracts
  (`Commands`, `WalletReader`, restrictions, reconciliation, wallet lifecycle).
- `infrastructure`: `postgres` adapters (SQL routines, projections, triggers).
- `interfaces`: `http` controllers
  (Admin Wallet/Ledger read, Grant, Deduction, linked Refund, Correction).

## Domain Language

| Term                      | Meaning                                                                                                                                               |
| ------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| Credit                    | Indivisible integer currency unit; floating point is forbidden                                                                                        |
| Credit Wallet             | Account boundary of exactly one Billing Account inside the Credits context (#916). The Wallet holds the account's unified balance; Organization Workspaces share the Organization Wallet through the stable Tenancy resolution, while Personal scopes (org-less, non-Default) spend the actor's own deterministic Personal Billing Account (payer = actor, #973). The Command Actor records the real operator |
| Billing Account Funds Identity | The stable `billing_accounts.internal_id` (`ba_` on the wire) that owns a Wallet. Workspace records the consumption location, User records the actual operator; financial Owner/Admin never implies Board content access |
| Initial Personal Account  | The deterministic per-user account (that user's Default personal workspace account, provisioned on demand) that every pre-916 personal balance, history row, and open Hold is attributed to. Personal funds never inject into an organization; new organization/shared accounts start at zero |
| Wallet Restriction        | Reason-coded spending limit on one account Wallet, independent of Wallet lifecycle status; traced by type, source, and related object, releasable independently per source  |
| Credit Account            | Double-entry posting landing point; each Wallet has available/reserved, System accounts shard by Billing Account                                     |
| Ledger Transaction        | One immutable accounting fact with at least two Postings under the same asset                                                                         |
| Posting                   | Signed delta on one Credit Account; the sum within one Transaction must be 0                                                                          |
| Balance Projection        | O(1) cache of `credit_accounts.balance`, updated only by the Posting trigger, never an independent fact source                                        |
| Grant                     | System Issuance → User Available                                                                                                                      |
| Hold                      | User Available → User Reserved                                                                                                                        |
| Capture                   | User Reserved → System Revenue                                                                                                                        |
| Release                   | User Reserved → User Available                                                                                                                        |
| Refund                    | Return of previously captured Credits from System Revenue to User Available, linked to the original Capture and bounded by its amount.                |
| Purchase Credit           | `System Issuance → User Available` for verified external payment; called by Payments with a fixed actor/source/reference, never a discretionary Grant |
| Purchase Refund           | `User Reserved → System Issuance` for a verified Stripe Refund; must link the Refund Hold created by Payments, idempotent terminal settlement         |
| Credit Reconciliation Run | One Credit Ledger consistency check and its outcome; distinct from Payment Reconciliation.                                                            |
| Finding                   | Re-runnable anomaly fact with a stable fingerprint from finding type + Wallet + subject                                                               |
| Finding Type              | Stable typed anomaly contract owned by Credits; Admin provides a uniform name and summary for known types, unknown types must degrade safely          |

> Superseded (Personal part of the Credit Wallet row, see [ADR-0042](../../../../docs/adr/0042-personal-payer-is-actor.md)):
> pre-#973 every Personal scope of one user shared the owner's survivor
> Wallet ("many Workspaces may spend one Wallet"). Personal scopes now
> spend the actor's own deterministic Personal Billing Account
> (payer = actor, no sharing, no owner fallback). The Organization
> shared-Wallet rule is unchanged.

A Refund reverses a Capture; a Purchase Refund settles an external-payment refund. They are distinct accounting operations. Canonical names (see `GLOSSARY.md`): funds identity is only Billing Account (`ba_`), the container is only Credit Wallet, a ledger leg is always the full name Credit Account, and refunds use only the full names linked Refund / linked Purchase Refund. Wallet lifecycle controls the account lifecycle; Wallet Restrictions express independently releasable spending constraints.

## Public Seams

- `Commands` is the union face for generic carriers; production callers
  depend on the narrow role faces in the same package
  (`AdministrativeCommands` for Control fund repair, `SettlementCommands`
  for workspace-scoped spending, `PaymentCommands` for Payments-owned
  external-money convergence, `Granter` for Rewards). Every Command addresses
  one Billing Account (`BillingAccountID`, the Wallet identity) with the
  real operator on `Metadata.Actor` (user/service/admin/system); funds
  identity and operator are never conflated. Workspace-scoped spenders
  (Tasks, Vision Search) resolve the settlement payer inside the
  caller transaction (fail closed): Organization scopes resolve the shared
  Organization pool through the stable Tenancy resolution; Personal scopes
  (org-less, non-Default) resolve the actor's own deterministic Personal
  account (payer = actor, #973/#974) with no owner fallback; scopeless spenders (Rewards, Payments,
  Welcome, Admin) resolve the deterministic initial personal
  account. There is no user-wallet fallback and no dual settlement.

  > Superseded (Personal part, see [ADR-0042](../../../../docs/adr/0042-personal-payer-is-actor.md)):
  > pre-#973 Workspace-scoped spenders in Personal scopes resolved the
  > Workspace owner's stable account (the survivor) through the stable
  > Tenancy resolution. The Organization shared-pool rule is unchanged.
  External-payment commands use independent typed `SECURITY DEFINER` routines;
  all other commands go through `execute_credit_command_v3`.
- `Service.Administrative()` / `Service.Settlement()` / `Service.Payment()`
  (pool-owned role faces) and `Service.SettlementInTx(pgx.Tx)` /
  `Service.AdministrativeInTx(pgx.Tx)` / `Service.GrantsInTx(pgx.Tx)` /
  `Service.PaymentInTx(pgx.Tx)` / `CommandsInTx(pgx.Tx)` (caller-owned
  transaction views): caller-owned transaction
  adapters over the same account contract.
  Task/River submission and task terminal convergence both use pgx;
  there is no `database/sql` Credit Command path.
  Callers own commit/rollback; the Tasking Task Store holds no
  application-lifetime Credits/Billing dependency.
- `WalletReader`: account-keyed Wallet balance plus keyset-paginated Ledger queries.
- `WalletRestrictionCommands`: idempotent create and precise release of
  account Restrictions through a transaction-capable typed seam.
  Other contexts must not write the Restriction table directly.
- `WalletLifecycle`: account lifecycle (`ProvisionAccountWallet`,
  `SetAccountWalletFrozen`), deterministic personal-account resolution,
  sole-owned freeze for ban/delete isolation
  (`SetSoleOwnedWalletsFrozen`, `RemoveSharedBillingMemberships`), and the
  Welcome claim (identity-deduped per user, granted to the personal account).
- `Service.GetCaptureUsage`: read-only Billing Account aggregation of settled Captures, narrowed by the
  authorized consumption Workspace and/or real initiating member
  at typed locations, composed into caller-owned Usage interfaces.
  Returns no Refunds; grants no Ledger write. The PostgreSQL adapter consumes
  `internal/consumptionread` for the shared settled-consumption attribution
  projection instead of knowing Task/Vision origin storage.
- User-side Billing Records: independent caller-owned Read Model over settled
  Ledger facts plus the immutable Task Billing Snapshot. Account managers see
  account billing; Workspace managers see local consumption; other members see
  their own account consumption. SQL authorization filtering precedes keyset
  pagination; scoped reads omit historical account balances and non-consumption entries.
  Reuses no Admin Ledger DTO; exposes no Hold/Release/Refund;
  grants no Ledger write.
- `Reconciliation`: runs on an independent River queue.
  Admin reads run history, summaries, and the Run-ID-independent global
  keyset Finding queue (open/resolved).
- Admin HTTP: single-user Wallet/Ledger read, Grant, Deduction,
  linked Refund, and Correction.
  No set-balance; no Ledger history update/delete.
  Ledger reads return the Hold/Capture reference explicitly, so Admins never
  infer settlement state from bare association IDs.
- Admin explicit-account operations (#923): the Control Plane addresses one
  stable Billing Account per call. `user_id` targets the user's
  deterministic initial personal account; `billing_account_id` names any
  shared/organization account directly (exactly one of the two, fail
  closed). Account wallet/Ledger reads, fund commands, and manual
  restrict/release all carry the ba_ funds identity with the platform-Admin
  operator as Actor; no single-user linkage is synthesized for shared
  accounts. Reconciliation Findings expose the same `billing_account_id`
  as the repair scope: automatic restriction/release affects only that
  account plus its own cause. Customer-side Billing Owner/Admin standing
  manages recharge/bills/quotas on its own accounts only and is never
  equivalent to the platform-Admin operator.

## Invariants

- `CreditAmount` is a positive integer;
  per-command cap is `9_000_000_000_000_000`;
  available/reserved are never negative.
- A Ledger Transaction supports N postings.
  PostgreSQL deferred constraints validate posting count, single asset,
  and zero sum at commit.
- Ledger Transactions and Postings forbid UPDATE/DELETE;
  errors are fixed only by appending reversal/correction transactions.
  Pre-916 history rows keep their amounts and postings untouched; only the
  additive `billing_account_id` attribution column is backfilled, mapping
  each personal balance/history/Hold 1:1 to its owner's initial personal
  account (no copy, no merge, no organization injection).
- Posting statements update projections in stable `account_id` order;
  Commands lock accounts in the same order before writing.
- Wallet/advisory/idempotency/actor/source/reference gates are unchanged,
  evaluated on the Billing Account: same key + same hash replays, same key
  + different hash conflicts, and a foreign account cannot settle another
  account's Hold or Charge.
- The API runtime role holds only SELECT on Credit tables:
  no direct Ledger/projection DML, no execution of internal trigger functions;
  only the migration owner holds schema. Runtime EXECUTE covers exactly the
  v3/v2 account routines plus lifecycle/restriction/reconciliation seams;
  the superseded user-keyed routines are dropped.
- System Issuance, Revenue, and Sink accounts shard by
  `billing-account:<billing_account_id>` and keep role/metadata,
  avoiding one global system row serializing all Wallets. Legacy
  `wallet:<user_id>` shards stay untouched as history.
- Scope deletion never deletes money facts: workspace hard delete removes
  the scope (boards, tasks, links, members, the `billing_accounts` row)
  while Wallet, projection, Ledger, snapshot, and operation rows are
  retained under their `billing_account_id` attribution (same precedent as
  `task_billing_snapshots.task_id`, which carries no task FK). Command
  functions re-verify wallet existence per call, so no dangling FK can be
  traversed.
- Each Command takes an idempotency-key advisory transaction lock first,
  then compares the server-side canonical SHA-256 request hash.
  Same key + same hash replays the original result;
  same key + different hash returns conflict.
- Other contexts must not update `credit_accounts.balance` directly,
  insert Ledger rows, or compute idempotency hashes themselves.
- Purchase accepts only the `service/payments` actor,
  the `stripe_purchase` source, and the `payment_order` business reference;
  it never creates Wallets and cannot be forged by Admin or clients.
  Verified external payment is a pre-existing platform obligation,
  so Purchase bypasses the frozen-lifecycle and active-Restriction gates,
  and must converge atomically with the Payments Payment Order terminal fact.
- Task Holds use no Credits-owned TTL auto-release;
  only the Task terminal transition Captures or Releases in the same transaction.
- Actor, source, and business reference are required and written into the immutable Transaction.
  Service/system/user commands also require a business reason.
- Deduction is an Admin-actor-only immutable `user_available → system_sink`
  transaction; it never touches reserved.
  Correction is authenticated-Admin-only and appends reverse Postings
  of the original; it may link the original Admin Grant/Deduction/Refund,
  or a user-origin Grant with fixed `source=reward_redemption`;
  other non-Admin-origin Grants stay rejected.
  Capture corrections use linked Refund.
- Frozen or anonymized-tombstone Wallets stay historically readable.
  Discretionary Grant/Deduction/Hold are rejected;
  existing Holds must still converge via Capture/Release;
  authenticated-Admin linked Refund/Correction remains available for repair;
  the Wallet starts no new spending.
- Ban and deletion freeze only solely-owned non-organization account
  Wallets; organization and other users' funds never freeze because one
  member was deactivated. Deletion additionally detaches the user from
  accounts that keep other managers while the tombstone row is retained.
  Pre-existing Holds and Purchase/Refund/Dispute obligations still converge
  on frozen Wallets through the typed settlement paths.
- Wallet lifecycle status and active Restrictions are orthogonal.
  Grant, Deduction, and Hold require active lifecycle plus zero active
  Restrictions; the Payments-typed PurchaseRefundHold is not spending and may
  bypass Restrictions while lifecycle is active to reserve pending Ideas.
  The same typed reservation also remains available on a permanently closed
  Organization pool for legal or purchase-promise refunds, without reactivating
  the Wallet or enabling spending; other frozen Wallets remain rejected;
  Capture and Release must still converge existing Holds;
  authenticated-Admin linked Refund/Correction keeps its repair duty.
  Idempotency replay returns the original result before the current gate.
- Restriction create/release locks the Wallet row first to linearize
  with the Credit Command gate;
  only one active row per `(Billing Account, type, source, related object)`;
  release keeps history and never clears other sources' Restrictions.
- Auth public registration uses canonical Email, username, and Password;
  after Email verification succeeds, the `canonical_email` verified identity
  triggers Wallet provisioning; no synthetic Email is generated.
- Reconciliation scans use MVCC reads and never lock hot Ledger/Account rows;
  a DB advisory lock plus an active-run unique index guarantee
  at most one active run.
- Credit Health is fixed to
  `unchecked | healthy | attention | critical | stale | failed`,
  evaluated in priority order: open high-risk Finding or its exact active
  `ledger_integrity/reconciliation` Restriction → critical;
  latest failed Run newer than latest success → failed;
  never succeeded and never failed → unchecked;
  latest success strictly older than 48h → stale;
  any other open Finding → attention; otherwise healthy.
  An active Run is reported separately and never overrides the status.
- The Reconciliation stale-Hold policy is fixed at 24h with the DB per-Run
  snapshot/constraint as fact source;
  Admin API/UI accepts no threshold, and the public Run DTO exposes
  no such internal fixed value.
- Manual Reconciliation records the authenticated Admin; scheduled Runs
  record the schedule slot.
  Daily schedule runs at 03:00 Asia/Shanghai;
  on River leader startup a catch-up fires when no successful Run
  exists in the last 24h.
- manual, daily, and catch-up share one transaction-bound start seam.
  DB advisory lock, active-run uniqueness, scheduled-slot uniqueness,
  and the `enqueue_required` return value guarantee multi-instance/repeated
  scheduling enqueues once;
  Run insert/reset and River enqueue stay in one transaction.
  ScheduleJob snoozes on another active Run without swallowing the target slot;
  a still-overdue failed target slot reuses the original Run,
  clears terminal fields, and re-enqueues.
- `warning/low` (long-held Hold, or complete Billing Snapshot/Settlement/Hold/
  terminal Ledger with only the Task trace missing) reports only
  and never restricts the Wallet.
  A Task trace drops to low only when Hold/terminal command amounts,
  Posting debit/credit shapes, and Snapshot totals all agree;
  any other missing data or mismatch stays `error|critical/high`
  funds-fact risk.
  High-risk Findings idempotently create a `ledger_integrity/reconciliation`
  Restriction bound to that Finding inside the same run-report transaction.
  On Finding resolved, downgraded to low, or Wallet-ownership lost/changed,
  only its own Restriction is precisely released; Reconciliation alerts
  are removed.
- Finding fingerprints exclude the run ID;
  repeat anomalies reuse the open Finding and add occurrence/run links;
  vanished anomalies mark resolved;
  reopen creates a new Restriction history row.
  A Finding's `isolated` derives from its exact active Restriction;
  scans never mutate Wallet lifecycle status, Ledger, or projections,
  and never auto-create Corrections.
- Run-Finding links keep only internal historical snapshots and audit
  associations; the Admin Finding read contract exposes only the global
  open/resolved queue, with no Run-scoped dual-track query.
- Identity atomically provisions the personal Billing Account Wallet and freezes/restores lifecycle
  status through the typed Wallet lifecycle seam, and idempotently issues
  the `100 Ideas` Welcome Grant (identity-deduped per user, credited to the personal account) keyed by a trusted
  canonical-registration-identity HMAC claim;
  Ban/Unban/delete never creates or clears standalone Restrictions,
  and claim identity plaintext never enters the Credits schema.
- The Welcome identity HMAC key binds the database fingerprint permanently
  via a typed `SECURITY DEFINER` command on first use;
  on config drift, startup/provisioning fails closed so the same identity
  cannot mint a new claim.
- Business-user deletion first freezes solely-owned non-organization
  account Wallets, detaches shared Billing Account memberships, and records
  a durable deletion phase in the local transaction, then idempotently deletes Auth,
  cleans Storage, and writes the user tombstone in order;
  failed requests retry to convergence from the persisted phase.
  The user tombstone primary key, Tasks, Wallets, Ledger, and Welcome claims
  are finally retained; re-registration under the same claim identity
  never re-issues the Welcome Grant.

## Verification

- Default suite uses isolated PostgreSQL schemas via `internal/testdb`
  for Commands, HTTP contracts, concurrency locks, the limited runtime role,
  and DB constraints.
- The million-row harness is opt-in by the migration owner/test DSN
  via `CREDITS_LOAD_TEST=1`; commands and baselines live
  in `docs/benchmarks/credits-million.md`.

### Schema ownership

`schema/fragments/credits.sql` owns the core Ledger.
`schema/fragments/credit_payment_settlement.sql` owns the Payments-typed Credit
commands (purchase, refund reservation/settlement, dispute debit/restore) and
the dispute-debit idempotency records. Payments calls these commands; it does
not own their posting logic. `integrity.sql` contains only cross-domain late
bindings, not Credit commands.
