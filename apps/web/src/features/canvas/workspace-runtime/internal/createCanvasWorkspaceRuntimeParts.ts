import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import { useCanvasViewportRuntime } from '../../composables/useCanvasViewportRuntime'
import { useCanvasBoardRuntime } from '../../composables/useCanvasBoardRuntime'
import { useCanvasFeedback } from '../../feedback/useCanvasFeedback'
import type { CanvasFlowPaneAdapter } from '../../flow-pane'
import {
  useCanvasGraphEditing,
  useCanvasGraphInteractions,
  type CanvasGraphEditingBrowserAdapter,
  type CanvasGraphInteractionBrowserAdapter,
  type CanvasGraphEditingUploadAdapter,
} from '../../graph-editing'
import { useCanvasImageWorkflow, type CanvasImageWorkflowUiAdapter } from '../../image-workflow'
import { createCanvasTaskLifecycle } from '../../task-lifecycle'
import { taskApi } from '../../../../shared/api/task'
import type { CanvasWorkspaceRuntime } from '../types'

export type UseCanvasWorkspaceRuntimeOptions = {
  route: RouteLocationNormalizedLoaded
  router: Router
  getUserIdentity: () => { userId: number; userUid: string }
  flowPaneAdapter: CanvasFlowPaneAdapter
  uploadApi: CanvasGraphEditingUploadAdapter
  graphEditingBrowser: CanvasGraphEditingBrowserAdapter
  graphInteractionBrowser: CanvasGraphInteractionBrowserAdapter
  openNodeImagePreview: (snapshot: ImageNodeItem, mode: 'latest-output') => Promise<void>
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  imageWorkflowUi: CanvasImageWorkflowUiAdapter
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
  startNodeDragInteraction: () => void
  stopNodeDragInteraction: () => void
  refreshFlowHandleVisibility: () => void
  onNativeRemove?: () => void
  openRecharge: () => void
}

export function createCanvasWorkspaceRuntimeParts(options: UseCanvasWorkspaceRuntimeOptions) {
  const feedback = useCanvasFeedback(options.openRecharge)
  const taskLifecycle = createCanvasTaskLifecycle({ cancel: taskApi.cancel })

  const board = useCanvasBoardRuntime({
    route: options.route,
    router: options.router,
    getUserIdentity: options.getUserIdentity,
    logoutAndRedirect: options.logoutAndRedirect,
    feedback: feedback.boardSession,
    onNativeRemove: options.onNativeRemove,
    cancelDeletedRunningTasks: taskLifecycle.cancelDeletedRunningTasks,
    vueFlowGraphActions: options.flowPaneAdapter.vueFlowGraphActions,
  })

  const editing = useCanvasGraphEditing({
    nodes: board.flowPane.nodes,
    edges: board.flowPane.edges,
    boardBaseSnapshot: board.boardSession.boardBaseSnapshot,
    currentBoardId: board.boardSession.currentBoardId,
    graphAdapter: board.graphEditing.adapter,
    deleteNode: board.graphEditing.deleteNode,
    updateNodeInternals: options.flowPaneAdapter.updateNodeInternals,
    screenToFlowCoordinate: options.flowPaneAdapter.screenToFlowCoordinate,
    browser: options.graphEditingBrowser,
    upload: options.uploadApi,
    preview: {
      openImageUrl: options.openImageUrlPreview,
    },
    feedback: feedback.graph,
  })

  const graphInteractions = useCanvasGraphInteractions({
    nodes: board.flowPane.nodes,
    edges: board.flowPane.edges,
    graphAdapter: board.graphEditing.adapter,
    createCurrentCanvasGraphStore: board.graphEditing.createCurrentCanvasGraphStore,
    selection: {
      selectedElements: options.flowPaneAdapter.getSelectedElements,
      clear: options.flowPaneAdapter.removeSelectedElements,
    },
    browser: options.graphInteractionBrowser,
    preview: { openNodeImage: options.openNodeImagePreview },
    feedback: feedback.graph,
  })

  const viewport = useCanvasViewportRuntime({
    nodes: board.flowPane.nodes,
    edges: board.flowPane.edges,
    fullNodes: board.flowPane.nodes,
    fullEdges: board.flowPane.edges,
    currentBoardId: board.boardSession.currentBoardId,
    getUserUid: () => options.getUserIdentity().userUid,
    viewport: options.flowPaneAdapter.viewport,
    getViewport: options.flowPaneAdapter.getViewport,
    setViewport: options.flowPaneAdapter.setViewport,
    setCenter: options.flowPaneAdapter.setCenter,
    zoomIn: options.flowPaneAdapter.zoomIn,
    zoomOut: options.flowPaneAdapter.zoomOut,
    panBy: options.flowPaneAdapter.panBy,
    loadInitialOrRouteBoard: board.boardSession.loadInitialOrRouteBoard,
    closeBoardHeaderEditing: board.boardChrome.closeBoardHeaderEditing,
    startCanvasInteraction: graphInteractions.runtimePort.startInteraction,
    stopCanvasInteraction: graphInteractions.runtimePort.stopInteraction,
    startNodeDragInteraction: options.startNodeDragInteraction,
    stopNodeDragInteraction: options.stopNodeDragInteraction,
    refreshFlowHandleVisibility: options.refreshFlowHandleVisibility,
  })

  async function flushAutosaveWithViewportPersistence(
    autosaveOptions?: Parameters<typeof board.boardSession.flushAutosave>[0],
  ) {
    viewport.flushViewportPersistence()
    await board.boardSession.flushAutosave(autosaveOptions)
  }

  const imageWorkflow = useCanvasImageWorkflow({
    graph: {
      nodes: board.flowPane.nodes,
      currentBoardId: board.boardSession.currentBoardId,
      getImageNodeDisplayPayload: editing.workflowPort.getImageNodeDisplayPayload,
      getImageNodeSnapshot: editing.workflowPort.getImageNodeSnapshot,
      createImageNodeFromFile: editing.workflowPort.createImageNodeFromFile,
      connectionAdapter: editing.flowPane.connectionAdapter,
      uploadImageToBoard: editing.workflowPort.uploadImageToBoard,
      clearSelectedFlowElements: graphInteractions.commands.clearSelection,
      startCanvasInteraction: graphInteractions.runtimePort.startInteraction,
      stopCanvasInteraction: graphInteractions.runtimePort.stopInteraction,
      getMediaInputImages: editing.workflowPort.getMediaInputImages,
      resolvePromptInputImageNodeIds: editing.workflowPort.resolvePromptInputImageNodeIds,
      updateSnapshotNodeParam: editing.workflowPort.updateNodeParams,
      markNodeExecutionDirty: editing.workflowPort.markNodeExecutionDirty,
    },
    execution: {
      edges: board.flowPane.edges,
      graphAdapter: board.graphEditing.adapter,
      createCurrentCanvasGraphStore: board.graphEditing.createCurrentCanvasGraphStore,
      boardBaseSnapshot: board.boardSession.boardBaseSnapshot,
      flushAutosave: flushAutosaveWithViewportPersistence,
      refreshNodeInternals: editing.commands.scheduleNodeInternalsRefresh,
    },
    flowPaneAdapter: options.flowPaneAdapter,
    openImageUrlPreview: options.openImageUrlPreview,
    ui: options.imageWorkflowUi,
    feedback: {
      imageTools: feedback.imageTools,
      taskExecution: feedback.taskExecution,
    },
  })

  const parts = {
    boardChrome: {
      currentBoardId: board.boardSession.currentBoardId,
      ...board.boardChrome,
    },
    flowPane: {
      nodes: board.flowPane.nodes,
      edges: board.flowPane.edges,
      onNodesChange: board.flowPane.onNodesChange,
      onEdgesChange: board.flowPane.onEdgesChange,
      connectionAdapter: editing.flowPane.connectionAdapter,
      nodeDragTransactions: board.flowPane.nodeDragTransactions,
    },
    persistence: {
      flushAutosave: flushAutosaveWithViewportPersistence,
      history: board.boardSession.history,
      loadCanvasBoardAndProbe: viewport.loadCanvasBoardAndProbe,
      reattachPendingImageResults: imageWorkflow.reattachPendingImageResults,
    },
    viewportControls: viewport,
    graphEditing: {
      graphAdapter: board.graphEditing.adapter,
      grouping: board.graphEditing.grouping,
      scheduleFlowNodeInternalsRefresh: editing.commands.scheduleNodeInternalsRefresh,
      createNodeAtPosition: editing.commands.createNodeAtPosition,
      createNodeInteractions: editing.commands.createNodeInteractions,
      hasSelectedFlowNodes: graphInteractions.viewModel.hasSelectedNodes,
      onExportSelectedImages: graphInteractions.commands.exportSelectedImages,
      isFlowInteractionActive: graphInteractions.viewModel.isInteractionActive,
      isFlowViewportPanInteractionActive:
        graphInteractions.viewModel.isViewportPanInteractionActive,
      isFlowInteractionLocked: graphInteractions.viewModel.isInteractionLocked,
      onFlowMediaInputImageDrop: editing.commands.mediaInputs.drop,
      onFlowMediaInputImageOpen: editing.commands.mediaInputs.open,
      onFlowMediaInputImagePreview: editing.commands.mediaInputs.preview,
      onFlowMediaInputImageRemove: editing.commands.mediaInputs.remove,
      onCanvasDrop: editing.commands.canvasFiles.drop,
      onCanvasDragOver: editing.commands.canvasFiles.dragOver,
      onFlowImageOpenPreview: graphInteractions.commands.openImagePreview,
      onFlowImageAppendToCanvas: graphInteractions.commands.appendImageToCanvas,
      beginFlowNodeDragDuplicateTracking: graphInteractions.commands.beginNodeDragDuplicateTracking,
      finishFlowNodeDragDuplicateTracking:
        graphInteractions.commands.finishNodeDragDuplicateTracking,
      getFlowNodeUiStateCached: graphInteractions.commands.getNodeUiState,
    },
    imageWorkflow,
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
      imageWorkflow: parts.imageWorkflow.viewModel,
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
        createNodeInteractions: parts.graphEditing.createNodeInteractions,
      },
      imageWorkflow: parts.imageWorkflow.commands,
    },
  } satisfies CanvasWorkspaceRuntime
}
