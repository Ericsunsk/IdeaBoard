import type { ImageNodeItem } from '../../../canvas-board/domain'
import type { useCanvasBoardRuntime } from '../../composables/useCanvasBoardRuntime'
import type { CanvasFeedback } from '../../feedback/useCanvasFeedback'
import type { CanvasFlowPaneAdapter } from '../../flow-pane'
import {
  useCanvasGraphEditing,
  useCanvasGraphInteractions,
  type CanvasGraphEditingBrowserAdapter,
  type CanvasGraphEditingUploadAdapter,
  type CanvasGraphInteractionBrowserAdapter,
} from '../../graph-editing'
import type { PendingImageExecutionCore } from '../../execution/pending-execution'
import type { LibraryAssetMaterializer } from '../../../library/libraryResource'

type CanvasWorkspaceBoardRuntime = ReturnType<typeof useCanvasBoardRuntime>

type CreateCanvasWorkspaceGraphRuntimeOptions = {
  board: CanvasWorkspaceBoardRuntime
  flowPaneAdapter: CanvasFlowPaneAdapter
  upload: CanvasGraphEditingUploadAdapter
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
  const editing = useCanvasGraphEditing({
    nodes: board.flowPane.nodes,
    edges: board.flowPane.edges,
    boardBaseSnapshot: board.boardSession.boardBaseSnapshot,
    currentBoardId: board.boardSession.currentBoardId,
    graphAdapter: board.graphEditing.adapter,
    deleteNode: board.graphEditing.deleteNode,
    updateNodeInternals: flowPaneAdapter.updateNodeInternals,
    screenToFlowCoordinate: flowPaneAdapter.screenToFlowCoordinate,
    browser: options.editingBrowser,
    upload: options.upload,
    materializeLibraryAsset: options.materializeLibraryAsset,
    readLibraryImportAttachment: options.executionCore.currentAttachment,
    selectOnlyNode: flowPaneAdapter.selectOnlyNode,
    preview: { openImageUrl: options.preview.openImageUrl },
    feedback: options.feedback,
  })

  const interactions = useCanvasGraphInteractions({
    nodes: board.flowPane.nodes,
    edges: board.flowPane.edges,
    graphAdapter: board.graphEditing.adapter,
    createCurrentCanvasGraphStore: board.graphEditing.createCurrentCanvasGraphStore,
    selection: {
      selectedElements: flowPaneAdapter.getSelectedElements,
      clear: flowPaneAdapter.removeSelectedElements,
    },
    browser: options.interactionBrowser,
    preview: { openNodeImage: options.preview.openNodeImage },
    feedback: options.feedback,
    isNodeExecutionActive: (nodeId) =>
      options.executionCore.captureActiveLeases([nodeId as `image-${number}`]).length > 0,
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
      createImageNodeFromFile: editing.workflowPort.createImageNodeFromFile,
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
