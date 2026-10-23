# Tasking Context

Tasking owns the durable task lifecycle from canonical submission through queue
execution, terminal convergence, media references, and task statistics. It does
not own provider implementations, media storage, the Credits ledger, or HTTP
transport details.

## Domain Language

| Term                    | Meaning                                                                                                                |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Task                    | A durable request to execute one Service in a Workspace, with inputs, outputs, lifecycle, and billing facts.           |
| Submission Action       | One intentional user request; transport retries belong to the same action, while an intentional rerun is a new action. |
| Execution Attempt       | One claimed execution of a Task, with its execution identity, timing, and outcome.                                     |
| Execution Token         | The identity fencing a claimed Task execution from stale workers.                                                      |
| Task Input Manifest     | The immutable ordered media references and roles accepted for a Task's inputs.                                         |
| Task Output             | An ordered result attributed to a Task; an image result references a Media Asset.                                      |
| Task Billing Snapshot   | Immutable price, quantity, and amount facts accepted for a Task.                                                       |
| Task Billing Settlement | The Task's accounting outcome: free, held, captured, or released.                                                      |

An Execution Plan belongs to Canvas and may produce multiple Tasks. A Routing Snapshot belongs to [Generation](../generation/catalog/CONTEXT.md); Tasking persists it and execution consumes it. Neither a River job nor a gateway request is the Task itself.

## Package map

- `domain`: task identity, payload, status, manifest, and lifecycle facts.
- `application`: submission, billing, runtime state transitions, task command/
  query use cases, statistics use cases, and retention use cases.
- `infrastructure`: PostgreSQL persistence, terminal settlement, and River
  queue adapters.
- `ports`: caller-owned queue, media, runtime, statistics, retention, and
  submission seams.
- `interfaces/http`: Gin adapters consuming Task Command/Admin/User Query
  interfaces; they do not depend on repository implementations.

## Invariants

- Task insert, billing hold, media reference claim, and River enqueue commit in
  one PostgreSQL transaction against the Workspace's stable Billing Account
  (#915 resolution joins the transaction, #916 spends it); the submitter
  rides as the operator Actor, never as the funds identity. Inside the
  transaction the submission gate (workspace row lock) precedes the durable
  idempotency claim: the claim's workspace FK takes `FOR KEY SHARE`, so
  locking first is the only deadlock-free entry order for concurrent
  submits.
- Quote, readiness, and submit spend the same stable account with the same
  consumption eligibility: workspace membership, the Board generate gate for
  Board-bound preflights (editors and owners generate, viewers never do;
  financial management alone grants no Board standing), the shared pool
  availability decision, and the Wallet funds gate (402 insufficient vs 403
  restricted, mirroring the submit Hold). Terminal Capture/Release converge
  on the submit-time row facts, so member departure, role/billing-owner
  changes, and payer-link changes never rewrite a Task's attribution; retry
  mints a fresh execution token with no second Hold.
- PostgreSQL and River are the task facts and queue source; NATS is event-only.
- Terminal success captures Credits; failure, cancellation, and timeout release
  the hold atomically with the task transition. Events stage only after the
  settlement unit is durable: synchronously for owned transactions, via a
  post-commit hook for guard-borrowed ones, never on rollback.
- Member-quota checks join the settlement transaction after Quote and before
  Hold, using the resolved Billing Account and actual submitting User. Holds,
  Capture, and Release settle against the submission-time quota window.
- Submission validates current Workspace membership, Board generation capability,
  and canonical Media read permission for every input and mask within the same
  transaction as Quote, Hold, Task insert, and queue insertion.
- Accepting a Media Task Output records its immutable producing Task, submitter,
  and original Board identity in the same transaction. Media origin survives raw
  Task and Board deletion; neither event grants new publication rights.
- Board-linked Task Outputs remain within the Board audience and never publish
  automatically to the Workspace Library. Boardless completed outputs may enter
  Quick Saves after persisted provenance and current membership are validated
  within each Library write transaction. Saves are best-effort and idempotent;
  failure never changes Task settlement or requeues execution. Completed-task
  redelivery replays saves without rerunning the provider. Manual publication
  requires current Media readability. Board-bound outputs require the producing
  Board Creator; submitting an Editor Task or owning its generated asset does not
  confer publication rights. Boardless outputs require their submitter, while
  ordinary uploads require asset-owner or source-Board Creator authority.
- Raw task rows live at most the retention window (default 7 days, terminal
  statuses only): each purge freezes statistics into task_daily_stats first,
  then deletes tasks in one transaction. Billing snapshots/settlements/claims,
  the credit ledger, and statistics aggregates are never purged.
- Lease identity is fenced by task plus the execution token, and nothing
  else: `tasks.lease_token` gates the pending -> running claim (a running row
  is only reclaimable by the same token), the River job carries the token so
  a stale delivery drops before claiming, and `task_execution_attempts` is
  keyed `UNIQUE (execution_token)` so the claim and the attempt fact can
  never disagree about which execution they describe. A retry mints a fresh
  token, which is how stale workers lose the race. There is no account,
  provider endpoint, credential, or gateway-request-ID dimension in the fence.
- Application packages depend on interfaces/contracts, never on concrete
  infrastructure adapters.

## Dependency direction

```text
interfaces -> application -> domain / ports
infrastructure -> ports / domain
runtime worker -> application / domain / ports
```

Keep SQL, River types, provider SDKs, and Gin handlers out of `domain`.

Terminal billing settlement belongs to the PostgreSQL adapter because it must
share the caller-owned SQL transaction and lock order with terminal Task facts.
The application billing service owns quote/hold orchestration only; durable
settlement facts and errors live under `ports/billing`.
