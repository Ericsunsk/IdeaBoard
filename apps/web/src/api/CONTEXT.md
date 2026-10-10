# API Context

> P1b split: transport and generated-query ownership relocated from `src/shared/CONTEXT.md#API Boundary`. This file owns the HTTP exit, transport contracts, and generated client policy. Horizontal contracts live in `src/core/CONTEXT.md`.

## Domain Language

| Term         | Meaning                                                                                                                                                                                                                                 |
| ------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| API Boundary | `Vue UI -> application use case -> resource/adapter -> generated API`; `orvalMutator` is the sole fetcher for all generated files; features call via `api/*` wrappers or their own `*Resource.ts`; generated code is never hand-edited. |
| Query Client | The single TanStack Query singleton (`stale 5s` / `gc 10m` / `retry 1` / no refocus); identity change clears all via `clearAuthScopedQueryCache`; features never build their own `QueryClient`.                                         |

## P1b Generated-Query Relocation (Complete)

P1b relocates the generated subtree opaquely from `src/shared/api/generated-query/**` to `src/api/generated-query/**` (contents byte-identical) with drift verification before/after. Orval output (`public/` + `control/` per `apps/web/orval.config.ts`; 417 files per `generatedMapping` in `docs/architecture/frontend-contract-inventory.md`) is never hand-edited. Backend `apps/api/internal/db/sqlc/*.go` is out of frontend scope.

- Generator baseline: `apps/api/openapi.public.yaml` + `apps/api/openapi.control.yaml` via `npm run api:generate`; drift gate `npm run api:check-generated` (`apps/web/scripts/checkApiGeneratedDrift.ts`); mutators `orvalMutator.ts` / `publicMutator.ts` / `controlMutator.ts` live in `src/api/`.
- Rules: (1) no hand edits to generated contents; (2) no relocation outside P1b; P1b updates hard-coded Orval/drift/cruiser/purity/governance paths in the same change (plan §5.5 P1b / §10.4).
- Consumers: all imports use the new `src/api/*` paths; shims at `src/shared/api/*.shim.ts` preserve the old specifiers for one phase. Shared-internal relative wrappers (`api/*`, `core/composables`) and feature resource/adapter absolute imports (`account`, `admin/*`, `board`, `library`, `search`, `workspace` selection snapshot) are canonical; route pages/components must not import generated code directly (via resource/adapter only).

## Public Seams

- `api` sole HTTP exit (same-origin ofetch to `/api`, 10s timeout, converged auth failure), `requireContractData`/`resolveContractData` envelope parsing, task/upload/media dispatch, SSE user stream, `stripUndefined`; never depends on `features/*`. Media delivery is split by responsibility: `mediaAssetUrls` (pure URL parsing/generation), `mediaAssetMetadata` (HTTP/cache/invalidation transport), `useMediaAssetDelivery` (Vue lifecycle binding); `mediaAssetDelivery` remains the stable import seam.
- `api/generated-query` Orval output (`public/` + `control/`); never hand-edited; route pages never call directly.

## Query Key Conventions (`QUERY_KEY_CONVENTIONS`)

- Prefer the generated orval `getXQueryKey()` helper when the operation exists (workspace list, control users, model catalog); never hand-roll a key for a generated op.
- Hand-rolled `*QueryKeys` factories (or `*_QUERY_ROOT` + cursor-index consts for cursor collections) exist only for composed queries with no generated key helper (account infra resources, admin health/monitor/statistics, run-detail, finding/run-history lists).
- No inline array literals in `queryKey:` positions or invalidation/removal calls: reference the factory or root const (`adminStatisticsQueryKeys`, `ADMIN_TASK_LIST_QUERY_KEY`, `walletQueryRoot`, `RUN_HISTORY_QUERY_ROOT`).
- Invalidation targets the narrowest stable root (prefix) for scope clears (`walletQueryRoot`, `[...RUN_HISTORY_QUERY_ROOT]`); composed keys append criteria/cursor after the root.
- Freshness/polling options ride `core/query/queryDefaults` presets (`noRetry` / `accountScoped` / `adminPolling` / `keepPrevious`); visibility-aware polling stays on `useQueryPolling`, never in a preset.

## Invariants

- Transport-DTO to feature-model translation happens in adapter/resource layers; domain code never imports Vue, router, Query, or generated DTOs.
- Auth, retry, correlation, and error classification live centrally in `api` and `core/auth`; features never roll their own.
- Never call generated APIs from pages directly; never hand-edit wrappers; never add a second fetcher.
- Never build per-feature clients; never cache session identity.

## Non-goals

- No feature-to-feature coupling through `api`; `api` never imports concrete features.
