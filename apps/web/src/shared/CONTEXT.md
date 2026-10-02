# Shared Context

Shared is the only horizontal dependency layer: HTTP exit, transport contracts, presentational components, cross-feature primitives, and theme.
It does NOT depend on any concrete feature module at runtime (except 3 contract-spec source reads for token checks).

## Domain Language

| Term           | Meaning                                                                                                                                                                                                                                        |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| API Boundary   | `Vue UI -> application use case -> resource/adapter -> generated API`; `orvalMutator` is the sole fetcher for all generated files; features call via `shared/api/*` wrappers or their own `*Resource.ts`; generated code is never hand-edited. |
| Error Catalog  | Stable-`code` errors go through `shared/errors/errorCatalog.ts` (`userFacingErrorCopy`); no prose matching or scattered copy; only backend `msg` marked `message_safe` is shown through.                                                       |
| Query Client   | The single TanStack Query singleton (`stale 5s` / `gc 10m` / `retry 1` / no refocus); identity change clears all via `clearAuthScopedQueryCache`; features never build their own `QueryClient`.                                                |
| MediaRef       | Canonical media fact (`assetId + tier + format`); URLs are runtime-delivery details via `buildMediaBrowserURL` and are never persisted; Canvas facts never store URLs.                                                                         |
| Control State  | Interactive control quad-state (`idle` / `selected` / `disabled` / `loading` + `aria-busy` / `aria-pressed`), resolved by `shared/interaction`; styles consume, never define.                                                                  |
| Theme Contract | Single source for standard UI primitives and CSS tokens (`src/components/ui` + `shared/theme`); overlay/dialog/round-control styles must consume tokens; contract specs reverse-check canvas/search/board/library/workspace usage.             |
| Locale Parity  | `en` resident, `zh` lazy; `en` / `zh-CN` / `es` / `ko` keys must match (`localeParity` contract); locale switch atomically updates i18n + `<html lang>` + storage, and writes nothing on failure.                                              |

## Public Seams

- `shared/api` sole HTTP exit (same-origin ofetch to `/api`, 10s timeout, converged auth failure), `requireContractData`/`resolveContractData` envelope parsing, task/upload/media dispatch, SSE user stream, `stripUndefined`; never depends on `features/*`. Media delivery is split by responsibility: `mediaAssetUrls` (pure URL parsing/generation), `mediaAssetMetadata` (HTTP/cache/invalidation transport), `useMediaAssetDelivery` (Vue lifecycle binding); `mediaAssetDelivery` remains the stable import seam.
- `shared/auth` Better Auth singleton, safe-redirect policy (`normalizeSafeInternalRedirectPath` against `//`, `\`, `%5c`, `..`), password and OAuth provider policy; never depends on feature route components.
- `shared/components` pure display components (account menu, balance button, confirm box, language picker, table frame, chart card); balance numbers are injected, never self-fetched; never depends on feature stores/resources.
- `shared/composables` `useAppToast`, `useAsyncAction` (pending count + lifecycle hooks), confirm box, overlay lifecycle, brand, logout redirect; never depends on feature composables. `useAsyncAction` is for plain async actions; ad-hoc flags stay where the semantics differ — reentrancy guards that must throw (`useRewardRedemption.pending`), overlay dismiss guards (`busy` wired into `useOverlayInteractionLifecycle`), and page-lifecycle `loading` states.
- `shared/config` `API_BASE_URL` (production forces `/api`), `AUTH_BASE_URL` derivation, asset/upload ref normalization; `pagination` (`PAGE_SIZES` + `normalizePageSize` for all cursor lists) and `timing` (cross-cutting transport waits only); never depends on business constants.
- `shared/constants` single source for route paths, legal links, media constants; never depends on i18n copy.
- `shared/domain` workflow dictionary (8 slugs), generation primitives (`outputCount` 1-4 strictly rejects, never clamps), `outputResolution`; never depends on backend SKU/pricing.
- `shared/errors` error-code convergence and user-facing copy; never depends on scattered `t('...')` error copy.
- `shared/i18n` i18n singleton, atomic locale switch, `"N Idea(s)"` currency format; never conflicts with feature-private keys.
- `shared/interaction` quad-state resolution; never depends on concrete component styles.
- `shared/lib` dependency-free pure functions (brand, page chrome, chart primitives); never depends on DOM/Query.
- `shared/media` MediaRef and upload pipeline (magic-byte sniffing, HEIC-to-JPEG, compression thresholds, fail-back-to-original); never stores URLs in Canvas facts.
- `shared/motion` duration/easing tokens plus dialog/popover/chart presets (`motionPolicyGuard.spec` guards hard-codes); never depends on business logic.
- `shared/query` queryClient singleton, task-result normalization (only `role=image + asset_id`), user-credits invalidation pub/sub; `queryDefaults` presets (`noRetry` / `accountScoped` / `adminPolling` / `keepPrevious`) own all freshness/polling overrides; credits invalidation must use this channel.
- `shared/theme` PT single source + CSS tokens; never depends on feature-authored overlay/dialog styles.
- `shared/utils` client identity single source (`get`/`set`/`clearStoredAuth` with Query clear), error-copy scrubbing, date/form/upload helpers; `localStorage['user']` only via `shared/utils/auth.ts`.
- `shared/seo` route SEO meta and static HTML; never depends on business data.
- `shared/assets` provider/storage/oauth pure static assets; never referenced except by code import.

## Copy Policy (User vs Admin Surfaces)

| Surface | Rule |
| ------- | ---- |
| User surfaces (`features/workspace`, `features/account`, `features/auth`, `features/board`, `features/canvas`, `features/search`, `features/library`, `features/legal`) | User-visible copy lives only in `shared/i18n` locale bundles and is referenced by key. `en` is canonical; `zh-CN` / `es` / `ko` must match it exactly in keys and shape (`localeParity`, enforced by `lint:locale-parity`). |
| Admin/ops surfaces (`features/admin/*`) | zh-only colocated messages: literals live next to the feedback factory or presenter that owns them (pattern: `features/admin/payments/messages.ts`); never wrapped in fake `t()` calls and never added to locale bundles. Toasts still exit only through `shared/lib/toast.ts`. |

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
- Freshness/polling options ride `shared/query/queryDefaults` presets (`noRetry` / `accountScoped` / `adminPolling` / `keepPrevious`); visibility-aware polling stays on `useQueryPolling`, never in a preset.

## Query Scope Rule (which features use Query)

- TanStack Query lives only in `account`, `admin/*`, `workspace` (selection snapshot), `shared` (client singleton, site branding), and `shared/api/generated-query` (orval hooks). `search`, `board`, `canvas`, and `library` use no TanStack Query: they coordinate through feature-owned stores, SSE subscriptions, and explicit fetch calls, so realtime canvas/board paths never inherit query-cache staleness semantics. New cached server state must justify Query in the feature `CONTEXT.md`; otherwise follow the feature-store pattern.
- Polling choice: visibility-aware live lists use `useQueryPolling` (document-visibility cadence with hidden-tab backoff); fixed active/idle cadences without visibility gating use the `adminPolling` preset; raw `refetchInterval` literals appear only as named `ADMIN_*_REFRESH_INTERVAL_MS`-style consts for single-shot snapshots (e.g. the monitor page), never as inline numbers.

## Invariants

- Prove three or more callers before adding a shared module; no generic layers for speculative reuse.
- Global state is limited to: queryClient singleton, identity store, media delivery cache, SSE singleton, i18n bundle cache, composable-local machines; no Pinia.
- Transport-DTO to feature-model translation happens in adapter/resource layers; domain code never imports Vue, router, Query, or generated DTOs.
- Auth, retry, correlation, and error classification live centrally in `shared/api` and `shared/auth`; features never roll their own.
- Never call generated APIs from pages directly; never hand-edit wrappers; never add a second fetcher.
- Never match errors by string; never leak unsafe messages.
- Never build per-feature clients; never cache session identity.
- Never persist provider URLs; never use tier-less asset refs.
- Never define per-component disabled logic; never ship controls without busy semantics.
- Never hand-author overlay CSS; never hard-code motion durations.
- Never ship partial translations or half-switched locales.

## Non-goals

- No feature-to-feature coupling through shared; shared never imports concrete features.
- No Pinia or extra global stores.
- No backend SKU/pricing knowledge in `shared/domain`.
