# API Context

This file owns the HTTP exit, transport contracts, and generated client policy. Horizontal contracts live in `src/core/CONTEXT.md`.

## Domain Language

| Term         | Meaning                                                                                                                                                                                                                                 |
| ------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| API Boundary | `Vue UI -> application use case -> resource/adapter -> generated API`; `orvalMutator` is the sole fetcher for all generated files; features call via `api/*` wrappers or their own `*Resource.ts`; generated code is never hand-edited. |
| Query Client | The single TanStack Query singleton (`stale 5s` / `gc 10m` / `retry 1` / no refocus); identity change clears all via `clearAuthScopedQueryCache`; features never build their own `QueryClient`.                                         |

## Generated Query Policy

Orval output (`public/` + `control/` per `apps/web/orval.config.ts`) lives under `src/api/generated-query/**` and is never hand-edited. Backend `apps/api/internal/db/sqlc/*.go` is out of frontend scope.

- Generator inputs: `apps/api/openapi.public.yaml` + `apps/api/openapi.control.yaml` via `npm run api:generate`; drift gate: `npm run api:check-generated` (`apps/web/scripts/checkApiGeneratedDrift.ts`).
- Mutators `orvalMutator.ts` / `publicMutator.ts` / `controlMutator.ts` live in `src/api/`.
- Consumers use `src/api/*` wrappers or feature-owned resource/adapter boundaries. Route pages/components must not import generated code directly.

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
- Never build per-feature clients or a second session cache; Auth owns the bounded session snapshot.

## Non-goals

- No feature-to-feature coupling through `api`; `api` never imports concrete features.
