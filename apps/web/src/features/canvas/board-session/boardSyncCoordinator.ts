import {
  createEmptyCanvasBoardSnapshot,
  InvalidBoardStateError,
  rebuildCanvasBoardMediaRefs,
  type CanvasBoardSnapshot,
  type ImageNodeOutputItem,
  type PromptNodeInputImage,
} from '../../canvas-board/domain'
import type { CanvasBoardSession } from './useCanvasBoardSession'
import { measureCanvasPerf, measureCanvasPerfAsync } from '../utils/canvasPerfProbe'

export type SaveStatus = 'idle' | 'saved' | 'pending' | 'retry' | 'conflict' | 'recover'

type SaveFailureKind = 'sync-failed' | 'conflict' | 'save-failed'
export type ValueRef<T> = { value: T }
export type SaveCurrentBoardSnapshotOptions = {
  waitForPreview?: boolean
  silent?: boolean
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
  canSaveBoard: () => boolean
  canApplyReconciliation: () => boolean
  onWarning: (text: string) => void
  onSuccess: (text: string) => void
  onError: (text: string) => void
}

export type BoardSyncCoordinator = {
  loadInitialOrRouteBoard: () => Promise<CanvasBoardLoadResult>
  saveCurrentBoardSnapshot: (
    options?: SaveCurrentBoardSnapshotOptions,
  ) => Promise<CanvasBoardSaveResult>
}

export type CanvasBoardLoadResult =
  | { status: 'empty'; snapshot: CanvasBoardSnapshot }
  | { status: 'loaded'; snapshot: CanvasBoardSnapshot }
  | { status: 'stale' }
  | { status: 'failed' }

export type CanvasBoardSaveReconciliation = {
  snapshot: CanvasBoardSnapshot
  hasLocalDraftAdvanced: boolean
}

export type CanvasBoardSaveResult = {
  saved: boolean
  reconciliation?: CanvasBoardSaveReconciliation
}

type MediaAssetReference = Pick<ImageNodeOutputItem, 'url' | 'mediaAssetId'>

function hasSameMediaAsset(left: MediaAssetReference, right: MediaAssetReference): boolean {
  return left.url === right.url && left.mediaAssetId === right.mediaAssetId
}

function applyStabilizedMediaAsset<T extends MediaAssetReference>(
  current: T,
  captured: MediaAssetReference,
  stabilized: MediaAssetReference,
): T {
  if (!hasSameMediaAsset(current, captured) || hasSameMediaAsset(captured, stabilized)) {
    return current
  }
  const next = { ...current, url: stabilized.url }
  if (stabilized.mediaAssetId === undefined) delete next.mediaAssetId
  else next.mediaAssetId = stabilized.mediaAssetId
  return next
}

function rebaseIndexedMediaAssets<T extends MediaAssetReference>(
  current: T[],
  captured: MediaAssetReference[],
  stabilized: MediaAssetReference[],
): T[] {
  return current.map((item, index) => {
    const capturedItem = captured[index]
    const stabilizedItem = stabilized[index]
    return capturedItem && stabilizedItem
      ? applyStabilizedMediaAsset(item, capturedItem, stabilizedItem)
      : item
  })
}

function rebaseInputMediaAssets(
  current: PromptNodeInputImage[],
  captured: PromptNodeInputImage[],
  stabilized: PromptNodeInputImage[],
): PromptNodeInputImage[] {
  const capturedById = new Map(captured.map((item) => [item.id, item] as const))
  const stabilizedById = new Map(stabilized.map((item) => [item.id, item] as const))
  return current.map((item) => {
    const capturedItem = capturedById.get(item.id)
    const stabilizedItem = stabilizedById.get(item.id)
    return capturedItem && stabilizedItem
      ? applyStabilizedMediaAsset(item, capturedItem, stabilizedItem)
      : item
  })
}

function rebaseStabilizedMedia(
  captured: CanvasBoardSnapshot,
  current: CanvasBoardSnapshot,
  stabilized: CanvasBoardSnapshot,
): CanvasBoardSnapshot {
  const capturedImageNodes = new Map(captured.imageNodes.map((node) => [node.id, node] as const))
  const stabilizedImageNodes = new Map(stabilized.imageNodes.map((node) => [node.id, node] as const))
  const capturedPromptNodes = new Map(captured.promptNodes.map((node) => [node.id, node] as const))
  const stabilizedPromptNodes = new Map(stabilized.promptNodes.map((node) => [node.id, node] as const))
  const capturedFusionNodes = new Map(captured.fusionNodes.map((node) => [node.id, node] as const))
  const stabilizedFusionNodes = new Map(stabilized.fusionNodes.map((node) => [node.id, node] as const))
  const rebased: CanvasBoardSnapshot = {
    ...current,
    imageNodes: current.imageNodes.map((node) => {
      const capturedNode = capturedImageNodes.get(node.id)
      const stabilizedNode = stabilizedImageNodes.get(node.id)
      if (!capturedNode || !stabilizedNode) return node
      const source = applyStabilizedMediaAsset(
        { url: node.sourceImageUrl, mediaAssetId: node.sourceMediaAssetId },
        { url: capturedNode.sourceImageUrl, mediaAssetId: capturedNode.sourceMediaAssetId },
        { url: stabilizedNode.sourceImageUrl, mediaAssetId: stabilizedNode.sourceMediaAssetId },
      )
      return {
        ...node,
        sourceImageUrl: source.url,
        sourceMediaAssetId: source.mediaAssetId,
        outputImages: rebaseIndexedMediaAssets(node.outputImages, capturedNode.outputImages, stabilizedNode.outputImages),
      }
    }),
    promptNodes: current.promptNodes.map((node) => {
      const capturedNode = capturedPromptNodes.get(node.id)
      const stabilizedNode = stabilizedPromptNodes.get(node.id)
      return capturedNode && stabilizedNode
        ? { ...node, inputImages: rebaseInputMediaAssets(node.inputImages, capturedNode.inputImages, stabilizedNode.inputImages) }
        : node
    }),
    fusionNodes: current.fusionNodes.map((node) => {
      const capturedNode = capturedFusionNodes.get(node.id)
      const stabilizedNode = stabilizedFusionNodes.get(node.id)
      return capturedNode && stabilizedNode
        ? {
            ...node,
            inputImages: rebaseInputMediaAssets(node.inputImages, capturedNode.inputImages, stabilizedNode.inputImages),
            outputImages: rebaseIndexedMediaAssets(node.outputImages, capturedNode.outputImages, stabilizedNode.outputImages),
          }
        : node
    }),
    mediaRefs: current.mediaRefs?.map((ref) => ({ ...ref })),
  }
  rebuildCanvasBoardMediaRefs(rebased, current)
  return rebased
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
  let deferredMediaStabilization:
    | { captured: CanvasBoardSnapshot; stabilized: CanvasBoardSnapshot }
    | null = null
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
    options.saveStatus.value = options.canSaveBoard() ? 'saved' : 'idle'
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
      options.onError('画布数据校验失败，已保留当前画布，请稍后重试')
      return
    }
    options.onError('导入画布快照失败，请稍后重试')
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
      options.onError('已保存到本地，但画布名称已存在，请更换后再试')
      return
    }
    options.onWarning('已保存到本地，远端同步失败，将在网络恢复后重试')
  }

  async function saveCurrentBoardSnapshot(
    saveOptions: SaveCurrentBoardSnapshotOptions = {},
  ): Promise<CanvasBoardSaveResult> {
    const boardId = options.boardSession.currentBoardId.value
    if (!boardId) {
      options.onWarning('缺少 boardId，无法保存')
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
      const shouldCommitToCurrentGraph = isSaveStillCurrent()
      let reconciliation: CanvasBoardSaveReconciliation | undefined
      if (shouldCommitToCurrentGraph) {
        const hasLocalDraftAdvanced = readGraphRevision() !== saveGraphRevision
        const currentSnapshot = hasLocalDraftAdvanced ? buildCurrentBoardSnapshot() : null
        const committedSnapshot = currentSnapshot
          ? rebaseStabilizedMedia(capturedSnapshot, currentSnapshot, stabilizedSnapshot)
          : stabilizedSnapshot
        const canApplyReconciliation = options.canApplyReconciliation()
        if (canApplyReconciliation) {
          deferredMediaStabilization = null
          reconciliation = { snapshot: committedSnapshot, hasLocalDraftAdvanced }
        } else {
          deferredMediaStabilization = {
            captured: capturedSnapshot,
            stabilized: stabilizedSnapshot,
          }
        }
        if (remoteSync.ok) {
          const hasUnreconciledDraft = hasLocalDraftAdvanced || !canApplyReconciliation
          options.isDirty.value = hasUnreconciledDraft
          options.saveStatus.value = hasUnreconciledDraft ? 'pending' : 'saved'
          if (!saveOptions.silent && !hasUnreconciledDraft) {
            options.onSuccess('已保存')
          }
        } else {
          options.isDirty.value = true
          const failureStatus = remoteSync.reason === 'conflict' ? 'conflict' : 'retry'
          options.saveStatus.value = failureStatus
          if (shouldNotifySaveFailure(saveOptions.silent, boardId, remoteSync.reason)) {
            notifyRemoteSyncFailure(remoteSync.reason)
          }
        }
      } else if (
        saveToken === saveSequence &&
        (options.saveStatus.value === 'pending' || options.saveStatus.value === 'recover')
      ) {
        options.saveStatus.value = previousSaveStatus
      }
      return {
        saved: remoteSync.ok,
        reconciliation,
      }
    } catch (error) {
      if (isSaveStillCurrent()) {
        options.saveStatus.value = 'retry'
      }
      console.warn('[canvas] save board snapshot failed', error)
      if (
        isSaveStillCurrent() &&
        shouldNotifySaveFailure(saveOptions.silent, boardId, 'save-failed')
      ) {
        options.onError(error instanceof Error ? error.message : '保存失败，请稍后重试')
      }
      return { saved: false }
    } finally {
      if (saveToken === saveSequence) {
        options.isSavingBoard.value = false
      }
    }
  }

  return {
    loadInitialOrRouteBoard,
    saveCurrentBoardSnapshot,
  }
}
