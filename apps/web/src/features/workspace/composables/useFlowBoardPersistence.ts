import { computed } from 'vue'
import { renderBoardPreviewDataUrl } from '../../boards/lib/boardPreviewSerializer'
import { setCanvasBoardPreviewSnapshot } from '../../boards/lib/canvasBoardState'
import { syncCanvasBoardToServer } from '../../boards/lib/boardSync'
import { getCanvasBoardById } from '../../boards/lib/canvasBoards'
import { loadBoardSnapshotForFlow, resolveBoardIdFromQueryValue } from '../services/flowBoardSnapshotLoader'
import { saveBoardSnapshotFromFlow } from '../services/flowBoardSnapshotSaver'
import { createFlowWorkspaceUploadLifecycle } from '../services/flowWorkspaceUploadLifecycle'
import {
  canvasBoardSnapshotToFlow,
  createEmptyCanvasBoardSnapshot,
  flowToCanvasBoardSnapshot,
  type FlowAdapterEdge,
  type FlowAdapterNode,
} from '../utils/flowSnapshotAdapter'
import type { CanvasBoardSnapshot } from '../types/canvasNodes'

type SaveStatus = 'idle' | 'saved' | 'failed'
type ValueRef<T> = { value: T }
type ArrayRef = { value: unknown[] }

export type UseFlowBoardPersistenceOptions = {
  nodes: ArrayRef
  edges: ArrayRef
  boardBaseSnapshot: ValueRef<CanvasBoardSnapshot>
  isDirty: ValueRef<boolean>
  saveStatus: ValueRef<SaveStatus>
  isLoadingBoardSnapshot: ValueRef<boolean>
  isSavingBoard: ValueRef<boolean>
  getBoardIdQuery: () => unknown
  onWarning: (text: string) => void
  onSuccess: (text: string) => void
  onError: (text: string) => void
  refreshSnapshot: () => void
  clearFlowHistoryBaseline: () => void
  getUserIdentity: () => { userId: number; userUid: string }
}

export function useFlowBoardPersistence(options: UseFlowBoardPersistenceOptions) {
  function readBoardIdFromQuery() {
    return resolveBoardIdFromQueryValue(options.getBoardIdQuery() as string | string[] | null | undefined)
  }

  const currentBoardId = computed(() =>
    readBoardIdFromQuery(),
  )
  const currentBoardExists = computed(() => {
    const boardId = currentBoardId.value
    if (!boardId) return false
    return !!getCanvasBoardById(boardId)
  })
  const canSaveBoard = computed(() => !!currentBoardId.value && currentBoardExists.value && !options.isLoadingBoardSnapshot.value)

  function clearDirtyAfterImport() {
    options.isDirty.value = false
    options.saveStatus.value = canSaveBoard.value ? 'saved' : 'idle'
  }

  function applyBoardSnapshot(snapshot: CanvasBoardSnapshot) {
    const flow = canvasBoardSnapshotToFlow(snapshot)
    options.nodes.value = flow.nodes as unknown[]
    options.edges.value = flow.edges as unknown[]
    options.boardBaseSnapshot.value = snapshot
    clearDirtyAfterImport()
    options.clearFlowHistoryBaseline()
    options.refreshSnapshot()
  }

  function importEmptyBoardSnapshot() {
    applyBoardSnapshot(createEmptyCanvasBoardSnapshot())
  }

  async function importBoardSnapshot() {
    const boardId = readBoardIdFromQuery()
    if (!boardId) {
      importEmptyBoardSnapshot()
      return
    }
    options.isLoadingBoardSnapshot.value = true
    try {
      const snapshot = await loadBoardSnapshotForFlow(boardId)
      if (!snapshot) {
        importEmptyBoardSnapshot()
        return
      }
      applyBoardSnapshot(snapshot)
    } catch (error) {
      console.warn('[workspace] load board snapshot failed', error)
      options.onError('导入画布快照失败，请稍后重试')
    } finally {
      options.isLoadingBoardSnapshot.value = false
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
    return flowToCanvasBoardSnapshot(
      {
        nodes: options.nodes.value as unknown as FlowAdapterNode[],
        edges: options.edges.value as unknown as FlowAdapterEdge[],
      },
      options.boardBaseSnapshot.value,
    )
  }

  async function refreshBoardPreviewAfterSave(boardId: string, stabilizedSnapshot: CanvasBoardSnapshot) {
    const snapshotDataUrl = await renderBoardPreviewDataUrl(stabilizedSnapshot)
    setCanvasBoardPreviewSnapshot(
      boardId,
      snapshotDataUrl || undefined,
      snapshotDataUrl ? Date.now() : undefined,
    )
    await syncCanvasBoardToServer(boardId)
  }

  async function saveCurrentBoardSnapshot(): Promise<boolean> {
    const boardId = currentBoardId.value
    if (!boardId) {
      options.onWarning('缺少 boardId，无法保存')
      return false
    }
    options.isSavingBoard.value = true
    try {
      const { userId, userUid } = options.getUserIdentity()
      if (!Number.isFinite(userId) || userId <= 0 || !userUid) {
        throw new Error('保存失败：缺少用户信息，请刷新后重试')
      }
      const uploadLifecycle = createFlowWorkspaceUploadLifecycle({
        userId,
        userUid,
        boardId,
        warning: (text) => {
          options.onWarning(text)
        },
      })
      const previousSnapshot = uploadLifecycle.readPreviousSnapshot()
      const snapshot = buildCurrentBoardSnapshot()
      const stabilizedSnapshot = await uploadLifecycle.stabilizeSnapshotForSave(snapshot)
      await saveBoardSnapshotFromFlow(boardId, stabilizedSnapshot)
      options.boardBaseSnapshot.value = stabilizedSnapshot
      const nextFlow = canvasBoardSnapshotToFlow(stabilizedSnapshot)
      options.nodes.value = nextFlow.nodes as unknown[]
      options.edges.value = nextFlow.edges as unknown[]
      options.clearFlowHistoryBaseline()
      options.isDirty.value = false
      options.saveStatus.value = 'saved'
      options.onSuccess('已保存')
      void refreshBoardPreviewAfterSave(boardId, stabilizedSnapshot).catch((error) => {
        console.warn('[workspace] refresh board preview after save failed', boardId, error)
      })
      await uploadLifecycle.recycleRemovedUploadsAfterSave(previousSnapshot, stabilizedSnapshot)
      options.refreshSnapshot()
      return true
    } catch (error) {
      options.saveStatus.value = 'failed'
      console.warn('[workspace] save board snapshot failed', error)
      options.onError(error instanceof Error ? error.message : '保存失败，请稍后重试')
      return false
    } finally {
      options.isSavingBoard.value = false
    }
  }

  return {
    currentBoardId,
    currentBoardExists,
    canSaveBoard,
    importEmptyBoardSnapshot,
    importBoardSnapshot,
    loadInitialOrRouteBoard,
    saveCurrentBoardSnapshot,
  }
}
