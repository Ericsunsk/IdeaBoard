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
- `infrastructure`: PostgreSQL and River, NATS, HTTP, config, and RED metrics adapters under `config/`, `database/`, `http/`, `metrics/`, `nats/`, and `migration/`.
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
- `infrastructure/migration.Runner` owns Atlas revision reconciliation and execution, then River schema migration and runtime privilege finalization. Composition supplies the River adapter and context-owned privilege policies; owner credentials never enter runtime pools. Retired media object layouts have no migration path; the current baseline and writers use canonical object keys only.
- `interfaces/http` owns HTTP routing and transport adaptation only.
  Billing overview delegates to `internal/billingoverview.Reader`: account
  provisioning precedes the required read-only snapshot; capability/scope
  projection, pending invite filtering, quota composition, and concurrent
  Credits/Usage cancellation and joining belong to that read module. HTTP
  supplies the identity/display adapter and maps aggregate results/errors.
  Snapshot identity and display reads must keep the ambient context carrier.
  Wallet and cached Usage reads retain their owning readers and may observe
  newer facts than the Tenancy/MemberQuotas snapshot.
- Invitation intents delegate to `tenancy/invitations.Service`, which owns trusted recipient identity resolution, pending-recipient reads, and the atomic invitation/membership, notification, and River mail-enqueue write. HTTP supplies only the actor and request input, never a caller-prepared recipient email or user binding. A runtime identity adapter joins the existing display, email-lookup, and user-directory readers; remote identity reads precede the write transaction. Auth outages may omit email facts, but only an existing matching user binding permits a decision without email; a trusted mismatching email still rejects it. HTTP never dispatches collaboration notifications after commit. Workspace mail jobs carry encrypted tokens and use the shared River client; Auth renders and sends through its internal invitation-mail contract.
- Invitation display delegates to `tenancy/invitations.Reader`: participant collection, deduplication, optional enrichment and common projection belong to that module. The runtime adapter reads public identity and names in one local batch and reuses member-display projection for Auth email and avatars. Auth failure does not discard local names or public identities. HTTP maps scope field names and wire times; public token previews never expose invitee email. Required member projections remain fail-closed and do not share invitation display omission policy.
- `interfaces/worker` owns queue consumption and provider execution adaptation only.
  Its provider transfer module prepares a fixed-policy `WorkerOutputSlot`;
  the slot owns exact-key binding, bounded object reads, complete image validation
  and actual/stored/reported MIME agreement, returning only an accepted data URL.
  Executors do not choose output policies or orchestrate acceptance checks.
  Worker JSON requests, responses, endpoint paths and error codes are owned by
  the neutral `platform/workerwire` module, shared with ModelRegistry. Runtime
  keeps timeout, resilience, metrics and local execution metadata outside the wire.
  The HTTP media adapter independently verifies HMAC scope and upload integrity;
  HTTP and Worker adapters remain mutually isolated.
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
