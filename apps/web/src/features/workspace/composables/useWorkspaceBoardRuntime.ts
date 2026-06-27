import { ref, type ComputedRef, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import {
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
} from '../../canvas-board/domain'
import { useFlowBoardHeaderActions } from './useFlowBoardHeaderActions'
import type { FlowHistory } from './useFlowHistory'
import type { FlowTransactions } from './useFlowTransactions'
import { useFlowWorkspaceRuntime } from './useFlowWorkspaceRuntime'

type SaveStatus = 'idle' | 'saved' | 'failed'

type FlowViewport = {
  x: number
  y: number
  zoom: number
}

type WorkspaceBoardRuntimeMessage = {
  warning: (message: string) => void
  success: (message: string) => void
  error: (message: string) => void
}

type UseWorkspaceBoardRuntimeOptions = {
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  getViewport: () => FlowViewport
  setViewport: (viewport: FlowViewport) => Promise<unknown> | unknown
  setNodes: (nodes: Node[]) => void
  setEdges: (edges: Edge[]) => void
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
  message: WorkspaceBoardRuntimeMessage
}

type BoardOption = { id: string; name: string }

type WorkspaceBoardRuntime = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  currentBoardId: ComputedRef<string | null>
  loadInitialOrRouteBoard: () => Promise<void>
  flushAutosave: (options?: { silent?: boolean }) => Promise<void>
  flowHistory: FlowHistory
  flowTransactions: FlowTransactions
  onNavigateRequest: (path: string) => Promise<void>
  onLogoutRequest: () => Promise<void>
  markBoardViewportChanged: () => void
  commitPendingBoardViewportChange: () => void
  isBoardMenuOpen: Ref<boolean>
  isRenamingBoard: Ref<boolean>
  boardNameDraft: Ref<string>
  currentBoardName: ComputedRef<string>
  boardOptions: ComputedRef<BoardOption[]>
  setBoardNameInputRef: (el: HTMLElement | null) => void
  refreshBoardMetas: () => void
  toggleBoardMenu: () => void
  startBoardRename: () => void
  cancelBoardRename: () => void
  closeBoardHeaderEditing: () => void
  onConfirmBoardRename: () => Promise<void>
  onSelectBoard: (boardId: string) => void
}

export function useWorkspaceBoardRuntime(
  options: UseWorkspaceBoardRuntimeOptions,
): WorkspaceBoardRuntime {
  const nodes: Ref<Node[]> = ref([])
  const edges: Ref<Edge[]> = ref([])
  const boardBaseSnapshot: Ref<CanvasBoardSnapshot> = ref(createEmptyCanvasBoardSnapshot())
  const isLoadingBoardSnapshot: Ref<boolean> = ref(false)
  const isSavingBoard: Ref<boolean> = ref(false)
  const isDirty: Ref<boolean> = ref(false)
  const saveStatus: Ref<SaveStatus> = ref('idle')
  const hasPendingViewportChange = ref(false)
  const isApplyingBoardViewport = ref(false)

  function setBoardViewport(viewport: FlowViewport) {
    isApplyingBoardViewport.value = true
    const result = options.setViewport(viewport)
    const clearApplyingFlag = () => {
      window.requestAnimationFrame(() => {
        isApplyingBoardViewport.value = false
      })
    }
    void Promise.resolve(result).then(clearApplyingFlag, clearApplyingFlag)
  }

  const {
    currentBoardId,
    loadInitialOrRouteBoard,
    flushAutosave,
    flowHistory,
    flowTransactions,
  } = useFlowWorkspaceRuntime({
    nodes,
    edges,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot,
    isSavingBoard,
    getBoardIdQuery: () => options.route.query.boardId,
    getUserIdentity: options.getUserIdentity,
    getViewport: options.getViewport,
    setViewport: setBoardViewport,
    setNodes: options.setNodes,
    setEdges: options.setEdges,
    onWarning: options.message.warning,
    onSuccess: options.message.success,
    onError: options.message.error,
  })

  const boardHeader = useFlowBoardHeaderActions({
    currentBoardId,
    message: options.message,
    router: options.router,
    route: options.route,
    flushAutosave: () => flushAutosave(),
  })

  async function onNavigateRequest(path: string) {
    await flushAutosave()
    await options.router.push(path)
  }

  async function onLogoutRequest() {
    await flushAutosave()
    await options.logoutAndRedirect('/user/home')
  }

  function markBoardViewportChanged() {
    hasPendingViewportChange.value = true
  }

  function commitPendingBoardViewportChange() {
    const shouldPersistViewport = hasPendingViewportChange.value
    hasPendingViewportChange.value = false
    if (
      !shouldPersistViewport ||
      isApplyingBoardViewport.value ||
      isLoadingBoardSnapshot.value
    ) {
      return
    }
    flowTransactions.commitDirty()
  }

  return {
    nodes,
    edges,
    currentBoardId,
    loadInitialOrRouteBoard,
    flushAutosave,
    flowHistory,
    flowTransactions,
    onNavigateRequest,
    onLogoutRequest,
    markBoardViewportChanged,
    commitPendingBoardViewportChange,
    isBoardMenuOpen: boardHeader.isBoardMenuOpen,
    isRenamingBoard: boardHeader.isRenamingBoard,
    boardNameDraft: boardHeader.boardNameDraft,
    currentBoardName: boardHeader.currentBoardName,
    boardOptions: boardHeader.boardOptions,
    setBoardNameInputRef: boardHeader.setBoardNameInputRef,
    refreshBoardMetas: boardHeader.refreshBoardMetas,
    toggleBoardMenu: boardHeader.toggleBoardMenu,
    startBoardRename: boardHeader.startBoardRename,
    cancelBoardRename: boardHeader.cancelBoardRename,
    closeBoardHeaderEditing: boardHeader.closeBoardHeaderEditing,
    onConfirmBoardRename: boardHeader.onConfirmBoardRename,
    onSelectBoard: boardHeader.onSelectBoard,
  }
}
