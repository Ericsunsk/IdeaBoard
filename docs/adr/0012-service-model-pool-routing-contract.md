# ADR-0012: Service Model Pool Routing Contract

## Context

Service Model Pool routing uses one shared contract across readiness, quote,
submit, billing, and worker execution. Accepted tasks pin an immutable routing
snapshot; later pool edits cannot change their selected Model or retry candidate.
The `service_resolution_unavailable` code is the dedicated 422 outcome for
an empty eligible pool. `billing_pricing_unavailable` (422 for an unpriced
SKU), `billing_quote_unavailable` (503 for a transient quote failure), and
`provider_unavailable` retain their own meanings; they do not represent an
empty eligible pool.

## Decision

1. **Terminology.** `Service` identifies the product capability with canonical
   slugs `generation`, `inpaint`, `render`, `variations`, `cutout`, `fusion`,
   `upscale`, and `outpaint`. Routing requests and task facts use `service`.
2. **Domain shape.**
   - `Service` owns exactly one Model pool: an unordered set of
     `ModelPoolEntry { model, enabled, supported_resolutions }`.
   - `model` is a catalog reference (`provider/model[@gateway-path]`); one
     Model may appear in many Services' pools.
   - `GenerationRequest { service, resolution, size, prompt }` is the only
     user-facing routing input. Model identity is never a user choice.
   - `RoutingSnapshot { service, requested_resolution, eligible_models,
     selected_model, retry_candidate, selection_seed, pool_version }` is the
     immutable submit-time routing fact pinned per task.
   - `GenerationTask { request, routing_snapshot, billing_snapshot,
     execution_attempts }`. Billing keys on `Service + resolution +
     quantity`; the `ai-image:<service>` SKU namespace is retained with the
     local key tracking the Service slug.
   - `ExecutionAttempt { task_id, attempt_no, execution_token, river_job_id,
     service, requested_model, resolved_model, provider, retry_layer,
     gateway_retry_count, status, failure_stage, failure_category,
     started_at, completed_at }` is a durable PostgreSQL fact. It is append-only
     by `(task_id, attempt_no)` for task attempts; each row is inserted once and
     its terminal fields are completed atomically with the task transition.
     `retry_layer` is exactly `gateway`,
     `model_fallback`, or `river_task`, and the fact never contains prompt,
     provider payload, credentials, or storage URLs.
2a. **Responsibilities.** The cross-context ownership is fixed as follows:
   - **Catalog/configuration** owns Service and Model identities, pool entries,
     capability validation, enabled state, and the per-Service pool version.
   - **Routing** owns eligibility, deterministic selection, retry-candidate
     derivation, and construction of the immutable `RoutingSnapshot`.
   - **Quote/Billing** owns SKU lookup, price version, quantity, hold, and
     settlement keyed by `Service + resolution + quantity`; it never selects a
     Model.
   - **Task lifecycle** owns idempotency, the atomic hold/task/River enqueue
     transaction, `ExecutionAttempt` facts, terminal convergence, and River
     retry policy.
   - **Provider execution** translates canonical resolution and size into the
     gateway request and executes only the pinned Model from the snapshot.
   - **Observability** reads durable task/routing/attempt facts and emits
     bounded metrics/traces; telemetry failure cannot change task or billing
     state.
   `RoutingResolver` is the sole cross-context routing seam; these owners may
   consume its result but may not perform a second live Model lookup.
3. **Canonical resolutions.** The contract speaks exactly `1k`, `2k`, and
   `4k`.
   There is no `default` request, billing, or pool capability value. A
   fixed-resolution Service still selects one of these three canonical values
   in its Service policy; an omitted request is normalized to that policy
   value, while an explicit different value is rejected. Pool entries use the
   same three values as their capability set.
4. **Eligibility.** For a requested resolution, the eligible set is exactly:
   entries with `enabled == true` AND `requested_resolution ∈
   supported_resolutions`. Disabled entries are invisible to routing (they
   stay editable/displayable in Admin). An enabled entry MUST list at least
   one resolution; a Service with zero enabled entries is unavailable, not
   validatable.
5. **No ordering fields.** Pool entries carry no `priority`, `fallback`, or
   `weight`. Before selection, eligible Model references are sorted by their
   canonical catalog reference. At submit, the resolver generates and persists
   a 128-bit cryptographically random `selection_seed`; it selects
   `SHA-256(selection_seed || U64BE(pool_version) || 0x00 || UTF8(service) ||
   0x00 || UTF8(resolution))` modulo the sorted candidate count. The next sorted candidate wraps as the single
   `retry_candidate`, or is empty when only one candidate exists. This gives
   uniform selection across independent seeds and deterministic replay for the
   same snapshot; the algorithm is part of this contract, not an implementation
   choice.
6. **One-candidate retry classification.** After a retryable upstream failure,
   execution tries exactly one remaining eligible candidate
   (`retry_candidate`, distinct from `selected_model`); there is no second
   fallback. Retryable = transient upstream conditions (HTTP 429/408/5xx,
   timeouts, network errors — the current `IsRetryableProviderError`/
   `ShouldFailoverProviderError` families). Deterministic business errors
   NEVER trigger Model fallback: invalid parameters, insufficient credits,
   content-safety rejection, authentication/authorization failures,
   not-found, and successful responses with unusable payloads go terminal
   directly. Fallback creates no second charge and no duplicate task.
7. **Three retry layers, separate counters and facts.** Gateway request retry
   (same Model, bounded HTTP policy, owned by the gateway adapter), Model
   fallback (next pinned candidate, at most once, owned by the worker),
   River/task retry (whole-task requeue per task policy, owned by the task
   lifecycle). `ExecutionAttempt.retry_layer` and
   `ExecutionAttempt.gateway_retry_count` distinguish the three layers in
   PostgreSQL, logs, metrics, traces, and Admin views; a failure in one layer
   never silently spends another layer's budget.
8. **Unavailable shape.** An empty eligible set converges to one canonical
   error before task creation, billing hold, or enqueue: HTTP `422`, code
   `service_resolution_unavailable`,
   identifying the request as `details.fields.service` and
   `details.fields.resolution` under the existing `details.fields` convention.
   Both values are the normalized request values. Quote, readiness, and submit
   render the same code, status, field names, and semantics.
9. **Snapshot pinning.** The submit transaction resolves via `RoutingResolver`
   and persists the `RoutingSnapshot` atomically with Credit Hold, task
   insert, and River enqueue. The worker consumes ONLY the snapshot: it must
   not read the live pool for an accepted task. `pool_version` is a per-Service
   monotonically increasing uint64 starting at
   1 and increments only when an effective pool entry, enabled state, or
   capability set changes. The snapshot is stored as immutable versioned JSONB
   on the task row in the submit transaction; it is not reconstructed from the
   live pool. Later Admin pool edits never invalidate an accepted task nor
   change its candidate set. The snapshot MUST be sufficient to replay and
   diagnose selection + fallback without the live pool.
10. **Single seam.** `RoutingResolver` is the only authoritative routing
    decision point. Readiness, quote, and submit call it; Worker execution
    consumes only its persisted snapshot. No layer performs an independent
    live Model lookup.
11. **One-time resolution canonicalization.** `params.output_resolution` is
    the single client resolution source of truth. Ingress normalizes it once
    (against the Service's resolution policy) into the execution plan; the
    billing resolution is derived from that normalized plan, and any
    top-level/independent billing resolution value MUST either be removed or
    checked for exact agreement — never silently preferred.
12. **Launched resolutions stay visible.** The frontend keeps showing launched
    `1K/2K/4K` options and MUST NOT depend on a dynamic
    `available_resolutions` list. An empty runtime pool surfaces as the `422
    service_resolution_unavailable` toast; it never hides launched options
    and never creates a task or hold.
13. **Technology boundary.** Routing uses PostgreSQL/pgx/sqlc, River,
    gateway, contract/error, OpenAPI, Vue Query, and Valibot primitives.
    No second ORM, queue, retry framework, transaction coordinator, routing
    system, or orchestration framework is permitted. Routing has no
    compatibility aliases, independent Model lookup, dual reads/writes, or
    fallback shims. Generated sqlc/OpenAPI/frontend artifacts are produced
    from their source contracts.
14. **Vision boundary.** Vision search and its provider path remain outside
    Service Model Pool routing. They keep their existing isolated catalog and
    billing semantics until a separate ADR explicitly brings them into this
    contract.

## Consequences

What gets easier: quote, readiness, submit, and worker share one
Service/resolution semantic; routing decisions become replayable from durable
facts; pool edits become safe (versioned, atomic) without disturbing accepted
tasks; retry-layer confusion becomes a testable violation instead of a
judgment call.

What is excluded: per-Model pricing (price ignores Model), Admin-tuned
traffic shaping, multi-fallback chains, parallel old/new routing systems, and
any frontend availability oracle.

## Runtime constraints

A missing routing resolver fails closed. `resolved_model` is a transient
worker handoff and a durable execution-attempt fact, not a task event envelope
field.

Ingress accepts only `1k`, `2k`, and `4k`. Omitted resolutions are normalized
to the fixed Service policy's canonical value; an explicit value that does
not match that policy is rejected as 422 `validation_failed`.

There is no startup reaper or `abandoned` attempt status. Stale `running`
attempts keep reporting `normal_running` pressure until River redelivery
converges them. Adding a reaper requires coordinated status, pressure-query,
Admin, and frontend changes. Vision search stays outside the pool (rule 14).

## Status

Accepted
