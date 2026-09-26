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
