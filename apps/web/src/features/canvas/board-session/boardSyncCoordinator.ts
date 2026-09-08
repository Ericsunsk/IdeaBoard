import {
  createEmptyCanvasBoardSnapshot,
  InvalidBoardStateError,
  type CanvasBoardSnapshot,
} from '../../canvas-board/domain'
import type { CanvasBoardSession } from './useCanvasBoardSession'
import { i18n } from '../../../shared/i18n'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import { measureCanvasPerf, measureCanvasPerfAsync } from '../utils/canvasPerfProbe'
import { rebaseStabilizedMedia } from './boardSyncMediaReconciliation'

export type SaveStatus = 'idle' | 'saved' | 'pending' | 'retry' | 'conflict' | 'recover'

type SaveFailureKind = 'sync-failed' | 'conflict' | 'save-failed'
export type ValueRef<T> = { value: T }
export type SaveCurrentBoardSnapshotOptions = {
  waitForPreview?: boolean | undefined
  silent?: boolean | undefined
}

export type BoardSyncCoordinatorOptions = {
  buildCurrentSnapshot: () => CanvasBoardSnapshot
  isDirty: ValueRef<boolean>
  saveStatus: ValueRef<SaveStatus>
  graphRevision: ValueRef<number>
  isLoadingBoardSnapshot: ValueRef<boolean>
  isSavingBoard: ValueRef<boolean>
  boardSession: CanvasBoardSession
  readBoardAttachmentEpoch: () => number
  canApplyReconciliation: () => boolean
  onWarning: (text: string) => void
  onSuccess: (text: string) => void
  onError: (text: string) => void
}

export type BoardSyncCoordinator = {
  canSaveBoard: () => boolean
  loadInitialOrRouteBoard: () => Promise<CanvasBoardLoadResult>
  saveCurrentBoardSnapshot: (
    options?: SaveCurrentBoardSnapshotOptions,
  ) => Promise<CanvasBoardSaveResult>
}

export type CanvasBoardLoadResult =
  | { status: 'empty'; snapshot: CanvasBoardSnapshot }
  | { status: 'loaded'; snapshot: CanvasBoardSnapshot }
  | { status: 'not-found' }
  | { status: 'stale' }
  | { status: 'failed' }

export type CanvasBoardSaveReconciliation = {
  snapshot: CanvasBoardSnapshot
  hasLocalDraftAdvanced: boolean
}

export type CanvasBoardSaveResult = {
  saved: boolean
  reconciliation?: CanvasBoardSaveReconciliation | undefined
}

function cloneCanvasBoardSnapshot(snapshot: CanvasBoardSnapshot): CanvasBoardSnapshot {
  // Canvas snapshots are JSON domain data. JSON serialization also unwraps any Vue
  // proxies that reached this boundary without retaining references to live state.
  return JSON.parse(JSON.stringify(snapshot)) as CanvasBoardSnapshot
}

function isRecoverableSaveStatus(status: SaveStatus): boolean {
  return status === 'retry' || status === 'conflict' || status === 'recover'
}

export function createBoardSyncCoordinator(
  options: BoardSyncCoordinatorOptions,
): BoardSyncCoordinator {
  let saveSequence = 0
  let loadSequence = 0
  let deferredMediaStabilization: {
    captured: CanvasBoardSnapshot
    stabilized: CanvasBoardSnapshot
  } | null = null
  let lastNotifiedSaveFailure: { boardId: string; kind: SaveFailureKind } | null = null

  function shouldNotifySaveFailure(
    silent: boolean | undefined,
    boardId: string,
    kind: SaveFailureKind,
  ): boolean {
    const isRepeatedSilentFailure =
      silent &&
      lastNotifiedSaveFailure?.boardId === boardId &&
      lastNotifiedSaveFailure.kind === kind
    if (isRepeatedSilentFailure) return false
    lastNotifiedSaveFailure = { boardId, kind }
    return true
  }

  function clearNotifiedSaveFailure(boardId: string) {
    if (lastNotifiedSaveFailure?.boardId === boardId) {
      lastNotifiedSaveFailure = null
    }
  }

  function readBoardIdFromQuery() {
    return options.boardSession.currentBoardId.value
  }

  function clearDirtyAfterImport() {
    options.isDirty.value = false
    options.saveStatus.value = canSaveBoard() ? 'saved' : 'idle'
  }

  function applyLoadedSnapshot() {
    deferredMediaStabilization = null
    clearDirtyAfterImport()
  }

  function importEmptyBoardSnapshot() {
    const snapshot = createEmptyCanvasBoardSnapshot()
    applyLoadedSnapshot()
    return snapshot
  }

  function notifyLoadFailure(error: unknown) {
    if (error instanceof InvalidBoardStateError) {
      options.onError(i18n.global.t('canvas.session.boardDataInvalid'))
      return
    }
    options.onError(i18n.global.t('canvas.session.importSnapshotFailed'))
  }

  async function importBoardSnapshot(): Promise<CanvasBoardLoadResult> {
    const boardId = readBoardIdFromQuery()
    if (!boardId) {
      loadSequence += 1
      options.isLoadingBoardSnapshot.value = false
      return { status: 'empty', snapshot: importEmptyBoardSnapshot() }
    }
    const loadToken = ++loadSequence
    deferredMediaStabilization = null
    const isLoadStillCurrent = () =>
      loadToken === loadSequence && options.boardSession.currentBoardId.value === boardId
    options.isLoadingBoardSnapshot.value = true
    try {
      const snapshot = await options.boardSession.load(boardId)
      if (!isLoadStillCurrent()) return { status: 'stale' }
      if (!snapshot) {
        // A route board ID must resolve to an entry in the hydrated catalog.
        // Treating a missing entry as an empty board creates an editable
        // phantom canvas that can never be persisted safely.
        if (!options.boardSession.exists(boardId)) {
          return { status: 'not-found' }
        }
        return { status: 'empty', snapshot: importEmptyBoardSnapshot() }
      }
      applyLoadedSnapshot()
      return { status: 'loaded', snapshot }
    } catch (error) {
      if (!isLoadStillCurrent()) return { status: 'stale' }
      console.warn('[canvas] load board snapshot failed', error)
      notifyLoadFailure(error)
      return { status: 'failed' }
    } finally {
      if (loadToken === loadSequence) {
        options.isLoadingBoardSnapshot.value = false
      }
    }
  }

  async function loadInitialOrRouteBoard() {
    const boardId = readBoardIdFromQuery()
    if (boardId) {
      return importBoardSnapshot()
    }
    return { status: 'empty', snapshot: importEmptyBoardSnapshot() } satisfies CanvasBoardLoadResult
  }

  function buildCurrentBoardSnapshot(): CanvasBoardSnapshot {
    return measureCanvasPerf(
      'board.save.buildSnapshot',
      () => ({
        boardId: options.boardSession.currentBoardId.value,
      }),
      () => {
        const current = options.buildCurrentSnapshot()
        if (!deferredMediaStabilization) return current
        return rebaseStabilizedMedia(
          deferredMediaStabilization.captured,
          current,
          deferredMediaStabilization.stabilized,
        )
      },
    )
  }

  function readGraphRevision(): number {
    return Number(options.graphRevision.value)
  }

  function notifyRemoteSyncFailure(reason: 'sync-failed' | 'conflict') {
    if (reason === 'conflict') {
      options.onError(i18n.global.t('canvas.session.nameConflictSavedLocally'))
      return
    }
    options.onWarning(i18n.global.t('canvas.session.savedLocallySyncFailed'))
  }

  type BoardSaveStep = Awaited<ReturnType<CanvasBoardSession['save']>>

  function reportSaveSyncOutcome(
    boardId: string,
    saveOptions: SaveCurrentBoardSnapshotOptions,
    remoteSync: BoardSaveStep['remoteSync'],
    hasLocalDraftAdvanced: boolean,
    canApplyReconciliation: boolean,
  ): void {
    if (remoteSync.ok) {
      const hasUnreconciledDraft = hasLocalDraftAdvanced || !canApplyReconciliation
      options.isDirty.value = hasUnreconciledDraft
      options.saveStatus.value = hasUnreconciledDraft ? 'pending' : 'saved'
      if (!saveOptions.silent && !hasUnreconciledDraft) {
        options.onSuccess(i18n.global.t('canvas.session.saved'))
      }
      return
    }
    options.isDirty.value = true
    const failureStatus = remoteSync.reason === 'conflict' ? 'conflict' : 'retry'
    options.saveStatus.value = failureStatus
    if (shouldNotifySaveFailure(saveOptions.silent, boardId, remoteSync.reason)) {
      notifyRemoteSyncFailure(remoteSync.reason)
    }
  }

  function settleCommittedSave(args: {
    boardId: string
    saveOptions: SaveCurrentBoardSnapshotOptions
    saveToken: number
    saveGraphRevision: number
    capturedSnapshot: CanvasBoardSnapshot
    stabilizedSnapshot: CanvasBoardSnapshot
    remoteSync: BoardSaveStep['remoteSync']
    previousSaveStatus: SaveStatus
    isSaveStillCurrent: () => boolean
  }): CanvasBoardSaveReconciliation | undefined {
    if (!args.isSaveStillCurrent()) {
      if (
        args.saveToken === saveSequence &&
        (options.saveStatus.value === 'pending' || options.saveStatus.value === 'recover')
      ) {
        options.saveStatus.value = args.previousSaveStatus
      }
      return undefined
    }
    const hasLocalDraftAdvanced = readGraphRevision() !== args.saveGraphRevision
    const currentSnapshot = hasLocalDraftAdvanced ? buildCurrentBoardSnapshot() : null
    const committedSnapshot = currentSnapshot
      ? rebaseStabilizedMedia(args.capturedSnapshot, currentSnapshot, args.stabilizedSnapshot)
      : args.stabilizedSnapshot
    const canApplyReconciliation = options.canApplyReconciliation()
    if (canApplyReconciliation) {
      deferredMediaStabilization = null
    } else {
      deferredMediaStabilization = {
        captured: args.capturedSnapshot,
        stabilized: args.stabilizedSnapshot,
      }
    }
    reportSaveSyncOutcome(
      args.boardId,
      args.saveOptions,
      args.remoteSync,
      hasLocalDraftAdvanced,
      canApplyReconciliation,
    )
    return canApplyReconciliation
      ? { snapshot: committedSnapshot, hasLocalDraftAdvanced }
      : undefined
  }

  function settleSaveFailure(
    error: unknown,
    boardId: string,
    saveOptions: SaveCurrentBoardSnapshotOptions,
    isSaveStillCurrent: () => boolean,
  ): void {
    if (isSaveStillCurrent()) {
      options.saveStatus.value = 'retry'
    }
    console.warn('[canvas] save board snapshot failed', error)
    if (
      isSaveStillCurrent() &&
      shouldNotifySaveFailure(saveOptions.silent, boardId, 'save-failed')
    ) {
      options.onError(userFacingErrorCopy(error, i18n.global.t('canvas.session.saveFailed')))
    }
  }

  async function saveCurrentBoardSnapshot(
    saveOptions: SaveCurrentBoardSnapshotOptions = {},
  ): Promise<CanvasBoardSaveResult> {
    const boardId = options.boardSession.currentBoardId.value
    if (!boardId) {
      options.onWarning(i18n.global.t('canvas.session.missingBoardIdSave'))
      return { saved: false }
    }
    const saveToken = ++saveSequence
    const saveAttachmentEpoch = options.readBoardAttachmentEpoch()
    const isSaveStillCurrent = () =>
      saveToken === saveSequence &&
      options.boardSession.currentBoardId.value === boardId &&
      options.readBoardAttachmentEpoch() === saveAttachmentEpoch
    const saveGraphRevision = readGraphRevision()
    const previousSaveStatus = options.saveStatus.value
    options.isSavingBoard.value = true
    options.saveStatus.value = isRecoverableSaveStatus(options.saveStatus.value)
      ? 'recover'
      : 'pending'
    try {
      const snapshot = buildCurrentBoardSnapshot()
      const capturedSnapshot = cloneCanvasBoardSnapshot(snapshot)
      const { snapshot: stabilizedSnapshot, remoteSync } = await measureCanvasPerfAsync(
        'board.save.session',
        () => ({
          imageNodes: snapshot.imageNodes.length,
          promptNodes: snapshot.promptNodes.length,
          fusionNodes: snapshot.fusionNodes.length,
          connections: snapshot.connections.length,
          waitForPreview: !!saveOptions.waitForPreview,
        }),
        () =>
          options.boardSession.save(snapshot, {
            waitForPreview: saveOptions.waitForPreview,
          }),
        50,
      )
      if (remoteSync.ok) {
        clearNotifiedSaveFailure(boardId)
      }
      const reconciliation = settleCommittedSave({
        boardId,
        saveOptions,
        saveToken,
        saveGraphRevision,
        capturedSnapshot,
        stabilizedSnapshot,
        remoteSync,
        previousSaveStatus,
        isSaveStillCurrent,
      })
      return {
        saved: remoteSync.ok,
        reconciliation,
      }
    } catch (error) {
      settleSaveFailure(error, boardId, saveOptions, isSaveStillCurrent)
      return { saved: false }
    } finally {
      if (saveToken === saveSequence) {
        options.isSavingBoard.value = false
      }
    }
  }

  function canSaveBoard(): boolean {
    const boardId = options.boardSession.currentBoardId.value
    if (!boardId) return false
    return options.boardSession.exists(boardId) && !options.isLoadingBoardSnapshot.value
  }

  return {
    canSaveBoard,
    loadInitialOrRouteBoard,
    saveCurrentBoardSnapshot,
  }
}
