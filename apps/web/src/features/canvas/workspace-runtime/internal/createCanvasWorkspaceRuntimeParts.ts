import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { RouteLocationNormalizedLoaded, Router } from 'vue-router'
import type { CanvasCreatableNodeKind, ImageNodeItem } from '../../../canvas-board/domain'
import type { CanvasFlowViewport } from '../../composables/useCanvasViewportRuntime'
import { useCanvasViewportRuntime } from '../../composables/useCanvasViewportRuntime'
import { useCanvasBoardRuntime } from '../../composables/useCanvasBoardRuntime'
import { useCanvasGraphRuntime } from '../../composables/useCanvasGraphRuntime'
import { useCanvasTaskRuntime } from '../../composables/useCanvasTaskRuntime'
import { useFlowImageActionOrchestrator } from '../../composables/useFlowImageActionOrchestrator'
import { useFlowPromptGenerationOrchestrator } from '../../composables/useFlowPromptGenerationOrchestrator'
import { useFlowCanvasNodeHandlers } from '../../composables/useFlowCanvasNodeHandlers'
import { useFlowNodeMediaInputs } from '../../composables/useFlowNodeMediaInputs'
import { useImageCanvasExtendOverlay } from '../../composables/useImageCanvasExtendOverlay'
import { useCanvasFeedback } from '../../feedback/useCanvasFeedback'
import { useCanvasImageTools } from '../../image-tools'
import type { UseCanvasImageToolsOptions } from '../../image-tools/useCanvasImageTools'
import type { CanvasVueFlowGraphActions } from '../../composables/useCanvasVueFlowGraphAdapter'

type ValueRef<T> = {
  readonly value: T
}

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

type CanvasWorkspaceImageToolsOptions = Omit<
  UseCanvasImageToolsOptions,
  | 'nodes'
  | 'currentBoardId'
  | 'getImageNodeDisplayPayload'
  | 'getImageNodeSnapshot'
  | 'screenToFlowCoordinate'
  | 'getViewport'
  | 'setViewport'
  | 'setCenter'
  | 'createImageNodeFromFile'
  | 'connectImageNodes'
  | 'uploadImageToBoard'
  | 'openImageUrlPreview'
  | 'updateOutputCount'
  | 'runInpaintAction'
  | 'runPromptGeneration'
  | 'isDirectActionRunning'
  | 'clearSelectedFlowElements'
  | 'startCanvasInteraction'
  | 'stopCanvasInteraction'
  | 'feedback'
>

type CanvasWorkspaceImageActionOptions = Omit<
  Parameters<typeof useFlowImageActionOrchestrator>[0],
  | 'runImageDirectAction'
  | 'runFusionPromptGeneration'
  | 'onFlowNodeParamUpdate'
  | 'onWarning'
>

type CanvasWorkspacePromptGenerationOptions = Pick<
  Parameters<typeof useFlowPromptGenerationOrchestrator>[0],
  'cancelFlowExpandMode' | 'cancelFlowMaskEditorMode'
>

type CanvasWorkspaceExpandOverlayOptions = Omit<
  Parameters<typeof useImageCanvasExtendOverlay>[0],
  'nodes' | 'getImageNodeDisplayPayload' | 'onBeforePointerCancel' | 'onCommit'
>

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
  viewport: Readonly<Ref<CanvasFlowViewport>>
  getViewport: () => CanvasFlowViewport
  setViewport: (
    viewport: CanvasFlowViewport,
    options?: { duration?: number; interpolate?: 'smooth' },
  ) => Promise<unknown> | unknown
  setCenter: (
    x: number,
    y: number,
    options?: { duration?: number; zoom?: number; interpolate?: 'smooth' },
  ) => Promise<unknown> | unknown
  zoomIn: () => Promise<unknown> | unknown
  zoomOut: () => Promise<unknown> | unknown
  panBy: (delta: { x: number; y: number }) => boolean
  updateNodeInternals: (nodeIds: string[]) => void
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  getSelectedElements: ValueRef<Array<Node | Edge>>
  removeSelectedElements: (elements: Array<Node | Edge>) => void
  uploadApi: CanvasGraphUploadApi
  sanitizeApiErrorMessage: (message: string | null | undefined, fallback: string) => string
  openNodeImagePreview: (snapshot: ImageNodeItem, mode: 'latest-output') => Promise<void>
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  logoutAndRedirect: (path: string) => Promise<unknown> | unknown
  startNodeDragInteraction: () => void
  stopNodeDragInteraction: () => void
  refreshFlowHandleVisibility: () => void
  onNativeRemove?: () => void
  vueFlowGraphActions?: CanvasVueFlowGraphActions
}

export function createCanvasWorkspaceRuntimeParts(
  options: UseCanvasWorkspaceRuntimeOptions,
) {
  const feedback = useCanvasFeedback()

  const board = useCanvasBoardRuntime({
    route: options.route,
    router: options.router,
    getUserIdentity: options.getUserIdentity,
    getViewport: options.getViewport,
    setViewport: (viewport) => options.setViewport(viewport),
    logoutAndRedirect: options.logoutAndRedirect,
    feedback: feedback.boardSession,
    onNativeRemove: options.onNativeRemove,
    vueFlowGraphActions: options.vueFlowGraphActions,
  })

  const graph = useCanvasGraphRuntime({
    nodes: board.fullNodes,
    edges: board.fullEdges,
    boardBaseSnapshot: board.boardBaseSnapshot,
    currentBoardId: board.currentBoardId,
    createNodeKind: options.createNodeKind,
    graphAdapter: board.graphAdapter,
    updateNodeInternals: options.updateNodeInternals,
    screenToFlowCoordinate: options.screenToFlowCoordinate,
    getSelectedElements: options.getSelectedElements,
    removeSelectedElements: options.removeSelectedElements,
    uploadApi: options.uploadApi,
    sanitizeApiErrorMessage: options.sanitizeApiErrorMessage,
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
    viewport: options.viewport,
    getViewport: options.getViewport,
    setViewport: options.setViewport,
    setCenter: options.setCenter,
    zoomIn: options.zoomIn,
    zoomOut: options.zoomOut,
    panBy: options.panBy,
    markBoardViewportChanged: board.markBoardViewportChanged,
    commitPendingBoardViewportChange: board.commitPendingBoardViewportChange,
    loadInitialOrRouteBoard: board.loadInitialOrRouteBoard,
    closeBoardHeaderEditing: board.closeBoardHeaderEditing,
    startCanvasInteraction: graph.startFlowInteraction,
    stopCanvasInteraction: graph.stopFlowInteraction,
    startNodeDragInteraction: options.startNodeDragInteraction,
    stopNodeDragInteraction: options.stopNodeDragInteraction,
    refreshFlowHandleVisibility: options.refreshFlowHandleVisibility,
  })

  const task = useCanvasTaskRuntime({
    nodes: board.fullNodes,
    edges: board.fullEdges,
    graphAdapter: board.graphAdapter,
    flushAutosave: board.flushAutosave,
    currentBoardId: board.currentBoardId,
    refreshNodeInternals: graph.scheduleFlowNodeInternalsRefresh,
    feedback: feedback.taskExecution,
  })

  function createImageTools(imageToolOptions: CanvasWorkspaceImageToolsOptions) {
    return useCanvasImageTools({
      ...imageToolOptions,
      nodes: board.nodes,
      currentBoardId: board.currentBoardId,
      getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
      getImageNodeSnapshot: graph.getImageNodeSnapshot,
      screenToFlowCoordinate: options.screenToFlowCoordinate,
      getViewport: options.getViewport,
      setViewport: options.setViewport,
      setCenter: options.setCenter,
      createImageNodeFromFile: graph.createImageNodeFromFile,
      connectImageNodes: graph.onConnect,
      uploadImageToBoard: graph.uploadImageToBoard,
      openImageUrlPreview: options.openImageUrlPreview,
      updateOutputCount: (nodeId, payload) => {
        graph._onFlowNodeParamUpdate(nodeId, payload)
        graph.markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
      },
      runInpaintAction: task.runImageInpaintAction,
      runPromptGeneration: task.runImagePromptGeneration,
      isDirectActionRunning: task.isNodeDirectActionRunning,
      clearSelectedFlowElements: graph.clearSelectedFlowElementsSync,
      startCanvasInteraction: graph.startFlowInteraction,
      stopCanvasInteraction: graph.stopFlowInteraction,
      feedback: feedback.imageTools,
    })
  }

  function createImageActionHandlers(actionOptions: CanvasWorkspaceImageActionOptions) {
    return useFlowImageActionOrchestrator({
      ...actionOptions,
      runImageDirectAction: task.runImageDirectAction,
      runFusionPromptGeneration: task.runFusionPromptGeneration,
      onFlowNodeParamUpdate: (nodeId, patch) => {
        graph._onFlowNodeParamUpdate(nodeId, patch)
        graph.markFlowExecutionNodeDirty(nodeId, { includeSelf: true })
      },
      onWarning: feedback.imageTools.actionUnavailable,
    })
  }

  function createPromptGenerationHandlers(
    promptOptions: CanvasWorkspacePromptGenerationOptions,
  ) {
    return useFlowPromptGenerationOrchestrator({
      ...promptOptions,
      getMediaInputImages: graph.getMediaInputImages,
      resolvePromptInputImageNodeIds: graph.resolvePromptInputImageNodeIds,
      runTextPromptGeneration: task.runTextPromptGeneration,
      runImagePromptGeneration: task.runImagePromptGeneration,
      runImagePromptGenerationFromSource: task.runImagePromptGenerationFromSource,
    })
  }

  function createCanvasExtendOverlay(overlayOptions: CanvasWorkspaceExpandOverlayOptions) {
    return useImageCanvasExtendOverlay({
      ...overlayOptions,
      nodes: board.nodes,
      getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
      onBeforePointerCancel: graph.clearSelectedFlowElementsSync,
      onCommit: async (payload) => {
        await task.runCanvasExtendAction(payload.nodeId, {
          frameRect: payload.frameRect,
          originalImageRect: payload.originalImageRect,
          prompt: payload.prompt,
          outputCount: payload.outputCount,
        })
      },
    })
  }

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
      currentBoardName: board.currentBoardName,
      boardOptions: board.boardOptions,
      setBoardNameInputRef: board.setBoardNameInputRef,
      refreshBoardMetas: board.refreshBoardMetas,
      toggleBoardMenu: board.toggleBoardMenu,
      startBoardRename: board.startBoardRename,
      cancelBoardRename: board.cancelBoardRename,
      closeBoardHeaderEditing: board.closeBoardHeaderEditing,
      onConfirmBoardRename: board.onConfirmBoardRename,
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
      flushAutosave: board.flushAutosave,
      history: board.flowHistory,
      loadCanvasBoardAndProbe: viewport.loadCanvasBoardAndProbe,
      reattachPendingImageResults: task.reattachPendingImageResults,
    },
    viewportControls: viewport,
    graphEditing: {
      graphAdapter: board.graphAdapter,
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
    imageWorkflow: {
      runImageDirectAction: task.runImageDirectAction,
      runCanvasExtendAction: task.runCanvasExtendAction,
    },
    createImageTools,
    createImageActionHandlers,
    createPromptGenerationHandlers,
    createCanvasExtendOverlay,
    createNodeMediaInputs,
    createNodeHandlers,
  }
}
