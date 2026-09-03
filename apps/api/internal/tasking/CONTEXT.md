# Tasking Context

Tasking owns the durable task lifecycle from canonical submission through queue
execution, terminal convergence, media references, and task statistics. It does
not own provider implementations, media storage, the Credits ledger, or HTTP
transport details.

## Package map

- `domain`: task identity, payload, status, manifest, and lifecycle facts.
- `application`: submission, billing, runtime state transitions, task command/
  query use cases, and statistics use cases.
- `infrastructure`: PostgreSQL persistence, terminal settlement, and River
  queue adapters.
- `ports`: caller-owned queue, media, runtime, statistics, and submission
  seams.
- `interfaces/http`: Gin adapters consuming Task Command/Admin/User Query
  interfaces; they do not depend on repository implementations.

## Invariants

- Task insert, billing hold, media reference claim, and River enqueue commit in
  one PostgreSQL transaction.
- PostgreSQL and River are the task facts and queue source; NATS is event-only.
- Terminal success captures Credits; failure, cancellation, and timeout release
  the hold atomically with the task transition.
- Lease identity is fenced by task, account, provider endpoint, credential, and
  lease token; stale workers cannot overwrite a newer lease.
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
