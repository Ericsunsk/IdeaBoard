import type { createCanvasWorkspaceRuntimeParts } from './internal/createCanvasWorkspaceRuntimeParts'
import type {
  CanvasGroupRuntimeCommands,
  CanvasGroupRuntimeView,
} from '../composables/useCanvasGroupRuntime'

type CanvasWorkspaceRuntimeParts = ReturnType<typeof createCanvasWorkspaceRuntimeParts>

export type CanvasWorkspaceRuntimeViewModel = {
  boardChrome: Pick<
    CanvasWorkspaceRuntimeParts['boardChrome'],
    | 'currentBoardId'
    | 'isBoardMenuOpen'
    | 'isRenamingBoard'
    | 'boardNameDraft'
    | 'boardRenameRecovery'
    | 'currentBoardName'
    | 'boardOptions'
  >
  flowPane: Pick<CanvasWorkspaceRuntimeParts['flowPane'], 'nodes' | 'edges'>
  persistence: Pick<CanvasWorkspaceRuntimeParts['persistence'], 'history'>
  viewportControls: Pick<CanvasWorkspaceRuntimeParts['viewportControls'], 'zoomLevel'>
  graphEditing: Pick<
    CanvasWorkspaceRuntimeParts['graphEditing'],
    | 'graphAdapter'
    | 'hasSelectedFlowNodes'
    | 'isFlowInteractionActive'
    | 'isFlowViewportPanInteractionActive'
    | 'isFlowInteractionLocked'
    | 'nodeContextMenu'
    | 'canvasNodeMenuItems'
  > & {
    grouping: CanvasGroupRuntimeView
  }
}

export type CanvasWorkspaceRuntimeCommands = {
  boardSession: Pick<
    CanvasWorkspaceRuntimeParts['boardChrome'],
    | 'setBoardNameInputRef'
    | 'refreshBoardMetas'
    | 'toggleBoardMenu'
    | 'startBoardRename'
    | 'cancelBoardRename'
    | 'closeBoardHeaderEditing'
    | 'onConfirmBoardRename'
    | 'retryBoardRename'
    | 'rollbackBoardRename'
    | 'onSelectBoard'
    | 'onNavigateRequest'
    | 'onLogoutRequest'
  >
  flowPane: Pick<
    CanvasWorkspaceRuntimeParts['flowPane'],
    | 'onNodesChange'
    | 'onEdgesChange'
    | 'connectionAdapter'
    | 'nodeDragTransactions'
  >
  persistence: Pick<
    CanvasWorkspaceRuntimeParts['persistence'],
    'flushAutosave' | 'loadCanvasBoardAndProbe' | 'reattachPendingImageResults'
  >
  viewportControls: Omit<CanvasWorkspaceRuntimeParts['viewportControls'], 'zoomLevel'>
  graphEditing: Pick<
    CanvasWorkspaceRuntimeParts['graphEditing'],
    | 'scheduleFlowNodeInternalsRefresh'
    | 'createNodeAtPosition'
    | 'onExportSelectedImages'
    | 'closeNodeContextMenu'
    | 'openNodeContextMenu'
    | 'onFlowMediaInputImageDrop'
    | 'onFlowMediaInputImageOpen'
    | 'onFlowMediaInputImagePreview'
    | 'onFlowMediaInputImageRemove'
    | 'onCanvasDrop'
    | 'onCanvasDragOver'
    | 'onFlowImageOpenPreview'
    | 'onFlowImageAppendToCanvas'
    | 'beginFlowNodeDragDuplicateTracking'
    | 'finishFlowNodeDragDuplicateTracking'
    | 'getFlowNodeUiStateCached'
  > & {
    grouping: CanvasGroupRuntimeCommands
  }
  imageWorkflow: Pick<
    CanvasWorkspaceRuntimeParts['imageWorkflow'],
    | 'isGenerationAvailable'
    | 'runImageDirectAction'
    | 'runCanvasExtendAction'
    | 'createImageTools'
    | 'createImageActionHandlers'
    | 'createPromptGenerationHandlers'
    | 'createCanvasExtendOverlay'
  >
  nodeInteraction: Pick<CanvasWorkspaceRuntimeParts, 'createNodeMediaInputs' | 'createNodeHandlers'>
}

export type CanvasWorkspaceRuntime = {
  viewModel: CanvasWorkspaceRuntimeViewModel
  commands: CanvasWorkspaceRuntimeCommands
}
