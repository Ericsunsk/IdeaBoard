import {
  canvasBoardSnapshotToFlow,
  flowToCanvasBoardSnapshot,
  type FlowAdapterEdge,
  type FlowAdapterNode,
} from '../utils/flowSnapshotAdapter'
import {
  createEmptyCanvasBoardSnapshot,
  InvalidBoardStateError,
  type CanvasBoardSnapshot,
} from '../../canvas-board/domain'
import type { CanvasBoardSession } from './useCanvasBoardSession'
import { measureCanvasPerf, measureCanvasPerfAsync } from '../utils/canvasPerfProbe'

export type SaveStatus = 'idle' | 'saved' | 'pending' | 'retry' | 'conflict' | 'recover'

type SaveFailureKind = 'sync-failed' | 'conflict' | 'save-failed'
export type ValueRef<T> = { value: T }
export type ArrayRef = { value: unknown[] }

type FlowNodeLike = {
  data?: {
    snapshotNode?: {
      type?: unknown
      sourceImageUrl?: unknown
      pendingTaskId?: unknown
      outputStatus?: unknown
      outputImages?: unknown
    }
  }
}

export type SaveCurrentBoardSnapshotOptions = {
  waitForPreview?: boolean
  silent?: boolean
}

export type FlowGraphCommitOptions = {
  preserveSelection?: boolean
}

export type BoardPersistenceSessionOptions = {
  fullNodes: ArrayRef
  fullEdges: ArrayRef
  boardBaseSnapshot: ValueRef<CanvasBoardSnapshot>
  isDirty: ValueRef<boolean>
  saveStatus: ValueRef<SaveStatus>
  localEditVersion?: ValueRef<number>
  isLoadingBoardSnapshot: ValueRef<boolean>
  isSavingBoard: ValueRef<boolean>
  boardSession: CanvasBoardSession
  canSaveBoard: () => boolean
  onWarning: (text: string) => void
  onSuccess: (text: string) => void
  onError: (text: string) => void
  clearFlowHistoryBaseline: () => void
  commitGraph: (
    graph: { nodes: unknown[]; edges: unknown[] },
    options?: FlowGraphCommitOptions,
  ) => void
  onGraphCommitted?: () => void
}

export type BoardPersistenceSession = {
  loadInitialOrRouteBoard: () => Promise<void>
  saveCurrentBoardSnapshot: (options?: SaveCurrentBoardSnapshotOptions) => Promise<boolean>
}

function hasPendingImageResultNode(nodes: unknown[]): boolean {
  return nodes.some((node) => {
    const snapshotNode = (node as FlowNodeLike)?.data?.snapshotNode
    if (!snapshotNode || snapshotNode.type !== 'image-node') return false
    const sourceUrl =
      typeof snapshotNode.sourceImageUrl === 'string' ? snapshotNode.sourceImageUrl.trim() : ''
    const outputImages = Array.isArray(snapshotNode.outputImages) ? snapshotNode.outputImages : []
    const hasOutputImage = outputImages.some(
      (item) =>
        typeof (item as { url?: unknown })?.url === 'string' &&
        ((item as { url: string }).url ?? '').trim().length > 0,
    )
    const pendingTaskId =
      typeof snapshotNode.pendingTaskId === 'string' ? snapshotNode.pendingTaskId.trim() : ''
    return (
      snapshotNode.outputStatus === 'running' && !sourceUrl && !hasOutputImage && !pendingTaskId
    )
  })
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
      preserveSelection: false,
    })
    clearDirtyAfterImport()
    options.clearFlowHistoryBaseline()
    options.onGraphCommitted?.()
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

  function readLocalEditVersion(): number {
    return Number(options.localEditVersion?.value ?? 0)
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
    const isSaveStillCurrent = () => options.boardSession.currentBoardId.value === boardId
    const saveEditVersion = readLocalEditVersion()
    const previousSaveStatus = options.saveStatus.value
    options.isSavingBoard.value = true
    options.saveStatus.value = isRecoverableSaveStatus(options.saveStatus.value)
      ? 'recover'
      : 'pending'
    try {
      const snapshot = buildCurrentBoardSnapshot()
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
        const hasLocalDraftAdvanced = readLocalEditVersion() > saveEditVersion
        options.boardBaseSnapshot.value = stabilizedSnapshot
        const nextFlow = canvasBoardSnapshotToFlow(stabilizedSnapshot)
        const isRemoteSynced = remoteSync.ok
        if (!hasLocalDraftAdvanced && !hasPendingImageResultNode(options.fullNodes.value)) {
          options.commitGraph(nextFlow as { nodes: unknown[]; edges: unknown[] }, {
            preserveSelection: true,
          })
          if (isRemoteSynced) {
            options.clearFlowHistoryBaseline()
          }
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
      if (isSaveStillCurrent()) {
        options.onGraphCommitted?.()
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
