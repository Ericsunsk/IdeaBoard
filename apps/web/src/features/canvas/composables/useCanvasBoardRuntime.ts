import { ref, type ComputedRef, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import {
  createEmptyCanvasBoardSnapshot,
  type CanvasBoardSnapshot,
} from '../../canvas-board/domain'
import { USER_HOME_PATH } from '../../../shared/constants/routePaths'
import { useCanvasBoardSession } from '../board-session'
import { useFlowBoardHeaderActions } from './useFlowBoardHeaderActions'
import type { FlowHistory } from './useFlowHistory'
import type { FlowTransactions } from './useFlowTransactions'
import { useFlowCanvasRuntime } from './useFlowCanvasRuntime'

type SaveStatus = 'idle' | 'saved' | 'failed'

type FlowViewport = {
  x: number
  y: number
  zoom: number
}

type CanvasBoardRuntimeMessage = {
  warning: (message: string) => void
  success: (message: string) => void
  error: (message: string) => void
}

type UseCanvasBoardRuntimeOptions = {
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  getViewport: () => FlowViewport
  setViewport: (viewport: FlowViewport) => Promise<unknown> | unknown
  setNodes: (nodes: Node[]) => void
  setEdges: (edges: Edge[]) => void
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
  message: CanvasBoardRuntimeMessage
}

type BoardOption = { id: string; name: string }

type CanvasBoardRuntime = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  fullNodes: Ref<Node[]>
  fullEdges: Ref<Edge[]>
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

export function useCanvasBoardRuntime(
  options: UseCanvasBoardRuntimeOptions,
): CanvasBoardRuntime {
  const nodes: Ref<Node[]> = ref([])
  const edges: Ref<Edge[]> = ref([])
  const fullNodes: Ref<Node[]> = ref([])
  const fullEdges: Ref<Edge[]> = ref([])
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

  const boardSession = useCanvasBoardSession({
    getBoardIdQuery: () => options.route.query.boardId,
    getUserIdentity: options.getUserIdentity,
    message: {
      warning: options.message.warning,
    },
  })

  const {
    currentBoardId,
    loadInitialOrRouteBoard,
    flushAutosave,
    flowHistory,
    flowTransactions,
  } = useFlowCanvasRuntime({
    nodes,
    edges,
    fullNodes,
    fullEdges,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot,
    isSavingBoard,
    boardSession,
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
    await options.logoutAndRedirect(USER_HOME_PATH)
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
    fullNodes,
    fullEdges,
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
