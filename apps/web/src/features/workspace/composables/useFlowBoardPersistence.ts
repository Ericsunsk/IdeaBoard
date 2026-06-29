import { computed } from 'vue'
import {
  canvasBoardSnapshotToFlow,
  flowToCanvasBoardSnapshot,
  type FlowAdapterEdge,
  type FlowAdapterNode,
} from '../utils/flowSnapshotAdapter'
import { createEmptyCanvasBoardSnapshot, type CanvasBoardSnapshot } from '../../canvas-board/domain'
import type { WorkspaceBoardSession } from '../board-session'

type SaveStatus = 'idle' | 'saved' | 'failed'
type ValueRef<T> = { value: T }
type ArrayRef = { value: unknown[] }
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

export type UseFlowBoardPersistenceOptions = {
  nodes: ArrayRef
  edges: ArrayRef
  boardBaseSnapshot: ValueRef<CanvasBoardSnapshot>
  isDirty: ValueRef<boolean>
  saveStatus: ValueRef<SaveStatus>
  isLoadingBoardSnapshot: ValueRef<boolean>
  isSavingBoard: ValueRef<boolean>
  boardSession: WorkspaceBoardSession
  getViewport: () => { x: number; y: number; zoom: number }
  setViewport: (viewport: { x: number; y: number; zoom: number }) => void
  onWarning: (text: string) => void
  onSuccess: (text: string) => void
  onError: (text: string) => void
  refreshSnapshot: () => void
  clearFlowHistoryBaseline: () => void
  commitNodes: (nodes: unknown[]) => void
  commitEdges: (edges: unknown[]) => void
  onGraphCommitted?: () => void
}

export function useFlowBoardPersistence(options: UseFlowBoardPersistenceOptions) {
  let saveSequence = 0
  let loadSequence = 0

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

  function readBoardIdFromQuery() {
    return options.boardSession.currentBoardId.value
  }

  const currentBoardId = options.boardSession.currentBoardId
  const currentBoardExists = computed(() => {
    const boardId = currentBoardId.value
    if (!boardId) return false
    return options.boardSession.exists(boardId)
  })
  const canSaveBoard = computed(
    () =>
      !!currentBoardId.value && currentBoardExists.value && !options.isLoadingBoardSnapshot.value,
  )

  function clearDirtyAfterImport() {
    options.isDirty.value = false
    options.saveStatus.value = canSaveBoard.value ? 'saved' : 'idle'
  }

  function applyBoardSnapshot(snapshot: CanvasBoardSnapshot) {
    const flow = canvasBoardSnapshotToFlow(snapshot)
    options.commitNodes(flow.nodes as unknown[])
    options.commitEdges(flow.edges as unknown[])
    options.boardBaseSnapshot.value = snapshot
    const vp = snapshot.viewport
    if (
      vp &&
      typeof vp.canvasX === 'number' &&
      typeof vp.canvasY === 'number' &&
      typeof vp.zoomLevel === 'number'
    ) {
      void Promise.resolve().then(() => {
        options.setViewport({
          x: vp.canvasX,
          y: vp.canvasY,
          zoom: Math.max(0.1, vp.zoomLevel / 100),
        })
      })
    }
    clearDirtyAfterImport()
    options.clearFlowHistoryBaseline()
    options.refreshSnapshot()
    options.onGraphCommitted?.()
  }

  function importEmptyBoardSnapshot() {
    applyBoardSnapshot(createEmptyCanvasBoardSnapshot())
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
    const isLoadStillCurrent = () => loadToken === loadSequence && currentBoardId.value === boardId
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
      console.warn('[workspace] load board snapshot failed', error)
      options.onError('导入画布快照失败，请稍后重试')
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
    const snapshot = flowToCanvasBoardSnapshot(
      {
        nodes: options.nodes.value as unknown as FlowAdapterNode[],
        edges: options.edges.value as unknown as FlowAdapterEdge[],
      },
      options.boardBaseSnapshot.value,
    )
    const vp = options.getViewport()
    snapshot.viewport = {
      canvasX: vp.x,
      canvasY: vp.y,
      zoomLevel: Math.round(vp.zoom * 100),
    }
    return snapshot
  }

  async function saveCurrentBoardSnapshot(
    saveOptions: SaveCurrentBoardSnapshotOptions = {},
  ): Promise<boolean> {
    const boardId = currentBoardId.value
    if (!boardId) {
      options.onWarning('缺少 boardId，无法保存')
      return false
    }
    const saveToken = ++saveSequence
    const isSaveStillCurrent = () => currentBoardId.value === boardId
    options.isSavingBoard.value = true
    try {
      const snapshot = buildCurrentBoardSnapshot()
      const { snapshot: stabilizedSnapshot } = await options.boardSession.save(snapshot, {
        waitForPreview: saveOptions.waitForPreview,
      })
      const shouldCommitToCurrentGraph = isSaveStillCurrent()
      if (shouldCommitToCurrentGraph) {
        options.boardBaseSnapshot.value = stabilizedSnapshot
        const nextFlow = canvasBoardSnapshotToFlow(stabilizedSnapshot)
        if (!hasPendingImageResultNode(options.nodes.value)) {
          options.commitNodes(nextFlow.nodes as unknown[])
          options.commitEdges(nextFlow.edges as unknown[])
          options.clearFlowHistoryBaseline()
        }
        options.isDirty.value = false
        options.saveStatus.value = 'saved'
        if (!saveOptions.silent) {
          options.onSuccess('已保存')
        }
      }
      if (isSaveStillCurrent()) {
        options.refreshSnapshot()
        options.onGraphCommitted?.()
      }
      return true
    } catch (error) {
      if (isSaveStillCurrent()) {
        options.saveStatus.value = 'failed'
      }
      console.warn('[workspace] save board snapshot failed', error)
      if (isSaveStillCurrent()) {
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
    currentBoardId,
    canSaveBoard,
    loadInitialOrRouteBoard,
    saveCurrentBoardSnapshot,
  }
}
