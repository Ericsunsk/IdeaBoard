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
  one PostgreSQL transaction.
- PostgreSQL and River are the task facts and queue source; NATS is event-only.
- Terminal success captures Credits; failure, cancellation, and timeout release
  the hold atomically with the task transition.
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
