import {
  canvasBoardSnapshotToFlow,
  flowToCanvasBoardSnapshot,
  type FlowAdapterEdge,
  type FlowAdapterNode,
} from '../utils/flowSnapshotAdapter'
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
export type ArrayRef = { value: unknown[] }

export type SaveCurrentBoardSnapshotOptions = {
  waitForPreview?: boolean
  silent?: boolean
}

export type FlowGraphCommitOptions = {
  source: 'hydrate-replace' | 'save-reconcile'
}

export type BoardPersistenceSessionOptions = {
  fullNodes: ArrayRef
  fullEdges: ArrayRef
  boardBaseSnapshot: ValueRef<CanvasBoardSnapshot>
  isDirty: ValueRef<boolean>
  saveStatus: ValueRef<SaveStatus>
  graphRevision: ValueRef<number>
  isLoadingBoardSnapshot: ValueRef<boolean>
  isSavingBoard: ValueRef<boolean>
  boardSession: CanvasBoardSession
  readBoardAttachmentEpoch: () => number
  canSaveBoard: () => boolean
  onWarning: (text: string) => void
  onSuccess: (text: string) => void
  onError: (text: string) => void
  clearFlowHistoryBaseline: () => void
  commitGraph: (
    graph: { nodes: unknown[]; edges: unknown[] },
    options: FlowGraphCommitOptions,
  ) => void
}

export type BoardPersistenceSession = {
  loadInitialOrRouteBoard: () => Promise<void>
  saveCurrentBoardSnapshot: (options?: SaveCurrentBoardSnapshotOptions) => Promise<boolean>
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
  const stabilizedImageNodes = new Map(
    stabilized.imageNodes.map((node) => [node.id, node] as const),
  )
  const capturedPromptNodes = new Map(captured.promptNodes.map((node) => [node.id, node] as const))
  const stabilizedPromptNodes = new Map(
    stabilized.promptNodes.map((node) => [node.id, node] as const),
  )
  const capturedFusionNodes = new Map(captured.fusionNodes.map((node) => [node.id, node] as const))
  const stabilizedFusionNodes = new Map(
    stabilized.fusionNodes.map((node) => [node.id, node] as const),
  )
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
        outputImages: rebaseIndexedMediaAssets(
          node.outputImages,
          capturedNode.outputImages,
          stabilizedNode.outputImages,
        ),
      }
    }),
    promptNodes: current.promptNodes.map((node) => {
      const capturedNode = capturedPromptNodes.get(node.id)
      const stabilizedNode = stabilizedPromptNodes.get(node.id)
      return capturedNode && stabilizedNode
        ? {
            ...node,
            inputImages: rebaseInputMediaAssets(
              node.inputImages,
              capturedNode.inputImages,
              stabilizedNode.inputImages,
            ),
          }
        : node
    }),
    fusionNodes: current.fusionNodes.map((node) => {
      const capturedNode = capturedFusionNodes.get(node.id)
      const stabilizedNode = stabilizedFusionNodes.get(node.id)
      return capturedNode && stabilizedNode
        ? {
            ...node,
            inputImages: rebaseInputMediaAssets(
              node.inputImages,
              capturedNode.inputImages,
              stabilizedNode.inputImages,
            ),
            outputImages: rebaseIndexedMediaAssets(
              node.outputImages,
              capturedNode.outputImages,
              stabilizedNode.outputImages,
            ),
          }
        : node
    }),
    mediaRefs: current.mediaRefs?.map((ref) => ({ ...ref })),
  }
  rebuildCanvasBoardMediaRefs(rebased, current)
  return rebased
}

function isRecoverableSaveStatus(status: SaveStatus): boolean {
  return status === 'retry' || status === 'conflict' || status === 'recover'
}

export function createBoardPersistenceSession(
  options: BoardPersistenceSessionOptions,
): BoardPersistenceSession {
  let saveSequence = 0
  let loadSequence = 0
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

  function applyBoardSnapshot(snapshot: CanvasBoardSnapshot) {
    const flow = canvasBoardSnapshotToFlow(snapshot)
    options.boardBaseSnapshot.value = snapshot
    options.commitGraph(flow as { nodes: unknown[]; edges: unknown[] }, {
      source: 'hydrate-replace',
    })
    clearDirtyAfterImport()
    options.clearFlowHistoryBaseline()
  }

  function importEmptyBoardSnapshot() {
    applyBoardSnapshot(createEmptyCanvasBoardSnapshot())
  }

  function notifyLoadFailure(error: unknown) {
    if (error instanceof InvalidBoardStateError) {
      options.onError('画布数据校验失败，已保留当前画布，请稍后重试')
      return
    }
    options.onError('导入画布快照失败，请稍后重试')
  }

  async function importBoardSnapshot() {
    const boardId = readBoardIdFromQuery()
    if (!boardId) {
      loadSequence += 1
      options.isLoadingBoardSnapshot.value = false
      importEmptyBoardSnapshot()
      return
    }
    const loadToken = ++loadSequence
    const isLoadStillCurrent = () =>
      loadToken === loadSequence && options.boardSession.currentBoardId.value === boardId
    options.isLoadingBoardSnapshot.value = true
    try {
      const snapshot = await options.boardSession.load(boardId)
      if (!isLoadStillCurrent()) return
      if (!snapshot) {
        importEmptyBoardSnapshot()
        return
      }
      applyBoardSnapshot(snapshot)
    } catch (error) {
      if (!isLoadStillCurrent()) return
      console.warn('[canvas] load board snapshot failed', error)
      notifyLoadFailure(error)
    } finally {
      if (loadToken === loadSequence) {
        options.isLoadingBoardSnapshot.value = false
      }
    }
  }

  async function loadInitialOrRouteBoard() {
    const boardId = readBoardIdFromQuery()
    if (boardId) {
      await importBoardSnapshot()
      return
    }
    importEmptyBoardSnapshot()
  }

  function buildCurrentBoardSnapshot(): CanvasBoardSnapshot {
    return measureCanvasPerf(
      'board.save.buildSnapshot',
      () => ({
        fullNodes: options.fullNodes.value.length,
        fullEdges: options.fullEdges.value.length,
      }),
      () => {
        return flowToCanvasBoardSnapshot(
          {
            nodes: options.fullNodes.value as unknown as FlowAdapterNode[],
            edges: options.fullEdges.value as unknown as FlowAdapterEdge[],
          },
          options.boardBaseSnapshot.value,
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
  ): Promise<boolean> {
    const boardId = options.boardSession.currentBoardId.value
    if (!boardId) {
      options.onWarning('缺少 boardId，无法保存')
      return false
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
      const capturedSnapshot = structuredClone(snapshot)
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
      if (shouldCommitToCurrentGraph) {
        const hasLocalDraftAdvanced = readGraphRevision() !== saveGraphRevision
        const currentSnapshot = hasLocalDraftAdvanced ? buildCurrentBoardSnapshot() : null
        const committedSnapshot = currentSnapshot
          ? rebaseStabilizedMedia(capturedSnapshot, currentSnapshot, stabilizedSnapshot)
          : stabilizedSnapshot
        options.boardBaseSnapshot.value = committedSnapshot
        const nextFlow = canvasBoardSnapshotToFlow(committedSnapshot)
        const isRemoteSynced = remoteSync.ok
        options.commitGraph(nextFlow as { nodes: unknown[]; edges: unknown[] }, {
          source: 'save-reconcile',
        })
        if (isRemoteSynced && !hasLocalDraftAdvanced) {
          options.clearFlowHistoryBaseline()
        }
        if (isRemoteSynced) {
          options.isDirty.value = hasLocalDraftAdvanced
          options.saveStatus.value = hasLocalDraftAdvanced ? 'pending' : 'saved'
          if (!saveOptions.silent && !hasLocalDraftAdvanced) {
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
      return remoteSync.ok
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
      return false
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
