import type { Edge, Node } from '@vue-flow/core'
import { computed, type ComputedRef, type Ref } from 'vue'
import type { CanvasCreatableNodeKind, ImageNodeItem } from '../../canvas-board/domain'
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
import { useFlowSnapshotNodeUpdater } from './useFlowSnapshotNodeUpdater'
import type { FlowTransactions } from './useFlowTransactions'

type ValueRef<T> = {
  readonly value: T
}

type CanvasGraphRuntimeMessage = {
  success: (message: string) => unknown
  warning: (message: string) => unknown
  loading: (
    message: string,
    options?: { duration?: number },
  ) => {
    destroy: () => void
  }
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

type UseCanvasGraphRuntimeOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  currentBoardId: ValueRef<string | null | undefined>
  createNodeKind: Ref<CanvasCreatableNodeKind>
  flowTransactions: Pick<
    FlowTransactions,
    'commitGraphMutation' | 'commitNodeMutation' | 'commitEdgeMutation'
  >
  updateNodeInternals: (nodeIds: string[]) => void
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  getSelectedElements: ValueRef<Array<Node | Edge>>
  removeSelectedElements: (elements: Array<Node | Edge>) => void
  uploadApi: CanvasGraphUploadApi
  sanitizeApiErrorMessage: (message: string | null | undefined, fallback: string) => string
  openNodeImagePreview: (snapshot: ImageNodeItem, mode: 'latest-output') => Promise<void>
  openImageUrlPreview: (src: string, ratio?: number) => Promise<void>
  message: CanvasGraphRuntimeMessage
}

export function useCanvasGraphRuntime(options: UseCanvasGraphRuntimeOptions) {
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
    createNodeKind: options.createNodeKind,
    commitGraphMutation: options.flowTransactions.commitGraphMutation,
    commitNodeMutation: options.flowTransactions.commitNodeMutation,
    commitEdgeMutation: options.flowTransactions.commitEdgeMutation,
    onWarning: (text) => {
      options.message.warning(text)
    },
  })

  const { updateSnapshotNode } = useFlowSnapshotNodeUpdater({
    updateSnapshotNodeBase: flowNodeMutations.updateSnapshotNode,
    scheduleFlowNodeInternalsRefresh,
  })

  const selectionExport = useFlowSelectionExport({
    nodes: options.nodes,
    message: options.message,
  })
  const hasSelectedFlowNodes: ComputedRef<boolean> = computed(() =>
    selectionExport.hasSelectedNodes(),
  )

  function onExportSelectedImages() {
    selectionExport.exportSelectedImages()
  }

  const {
    isCanvasInteractionActive: isFlowInteractionActive,
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
      sanitizeApiErrorMessage: options.sanitizeApiErrorMessage,
      onWarning: (msg) => options.message.warning(msg),
      onSuccess: (msg) => options.message.success(msg),
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
    commitGraphMutation: options.flowTransactions.commitGraphMutation,
    markFlowExecutionNodeDirty: flowNodeMutations.markFlowExecutionNodeDirty,
    getImageNodeDisplayPayload,
    openImageUrlPreview: options.openImageUrlPreview,
    onWarning: (message) => {
      options.message.warning(message)
    },
  })

  const canvasDrop = useFlowCanvasDrop({
    screenToFlowCoordinate: options.screenToFlowCoordinate,
    onWarning: (msg) => options.message.warning(msg),
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
    commitNodeMutation: options.flowTransactions.commitNodeMutation,
  })

  const { getFlowNodeUiStateCached } = useFlowNodeUiState({
    nodes: options.nodes,
    edges: options.edges,
  })

  return {
    scheduleFlowNodeInternalsRefresh,
    clearSelectedFlowElementsSync,
    isValidConnection: flowNodeMutations.isValidConnection,
    onConnect: flowNodeMutations.onConnect,
    onEdgeUpdate: flowNodeMutations.onEdgeUpdate,
    createNodeAtPosition: flowNodeMutations.createNodeAtPosition,
    deleteSelectedFlowElements: flowNodeMutations.deleteSelectedFlowElements,
    _onFlowDeleteNode: flowNodeMutations.onFlowDeleteNode,
    _onFlowPromptTextUpdate: flowNodeMutations.onFlowPromptTextUpdate,
    _onFlowNodeParamUpdate: flowNodeMutations.onFlowNodeParamUpdate,
    markFlowExecutionNodeDirty: flowNodeMutations.markFlowExecutionNodeDirty,
    updateSnapshotNodeBase: flowNodeMutations.updateSnapshotNode,
    updateSnapshotNode,
    hasSelectedFlowNodes,
    onExportSelectedImages,
    isFlowInteractionActive,
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
