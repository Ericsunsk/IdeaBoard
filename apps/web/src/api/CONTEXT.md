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
- Mutators `orvalMutator.ts` / `publicMutator.ts` / `controlMutator.ts` live in `src/api/http/`.
- Consumers use `src/api/*` wrappers, subdomains (`src/api/{http,realtime,media,tasks}`), or feature-owned resource/adapter boundaries. Route pages/components must not import generated code directly.

## Public Seams

- `api` sole HTTP exit (same-origin ofetch to `/api`, 10s timeout, converged auth failure), `requireContractData`/`resolveContractData` envelope parsing, task/upload/media dispatch, SSE user stream, `stripUndefined`; never depends on `features/*`. Subdomain layout:
  - `src/api/http/`: client, contractKernel, stripUndefined, mutators, idempotencyKey, userSession, sessionUserType.
  - `src/api/realtime/`: workspaceEventStream (SSE).
  - `src/api/media/`: mediaAssetUrls (pure URL generation), mediaAssetMetadata (HTTP/cache/invalidation transport), mediaWorkspaceScope, mediaAssetDelivery, upload.
  - `src/api/tasks/`: workspaceTaskContract, workspaceTaskTransport, workspaceTaskWaiter, workspaceTasks.
    Media delivery composables (`useMediaAssetDelivery`) live in `src/core/media/` to keep `src/api` strictly free of Vue reactive lifecycle.
- `api/generated-query` Orval output (`public/` + `control/`); never hand-edited; route pages never call directly.

## Query Key Conventions (`QUERY_KEY_CONVENTIONS`)

- Prefer the generated orval `getXQueryKey()` helper when the operation exists (workspace list, control users, model catalog); never hand-roll a key for a generated op.
- Hand-rolled `*QueryKeys` factories (or `*_QUERY_ROOT` + cursor-index consts for cursor collections) exist only for composed queries with no generated key helper (account infra resources, admin health/monitor/statistics, run-detail, finding/run-history lists).
- No inline array literals in `queryKey:` positions or invalidation/removal calls: reference the factory or root const (`adminStatisticsQueryKeys`, `ADMIN_TASK_LIST_QUERY_KEY`, `walletQueryRoot`, `RUN_HISTORY_QUERY_ROOT`).
- Invalidation targets the narrowest stable root (prefix) for scope clears (`walletQueryRoot`, `[...RUN_HISTORY_QUERY_ROOT]`); composed keys append criteria/cursor after the root.
- Freshness/polling options ride `core/query/queryDefaults` presets (`noRetry` / `accountScoped` / `adminPolling` / `keepPrevious`); visibility-aware polling stays on `useQueryPolling`, never in a preset.

## Request-Count Hard Constraints

Same feature, fewer backend round trips. Three rules, no exceptions without a contract-spec entry:

1. **Single provider + scalar keys**: one screen shares one `useQuery` per domain (roster is the exemplar: MemberSection/LimitsSection/TransferSection/ShareRoster share `getListWorkspaceMembersQueryKey`). Keys carry only `workspaceId` / `billingAccountId` / `identity` scalars (`identity` suffix allowed for per-viewer isolation); never embed scope objects or arrays. No dual-key systems for the same endpoint (generated helper and hand-rolled factory must interop on one prefix, see `billingTransferQueryKeys.members`).
2. **Minimal invalidation**: default `exact: true` or predicate-scoped refresh; never invalidate `*.all` roots or the credits/billing/member-quota triple on a single-domain write (transfer invite/reject/cancel refresh invites + managers + roster; acceptance also refreshes the affected ownership, quota and consumption projections). `retry()` is per-error-state and single-instance, never an unconditional refetch-all.
3. **Lazy + aggregate-first**: `managers` / `quota-list` load on expand/role only; Canvas/Gallery default `shouldIncludeDocuments: false` and fetch a single document by `boardId`; new same-screen GETs go into `billing-overview` / `?expand=` first, never as new parallel endpoints.

Gates: every hand-rolled `*QueryKeys` / `*_QUERY_ROOT` factory must be registered in `src/features/account/session/queryKeyRequestBudget.contract.spec.ts` (unregistered factories fail CI); same-screen cold-cache distinct GETs over 4 must land as aggregate-read or lazy-load, not as parallel endpoints.

## Invariants

- Transport-DTO to feature-model translation happens in adapter/resource layers; domain code never imports Vue, router, Query, or generated DTOs.
- Auth, retry, correlation, and error classification live centrally in `api` and `core/auth`; features never roll their own.
- Never call generated APIs from pages directly; never hand-edit wrappers; never add a second fetcher.
- Never build per-feature clients or a second session cache; Auth owns the bounded session snapshot.

## Non-goals

- No feature-to-feature coupling through `api`; `api` never imports concrete features.
