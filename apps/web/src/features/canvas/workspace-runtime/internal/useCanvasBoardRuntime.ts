import { computed, ref, shallowRef, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { createEmptyCanvasBoardSnapshot, type CanvasBoardSnapshot } from '../../../canvas-board/domain'
import { HOME_PATH } from '../../../../shared/constants/routePaths'
import { useCanvasBoardSession, type SaveStatus } from '../../board-session'
import { useFlowBoardHeaderActions } from '../../canvas-chrome/useFlowBoardHeaderActions'
import { useFlowCanvasRuntime } from './useFlowCanvasRuntime'
import type { CanvasVueFlowGraphActions } from '../../renderer/vue-flow/useCanvasVueFlowGraphAdapter'
import type { CanvasFeedback } from '../../feedback/useCanvasFeedback'
import type { PendingImageExecutionCore } from '../../execution/pending-execution'

type CanvasBoardRuntimeFeedback = CanvasFeedback['boardSession']

type UseCanvasBoardRuntimeOptions = {
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
  feedback: CanvasBoardRuntimeFeedback
  executionCore: PendingImageExecutionCore
  hydratePendingImageLeases?: (snapshot: CanvasBoardSnapshot) => void
  cancelTasks: (taskIds: readonly string[]) => void
  vueFlowGraphActions?: CanvasVueFlowGraphActions
}

function boardSessionIdentityKey(identity: { userId: number; userUid: string }): string {
  const userUid = typeof identity.userUid === 'string' ? identity.userUid.trim() : ''
  if (!Number.isFinite(identity.userId) || identity.userId <= 0 || !userUid) return ''
  return `${identity.userId}:${userUid}`
}

export function useCanvasBoardRuntime(options: UseCanvasBoardRuntimeOptions) {
  const message = {
    warning: options.feedback.warning,
    success: options.feedback.saveSucceeded,
    error: options.feedback.saveFailed,
  }
  const nodes: Ref<Node[]> = shallowRef([])
  const edges: Ref<Edge[]> = shallowRef([])
  const boardBaseSnapshot: Ref<CanvasBoardSnapshot> = ref(createEmptyCanvasBoardSnapshot())
  const isLoadingBoardSnapshot: Ref<boolean> = ref(false)
  const isSavingBoard: Ref<boolean> = ref(false)
  const isDirty: Ref<boolean> = ref(false)
  const saveStatus: Ref<SaveStatus> = ref('idle')
  const autosaveResumeSignal = computed(() => boardSessionIdentityKey(options.getUserIdentity()))

  const boardSession = useCanvasBoardSession({
    getBoardIdQuery: () => options.route.query.boardId,
    getUserIdentity: options.getUserIdentity,
    message: {
      warning: options.feedback.warning,
    },
  })

  const flowRuntime = useFlowCanvasRuntime({
    nodes,
    edges,
    boardBaseSnapshot,
    isDirty,
    saveStatus,
    isLoadingBoardSnapshot,
    isSavingBoard,
    autosaveResumeSignal,
    boardSession,
    executionCore: options.executionCore,
    hydratePendingImageLeases: options.hydratePendingImageLeases,
    cancelTasks: options.cancelTasks,
    onWarning: options.feedback.warning,
    onSuccess: options.feedback.saveSucceeded,
    onError: options.feedback.saveFailed,
    vueFlowGraphActions: options.vueFlowGraphActions,
  })

  const boardHeader = useFlowBoardHeaderActions({
    currentBoardId: flowRuntime.persistence.currentBoardId,
    message,
    router: options.router,
    route: options.route,
    flushAutosave: () => flowRuntime.persistence.flushAutosave(),
  })

  async function onNavigateRequest(path: string) {
    await flowRuntime.persistence.flushAutosave()
    await options.router.push(path)
  }

  async function onLogoutRequest() {
    await flowRuntime.persistence.flushAutosave()
    await options.logoutAndRedirect(HOME_PATH)
  }

  const runtime = {
    boardSession: {
      boardBaseSnapshot,
      currentBoardId: flowRuntime.persistence.currentBoardId,
      isDirty,
      isSavingBoard,
      saveStatus,
      loadInitialOrRouteBoard: flowRuntime.persistence.loadInitialOrRouteBoard,
      flushAutosave: flowRuntime.persistence.flushAutosave,
      history: flowRuntime.persistence.history,
    },
    flowPane: {
      ...flowRuntime.flowPane,
    },
    graphEditing: flowRuntime.graphEditing,
    boardChrome: {
      isBoardMenuOpen: boardHeader.isBoardMenuOpen,
      isRenamingBoard: boardHeader.isRenamingBoard,
      boardNameDraft: boardHeader.boardNameDraft,
      isRenamePending: boardHeader.isRenamePending,
      catalogRecovery: boardHeader.catalogRecovery,
      currentBoardName: boardHeader.currentBoardName,
      boardOptions: boardHeader.boardOptions,
      setBoardNameInputRef: boardHeader.setBoardNameInputRef,
      toggleBoardMenu: boardHeader.toggleBoardMenu,
      startBoardRename: boardHeader.startBoardRename,
      cancelBoardRename: boardHeader.cancelBoardRename,
      closeBoardHeaderEditing: boardHeader.closeBoardHeaderEditing,
      onConfirmBoardRename: boardHeader.onConfirmBoardRename,
      reloadCatalog: boardHeader.reloadCatalog,
      onSelectBoard: boardHeader.onSelectBoard,
      onNavigateRequest,
      onLogoutRequest,
    },
  }
  return runtime
}
