# Search Context

Search owns the public image-text Feed, paid image-based search, Library bridging, anonymous conversion gate, and post-login continuation.
It does NOT own the Library gallery, Canvas, or auth overlay UI.

## Domain Language

| Term                         | Meaning                                                                                                                                                                                                                           |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Search Feed                  | Feed fact owner in `application/createSearchFeed`: `results`, `nextCursor`, `hasMore`, `loading`, `loadingMore`, `hasError`, `savedKeys`, `failedImages`; stale requests cancelled in order via `AbortController` + `generation`. |
| Submitted Query              | `route.query.q`, the single source of truth for search; `useSearchSession` drives `feed.load(true)` through an `immediate` watcher.                                                                                               |
| Vision Reference             | Image-search input reference: library asset ID or external reference plus optional `File`/`blob` preview; only the serializable part is persisted.                                                                                |
| Library Bridge / Coordinator | Pure mapping between Feed and Library (`SearchResult`/`LibraryAssetView` <-> `VisionReference`) plus pub/sub that writes saved state back to the feed.                                                                            |
| Auth Continuation            | Pending intent resumed after login for anonymous actions (search, save, open Library); `vision-search` intent stays memory-only, others may use `sessionStorage`.                                                                 |
| Scope                        | Storage isolation key `userPublicID ?? anonymous`; post-login scope switch migrates only vision references and disposes the old scope.                                                                                            |

## Public Seams

- `features/search/index.ts` unified public export: exports domain models (`SearchResult`, `SearchPage`) and intent-continuation API (`clearPendingSearchAuthAction`).
- `ports/` (feature-root hexagonal seam): `libraryCoordinatorPort` + `persistencePort` are the only application↔infrastructure contracts; application and infrastructure never import each other directly.
- `SearchPage` the only route page (`meta.public`); pure assembly, lazy-loaded by `workspaceRoutes` via `useSearchScreenRuntime`.
- Search viewport components virtualized masonry viewport, save dialog, batch dock; callback props only (`load-next-page`, `scroll-position`), no data ownership.
- `application/createSearchFeed` feed factory; `reload` keeps cards via `keepResultsOnNextLoad`.
- `infrastructure/resource.ts` sole generated-API caller for inspiration search and vision extraction; `inFlightRequests` dedup, query capped at 200 rune.
- `infrastructure/visionBillingResource.ts` quote-only pricing display with `sku=ai-search:vision`.
- `requireLogin` sets the pending intent and opens the overlay; `resumePendingAction` resumes it after login.
- `infrastructure/libraryBridge` pure mapping; `infrastructure/libraryCoordinator` pub/sub over `provide/inject search:library-coordinator`.
- `infrastructure/browserPersistence` scope-isolated `sessionStorage` primitives via `searchScopedStorageKey(:scope)`; `anonymous-query-gate` dedups by query with a cap of 5.

Provider IDs and recognition share the pure `shared/domain/externalImageProviders` vocabulary with Admin; payload mapping and validation errors remain feature-owned.

## Invariants

- `?q` is the single source of truth: draft is input-box only, submit writes the URL via `router.replace`, feed reacts only to the URL.
- Never use `useQuery` for feed or server-derived state; `savedKeys` manual backfill must carry `savedMutationRevision` against stale overwrites.
- Never cache the feed with `useQuery`; never leave pagination uncancelled.
- Never treat draft as source of truth; never duplicate query refs.
- Vision references: max 4 refs, single file 10MB, batch max 8; `File`/`blob` never enters storage; `blob:` URL revoke is owned by the reference set.
- Feed snapshot has 30min TTL plus scroll position; snapshot writes defer to idle; SWR hydrates the old snapshot as first-screen skeleton.
- `savedKeys` rollback on error: with cache, roll back cursor/`hasMore` to keep one page consistent; empty results show the retry panel; `AbortError` is ignored.
- Feed pagination: `idle` -> `load(reset)` aborts old requests -> `idle` (`hasMore = cursor`); `load(false)` with `hasMore && !loadingMore` -> `loadingMore` with bottom spinner.
- Vision flow: `idle` -> `submit` (refs present + logged in, fingerprint dedup) -> `extracting` -> `ready` (extracted terms written back to `submittedQuery` and results loaded).
- Vision failure (stillProcessing, empty result, throw) -> `idle + error` toast; deleting a ref -> `markStale` (keep results) or clearing all -> `reset` and write back `?q`.
- Auth continuation: anonymous `submit`/`save`/`openLibrary` -> same-query dedup (over 5 new queries is blocked) -> `requireLogin` (set pending + open overlay) -> on login with scope switch -> `resumePendingAction`.
- Resume policy: text intent writes `?q` then consumes; vision consumes only when resumed to ready; save consumes only after every item succeeds; open-library sets `island.open = true`; overlay close runs `clearPending`.
- Cross-feature touches only `auth/authOverlay` (`requireLogin`) and the `features/library` public export via the bridge; it never owns the gallery and never touches Canvas.
- Never keep feed-owned library state; never mutate the gallery directly.
- Never lose post-login intent; never leak anonymous state across scopes; never read cross-user cache; never bypass the anonymous query gate.
- Lifecycle isolation uses per-user `effectScope`; the old scope must be disposed.
- Underlying writes go through the `features/library` public export; the bridge only projects and writes back.
- `hold`/`capture` is never shown on the search side; it only enters the admin ledger.

## Non-goals

- No gallery ownership; no Canvas editing.
- No second fetcher or hand-edited generated wrappers.
- No persisted `blob:` URLs or oversized reference sets.
