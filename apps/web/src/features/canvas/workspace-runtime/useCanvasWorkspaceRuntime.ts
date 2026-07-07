import {
  createCanvasWorkspaceRuntimeParts,
  type UseCanvasWorkspaceRuntimeOptions,
} from './internal/createCanvasWorkspaceRuntimeParts'
import type { CanvasWorkspaceRuntime } from './types'

export function useCanvasWorkspaceRuntime(
  options: UseCanvasWorkspaceRuntimeOptions,
): CanvasWorkspaceRuntime {
  const runtime = createCanvasWorkspaceRuntimeParts(options)

  return {
    viewModel: {
      boardChrome: {
        currentBoardId: runtime.boardChrome.currentBoardId,
        isBoardMenuOpen: runtime.boardChrome.isBoardMenuOpen,
        isRenamingBoard: runtime.boardChrome.isRenamingBoard,
        boardNameDraft: runtime.boardChrome.boardNameDraft,
        currentBoardName: runtime.boardChrome.currentBoardName,
        boardOptions: runtime.boardChrome.boardOptions,
      },
      flowPane: {
        nodes: runtime.flowPane.nodes,
        edges: runtime.flowPane.edges,
      },
      persistence: {
        history: runtime.persistence.history,
      },
      viewportControls: {
        zoomLevel: runtime.viewportControls.zoomLevel,
      },
      graphEditing: {
        graphAdapter: runtime.graphEditing.graphAdapter,
        hasSelectedFlowNodes: runtime.graphEditing.hasSelectedFlowNodes,
        isFlowInteractionActive: runtime.graphEditing.isFlowInteractionActive,
        isFlowViewportPanInteractionActive: runtime.graphEditing.isFlowViewportPanInteractionActive,
        isFlowInteractionLocked: runtime.graphEditing.isFlowInteractionLocked,
        nodeContextMenu: runtime.graphEditing.nodeContextMenu,
        canvasNodeMenuItems: runtime.graphEditing.canvasNodeMenuItems,
      },
    },
    commands: {
      boardSession: {
        setBoardNameInputRef: runtime.boardChrome.setBoardNameInputRef,
        refreshBoardMetas: runtime.boardChrome.refreshBoardMetas,
        toggleBoardMenu: runtime.boardChrome.toggleBoardMenu,
        startBoardRename: runtime.boardChrome.startBoardRename,
        cancelBoardRename: runtime.boardChrome.cancelBoardRename,
        closeBoardHeaderEditing: runtime.boardChrome.closeBoardHeaderEditing,
        onConfirmBoardRename: runtime.boardChrome.onConfirmBoardRename,
        onSelectBoard: runtime.boardChrome.onSelectBoard,
        onNavigateRequest: runtime.boardChrome.onNavigateRequest,
        onLogoutRequest: runtime.boardChrome.onLogoutRequest,
      },
      flowPane: {
        onNodesChange: runtime.flowPane.onNodesChange,
        onEdgesChange: runtime.flowPane.onEdgesChange,
        isValidConnection: runtime.flowPane.isValidConnection,
        onConnect: runtime.flowPane.onConnect,
        onEdgeUpdate: runtime.flowPane.onEdgeUpdate,
        nodeDragTransactions: runtime.flowPane.nodeDragTransactions,
      },
      persistence: {
        flushAutosave: runtime.persistence.flushAutosave,
        loadCanvasBoardAndProbe: runtime.persistence.loadCanvasBoardAndProbe,
        reattachPendingImageResults: runtime.persistence.reattachPendingImageResults,
      },
      viewportControls: {
        onZoomIn: runtime.viewportControls.onZoomIn,
        onZoomOut: runtime.viewportControls.onZoomOut,
        onPanViewport: runtime.viewportControls.onPanViewport,
        onMiniMapCenterRequested: runtime.viewportControls.onMiniMapCenterRequested,
        onMiniMapDragStart: runtime.viewportControls.onMiniMapDragStart,
        onMiniMapDragMove: runtime.viewportControls.onMiniMapDragMove,
        onMiniMapDragEnd: runtime.viewportControls.onMiniMapDragEnd,
        onViewportChangeEnd: runtime.viewportControls.onViewportChangeEnd,
        onFlowMoveStart: runtime.viewportControls.onFlowMoveStart,
        onFlowMove: runtime.viewportControls.onFlowMove,
        onFlowMoveEnd: runtime.viewportControls.onFlowMoveEnd,
        onNodeDragStart: runtime.viewportControls.onNodeDragStart,
        onNodeDragStop: runtime.viewportControls.onNodeDragStop,
        flushViewportPersistence: runtime.viewportControls.flushViewportPersistence,
        loadCanvasBoardAndProbe: runtime.viewportControls.loadCanvasBoardAndProbe,
      },
      graphEditing: {
        scheduleFlowNodeInternalsRefresh: runtime.graphEditing.scheduleFlowNodeInternalsRefresh,
        createNodeAtPosition: runtime.graphEditing.createNodeAtPosition,
        onExportSelectedImages: runtime.graphEditing.onExportSelectedImages,
        closeNodeContextMenu: runtime.graphEditing.closeNodeContextMenu,
        openNodeContextMenu: runtime.graphEditing.openNodeContextMenu,
        onFlowMediaInputImageDrop: runtime.graphEditing.onFlowMediaInputImageDrop,
        onFlowMediaInputImageOpen: runtime.graphEditing.onFlowMediaInputImageOpen,
        onFlowMediaInputImagePreview: runtime.graphEditing.onFlowMediaInputImagePreview,
        onFlowMediaInputImageRemove: runtime.graphEditing.onFlowMediaInputImageRemove,
        onCanvasDrop: runtime.graphEditing.onCanvasDrop,
        onCanvasDragOver: runtime.graphEditing.onCanvasDragOver,
        onFlowImageOpenPreview: runtime.graphEditing.onFlowImageOpenPreview,
        onFlowImageAppendToCanvas: runtime.graphEditing.onFlowImageAppendToCanvas,
        beginFlowNodeDragDuplicateTracking: runtime.graphEditing.beginFlowNodeDragDuplicateTracking,
        finishFlowNodeDragDuplicateTracking:
          runtime.graphEditing.finishFlowNodeDragDuplicateTracking,
        getFlowNodeUiStateCached: runtime.graphEditing.getFlowNodeUiStateCached,
      },
      imageWorkflow: {
        runImageDirectAction: runtime.imageWorkflow.runImageDirectAction,
        runCanvasExtendAction: runtime.imageWorkflow.runCanvasExtendAction,
        createImageTools: runtime.imageWorkflow.createImageTools,
        createImageActionHandlers: runtime.imageWorkflow.createImageActionHandlers,
        createPromptGenerationHandlers: runtime.imageWorkflow.createPromptGenerationHandlers,
        createCanvasExtendOverlay: runtime.imageWorkflow.createCanvasExtendOverlay,
      },
      nodeInteraction: {
        createNodeMediaInputs: runtime.createNodeMediaInputs,
        createNodeHandlers: runtime.createNodeHandlers,
      },
    },
  }
}
