# Generation Catalog Context

Generation owns Service Model Pools and routing decisions. `internal/generation/catalog`
owns their persistence; the sibling `routing` package owns eligibility, selection,
and immutable Routing Snapshots. Model catalog metadata belongs to AI Gateway.

## Domain language

**Service**: An executable AI image capability defined by Workflow; Vision Search is separate.

**Service Model Pool**: The unordered set of Models configured for one Service and their enabled intent and supported Output Resolutions.

**Model Pool Entry**: One Model's membership and capabilities in one Service Model Pool.

**Output Resolution**: The requested output-size class: `1k`, `2k`, or `4k`.
_Avoid_: Default resolution as a fourth capability; Display Tier as a synonym.

**Pool Version**: The revision of one Service Model Pool's effective configuration.

**Routing Snapshot**: The immutable selection facts accepted for a Task, including its eligible Models, selected Model, and retry candidate.
_Avoid_: Live pool lookup as the meaning of an accepted Task's routing.

## Rules

- Services are validated by `workflow.IsAIImageFeature`; `vision` is rejected. Pool entries have `UNIQUE(service, model_id)` and no priority, fallback, or weight fields.
- Pool versions start at 1; read-model 0 means never written. `default` is not a request, price, or pool capability.
- ReplacePool is the only pool writer: advisory-locked, OCC-guarded,
  fully replacing, single-transaction. No-op saves do not bump.
- PoolStore (`ListPool`) never errors on emptiness; store outages
  propagate unwrapped and must never render `service_resolution_unavailable`.
- Snapshot save/load validate via `RoutingSnapshot.Verify`. Missing or
  corrupt snapshots fail closed; never reconstruct from the live pool.
- Catalog persists pools and Routing Snapshots. `routing` owns selection and retry classification; Tasking owns submission and execution lifecycle; Billing owns prices; Admin owns presentation and operator commands.
