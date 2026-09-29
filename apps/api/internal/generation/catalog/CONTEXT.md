# Catalog CONTEXT

English agent doc for `internal/generation/catalog` (persistence + catalog
ownership, #848 wave 1). User-facing docs stay in Simplified Chinese.

## Domain language

- Service: product capability (`generation`, `inpaint`, `render`,
  `variations`, `cutout`, `fusion`, `upscale`, `outpaint`). Validated
  against `workflow.IsAIImageFeature`; `vision` is rejected.
- Model pool entry: `{model, enabled, supported_resolutions}` per
  service. No priority/fallback/weight. `UNIQUE(service, model_id)`.
- Capability: `1k`, `2k`, or `4k` only. `default` is never a capability.
- Pool version: per-service monotonic uint64, opaque to routing.
  Stored `>= 1`; read-model `0` means never written.
- RoutingSnapshot: immutable submit-time fact, `routing.RoutingSnapshot`
  JSON on `tasks.routing_snapshot`. Workers consume only this.

## Rules

- ReplacePool is the only pool writer: advisory-locked, OCC-guarded,
  fully replacing, single-transaction. No-op saves do not bump.
- PoolStore (`ListPool`) never errors on emptiness; store outages
  propagate unwrapped and must never render `service_resolution_unavailable`.
- Snapshot save/load validate via `RoutingSnapshot.Verify`. Missing or
  corrupt snapshots fail closed; never reconstruct from the live pool.
- This package owns persistence + catalog only. Selection, retry
  classification, submit, worker, billing, and Admin UI belong to the
  sibling wave / later waves. Do not add them here.
