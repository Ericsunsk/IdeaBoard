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
| Credit Wallet             | Account boundary of a single user inside the Credits context                                                                                          |
| Wallet Restriction        | Reason-coded spending limit, independent of Wallet lifecycle status; traced by type, source, and related object, releasable independently per source  |
| Credit Account            | Double-entry posting landing point; each Wallet has available/reserved, System accounts shard by Wallet                                               |
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

A Refund reverses a Capture; a Purchase Refund settles an external-payment refund. They are distinct accounting operations. Wallet lifecycle controls the account lifecycle; Wallet Restrictions express independently releasable spending constraints.

## Public Seams

- `Commands`: sole application write entrypoint for Grant, Deduct, Hold,
  Capture, Release, Refund, Purchase, PurchaseRefundHold, PurchaseRefund,
  PaymentDisputeDebit, and PaymentDisputeRestore.
  External-payment commands use independent typed `SECURITY DEFINER` routines;
  all other commands go through `execute_credit_command_v2`.
- `Service.InTx(pgx.Tx)` / `CommandsInTx(pgx.Tx)`: caller-owned transaction
  adapters over the same `Commands` contract.
  Task/River submission and task terminal convergence both use pgx;
  there is no `database/sql` Credit Command path.
  Callers own commit/rollback; the Tasking Task Store holds no
  application-lifetime Credits/Billing dependency.
- `WalletReader`: Wallet balance plus keyset-paginated Ledger queries.
- `WalletRestrictionCommands`: idempotent create and precise release of
  Restrictions through a transaction-capable typed seam.
  Other contexts must not write the Restriction table directly.
- `Service.GetCaptureUsage`: read-only per-user aggregation of settled Captures
  at typed locations, composed into caller-owned Usage interfaces.
  Returns no Refunds; grants no Ledger write.
- User-side Billing Records: independent caller-owned Read Model over settled
  Ledger facts plus the immutable Task Billing Snapshot.
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

## Invariants

- `CreditAmount` is a positive integer;
  per-command cap is `9_000_000_000_000_000`;
  available/reserved are never negative.
- A Ledger Transaction supports N postings.
  PostgreSQL deferred constraints validate posting count, single asset,
  and zero sum at commit.
- Ledger Transactions and Postings forbid UPDATE/DELETE;
  errors are fixed only by appending reversal/correction transactions.
- Posting statements update projections in stable `account_id` order;
  Commands lock accounts in the same order before writing.
- The API runtime role holds only SELECT on Credit tables:
  no direct Ledger/projection DML, no execution of internal trigger functions;
  only the migration owner holds schema.
- System Issuance, Revenue, and Sink accounts shard by `wallet:<user_id>`
  and keep role/metadata, avoiding one global system row
  serializing all Wallets.
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
- Wallet lifecycle status and active Restrictions are orthogonal.
  Grant, Deduction, and Hold require active lifecycle plus zero active
  Restrictions; the Payments-typed PurchaseRefundHold is not spending and may
  bypass Restrictions while lifecycle is active to reserve pending Ideas;
  Capture and Release must still converge existing Holds;
  authenticated-Admin linked Refund/Correction keeps its repair duty.
  Idempotency replay returns the original result before the current gate.
- Restriction create/release locks the Wallet row first to linearize
  with the Credit Command gate;
  only one active row per `(Wallet, type, source, related object)`;
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
- Identity atomically provisions the Wallet and freezes/restores lifecycle
  status through the typed Wallet lifecycle seam, and idempotently issues
  the `100 Ideas` Welcome Grant keyed by a trusted
  canonical-registration-identity HMAC claim;
  Ban/Unban/delete never creates or clears standalone Restrictions,
  and claim identity plaintext never enters the Credits schema.
- The Welcome identity HMAC key binds the database fingerprint permanently
  via a typed `SECURITY DEFINER` command on first use;
  on config drift, startup/provisioning fails closed so the same identity
  cannot mint a new claim.
- Business-user deletion first freezes the Wallet and records a durable
  deletion phase in the local transaction, then idempotently deletes Auth,
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
