/**
 * Gallery subdomain public entrypoint.
 * Re-exports the unified gallery domain surface for board features and routes.
 */

// Views
export { default as BoardsPage } from './views/BoardsPage.vue'

// Controllers
export { createGalleryController } from './controllers/galleryController'
export { createCommandHandlers } from './controllers/commandHandlers'
export { createCatalog, useCatalog } from './controllers/useCatalog'
export {
  createBoardFeedback,
  useBoardFeedback,
  type BoardFeedback,
} from './controllers/useBoardFeedback'

// Styling, Skin & Contracts
export { blankBoardPreviewSurfaceClass } from './styling/skin'
export { formatCatalogDate } from './styling/format'
export type {
  CatalogEntry,
  CatalogSortMode,
  Catalog,
  CatalogNavigation,
} from './styling/contract'
