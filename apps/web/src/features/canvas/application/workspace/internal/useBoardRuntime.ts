import { computed, ref, shallowRef, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import { createEmptyBoardSnapshot, type BoardSnapshot } from '../../../../board'
import { HOME_PATH, USER_BOARDS_PATH } from '../../../../../shared/constants/routePaths'
import { useBoardSession, type SaveStatus } from '../../board-session'
import { useBoardHeaderActions } from '../../../presentation/chrome/useBoardHeaderActions.ts'
import { useFlowCanvasRuntime } from './useFlowCanvasRuntime'
import type { CanvasVueFlowGraphActions } from '../../../infrastructure/vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import type { CanvasFeedback } from '../useCanvasFeedback.ts'
import type { PendingImageExecutionCore } from '../../execution/pending-execution'

type BoardRuntimeFeedback = CanvasFeedback['boardSession']

type UseBoardRuntimeOptions = {
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
  feedback: BoardRuntimeFeedback
  executionCore: PendingImageExecutionCore
  hydratePendingImageLeases?: (snapshot: BoardSnapshot) => void
  cancelTasks: (taskIds: readonly string[]) => void
  vueFlowGraphActions?: CanvasVueFlowGraphActions
}

function boardSessionIdentityKey(identity: { userId: number; userUid: string }): string {
  const userUid = typeof identity.userUid === 'string' ? identity.userUid.trim() : ''
  if (!Number.isFinite(identity.userId) || identity.userId <= 0 || !userUid) return ''
  return `${identity.userId}:${userUid}`
}

export function useBoardRuntime(options: UseBoardRuntimeOptions) {
  const message = {
    warning: options.feedback.warning,
    success: options.feedback.saveSucceeded,
    error: options.feedback.saveFailed,
  }
  const nodes: Ref<Node[]> = shallowRef([])
  const edges: Ref<Edge[]> = shallowRef([])
  const boardBaseSnapshot: Ref<BoardSnapshot> = ref(createEmptyBoardSnapshot())
  const isLoadingBoardSnapshot: Ref<boolean> = ref(false)
  const isSavingBoard: Ref<boolean> = ref(false)
  const isDirty: Ref<boolean> = ref(false)
  const saveStatus: Ref<SaveStatus> = ref('idle')
  const autosaveResumeSignal = computed(() => boardSessionIdentityKey(options.getUserIdentity()))

  const boardSession = useBoardSession({
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

  const boardHeader = useBoardHeaderActions({
    currentBoardId: flowRuntime.persistence.currentBoardId,
    message,
    router: options.router,
    route: options.route,
    flushAutosave: () => flowRuntime.persistence.flushAutosave(),
  })

  async function loadInitialOrRouteBoard() {
    const result = await flowRuntime.persistence.loadInitialOrRouteBoard()
    if (result.status === 'not-found') {
      // Never leave an unknown route ID attached to an editable canvas.
      // Return to the board catalog so the user can choose a valid board.
      await options.router.replace(USER_BOARDS_PATH)
    }
    return result
  }

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
      loadInitialOrRouteBoard,
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
