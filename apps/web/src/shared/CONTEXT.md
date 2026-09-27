# Shared Context
Shared is the only horizontal dependency layer: HTTP exit, transport contracts, presentational components, cross-feature primitives, and theme.
It does NOT depend on any concrete feature module at runtime (except 3 contract-spec source reads for token checks).

## Domain Language
| Term | Meaning |
| ---- | ------- |
| API Boundary | `Vue UI -> application use case -> resource/adapter -> generated API`; `orvalMutator` is the sole fetcher for all generated files; features call via `shared/api/*` wrappers or their own `*Resource.ts`; generated code is never hand-edited. |
| Error Catalog | Stable-`code` errors go through `shared/errors/errorCatalog.ts` (`userFacingErrorCopy`); no prose matching or scattered copy; only backend `msg` marked `message_safe` is shown through. |
| Query Client | The single TanStack Query singleton (`stale 5s` / `gc 10m` / `retry 1` / no refocus); identity change clears all via `clearAuthScopedQueryCache`; features never build their own `QueryClient`. |
| MediaRef | Canonical media fact (`assetId + tier + format`); URLs are runtime-delivery details via `buildMediaBrowserURL` and are never persisted; Canvas facts never store URLs. |
| Control State | Interactive control quad-state (`idle` / `selected` / `disabled` / `loading` + `aria-busy` / `aria-pressed`), resolved by `shared/interaction`; styles consume, never define. |
| Theme Contract | Single source for standard UI primitives and CSS tokens (`src/components/ui` + `shared/theme`); overlay/dialog/round-control styles must consume tokens; contract specs reverse-check canvas/search/board/library/workspace usage. |
| Locale Parity | `en` resident, `zh` lazy; `en` / `zh-CN` / `es` / `ko` keys must match (`localeParity` contract); locale switch atomically updates i18n + `<html lang>` + storage, and writes nothing on failure. |

## Public Seams
- `shared/api` sole HTTP exit (same-origin ofetch to `/api`, 10s timeout, converged auth failure), `requireContractData`/`resolveContractData` envelope parsing, task/upload/media dispatch, SSE user stream, `stripUndefined`; never depends on `features/*`.
- `shared/auth` Better Auth singleton, safe-redirect policy (`normalizeSafeInternalRedirectPath` against `//`, `\`, `%5c`, `..`), password and OAuth provider policy; never depends on feature route components.
- `shared/components` pure display components (account menu, balance button, confirm box, language picker, table frame, chart card); balance numbers are injected, never self-fetched; never depends on feature stores/resources.
- `shared/composables` `useAppToast`, `useAsyncAction` (pending count + lifecycle hooks), confirm box, overlay lifecycle, brand, logout redirect; never depends on feature composables.
- `shared/config` `API_BASE_URL` (production forces `/api`), `AUTH_BASE_URL` derivation, asset/upload ref normalization; never depends on business constants.
- `shared/constants` single source for route paths, legal links, media constants; never depends on i18n copy.
- `shared/domain` workflow dictionary (8 slugs), generation primitives (`outputCount` 1-4 strictly rejects, never clamps), `outputResolution`; never depends on backend SKU/pricing.
- `shared/errors` error-code convergence and user-facing copy; never depends on scattered `t('...')` error copy.
- `shared/i18n` i18n singleton, atomic locale switch, `"N Idea(s)"` currency format; never conflicts with feature-private keys.
- `shared/interaction` quad-state resolution; never depends on concrete component styles.
- `shared/lib` dependency-free pure functions (brand, page chrome, chart primitives); never depends on DOM/Query.
- `shared/media` MediaRef and upload pipeline (magic-byte sniffing, HEIC-to-JPEG, compression thresholds, fail-back-to-original); never stores URLs in Canvas facts.
- `shared/motion` duration/easing tokens plus dialog/popover/chart presets (`motionPolicyGuard.spec` guards hard-codes); never depends on business logic.
- `shared/query` queryClient singleton, task-result normalization (only `role=image + asset_id`), user-credits invalidation pub/sub; credits invalidation must use this channel.
- `shared/theme` PT single source + CSS tokens; never depends on feature-authored overlay/dialog styles.
- `shared/utils` client identity single source (`get`/`set`/`clearStoredAuth` with Query clear), error-copy scrubbing, date/form/upload helpers; `localStorage['user']` only via `shared/utils/auth.ts`.
- `shared/seo` route SEO meta and static HTML; never depends on business data.
- `shared/assets` provider/storage/oauth pure static assets; never referenced except by code import.

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
