import type { Ref } from 'vue'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import type { CanvasCreatableNodeKind, ImageNodeItem } from '../../../canvas-board/domain'
import { useCanvasViewportRuntime } from '../../composables/useCanvasViewportRuntime'
import { useCanvasBoardRuntime } from '../../composables/useCanvasBoardRuntime'
import { useCanvasGraphRuntime } from '../../composables/useCanvasGraphRuntime'
import { useCanvasTaskRuntime } from '../../composables/useCanvasTaskRuntime'
import { useFlowCanvasNodeHandlers } from '../../composables/useFlowCanvasNodeHandlers'
import { useFlowNodeMediaInputs } from '../../composables/useFlowNodeMediaInputs'
import { useCanvasFeedback } from '../../feedback/useCanvasFeedback'
import type { CanvasFlowPaneAdapter } from '../../flow-pane'
import { useCanvasImageWorkflow } from '../../image-workflow'
import { cancelTask } from '../../../../shared/api/task'

const STOP_DELETED_TASK_RETRY_DELAYS_MS = [0, 1000, 3000] as const

type CanvasGraphUploadApi = {
  upload: (
    file: File,
    boardId: string,
  ) => Promise<{
    code: number
    msg?: string
    data?: {
      url?: string
      width?: number
      height?: number
      mime?: string
      ext?: string
    }
  }>
}

type CanvasWorkspaceNodeMediaOptions = Omit<
  Parameters<typeof useFlowNodeMediaInputs>[0],
  'applyImageFileToImageNode' | 'updateSnapshotNode' | 'markFlowExecutionNodeDirty'
> & {
  discardNodeEdits: (nodeId: string) => void
}

type CanvasWorkspaceNodeHandlersOptions = Omit<
  Parameters<typeof useFlowCanvasNodeHandlers>[0],
  | '_onFlowDeleteNode'
  | '_onFlowPromptTextUpdate'
  | '_onFlowNodeParamUpdate'
  | 'markFlowExecutionNodeDirty'
>

export type UseCanvasWorkspaceRuntimeOptions = {
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  createNodeKind: Ref<CanvasCreatableNodeKind>
  flowPaneAdapter: CanvasFlowPaneAdapter
  uploadApi: CanvasGraphUploadApi
  sanitizeApiErrorMessage: (message: string | null | undefined, fallback: string) => string
  openNodeImagePreview: (snapshot: ImageNodeItem, mode: 'latest-output') => Promise<void>
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
  startNodeDragInteraction: () => void
  stopNodeDragInteraction: () => void
  refreshFlowHandleVisibility: () => void
  onNativeRemove?: () => void
}

export function createCanvasWorkspaceRuntimeParts(options: UseCanvasWorkspaceRuntimeOptions) {
  const feedback = useCanvasFeedback()

  function stopDeletedRunningTask(taskId: string, attempt = 0) {
    void cancelTask(taskId).catch((error: unknown) => {
      const nextAttempt = attempt + 1
      const retryDelay = STOP_DELETED_TASK_RETRY_DELAYS_MS[nextAttempt]
      if (retryDelay !== undefined) {
        window.setTimeout(() => {
          stopDeletedRunningTask(taskId, nextAttempt)
        }, retryDelay)
        return
      }
      console.warn('[canvas] failed to stop deleted task', { taskId, error })
    })
  }

  function stopDeletedRunningTasks(taskIds: string[]) {
    const uniqueTaskIds = Array.from(
      new Set(taskIds.map((taskId) => taskId.trim()).filter(Boolean)),
    )
    if (uniqueTaskIds.length === 0) return

    uniqueTaskIds.forEach((taskId) => stopDeletedRunningTask(taskId))
  }

  const board = useCanvasBoardRuntime({
    route: options.route,
    router: options.router,
    getUserIdentity: options.getUserIdentity,
    logoutAndRedirect: options.logoutAndRedirect,
    feedback: feedback.boardSession,
    onNativeRemove: options.onNativeRemove,
    stopDeletedRunningTasks,
    vueFlowGraphActions: options.flowPaneAdapter.vueFlowGraphActions,
  })

  const graph = useCanvasGraphRuntime({
    nodes: board.fullNodes,
    edges: board.fullEdges,
    boardBaseSnapshot: board.boardBaseSnapshot,
    currentBoardId: board.currentBoardId,
    createNodeKind: options.createNodeKind,
    graphAdapter: board.graphAdapter,
    updateNodeInternals: options.flowPaneAdapter.updateNodeInternals,
    screenToFlowCoordinate: options.flowPaneAdapter.screenToFlowCoordinate,
    getSelectedElements: options.flowPaneAdapter.getSelectedElements,
    removeSelectedElements: options.flowPaneAdapter.removeSelectedElements,
    uploadApi: options.uploadApi,
    sanitizeApiErrorMessage: options.sanitizeApiErrorMessage,
    openNodeImagePreview: options.openNodeImagePreview,
    openImageUrlPreview: options.openImageUrlPreview,
    feedback: feedback.graph,
    stopDeletedRunningTasks,
  })

  const viewport = useCanvasViewportRuntime({
    nodes: board.nodes,
    edges: board.edges,
    fullNodes: board.fullNodes,
    fullEdges: board.fullEdges,
    currentBoardId: board.currentBoardId,
    getUserUid: () => options.getUserIdentity().userUid,
    viewport: options.flowPaneAdapter.viewport,
    getViewport: options.flowPaneAdapter.getViewport,
    setViewport: options.flowPaneAdapter.setViewport,
    setCenter: options.flowPaneAdapter.setCenter,
    zoomIn: options.flowPaneAdapter.zoomIn,
    zoomOut: options.flowPaneAdapter.zoomOut,
    panBy: options.flowPaneAdapter.panBy,
    loadInitialOrRouteBoard: board.loadInitialOrRouteBoard,
    closeBoardHeaderEditing: board.closeBoardHeaderEditing,
    startCanvasInteraction: graph.startFlowInteraction,
    stopCanvasInteraction: graph.stopFlowInteraction,
    startNodeDragInteraction: options.startNodeDragInteraction,
    stopNodeDragInteraction: options.stopNodeDragInteraction,
    refreshFlowHandleVisibility: options.refreshFlowHandleVisibility,
  })

  async function flushAutosaveWithViewportPersistence(
    autosaveOptions?: Parameters<typeof board.flushAutosave>[0],
  ) {
    viewport.flushViewportPersistence()
    await board.flushAutosave(autosaveOptions)
  }

  const task = useCanvasTaskRuntime({
    nodes: board.fullNodes,
    edges: board.fullEdges,
    graphAdapter: board.graphAdapter,
    createCurrentCanvasGraphStore: board.graphRuntime.createCurrentCanvasGraphStore,
    boardBaseSnapshot: board.boardBaseSnapshot,
    flushAutosave: flushAutosaveWithViewportPersistence,
    currentBoardId: board.currentBoardId,
    refreshNodeInternals: graph.scheduleFlowNodeInternalsRefresh,
    feedback: feedback.taskExecution,
  })

  const imageWorkflow = useCanvasImageWorkflow({
    graph: {
      nodes: board.nodes,
      currentBoardId: board.currentBoardId,
      getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
      getImageNodeSnapshot: graph.getImageNodeSnapshot,
      createImageNodeFromFile: graph.createImageNodeFromFile,
      connectImageNodes: graph.onConnect,
      uploadImageToBoard: graph.uploadImageToBoard,
      clearSelectedFlowElements: graph.clearSelectedFlowElementsSync,
      startCanvasInteraction: graph.startFlowInteraction,
      stopCanvasInteraction: graph.stopFlowInteraction,
      getMediaInputImages: graph.getMediaInputImages,
      resolvePromptInputImageNodeIds: graph.resolvePromptInputImageNodeIds,
      updateSnapshotNodeParam: graph._onFlowNodeParamUpdate,
      markNodeExecutionDirty: graph.markFlowExecutionNodeDirty,
    },
    task,
    flowPaneAdapter: options.flowPaneAdapter,
    openImageUrlPreview: options.openImageUrlPreview,
    feedback: feedback.imageTools,
  })

  function createNodeMediaInputs({
    discardNodeEdits,
    ...mediaOptions
  }: CanvasWorkspaceNodeMediaOptions) {
    return useFlowNodeMediaInputs({
      ...mediaOptions,
      applyImageFileToImageNode: async (nodeId, file): Promise<boolean> => {
        const updated = await graph.applyImageFileToImageNode(nodeId, file)
        if (updated) discardNodeEdits(nodeId)
        return updated
      },
      updateSnapshotNode: graph.updateSnapshotNode,
      markFlowExecutionNodeDirty: graph.markFlowExecutionNodeDirty,
    })
  }

  function createNodeHandlers(handlerOptions: CanvasWorkspaceNodeHandlersOptions) {
    return useFlowCanvasNodeHandlers({
      ...handlerOptions,
      _onFlowDeleteNode: graph._onFlowDeleteNode,
      _onFlowPromptTextUpdate: graph._onFlowPromptTextUpdate,
      _onFlowNodeParamUpdate: graph._onFlowNodeParamUpdate,
      markFlowExecutionNodeDirty: graph.markFlowExecutionNodeDirty,
    })
  }

  return {
    boardChrome: {
      currentBoardId: board.currentBoardId,
      isBoardMenuOpen: board.isBoardMenuOpen,
      isRenamingBoard: board.isRenamingBoard,
      boardNameDraft: board.boardNameDraft,
      boardRenameRecovery: board.boardRenameRecovery,
      currentBoardName: board.currentBoardName,
      boardOptions: board.boardOptions,
      setBoardNameInputRef: board.setBoardNameInputRef,
      refreshBoardMetas: board.refreshBoardMetas,
      toggleBoardMenu: board.toggleBoardMenu,
      startBoardRename: board.startBoardRename,
      cancelBoardRename: board.cancelBoardRename,
      closeBoardHeaderEditing: board.closeBoardHeaderEditing,
      onConfirmBoardRename: board.onConfirmBoardRename,
      retryBoardRename: board.retryBoardRename,
      rollbackBoardRename: board.rollbackBoardRename,
      onSelectBoard: board.onSelectBoard,
      onNavigateRequest: board.onNavigateRequest,
      onLogoutRequest: board.onLogoutRequest,
    },
    flowPane: {
      nodes: board.nodes,
      edges: board.edges,
      onNodesChange: board.onNodesChange,
      onEdgesChange: board.onEdgesChange,
      isValidConnection: graph.isValidConnection,
      onConnect: graph.onConnect,
      onEdgeUpdate: graph.onEdgeUpdate,
      nodeDragTransactions: {
        begin: board.graphRuntime.nodeDrag.begin,
        commit: board.graphRuntime.nodeDrag.commit,
      },
    },
    persistence: {
      flushAutosave: flushAutosaveWithViewportPersistence,
      isDirty: board.isDirty,
      isSavingBoard: board.isSavingBoard,
      saveStatus: board.saveStatus,
      history: board.flowHistory,
      loadCanvasBoardAndProbe: viewport.loadCanvasBoardAndProbe,
      reattachPendingImageResults: task.reattachPendingImageResults,
    },
    viewportControls: viewport,
    graphEditing: {
      graphAdapter: board.graphAdapter,
      grouping: board.graphRuntime.grouping,
      scheduleFlowNodeInternalsRefresh: graph.scheduleFlowNodeInternalsRefresh,
      createNodeAtPosition: graph.createNodeAtPosition,
      hasSelectedFlowNodes: graph.hasSelectedFlowNodes,
      onExportSelectedImages: graph.onExportSelectedImages,
      isFlowInteractionActive: graph.isFlowInteractionActive,
      isFlowViewportPanInteractionActive: graph.isFlowViewportPanInteractionActive,
      isFlowInteractionLocked: graph.isFlowInteractionLocked,
      nodeContextMenu: graph.nodeContextMenu,
      canvasNodeMenuItems: graph.canvasNodeMenuItems,
      closeNodeContextMenu: graph.closeNodeContextMenu,
      openNodeContextMenu: graph.openNodeContextMenu,
      onFlowMediaInputImageDrop: graph.onFlowMediaInputImageDrop,
      onFlowMediaInputImageOpen: graph.onFlowMediaInputImageOpen,
      onFlowMediaInputImagePreview: graph.onFlowMediaInputImagePreview,
      onFlowMediaInputImageRemove: graph.onFlowMediaInputImageRemove,
      onCanvasDrop: graph.onCanvasDrop,
      onCanvasDragOver: graph.onCanvasDragOver,
      onFlowImageOpenPreview: graph.onFlowImageOpenPreview,
      onFlowImageAppendToCanvas: graph.onFlowImageAppendToCanvas,
      beginFlowNodeDragDuplicateTracking: graph.beginFlowNodeDragDuplicateTracking,
      finishFlowNodeDragDuplicateTracking: graph.finishFlowNodeDragDuplicateTracking,
      getFlowNodeUiStateCached: graph.getFlowNodeUiStateCached,
    },
    imageWorkflow,
    createNodeMediaInputs,
    createNodeHandlers,
  }
}
