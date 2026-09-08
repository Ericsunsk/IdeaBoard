// Public entrypoint for search feature
export type {
  SearchResult,
  SearchPage,
  VisionReference,
  VisionExternalReference,
  VisionExtraction,
  SearchCatalogProvider,
} from './domain/searchModels'
export type {
  SearchAuthAction,
  LibrarySearchReference,
} from './domain/searchAuthAction'
export { clearPendingSearchAuthAction } from './infrastructure/searchAuthContinuation'
