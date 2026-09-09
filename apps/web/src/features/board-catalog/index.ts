/**
 * Board catalog feature public entrypoint (issue #683).
 *
 * External consumers (routes, cross-feature seams) import the catalog
 * through this barrel. Internal modules stay on relative imports.
 */
export { createBoardCatalog, formatCatalogDate, useBoardCatalog } from './useBoardCatalog'
export type {
  BoardCatalog,
  BoardCatalogEntry,
  BoardCatalogNavigation,
  BoardCatalogSortMode,
} from './contract'
export { default as BoardsPage } from './pages/BoardsPage.vue'
