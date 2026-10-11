# Core Context

> P1b move: horizontal contracts relocated from `src/shared/CONTEXT.md`. `src/shared/CONTEXT.md` remains as a one-phase pointer; this file is the horizontal source of truth.

`src/core` owns horizontal primitives and infrastructure runtime; the directory is not entirely pure. `src/api`, `src/theme`, `src/lib` and `src/components` are horizontal roots with the same dependency-direction contract. HTTP and generated client policy live in [API Context](../api/CONTEXT.md).
Horizontal roots have zero runtime or type-only imports of concrete features. Contract specs may read feature source text to verify tokens without importing feature modules. Feature-owned account chrome and legal links are not horizontal primitives.

## Domain Language

| Term           | Meaning                                                                                                                                                                                                                     |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Error Catalog  | Stable-`code` errors go through `core/errors/errorCatalog.ts` (`userFacingErrorCopy`); no prose matching or scattered copy; only backend `msg` marked `message_safe` is shown through.                                      |
| Query Client   | The single TanStack Query singleton (`stale 5s` / `gc 10m` / `retry 1` / no refocus); identity change clears all via `clearAuthScopedQueryCache`; features never build their own `QueryClient`.                             |
| MediaRef       | Canonical media fact (`assetId + tier + format`); URLs are runtime-delivery details via `buildMediaBrowserURL` and are never persisted; Canvas facts never store URLs.                                                      |
| Control State  | Interactive control quad-state (`idle` / `selected` / `disabled` / `loading` + `aria-busy` / `aria-pressed`), resolved by `core/interaction`; styles consume, never define.                                                 |
| Theme Contract | Single source for standard UI primitives and CSS tokens (`src/components/ui` + `theme`); overlay/dialog/round-control styles must consume tokens; contract specs reverse-check canvas/search/board/library/workspace usage. |
| Locale Parity  | `en` resident, `zh` lazy; `en` / `zh-CN` / `es` / `ko` keys must match (`localeParity` contract); locale switch atomically updates i18n + `<html lang>` + storage, and writes nothing on failure.                           |

## Public Seams

- `core/auth` Better Auth singleton, safe-redirect policy (`normalizeSafeInternalRedirectPath` against `//`, `\`, `%5c`, `..`), password and OAuth provider policy. Pure `routePolicy` owns only identity-failure and safe-redirect route invariants; feature `routePaths.ts` re-export those values and own their remaining paths. Auth never imports concrete features.
- `core/components` horizontal display components (confirm box, table frame, chart card); never depends on feature stores/resources. Account menu and balance button belong to `features/account/ui/chrome`; balance numbers remain injected, never self-fetched.
- `core/composables` `useAppToast`, `useAsyncAction` (pending count + lifecycle hooks), confirm box, overlay lifecycle, brand, logout redirect; never depends on feature composables. `useAsyncAction` is for plain async actions; ad-hoc flags stay where the semantics differ — reentrancy guards that must throw (`useRewardRedemption.pending`), overlay dismiss guards (`busy` wired into `useOverlayInteractionLifecycle`), and page-lifecycle `loading` states.
- `core/config` `API_BASE_URL` (production forces `/api`), `AUTH_BASE_URL` derivation, asset/upload ref normalization; `pagination` (`PAGE_SIZES` + `normalizePageSize` for all cursor lists) and `timing` (cross-cutting transport waits only); never depends on business constants.
- `core/constants` horizontal media constants; never depends on i18n copy. Full route catalogs belong to `features/workspace/routePaths.ts`, `features/admin/routePaths.ts` and `features/legal/routePaths.ts`; legal links belong to `features/legal/legalLinks.ts`.
- `core/contracts` workflow dictionary (8 slugs), generation primitives (`outputCount` 1-4 strictly rejects, never clamps), `outputResolution`; never depends on backend SKU/pricing.
- `core/errors` error-code convergence and user-facing copy; never depends on scattered `t('...')` error copy.
- `core/i18n` i18n singleton, atomic locale switch, `"N Idea(s)"` currency format; never conflicts with feature-private keys.
- `core/interaction` quad-state resolution; never depends on concrete component styles.
- `core/lib` dependency-free pure functions (brand, page chrome, chart primitives); never depends on DOM/Query.
- `core/media` MediaRef and upload pipeline (magic-byte sniffing, HEIC-to-JPEG, compression thresholds, fail-back-to-original); never stores URLs in Canvas facts.
- `core/motion` duration/easing tokens plus dialog/popover/chart presets (`motionPolicyGuard.spec` guards hard-codes); never depends on business logic.
- `core/query` queryClient singleton, task-result normalization (only `role=image + asset_id`), user-credits invalidation pub/sub; `queryDefaults` presets (`noRetry` / `accountScoped` / `adminPolling` / `keepPrevious`) own all freshness/polling overrides; credits invalidation must use this channel.
- `theme` PT single source + CSS tokens; never depends on feature-authored overlay/dialog styles.
- `core/utils` client identity single source (`get`/`set`/`clearStoredAuth` with Query clear), error-copy scrubbing, date/form/upload helpers; `localStorage['user']` only via `core/utils/auth.ts`.
- `core/seo` route SEO meta and static HTML; never depends on business data.
- `core/assets` horizontal logo and OAuth static assets; never referenced except by code import. Admin provider/storage icons belong to `features/admin/shared/ui/assets`.

## Copy Policy (User vs Admin Surfaces)

| Surface                                                                                                                                                                 | Rule                                                                                                                                                                                                                                                                                                                          |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| User surfaces (`features/workspace`, `features/account`, `features/auth`, `features/board`, `features/canvas`, `features/search`, `features/library`, `features/legal`) | User-visible copy lives only in `core/i18n` locale bundles and is referenced by key. `en` is canonical; `zh-CN` / `es` / `ko` must match it exactly in keys and shape (`localeParity`, enforced by `lint:locale-parity`).                                                                                                     |
| Admin/ops surfaces (`features/admin/*`)                                                                                                                                 | zh-only colocated messages: literals live next to the feedback factory or presenter that owns them (pattern: `features/admin/payments/messages.ts`); never wrapped in fake `t()` calls and never added to locale bundles. Admin toast adapters in `features/admin/shared/lib/toast.ts` use `core/composables/useAppToast.ts`. |

- `lint:user-copy` (`scripts/checkUserCopyLocale.ts` + `scripts/user-copy-locale-allowlist.json`) guards the boundary: CJK outside `features/admin` fails unless a reviewed allowlist entry marks it a true exception (protocol matchers, endonyms, backend-shaped test fixtures, admin-only shared helpers).
- New user-surface strings ship with all four locale bundles in the same change; never land a partial translation.

## Feedback Factory Map (per-surface split, intentional)

- User surfaces own one `*Feedback` factory each over `useAppToast` + locale keys: account `useProfileFeedback`, auth `useAuthFeedback`, board `createBoardFeedback`/`useBoardFeedback`, canvas `createCanvasFeedback`/`useCanvasFeedback`, library delete/remove confirmations in `components/confirmations.ts` over `useUserConfirmDialog`.
- Admin verticals own one factory each over `useAdminToast` + colocated zh messages: credits billing ops, reward codes, model catalog/gateway config, monitor task result/cancellation, admin login, stripe settings/payment orders, prompt config, settings (branding/access/media-upload/outbound/mail/search-provider/logo-file), storage, users.
- Factories never cross surfaces (no shared generic factory, no user-copy in admin factories, no zh literals in user factories); confirm dialogs split the same way (`UserConfirmDialog` + `useUserConfirmDialog` for user surfaces, `AdminConfirmDialog` for admin).

## Query Key Conventions (`QUERY_KEY_CONVENTIONS`)

- Prefer the generated orval `getXQueryKey()` helper when the operation exists (workspace list, control users, model catalog); never hand-roll a key for a generated op.
- Hand-rolled `*QueryKeys` factories (or `*_QUERY_ROOT` + cursor-index consts for cursor collections) exist only for composed queries with no generated key helper (account infra resources, admin health/monitor/statistics, run-detail, finding/run-history lists).
- No inline array literals in `queryKey:` positions or invalidation/removal calls: reference the factory or root const (`adminStatisticsQueryKeys`, `ADMIN_TASK_LIST_QUERY_KEY`, `walletQueryRoot`, `RUN_HISTORY_QUERY_ROOT`).
- Invalidation targets the narrowest stable root (prefix) for scope clears (`walletQueryRoot`, `[...RUN_HISTORY_QUERY_ROOT]`); composed keys append criteria/cursor after the root.
- Freshness/polling options ride `core/query/queryDefaults` presets (`noRetry` / `accountScoped` / `adminPolling` / `keepPrevious`); visibility-aware polling stays on `useQueryPolling`, never in a preset.

## Query Scope Rule (which features use Query)

- TanStack Query lives in `account`, `admin/*`, `workspace` (selection snapshot), `board` (production server-snapshot ownership in ADR-0014), `core` (client singleton, site branding), and `api/generated-query` (orval hooks). Board `session/sync` owns the Query catalog list and one document entry per board; `session/catalog/repository.ts` binds production observers and projects the server view. The feature-owned store retains draft overlays. Gallery reads list-only snapshots; Canvas consumes the Board port's document/draft view without importing Query. Board key shapes, freshness, focus, clean/dirty guards and SSE policies are canonical in [Board Sync / Query Ownership](../features/board/CONTEXT.md). The P2.5 private design harness is retired; historical design acceptance and current P3 verification remain separate ledger records. `search`, `canvas`, and `library` use no TanStack Query directly: they coordinate through feature-owned stores, SSE subscriptions and ports. New cached server state must justify Query in the feature `CONTEXT.md`; otherwise follow the feature-store pattern.
- Polling choice: visibility-aware live lists use `useQueryPolling` (document-visibility cadence with hidden-tab backoff); fixed active/idle cadences without visibility gating use the `adminPolling` preset; raw `refetchInterval` literals appear only as named `ADMIN_*_REFRESH_INTERVAL_MS`-style consts for single-shot snapshots (e.g. the monitor page), never as inline numbers.

The P0 `governance/query-placement` rule remains in the actual ESLint config: Canvas product dependencies on Query packages/client/defaults are errors (including type dependencies); other unapproved feature usage is warn-only. Query provider specs and colocated testing helpers are exempt. `@tanstack/vue-virtual`, task-result normalization, and credit invalidation events are not Query cache dependencies. Auth may import only the named `clearAuthScopedQueryCache` identity-cleanup capability from the client module; this does not authorize cache reads. Both canonical `core/query` and retired `shared/query` paths are checked. Root `core` eligibility here concerns horizontal runtime placement only: `core/contracts`, `core/lib`, and feature-private pure core still must not acquire Query runtime dependencies. Module direction, transitive purity, and cross-feature deep imports remain dependency-cruiser's responsibility.

The following migration items are the canonical Query acceptance backlog, relocated with the runtime seam in P1. P2.5 delegated acceptance and local design/spec integration are recorded in the canonical ledger; P3 remains blocked until separately authorized and must establish its own before-start green checkpoint. Execution/review evidence is canonical in `docs/architecture/frontend-contract-inventory.md` (`behaviors` and `syncDesign`).

| ID                           | Level           | Owner              | Acceptance                                                                                                                                                                                                                                                                                                                                        |
| ---------------------------- | --------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `boards-query-migration`     | `todo-blocking` | P2.5/P3 Board lane | ADR-0014 and six executable permanent specs define the accepted Sync design; P2.5 delegated acceptance/local integration are complete, P3 production repository reads/observers, scope cancellation and SSE projections are implemented. Formal acceptance and frozen trio original red/green obligations remain pending in the canonical ledger. |
| `search-query-justification` | `todo-advisory` | P3 Search lane     | Keep store/SSE/explicit fetch coordination; document a feature-local justification before adding cached server state.                                                                                                                                                                                                                             |
| `account-query-hygiene`      | `todo-advisory` | P3 Account lane    | Consume Query defaults; keep visibility-aware polling in `useQueryPolling`, outside presets.                                                                                                                                                                                                                                                      |

## Invariants

- Prove three or more callers before adding a shared module; no generic layers for speculative reuse.
- Global state is limited to: queryClient singleton, identity store, media delivery cache, SSE singleton, i18n bundle cache, composable-local machines; no Pinia.
- Transport-DTO to feature-model translation happens in adapter/resource layers; domain code never imports Vue, router, Query, or generated DTOs.
- Auth, retry, correlation, and error classification live centrally in `api` and `core/auth`; features never roll their own.
- Never call generated APIs from pages directly; never hand-edit wrappers; never add a second fetcher.
- Never match errors by string; never leak unsafe messages.
- Never build per-feature clients; never cache session identity.
- Never persist provider URLs; never use tier-less asset refs.
- Never define per-component disabled logic; never ship controls without busy semantics.
- Never hand-author overlay CSS; never hard-code motion durations.
- Never ship partial translations or half-switched locales.

## Non-goals

- No feature-to-feature coupling through horizontal roots; only the reviewed legacy `shared/*.shim.ts` paths may forward to feature destinations for one phase.
- No Pinia or extra global stores.
- No backend SKU/pricing knowledge in `core/contracts`.
