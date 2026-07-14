import type { Edge, Node } from '@vue-flow/core'
import { computed, type ComputedRef, type Ref } from 'vue'
import type {
  CanvasBoardSnapshot,
  CanvasCreatableNodeKind,
  CanvasNodeItem,
  ImageNodeItem,
} from '../../canvas-board/domain'
import { useFlowCanvasDrop } from './useFlowCanvasDrop'
import { useFlowCanvasInteractionState } from './useFlowCanvasInteractionState'
import { useFlowImageNodeInteractions } from './useFlowImageNodeInteractions'
import { useFlowImageNodeSnapshots } from './useFlowImageNodeSnapshots'
import { useFlowImageUploadActions } from './useFlowImageUploadActions'
import { useFlowMediaInputImages } from './useFlowMediaInputImages'
import { useFlowNodeContextMenu } from './useFlowNodeContextMenu'
import { useFlowNodeInternalsRefreshQueue } from './useFlowNodeInternalsRefreshQueue'
import { useFlowNodeMutations } from './useFlowNodeMutations'
import { useFlowNodeUiState } from './useFlowNodeUiState'
import { useFlowSelectionExport } from './useFlowSelectionExport'
import type { CanvasVueFlowGraphAdapter } from './useCanvasVueFlowGraphAdapter'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'

type ValueRef<T> = {
  readonly value: T
}

type CanvasGraphRuntimeFeedback = CanvasFeedback['graph']

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

type SnapshotNodeMutator = (snapshotNode: CanvasNodeItem) => CanvasNodeItem

type UseCanvasGraphRuntimeOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  boardBaseSnapshot?: Ref<CanvasBoardSnapshot>
  currentBoardId: ValueRef<string | null | undefined>
  createNodeKind: Ref<CanvasCreatableNodeKind>
  graphAdapter: CanvasVueFlowGraphAdapter
  deleteNode: (nodeId: string) => boolean
  updateNodeInternals: (nodeIds: string[]) => void
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  getSelectedElements: ValueRef<Array<Node | Edge>>
  removeSelectedElements: (elements: Array<Node | Edge>) => void
  uploadApi: CanvasGraphUploadApi
  openNodeImagePreview: (snapshot: ImageNodeItem, mode: 'latest-output') => Promise<void>
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  feedback: CanvasGraphRuntimeFeedback
}

export function useCanvasGraphRuntime(options: UseCanvasGraphRuntimeOptions) {
  const message = {
    success: options.feedback.operationSucceeded,
    warning: options.feedback.operationWarning,
    loading: options.feedback.operationLoading,
  }
  const { scheduleFlowNodeInternalsRefresh } = useFlowNodeInternalsRefreshQueue({
    updateNodeInternals: options.updateNodeInternals,
  })

  function clearSelectedFlowElementsSync() {
    const selectedElements = [...options.getSelectedElements.value]
    if (selectedElements.length === 0) return
    options.removeSelectedElements(selectedElements)
  }

  const flowNodeMutations = useFlowNodeMutations({
    nodes: options.nodes,
    edges: options.edges,
    boardBaseSnapshot: options.boardBaseSnapshot,
    createNodeKind: options.createNodeKind,
    graphAdapter: options.graphAdapter,
    onWarning: (text) => {
      options.feedback.operationWarning(text)
    },
  })

  function updateSnapshotNode(
    nodeId: string,
    mutate: SnapshotNodeMutator,
    updateOptions?: { history?: boolean },
  ) {
    flowNodeMutations.updateSnapshotNode(nodeId, mutate, updateOptions)
    scheduleFlowNodeInternalsRefresh([nodeId])
  }

  const selectionExport = useFlowSelectionExport({
    nodes: options.nodes,
    message,
  })
  const hasSelectedFlowNodes: ComputedRef<boolean> = computed(() =>
    selectionExport.hasSelectedNodes(),
  )

  function onExportSelectedImages() {
    selectionExport.exportSelectedImages()
  }

  const {
    isCanvasInteractionActive: isFlowInteractionActive,
    isViewportPanInteractionActive: isFlowViewportPanInteractionActive,
    isCanvasInteractionLocked: isFlowInteractionLocked,
    startCanvasInteraction: startFlowInteraction,
    stopCanvasInteraction: stopFlowInteraction,
  } = useFlowCanvasInteractionState()

  const { nodeContextMenu, canvasNodeMenuItems, closeNodeContextMenu, openNodeContextMenu } =
    useFlowNodeContextMenu()

  const { getImageNodeSnapshot, getImageNodeDisplayPayload } = useFlowImageNodeSnapshots({
    nodes: options.nodes,
  })

  const { uploadImageToBoard, applyImageFileToImageNode, createImageNodeFromFile } =
    useFlowImageUploadActions({
      currentBoardId: options.currentBoardId,
      createNodeAtPosition: flowNodeMutations.createNodeAtPosition,
      updateSnapshotNode,
      getImageNodeSnapshot,
      uploadApi: options.uploadApi,
      onWarning: (msg) => options.feedback.operationWarning(msg),
      onSuccess: (msg) => options.feedback.operationSucceeded(msg),
      onNodeMutated: (nodeId) => flowNodeMutations.markFlowExecutionNodeDirty(nodeId),
    })

  const {
    onFlowMediaInputImageDrop,
    onFlowMediaInputImageOpen,
    onFlowMediaInputImagePreview,
    onFlowMediaInputImageRemove,
    getMediaInputImages,
    resolvePromptInputImageNodeIds,
  } = useFlowMediaInputImages({
    nodes: options.nodes,
    edges: options.edges,
    uploadImageToBoard,
    updateSnapshotNodeBase: flowNodeMutations.updateSnapshotNode,
    graphAdapter: options.graphAdapter,
    markFlowExecutionNodeDirty: flowNodeMutations.markFlowExecutionNodeDirty,
    getImageNodeDisplayPayload,
    openImageUrlPreview: options.openImageUrlPreview,
    onWarning: (message) => {
      options.feedback.operationWarning(message)
    },
  })

  const canvasDrop = useFlowCanvasDrop({
    screenToFlowCoordinate: options.screenToFlowCoordinate,
    onWarning: (msg) => options.feedback.operationWarning(msg),
    createImageNodeFromFile,
  })

  async function onCanvasDrop(event: DragEvent) {
    await canvasDrop.onCanvasDrop(event)
  }

  function onFlowImageOpenPreview(nodeId: string) {
    const snapshot = getImageNodeSnapshot(nodeId)
    if (!snapshot) return
    void options.openNodeImagePreview(snapshot, 'latest-output')
  }

  const {
    onFlowImageAppendToCanvas,
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
  } = useFlowImageNodeInteractions({
    nodes: options.nodes,
    getImageNodeDisplayPayload,
    getImageNodeSnapshot: (nodeId) => {
      const snapshot = getImageNodeSnapshot(nodeId)
      if (!snapshot) return null
      return {
        x: Number(snapshot.x ?? 0),
        y: Number(snapshot.y ?? 0),
        width: Number(snapshot.width ?? 300),
        outputCount: snapshot.outputCount,
      }
    },
    graphAdapter: options.graphAdapter,
  })

  const { getFlowNodeUiStateCached } = useFlowNodeUiState({
    nodes: options.nodes,
    edges: options.edges,
  })

  return {
    scheduleFlowNodeInternalsRefresh,
    clearSelectedFlowElementsSync,
    connectionAdapter: flowNodeMutations.connectionAdapter,
    createNodeAtPosition: flowNodeMutations.createNodeAtPosition,
    onFlowDeleteNode: options.deleteNode,
    _onFlowPromptTextUpdate: flowNodeMutations.onFlowPromptTextUpdate,
    _onFlowNodeParamUpdate: flowNodeMutations.onFlowNodeParamUpdate,
    markFlowExecutionNodeDirty: flowNodeMutations.markFlowExecutionNodeDirty,
    updateSnapshotNodeBase: flowNodeMutations.updateSnapshotNode,
    updateSnapshotNode,
    hasSelectedFlowNodes,
    onExportSelectedImages,
    isFlowInteractionActive,
    isFlowViewportPanInteractionActive,
    isFlowInteractionLocked,
    startFlowInteraction,
    stopFlowInteraction,
    nodeContextMenu,
    canvasNodeMenuItems,
    closeNodeContextMenu,
    openNodeContextMenu,
    getImageNodeSnapshot,
    getImageNodeDisplayPayload,
    uploadImageToBoard,
    applyImageFileToImageNode,
    createImageNodeFromFile,
    onFlowMediaInputImageDrop,
    onFlowMediaInputImageOpen,
    onFlowMediaInputImagePreview,
    onFlowMediaInputImageRemove,
    getMediaInputImages,
    resolvePromptInputImageNodeIds,
    onCanvasDrop,
    onCanvasDragOver: canvasDrop.onCanvasDragOver,
    onFlowImageOpenPreview,
    onFlowImageAppendToCanvas,
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    getFlowNodeUiStateCached,
  }
}
