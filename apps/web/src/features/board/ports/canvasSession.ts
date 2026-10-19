/** Browser persistence, catalog, and synchronization for Canvas Board Sessions. */

// Local persistence and document adapters.
export { loadBoardState, setBoardState } from '../session/persistence/state'
export { serializeBoardState } from '../session/persistence/boardStateSerialization'

// Catalog and revision-gated sync use cases.
export { getBoardById, normalizeBoardName, touchBoard } from '../session/catalog/boards'
export { useBoardRepository, type BoardRepository } from '../session/catalog/repository'
export {
  fetchBoardArchivedAt,
  isBoardArchivedError,
  isBoardSyncConflictError,
  markBoardArchivedLocally,
  syncBoardToServer,
  syncRestoreBoardToServer,
  type BoardSyncConflictKind,
} from '../session/sync/boardSync'

export { useSharePanel } from '../session/share/useSharePanel'
export { default as ShareDialog } from '../ui/share/ShareDialog.vue'

// Only confirmed revocation purges protected state; transport failures do not.
export {
  isBoardAccessLost,
  isBoardAccessStale,
  isConfirmedAccessLoss,
} from '../session/revocation/accessLoss'
export {
  observeOpenBoardAccessLoss,
  revalidateAndConvergeBoardAccess,
  subscribeBoardAccessLost,
} from '../session/revocation/convergeAccessLoss'

export { getBoardScope, isBoardScopeCurrent, type BoardScope } from '../session/catalog/scope'

// Room admission uses the sole Board generated-query adapter.
export { requestBoardRealtimeTicket } from '../session/adapters/boardsResource'
