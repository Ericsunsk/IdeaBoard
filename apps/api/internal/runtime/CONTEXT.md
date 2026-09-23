# Platform Runtime Context

`internal/runtime` is the Platform Runtime Context of the IdeaBoard API process.
It owns its own facts, an explicit lifecycle state machine, ownership rules, and architecture invariants.
It is not a business context and must never decay into a wiring package; no business context may import it.

## Package map

- `domain`: runtime kernel with the lifecycle `StateMachine`, transition rules, component health facts (`ComponentStatus`), and Readiness evaluation.
- `ports`: runtime seam contracts for component lifecycle `Runner`, readiness `HealthChecker`, startup recovery `StartupRecoveryCoordinator`, and resource cleanup `ResourceCloser`.
- `application/lifecycle`: state-machine-driven lifecycle coordination (`Coordinator`) with shutdown-budget management and drain.
- `application/orchestration`: dependency-graph startup plan (`ExecuteStartup`) and task startup recovery (`TaskRecoveryCoordinator`).
- `composition`: the process-wide sole dependency assembly root and CLI command dispatcher; loads config, builds the database pool and services, and constructs the lifecycle-managed `App`.
- `infrastructure`: PostgreSQL and River, NATS, HTTP, config, and RED metrics adapters under `config/`, `database/`, `http/`, `metrics/`, and `nats/`.
- `interfaces/http`: HTTP REST transport adapter with route registration, Session and Auth middleware, rate limiting, and static-asset proxying.
- `interfaces/worker`: River queue consumption, external AI Provider execution adapters, and background periodic reconciliation tasks; the `worker` root keeps only the public facade (`worker.go`) plus cross-domain end-to-end contract tests.
- `adapters/billingregistry`: runtime assembly mapping from Workflow capabilities to Billing item definitions.
- `adapters/webdist`: static-asset adapter embedding the compiled frontend SPA build.

## Public Seams

- `Runner` owns one component's lifecycle contract.
- `HealthChecker` owns the readiness-check contract.
- `StartupRecoveryCoordinator` owns the startup-recovery coordination contract.
- `ResourceCloser` owns the resource-cleanup contract.
- `Coordinator` drives state-machine lifecycle coordination and owns the shutdown budget and drain.
- `ExecuteStartup` owns the dependency-graph startup plan.
- `TaskRecoveryCoordinator` owns task startup recovery.
- `App` is the lifecycle-managed application object built by the composition root.
- `composition` is the sole assembly root and CLI dispatcher; business packages and other runtime subpackages hold no global wiring.
- `interfaces/http` owns HTTP routing and transport adaptation only.
- `interfaces/worker` owns queue consumption and provider execution adaptation only.
- `adapters/billingregistry` owns the Workflow-to-Billing mapping adaptation.
- `adapters/webdist` owns the embedded frontend SPA asset adaptation.

## Invariants

- Lifecycle order is `created -> initializing -> recovering -> ready -> draining -> stopped`.
- `ready` requires all mandatory databases, River, NATS, and adapters healthy before serving traffic.
- `recovering` converges previously unfinished running tasks and cleans up leftover River Jobs; accepting new task execution in this phase is a hard gate violation.
- `draining` catches the shutdown signal, accepts no new work, and gracefully closes each Runner inside a fixed shutdown budget.
- `stopped` means every Runtime-held pool, underlying resource, and Closer is safely released.
- Strict one-way dependency: `domain` is the innermost kernel and depends on no outer runtime layer or business context; `ports` depends only on `domain`; `application` depends only on `domain` and `ports`; `composition` assembles all layers with the business contexts.
- No business context may reverse-import `internal/runtime`.
- Transport and execution interfaces stay mutually isolated: `interfaces/http` and `interfaces/worker` never depend on or know each other, coordinate only indirectly through PostgreSQL facts and the NATS JetStream event bus, and never reverse-depend on `composition`.
- Worker subpackages stay decoupled and one-directional: `execution` and `providers` never import each other and decouple on `runtime/ports` contracts; `billing` handles only background reconciliation and payment cron tasks and never reverse-depends on `execution` or `providers`; `river` acts only as the queue assembly adapter depending on `execution` and `providers`.
- `composition/` is the only in-process object-construction and dependency-assembly center.
- The `internal/runtime/` root keeps no implementation code, only the context contract plus `architecture_test.go`, which enforces layering and adapter isolation.
- Business Task submission and River Job enqueueing must complete atomically in one PostgreSQL transaction.
