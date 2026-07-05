import type { useCanvasWorkspaceRuntime as useLegacyCanvasWorkspaceRuntime } from '../composables/useCanvasWorkspaceRuntime'

export type { UseCanvasWorkspaceRuntimeOptions } from '../composables/useCanvasWorkspaceRuntime'

type LegacyCanvasWorkspaceRuntime = ReturnType<typeof useLegacyCanvasWorkspaceRuntime>

export type CanvasWorkspaceRuntimeViewModel = {
  boardChrome: Pick<
    LegacyCanvasWorkspaceRuntime['boardChrome'],
    | 'currentBoardId'
    | 'isBoardMenuOpen'
    | 'isRenamingBoard'
    | 'boardNameDraft'
    | 'currentBoardName'
    | 'boardOptions'
  >
  flowPane: Pick<LegacyCanvasWorkspaceRuntime['flowPane'], 'nodes' | 'edges'>
  persistence: Pick<LegacyCanvasWorkspaceRuntime['persistence'], 'history'>
  viewportControls: Pick<LegacyCanvasWorkspaceRuntime['viewportControls'], 'zoomLevel'>
  graphEditing: Pick<
    LegacyCanvasWorkspaceRuntime['graphEditing'],
    | 'graphAdapter'
    | 'hasSelectedFlowNodes'
    | 'isFlowInteractionActive'
    | 'isFlowViewportPanInteractionActive'
    | 'isFlowInteractionLocked'
    | 'nodeContextMenu'
    | 'canvasNodeMenuItems'
  >
}

export type CanvasWorkspaceRuntimeCommands = {
  boardSession: Pick<
    LegacyCanvasWorkspaceRuntime['boardChrome'],
    | 'setBoardNameInputRef'
    | 'refreshBoardMetas'
    | 'toggleBoardMenu'
    | 'startBoardRename'
    | 'cancelBoardRename'
    | 'closeBoardHeaderEditing'
    | 'onConfirmBoardRename'
    | 'onSelectBoard'
    | 'onNavigateRequest'
    | 'onLogoutRequest'
  >
  flowPane: Pick<
    LegacyCanvasWorkspaceRuntime['flowPane'],
    | 'onNodesChange'
    | 'onEdgesChange'
    | 'isValidConnection'
    | 'onConnect'
    | 'onEdgeUpdate'
    | 'nodeDragTransactions'
  >
  persistence: Pick<
    LegacyCanvasWorkspaceRuntime['persistence'],
    'flushAutosave' | 'loadCanvasBoardAndProbe' | 'reattachPendingImageResults'
  >
  viewportControls: Omit<LegacyCanvasWorkspaceRuntime['viewportControls'], 'zoomLevel'>
  graphEditing: Pick<
    LegacyCanvasWorkspaceRuntime['graphEditing'],
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
  >
  imageTools: Pick<LegacyCanvasWorkspaceRuntime, 'createImageTools'>
  imageWorkflow: Pick<
    LegacyCanvasWorkspaceRuntime['imageWorkflow'],
    'runImageDirectAction' | 'runCanvasExtendAction'
  > &
    Pick<
      LegacyCanvasWorkspaceRuntime,
      | 'createImageActionHandlers'
      | 'createPromptGenerationHandlers'
      | 'createCanvasExtendOverlay'
    >
  nodeInteraction: Pick<
    LegacyCanvasWorkspaceRuntime,
    'createNodeMediaInputs' | 'createNodeHandlers'
  >
}

export type CanvasWorkspaceRuntime = {
  viewModel: CanvasWorkspaceRuntimeViewModel
  commands: CanvasWorkspaceRuntimeCommands
}
