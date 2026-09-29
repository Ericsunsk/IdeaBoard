import type { ImageNodeItem } from '../../../../board/ports/canvas'
import { onScopeDispose, ref } from 'vue'
import type { useBoardRuntime } from './useBoardRuntime'
import type { CanvasFeedback } from '../useCanvasFeedback.ts'
import type { CanvasVueFlowAdapter } from '../../../infrastructure/vue-flow-adapter'
import {
  useCanvasGraphEditing,
  useCanvasGraphInteractions,
  type CanvasGraphEditingBrowserAdapter,
  type CanvasGraphEditingUploadAdapter,
  type CanvasGraphInteractionBrowserAdapter,
} from '../../editing'
import type { PendingImageExecutionCore } from '../../execution/pending-execution'
import type { LibraryAssetMaterializer } from '../../../../library'

type CanvasWorkspaceBoardRuntime = ReturnType<typeof useBoardRuntime>

type CreateCanvasWorkspaceGraphRuntimeOptions = {
  board: CanvasWorkspaceBoardRuntime
  flowPaneAdapter: CanvasVueFlowAdapter
  upload: CanvasGraphEditingUploadAdapter
  getWorkspaceId?: (() => string) | undefined
  materializeLibraryAsset: LibraryAssetMaterializer
  editingBrowser: CanvasGraphEditingBrowserAdapter
  interactionBrowser: CanvasGraphInteractionBrowserAdapter
  preview: {
    openImageUrl: (src: string, ratio?: number) => Promise<void>
    openNodeImage: (snapshot: ImageNodeItem, mode: 'latest-output') => Promise<void>
  }
  feedback: CanvasFeedback['graph']
  executionCore: PendingImageExecutionCore
}

export function createCanvasWorkspaceGraphRuntime(
  options: CreateCanvasWorkspaceGraphRuntimeOptions,
) {
  const { board, flowPaneAdapter } = options
  // Reactive mirror of the pending-execution snapshot version. Lease state is
  // non-reactive, so ui-state caches track this epoch to flip running labels
  // without waiting for a graph change.
  const executionEpoch = useExecutionEpoch(options.executionCore)
  const editing = useCanvasGraphEditing({
    nodes: board.flowPane.nodes,
    edges: board.flowPane.edges,
    boardBaseSnapshot: board.boardSession.boardBaseSnapshot,
    currentBoardId: board.boardSession.currentBoardId,
    graphAdapter: board.graphEditing.adapter,
    readCanvasGraphStore: board.graphEditing.readCanvasGraphStore,
    deleteNode: board.graphEditing.deleteNode,
    updateNodeInternals: flowPaneAdapter.updateNodeInternals,
    screenToFlowCoordinate: flowPaneAdapter.screenToFlowCoordinate,
    browser: options.editingBrowser,
    upload: options.upload,
    getWorkspaceId: options.getWorkspaceId,
    materializeLibraryAsset: options.materializeLibraryAsset,
    readLibraryImportAttachment: options.executionCore.currentAttachment,
    selectOnlyNode: flowPaneAdapter.selectOnlyNode,
    preview: { openImageUrl: options.preview.openImageUrl },
    feedback: options.feedback,
  })

  const interactions = useCanvasGraphInteractions({
    nodes: board.flowPane.nodes,
    edges: board.flowPane.edges,
    readCanvasGraphStore: board.graphEditing.readCanvasGraphStore,
    isGraphSessionAttached: board.graphEditing.isGraphSessionAttached,
    writeCanvasGraphStore: board.graphEditing.readCanvasGraphStore,
    rendererMetrics: flowPaneAdapter.rendererMetrics,
    selection: {
      selectedElements: flowPaneAdapter.getSelectedElements,
      clear: flowPaneAdapter.removeSelectedElements,
    },
    browser: options.interactionBrowser,
    preview: { openNodeImage: options.preview.openNodeImage },
    feedback: options.feedback,
    isNodeExecutionActive: (nodeId) =>
      options.executionCore.captureActiveLeases([nodeId as `image-${number}`]).length > 0,
    executionEpoch,
  })

  return {
    viewModel: {
      grouping: board.graphEditing.grouping.view,
      hasSelectedFlowNodes: interactions.viewModel.hasSelectedNodes,
      isFlowInteractionActive: interactions.viewModel.isInteractionActive,
      isFlowViewportPanInteractionActive: interactions.viewModel.isViewportPanInteractionActive,
      isFlowInteractionLocked: interactions.viewModel.isInteractionLocked,
    },
    commands: {
      grouping: board.graphEditing.grouping.commands,
      createNodeAtPosition: editing.commands.createNodeAtPosition,
      libraryImport: editing.commands.libraryImport,
      imageImport: editing.commands.imageImport,
      createNodeInteractions: editing.commands.createNodeInteractions,
      onExportSelectedImages: interactions.commands.exportSelectedImages,
      onFlowMediaInputImageDrop: editing.commands.mediaInputs.drop,
      onFlowMediaInputImageOpen: editing.commands.mediaInputs.open,
      onFlowMediaInputImagePreview: editing.commands.mediaInputs.preview,
      onFlowMediaInputImageRemove: editing.commands.mediaInputs.remove,
      onCanvasDrop: editing.commands.canvasFiles.drop,
      onCanvasDragOver: editing.commands.canvasFiles.dragOver,
      onFlowImageOpenPreview: interactions.commands.openImagePreview,
      onFlowImageAppendToCanvas: interactions.commands.appendImageToCanvas,
      beginFlowNodeDragDuplicateTracking: interactions.commands.beginNodeDragDuplicateTracking,
      finishFlowNodeDragDuplicateTracking: interactions.commands.finishNodeDragDuplicateTracking,
      getFlowNodeUiStateCached: interactions.commands.getNodeUiState,
    },
    flowPane: {
      connectionAdapter: editing.flowPane.connectionAdapter,
    },
    interactionPort: interactions.runtimePort,
    recoveryPort: editing.recoveryPort,
    imageWorkflowPort: {
      nodes: board.flowPane.nodes,
      currentBoardId: board.boardSession.currentBoardId,
      getImageNodeDisplayPayload: editing.workflowPort.getImageNodeDisplayPayload,
      getImageNodeSnapshot: editing.workflowPort.getImageNodeSnapshot,
      updateSnapshotNode: editing.workflowPort.updateSnapshotNode,
      createPendingRetouchNode: editing.workflowPort.createPendingRetouchNode,
      applyImageFileToImageNode: editing.workflowPort.applyImageFileToImageNode,
      removeNode: editing.workflowPort.deleteNode,
      connectionAdapter: editing.flowPane.connectionAdapter,
      uploadImageToBoard: editing.workflowPort.uploadImageToBoard,
      clearSelectedFlowElements: interactions.commands.clearSelection,
      startCanvasInteraction: interactions.runtimePort.startInteraction,
      stopCanvasInteraction: interactions.runtimePort.stopInteraction,
      isCanvasInteractionLocked: interactions.viewModel.isInteractionLocked,
      getMediaInputImages: editing.workflowPort.getMediaInputImages,
      resolvePromptInputImageNodeIds: editing.workflowPort.resolvePromptInputImageNodeIds,
      updateSnapshotNodeParam: editing.workflowPort.updateNodeParams,
      markNodeExecutionDirty: editing.workflowPort.markNodeExecutionDirty,
    },
    executionPort: {
      refreshNodeInternals: editing.commands.scheduleNodeInternalsRefresh,
    },
  }
}

function useExecutionEpoch(executionCore: PendingImageExecutionCore) {
  const epoch = ref(executionCore.snapshot().version)
  const unsubscribe = executionCore.subscribe((snapshot) => {
    epoch.value = snapshot.version
  })
  onScopeDispose(unsubscribe)
  return epoch
}
