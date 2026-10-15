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
