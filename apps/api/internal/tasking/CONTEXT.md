# Tasking Context

Tasking owns the durable task lifecycle from canonical submission through queue
execution, terminal convergence, media references, and task statistics. It does
not own provider implementations, media storage, the Credits ledger, or HTTP
transport details.

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
  provider endpoint, or credential dimension in the fence — ADR-0009
  retired that quadruple; do not reintroduce it.
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
