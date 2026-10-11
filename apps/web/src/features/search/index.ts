// Public entrypoint for the search feature.
//
// Layout: `core/` owns pure models and policies; `session/` owns feed,
// session, and vision orchestration with `session/adapters/` as the only
// generated-API/storage boundary; `ui/` owns browser primitives,
// components, and the route page; `ports/` owns the application↔adapter
// contracts. Cross-feature consumers import Search facts only through this
// barrel; `screenRuntime` stays private to the route page loaded below.
export type {
  SearchResult,
  SearchPage,
  VisionReference,
  VisionExternalReference,
  SearchCatalogProvider,
} from './core/models'
export type { SearchAuthAction, LibrarySearchReference } from './core/authAction'
export { clearPendingSearchAuthAction } from './session/adapters/authContinuation'
export * from './session/adapters/workspace'

/**
 * Lazily loads the search page through the search entrypoint so route wiring
 * keeps its code-split chunk instead of importing `ui/` directly.
 */
export const loadSearchPage = () => import('./ui/pages/SearchPage.vue')
