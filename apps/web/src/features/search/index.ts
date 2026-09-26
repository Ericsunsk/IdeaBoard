// Public entrypoint for search feature
export type {
  SearchResult,
  SearchPage,
  VisionReference,
  VisionExternalReference,
  SearchCatalogProvider,
} from './domain/models'
export type { SearchAuthAction, LibrarySearchReference } from './domain/authAction'
export { clearPendingSearchAuthAction } from './infrastructure/authContinuation'
export * from './infrastructure/workspace'

/**
 * Lazily loads the search page through the search entrypoint so route wiring
 * keeps its code-split chunk instead of importing `presentation/` directly.
 */
export const loadSearchPage = () => import('./presentation/pages/SearchPage.vue')
