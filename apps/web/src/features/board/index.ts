/**
 * Board feature public entrypoint.
 *
 * `board` is the only Board repository/synchronization fact source.
 * Cross-feature consumers import Board facts through this barrel;
 * Canvas `board-session` orchestrates through the narrow `ports/canvas` entry.
 *
 * Export groups are intentionally classified by responsibility:
 * - core: durable Board document facts and pure validation rules
 * - session: catalog and sync use cases/ports
 * - session/persistence + session/preview: stable adapters with contracts
 * - ui/gallery: catalog view composition consumed by route wiring
 *
 * The generated API adapter remains private to Board session. Consumers
 * must not import `session/adapters` or any other Board implementation path.
 */
// Durable Board facts and pure domain rules.
export * from './core/document/nodes'
export * from './core/document/nodeRegistry'
export * from './core/document/latestBoardState'
export * from './core/document/fusionBlend'
export * from './core/document/imageNodeSizing'
export * from './core/document/promptAspectRatio'
export * from './core/schema/stateSchema'
export * from './core/schema/boardStateEnvelopeContract'
export * from './core/media/references'
// Session use cases and repository/sync ports.
export * from './session/catalog/boards'
export * from './session/catalog/repository'
export * from './session/catalog/commands'
export * from './session/catalog/scope'
export {
  hydrateBoardsFromServer,
  syncBoardToServer,
  syncBoardRenameToServer,
  clearPendingBoardSyncRollback,
  syncDeleteBoardToServer,
  getBoardSyncConflictKind,
  isBoardSyncConflictError,
  isBoardArchivedError,
  fetchBoardArchivedAt,
  markBoardArchivedLocally,
  syncRestoreBoardToServer,
  type HydrateBoardsFromServerResult,
  type HydrateBoardsFromServerOptions,
  type BoardSyncConflictKind,
} from './session/sync/boardSync'
export * from './session/sync/boardQueryKeys'
export * from './session/sync/boardQueries'
export * from './session/sync/boardSseInvalidation'
// Stable adapters for persistence and gallery scene boundaries.
export * from './session/persistence/state'
export * from './session/persistence/boardStateSerialization'
export { blankBoardPreviewSurfaceClass } from './ui/gallery/skin'
export {
  captureShareScope,
  classifyShareError,
  invalidateShareQueries,
  isShareScopeCurrent,
  mintShareAttempt,
  type ShareActionScope,
  type ShareAttempt,
  type ShareFailureReason,
  type ShareTransport,
} from './session/share/commands'
export {
  readShareBoardFacts,
  shareAccessOptions,
  toShareBoardFacts,
  type ShareBoardFacts,
} from './session/share/queries'
export {
  useSharePanel,
  type SharePanel,
  type SharePanelTarget,
} from './session/share/useSharePanel'
export {
  useShareDialog,
  type ShareDialogController,
  type ShareDialogDeps,
} from './session/share/useShareDialog'
export {
  createShareFeedback,
  failureCopyKey,
  useShareFeedback,
  type ShareFeedback,
} from './session/share/feedback'
export { default as ShareDialog } from './ui/share/ShareDialog.vue'
// Catalog presentation composition (route-level public surface).
export type {
  CatalogEntry,
  CatalogSortMode,
  Catalog,
  CatalogNavigation,
} from './ui/gallery/contract'
export { createCatalog, formatCatalogDate, useCatalog } from './ui/gallery/useCatalog'
export { createGalleryController } from './ui/gallery/controller'
export { createCommandHandlers } from './ui/gallery/commandHandlers'
export {
  createBoardFeedback,
  useBoardFeedback,
  type BoardFeedback,
} from './ui/gallery/useBoardFeedback'
export { default as BoardsPage } from './ui/gallery/BoardsPage.vue'
