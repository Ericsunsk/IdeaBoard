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
import { taskApi } from '../../../../shared/api/task'
import type {
  CanvasWorkspaceNodeHandlersOptions,
  CanvasWorkspaceNodeMediaInputsOptions,
  CanvasWorkspaceRuntime,
} from '../types'

const STOP_DELETED_TASK_RETRY_DELAYS_MS = [0, 1000, 3000] as const

type CanvasGraphUploadApi = {
  upload: (
    file: File,
    boardId: string,
  ) => Promise<{
    url: string
    width?: number
    height?: number
    mime?: string
    ext?: string
  }>
}

export type UseCanvasWorkspaceRuntimeOptions = {
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  createNodeKind: Ref<CanvasCreatableNodeKind>
  flowPaneAdapter: CanvasFlowPaneAdapter
  uploadApi: CanvasGraphUploadApi
  openNodeImagePreview: (snapshot: ImageNodeItem, mode: 'latest-output') => Promise<void>
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
  startNodeDragInteraction: () => void
  stopNodeDragInteraction: () => void
  refreshFlowHandleVisibility: () => void
  onNativeRemove?: () => void
  openRecharge: () => void
}

export function createCanvasWorkspaceRuntimeParts(options: UseCanvasWorkspaceRuntimeOptions) {
  const feedback = useCanvasFeedback(options.openRecharge)

  function stopDeletedRunningTask(taskId: string, attempt = 0) {
    void taskApi.cancel(taskId).catch((error: unknown) => {
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
    deleteNode: board.graphRuntime.deleteNode,
    updateNodeInternals: options.flowPaneAdapter.updateNodeInternals,
    screenToFlowCoordinate: options.flowPaneAdapter.screenToFlowCoordinate,
    getSelectedElements: options.flowPaneAdapter.getSelectedElements,
    removeSelectedElements: options.flowPaneAdapter.removeSelectedElements,
    uploadApi: options.uploadApi,
    openNodeImagePreview: options.openNodeImagePreview,
    openImageUrlPreview: options.openImageUrlPreview,
    feedback: feedback.graph,
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
      connectionAdapter: graph.connectionAdapter,
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
  }: CanvasWorkspaceNodeMediaInputsOptions) {
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
      _onFlowDeleteNode: graph.onFlowDeleteNode,
      _onFlowPromptTextUpdate: graph._onFlowPromptTextUpdate,
      _onFlowNodeParamUpdate: graph._onFlowNodeParamUpdate,
      markFlowExecutionNodeDirty: graph.markFlowExecutionNodeDirty,
    })
  }

  const parts = {
    boardChrome: {
      currentBoardId: board.currentBoardId,
      isBoardMenuOpen: board.isBoardMenuOpen,
      isRenamingBoard: board.isRenamingBoard,
      boardNameDraft: board.boardNameDraft,
      isRenamePending: board.isRenamePending,
      catalogRecovery: board.catalogRecovery,
      currentBoardName: board.currentBoardName,
      boardOptions: board.boardOptions,
      setBoardNameInputRef: board.setBoardNameInputRef,
      toggleBoardMenu: board.toggleBoardMenu,
      startBoardRename: board.startBoardRename,
      cancelBoardRename: board.cancelBoardRename,
      closeBoardHeaderEditing: board.closeBoardHeaderEditing,
      onConfirmBoardRename: board.onConfirmBoardRename,
      reloadCatalog: board.reloadCatalog,
      onSelectBoard: board.onSelectBoard,
      onNavigateRequest: board.onNavigateRequest,
      onLogoutRequest: board.onLogoutRequest,
    },
    flowPane: {
      nodes: board.nodes,
      edges: board.edges,
      onNodesChange: board.onNodesChange,
      onEdgesChange: board.onEdgesChange,
      connectionAdapter: graph.connectionAdapter,
      nodeDragTransactions: {
        begin: board.graphRuntime.nodeDrag.begin,
        commit: board.graphRuntime.nodeDrag.commit,
      },
    },
    persistence: {
      flushAutosave: flushAutosaveWithViewportPersistence,
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

  return parts
}

export function createCanvasWorkspaceRuntime(
  options: UseCanvasWorkspaceRuntimeOptions,
): CanvasWorkspaceRuntime {
  return createCanvasWorkspaceRuntimeFacade(createCanvasWorkspaceRuntimeParts(options))
}

function createCanvasWorkspaceRuntimeFacade(
  parts: ReturnType<typeof createCanvasWorkspaceRuntimeParts>,
): CanvasWorkspaceRuntime {
  const grouping = parts.graphEditing.grouping

  return {
    viewModel: {
      boardChrome: {
        currentBoardId: parts.boardChrome.currentBoardId,
        isBoardMenuOpen: parts.boardChrome.isBoardMenuOpen,
        isRenamingBoard: parts.boardChrome.isRenamingBoard,
        boardNameDraft: parts.boardChrome.boardNameDraft,
        isRenamePending: parts.boardChrome.isRenamePending,
        catalogRecovery: parts.boardChrome.catalogRecovery,
        currentBoardName: parts.boardChrome.currentBoardName,
        boardOptions: parts.boardChrome.boardOptions,
      },
      flowPane: {
        nodes: parts.flowPane.nodes,
        edges: parts.flowPane.edges,
      },
      persistence: {
        history: parts.persistence.history,
      },
      viewportControls: {
        zoomLevel: parts.viewportControls.zoomLevel,
      },
      graphEditing: {
        grouping: grouping.view,
        hasSelectedFlowNodes: parts.graphEditing.hasSelectedFlowNodes,
        isFlowInteractionActive: parts.graphEditing.isFlowInteractionActive,
        isFlowViewportPanInteractionActive: parts.graphEditing.isFlowViewportPanInteractionActive,
        isFlowInteractionLocked: parts.graphEditing.isFlowInteractionLocked,
      },
    },
    commands: {
      boardSession: {
        setBoardNameInputRef: parts.boardChrome.setBoardNameInputRef,
        toggleBoardMenu: parts.boardChrome.toggleBoardMenu,
        startBoardRename: parts.boardChrome.startBoardRename,
        cancelBoardRename: parts.boardChrome.cancelBoardRename,
        closeBoardHeaderEditing: parts.boardChrome.closeBoardHeaderEditing,
        onConfirmBoardRename: parts.boardChrome.onConfirmBoardRename,
        reloadCatalog: parts.boardChrome.reloadCatalog,
        onSelectBoard: parts.boardChrome.onSelectBoard,
        onNavigateRequest: parts.boardChrome.onNavigateRequest,
        onLogoutRequest: parts.boardChrome.onLogoutRequest,
      },
      flowPane: {
        onNodesChange: parts.flowPane.onNodesChange,
        onEdgesChange: parts.flowPane.onEdgesChange,
        connectionAdapter: parts.flowPane.connectionAdapter,
        nodeDragTransactions: parts.flowPane.nodeDragTransactions,
      },
      persistence: {
        flushAutosave: parts.persistence.flushAutosave,
        loadCanvasBoardAndProbe: parts.persistence.loadCanvasBoardAndProbe,
        reattachPendingImageResults: parts.persistence.reattachPendingImageResults,
      },
      viewportControls: {
        onZoomIn: parts.viewportControls.onZoomIn,
        onZoomOut: parts.viewportControls.onZoomOut,
        onPanViewport: parts.viewportControls.onPanViewport,
        onMiniMapCenterRequested: parts.viewportControls.onMiniMapCenterRequested,
        onMiniMapDragStart: parts.viewportControls.onMiniMapDragStart,
        onMiniMapDragMove: parts.viewportControls.onMiniMapDragMove,
        onMiniMapDragEnd: parts.viewportControls.onMiniMapDragEnd,
        onViewportChangeEnd: parts.viewportControls.onViewportChangeEnd,
        onFlowMoveStart: parts.viewportControls.onFlowMoveStart,
        onFlowMove: parts.viewportControls.onFlowMove,
        onFlowMoveEnd: parts.viewportControls.onFlowMoveEnd,
        onNodeDragStart: parts.viewportControls.onNodeDragStart,
        onNodeDragStop: parts.viewportControls.onNodeDragStop,
        flushViewportPersistence: parts.viewportControls.flushViewportPersistence,
        loadCanvasBoardAndProbe: parts.viewportControls.loadCanvasBoardAndProbe,
      },
      graphEditing: {
        grouping: grouping.commands,
        scheduleFlowNodeInternalsRefresh: parts.graphEditing.scheduleFlowNodeInternalsRefresh,
        createNodeAtPosition: parts.graphEditing.createNodeAtPosition,
        onExportSelectedImages: parts.graphEditing.onExportSelectedImages,
        onFlowMediaInputImageDrop: parts.graphEditing.onFlowMediaInputImageDrop,
        onFlowMediaInputImageOpen: parts.graphEditing.onFlowMediaInputImageOpen,
        onFlowMediaInputImagePreview: parts.graphEditing.onFlowMediaInputImagePreview,
        onFlowMediaInputImageRemove: parts.graphEditing.onFlowMediaInputImageRemove,
        onCanvasDrop: parts.graphEditing.onCanvasDrop,
        onCanvasDragOver: parts.graphEditing.onCanvasDragOver,
        onFlowImageOpenPreview: parts.graphEditing.onFlowImageOpenPreview,
        onFlowImageAppendToCanvas: parts.graphEditing.onFlowImageAppendToCanvas,
        beginFlowNodeDragDuplicateTracking: parts.graphEditing.beginFlowNodeDragDuplicateTracking,
        finishFlowNodeDragDuplicateTracking: parts.graphEditing.finishFlowNodeDragDuplicateTracking,
        getFlowNodeUiStateCached: parts.graphEditing.getFlowNodeUiStateCached,
      },
      imageWorkflow: {
        isGenerationAvailable: parts.imageWorkflow.isGenerationAvailable,
        runImageDirectAction: parts.imageWorkflow.runImageDirectAction,
        runCanvasExtendAction: parts.imageWorkflow.runCanvasExtendAction,
        createImageTools: parts.imageWorkflow.createImageTools,
        createImageActionHandlers: parts.imageWorkflow.createImageActionHandlers,
        createPromptGenerationHandlers: parts.imageWorkflow.createPromptGenerationHandlers,
        createCanvasExtendOverlay: parts.imageWorkflow.createCanvasExtendOverlay,
      },
      nodeInteraction: {
        createNodeMediaInputs: parts.createNodeMediaInputs,
        createNodeHandlers: parts.createNodeHandlers,
      },
    },
  } satisfies CanvasWorkspaceRuntime
}
