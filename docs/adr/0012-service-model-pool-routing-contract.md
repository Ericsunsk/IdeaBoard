# ADR-0012: Service Model Pool Routing Contract

## Context

GitHub issue #853 is the shared design gate for the Model Pool feature and
Generation Pipeline reset (#847–#854). The execution companion is
`docs/agents/model-pool-execution-guide.md`; this ADR freezes the routing rules
so #847/#848 implementers never invent competing versions. The guide owns issue
order, repository workflow, verification gates, and rollout constraints; this
ADR owns the cross-context behavior and data contract.

Today routing is a single-value `feature -> model` lookup with no resolution
awareness and no pool: `ConfigRepository.ResolveModel`
(`apps/api/internal/aigateway/model.go:75`) reads one settings key per
workflow feature (`modelMappingKeyPrefix`, `model.go:27`), submit freezes one
model string via `mintResolvedModel` (`apps/api/internal/tasking/workspacetasks/settle.go:210`),
and the worker re-resolves the live mapping per execution
(`apps/api/internal/runtime/interfaces/worker/execution/orchestrator.go:189`).
Resolution travels on two unjoined tracks: `params.output_resolution`
(`apps/api/internal/workflow/paramkeys/keys.go:43`,
`apps/api/internal/workflow/feature_execution_codec.go:30`) for the execution
plan, and a top-level/independent billing `resolution` on submit/quote
(`apps/api/internal/runtime/interfaces/http/contract_v1_tasks.go:189,300,557`)
that defaults to `"default"` in the settlement layer
(`apps/api/internal/tasking/workspacetasks/settle.go:100,147`).
No `service_resolution_unavailable` code exists today
(`apps/api/internal/platform/contract/errorcodes.go` has only
`billing_pricing_unavailable` 422 / `billing_quote_unavailable` 503 plus the
`provider_unavailable` submission error in
`apps/api/internal/tasking/application/task/task_intake.go:393`).

## Decision

1. **Terminology.** `Service` is the product capability formerly named by
   workflow feature slugs (`generation`, `inpaint`, `render`, `variations`,
   `cutout`, `fusion`, `upscale`, `outpaint`;
   `apps/api/internal/workflow/features.go:15`). `workflow_kind` and the
   `feature` request/param fields are renamed to `service` with no
   compatibility alias (#847 owns the mechanical rename; #854 deletes every
   remainder listed under Consequences).
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
   `4k` (today `workflow/output_resolution.go:6`, `billing/domain.go:17`).
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
   `service_resolution_unavailable` (new; does not exist today, see Context),
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
    live Model lookup. Today's
    independent lookups (`task_intake.go:397` readiness gate,
    `settle.go:214` submit mint, `orchestrator.go:189` worker re-resolve) all
    collapse into this seam.
11. **One-time resolution canonicalization.** `params.output_resolution` is
    the single client resolution source of truth. Ingress normalizes it once
    (against the Service's resolution policy) into the execution plan; the
    billing resolution is derived from that normalized plan, and any
    top-level/independent billing resolution value MUST either be removed or
    checked for exact agreement — never silently preferred. Today's dual
    track (`canonical_intake.go:190` plan write-back vs
    `contract_v1_tasks.go:300` top-level `resolution` vs `settle.go:100`
    defaulting) is the gap this rule closes.
12. **Launched resolutions stay visible.** The frontend keeps showing launched
    `1K/2K/4K` options and MUST NOT depend on a dynamic
    `available_resolutions` list (no such source exists today and none is
    introduced). An empty runtime pool surfaces as the `422
    service_resolution_unavailable` toast; it never hides launched options
    and never creates a task or hold.
13. **Migration and technology boundary.** The reset uses the existing
    PostgreSQL/pgx/sqlc, River, gateway, contract/error, OpenAPI, Vue Query,
    and Valibot primitives. No second ORM, queue, retry framework, transaction
    coordinator, routing system, or orchestration framework is introduced.
    One coordinated schema/API migration moves all callers to this contract;
    the clean break removes `workflow_kind`, single-value Model mappings,
    independent `ResolveModel` paths, compatibility aliases, dual reads/writes,
    and fallback shims. Generated sqlc/OpenAPI/frontend artifacts are produced
    from their source contracts during that migration.
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

#854 removal list (verified live sites; production code only, tests and
fixtures follow the same rename):

- `workflow_kind` field/validation: `contract_v1_tasks.go:170,182,196,543,600,613,619`,
  `workspacetasks/tasks.go:205,301`, `tasking/workspacetasks/settle.go`
  `CreateInput`/`QuoteParams` kind plumbing, `events/v1_envelope.go:138,322`,
  `openapi.public.yaml` / `openapi.events.yaml` task schemas, frontend
  `workspaceTasks.ts:120,135,218,280,284`,
  `canvasWorkspaceTaskAdapter.ts:191,408,441,449,509,519` (+ generated
  clients, regenerated not hand-edited).
- Single-value `feature -> model` mapping: `aigateway/model.go:27,30,75,149`
  (`modelMappingKeyPrefix`, `modelMappingKey`, `ResolveModel`,
  `SaveModelMapping`), `ModelResolver`/`ModelStore` seams
  (`workspacetasks/tasks.go:297`, `settle.go:34`,
  `task/application/task/task_intake.go:54`), submit mint
  (`settle.go:210`), readiness gate (`task_intake.go:393`), worker live
  lookup (`orchestrator.go:27,189`), `ResolvedModel` single-string durable
  plan field (`workspacetasks/tasks.go:209`) superseded by `RoutingSnapshot`.
- Dual resolution tracks: top-level `resolution` on submit/quote
  (`contract_v1_tasks.go:189,300,557`) vs `params.output_resolution`
  (`paramkeys/keys.go:43`, `canonical_intake.go:190,261`); settlement
  defaulting (`settle.go:100,147`) moves under rule 11.
- Error-code delta: `service_resolution_unavailable` (422) is new; today's
  neighbors (`billing_pricing_unavailable` 422 unpriced SKU,
  `billing_quote_unavailable` 503 transient, `provider_unavailable`
  submission error, `ErrModelMappingMissing`) keep their meanings and MUST
  NOT be reused for empty-pool.

The following decisions are frozen for #847–#854 implementation: the
selection algorithm and 128-bit seed above, monotonic per-Service uint64 pool
versions, immutable task-row JSONB routing snapshots, 422 fields named
`service` and `resolution`, and vision search remaining outside the pool.

## Status

Accepted
