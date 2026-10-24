# Platform Runtime Context

`internal/runtime` is the Platform Runtime Context of the IdeaBoard API process.
It owns its own facts, an explicit lifecycle state machine, ownership rules, and architecture invariants.
It is not a business context and must never decay into a wiring package; no business context may import it.

## Package map

- `domain`: runtime kernel with the lifecycle `StateMachine`, transition rules, component health facts (`ComponentStatus`), and Readiness evaluation.
- `ports`: runtime seam contracts for component lifecycle `Runner`, readiness `HealthChecker`, and resource cleanup `ResourceCloser`.
- `application/lifecycle`: state-machine-driven lifecycle coordination (`Coordinator`) with shutdown-budget management and drain.
- `application/orchestration`: dependency-graph startup plan (`ExecuteStartup`). Interrupted task recovery is owned by River redelivery and the tasking Worker lease fence.
- `composition`: the process-wide sole dependency assembly root and CLI command dispatcher; loads config, builds the database pool and services, and constructs the lifecycle-managed `App`.
- `infrastructure`: PostgreSQL and River, NATS, HTTP, config, and RED metrics adapters under `config/`, `database/`, `http/`, `metrics/`, and `nats/`.
- `interfaces/http`: HTTP REST transport adapter with route registration, Session and Auth middleware, rate limiting, and authenticated or public media response adaptation.
- `interfaces/worker`: external AI Provider adapters and background periodic reconciliation tasks. Workspace task queue consumption lives in `internal/tasking/interfaces/worker`.
- `adapters/billingregistry`: runtime assembly mapping from Workflow capabilities to Billing item definitions.
- `adapters/webdist`: static-asset adapter embedding the compiled frontend SPA build.

## Public Seams

- `Runner` owns one component's lifecycle contract.
- `HealthChecker` owns the readiness-check contract.
- `ResourceCloser` owns the resource-cleanup contract.
- `Coordinator` drives state-machine lifecycle coordination and owns the shutdown budget and drain.
- `ExecuteStartup` owns the dependency-graph startup plan.
- `App` is the lifecycle-managed application object built by the composition root.
- `composition` is the sole assembly root and CLI dispatcher; business packages and other runtime subpackages hold no global wiring. Process role (`API_MODE=all|http-only|worker-only`, ADR-0021) only selects which runners start: http-only skips River worker registration and periodic schedules (enqueue-only), worker-only skips all HTTP listeners.
- `interfaces/http` owns HTTP routing and transport adaptation only.
  Billing overview delegates to `internal/billingoverview.Reader`: account
  provisioning precedes the required read-only snapshot; capability/scope
  projection, pending invite filtering, quota composition, and concurrent
  Credits/Usage cancellation and joining belong to that read module. HTTP
  supplies the identity/display adapter and maps aggregate results/errors.
  Snapshot identity and display reads must keep the ambient context carrier.
  Wallet and cached Usage reads retain their owning readers and may observe
  newer facts than the Tenancy/MemberQuotas snapshot.
- `interfaces/worker` owns queue consumption and provider execution adaptation only.
- `adapters/billingregistry` owns the Workflow-to-Billing mapping adaptation.
- `adapters/webdist` owns the embedded frontend SPA asset adaptation.

## Invariants

- Lifecycle order is `created -> initializing -> recovering -> ready -> draining -> stopped`.
- `ready` requires all mandatory databases, River, NATS, and adapters healthy before serving traffic.
- `recovering` is the lifecycle transition before runners become ready; interrupted running tasks recover through River redelivery with same-token lease reacquisition, not a startup reaper.
- `draining` catches the shutdown signal, accepts no new work, and gracefully closes each Runner inside a fixed shutdown budget.
- `stopped` means every Runtime-held pool, underlying resource, and Closer is safely released.
- Strict one-way dependency: `domain` is the innermost kernel and depends on no outer runtime layer or business context; `ports` depends only on `domain`; `application` depends only on `domain` and `ports`; `composition` assembles all layers with the business contexts.
- No business context may reverse-import `internal/runtime`.
- Transport and execution interfaces stay mutually isolated: `interfaces/http` and `interfaces/worker` never depend on or know each other, coordinate only indirectly through PostgreSQL facts and the NATS JetStream event bus, and never reverse-depend on `composition`.
- Worker subpackages stay decoupled and one-directional: `providers` decouples on `runtime/ports` contracts; `billing` handles only background reconciliation and payment cron tasks; `river` acts only as the queue assembly adapter. Workspace task execution stays in the tasking context.
- `composition/` is the only in-process object-construction and dependency-assembly center.
- The `internal/runtime/` root keeps no implementation code, only the context contract plus `architecture_test.go`, which enforces layering and adapter isolation.
- Business Task submission and River Job enqueueing must complete atomically in one PostgreSQL transaction.
