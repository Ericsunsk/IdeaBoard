import type { Edge, EdgeChange, Node, NodeChange } from '@vue-flow/core'
import type { ComputedRef, Ref } from 'vue'
import type { CanvasCreatableNodeKind, CanvasNodeItemByKind } from '../../canvas-board/domain'
import type {
  CanvasGroupRuntimeCommands,
  CanvasGroupRuntimeView,
} from '../composables/useCanvasGroupRuntime'
import type { CanvasFlowPaneConnectionAdapter } from '../flow-pane'
import type { CanvasGraphNodeIdForKind } from '../graph-runtime'
import type { CanvasImageWorkflow } from '../image-workflow'
import type {
  CanvasGraphEditingNodeInteractions,
  CanvasGraphEditingNodeInteractionsOptions,
} from '../graph-editing'
import type { NodeUiState, PromptNodeUiState } from '../types/canvasNodeUi'

type CanvasWorkspaceCatalogRecoveryState =
  { status: 'idle' } | { status: 'reload-required' | 'reloading'; boardId: string }

type CanvasWorkspaceFlowViewport = {
  x: number
  y: number
  zoom: number
}

type CanvasWorkspaceFlowMovePayload = {
  flowTransform?: Partial<CanvasWorkspaceFlowViewport>
}

type CanvasWorkspaceMiniMapCenterRequest = {
  position: { x: number; y: number }
}

type CanvasWorkspaceMiniMapDragDelta = {
  x: number
  y: number
}

type CanvasWorkspaceHistory = {
  undo: () => void
  redo: () => void
}

export type CanvasWorkspaceRuntimeViewModel = {
  boardChrome: {
    currentBoardId: ComputedRef<string | null>
    isBoardMenuOpen: Ref<boolean>
    isRenamingBoard: Ref<boolean>
    boardNameDraft: Ref<string>
    isRenamePending: Ref<boolean>
    catalogRecovery: Ref<CanvasWorkspaceCatalogRecoveryState>
    currentBoardName: ComputedRef<string>
    boardOptions: ComputedRef<Array<{ id: string; name: string }>>
  }
  flowPane: {
    nodes: Ref<Node[]>
    edges: Ref<Edge[]>
  }
  persistence: {
    history: CanvasWorkspaceHistory
  }
  viewportControls: {
    zoomLevel: ComputedRef<number>
  }
  graphEditing: {
    hasSelectedFlowNodes: ComputedRef<boolean>
    isFlowInteractionActive: ComputedRef<boolean>
    isFlowViewportPanInteractionActive: ComputedRef<boolean>
    isFlowInteractionLocked: ComputedRef<boolean>
    grouping: CanvasGroupRuntimeView
  }
  imageWorkflow: CanvasImageWorkflow['viewModel']
}

export type CanvasWorkspaceRuntimeCommands = {
  boardSession: {
    setBoardNameInputRef: (element: HTMLElement | null) => void
    toggleBoardMenu: () => void
    startBoardRename: () => void
    cancelBoardRename: () => void
    closeBoardHeaderEditing: () => void
    onConfirmBoardRename: () => Promise<void>
    reloadCatalog: () => Promise<void>
    onSelectBoard: (boardId: string) => void
    onNavigateRequest: (path: string) => Promise<void>
    onLogoutRequest: () => Promise<void>
  }
  flowPane: {
    onNodesChange: (changes: NodeChange[]) => void
    onEdgesChange: (changes: EdgeChange[]) => void
    connectionAdapter: CanvasFlowPaneConnectionAdapter
    nodeDragTransactions: {
      begin: () => void
      commit: () => void
    }
  }
  persistence: {
    flushAutosave: (options?: { silent?: boolean }) => Promise<void>
    loadCanvasBoardAndProbe: () => Promise<void>
    reattachPendingImageResults: () => Promise<void> | void
  }
  viewportControls: {
    onZoomIn: () => void
    onZoomOut: () => void
    onPanViewport: (delta: { x: number; y: number }) => void
    onMiniMapCenterRequested: (payload: CanvasWorkspaceMiniMapCenterRequest) => void
    onMiniMapDragStart: () => void
    onMiniMapDragMove: (delta: CanvasWorkspaceMiniMapDragDelta) => void
    onMiniMapDragEnd: () => void
    onViewportChangeEnd: (viewport?: CanvasWorkspaceFlowViewport) => void
    onFlowMoveStart: () => void
    onFlowMove: (payload: CanvasWorkspaceFlowMovePayload) => void
    onFlowMoveEnd: (payload: CanvasWorkspaceFlowMovePayload) => void
    onNodeDragStart: () => void
    onNodeDragStop: () => void
    flushViewportPersistence: () => void
    loadCanvasBoardAndProbe: () => Promise<void>
  }
  graphEditing: {
    createNodeAtPosition: <T extends CanvasCreatableNodeKind>(
      kind: T,
      x: number,
      y: number,
      initialSnapshotPatch?: Partial<CanvasNodeItemByKind[T]>,
    ) => CanvasGraphNodeIdForKind<T> | null
    onExportSelectedImages: () => void
    onFlowMediaInputImageDrop: (nodeId: string, event: DragEvent) => Promise<void>
    onFlowMediaInputImageOpen: (nodeId: string) => void
    onFlowMediaInputImagePreview: (image: { url: string; ratio: number }) => void
    onFlowMediaInputImageRemove: (nodeId: string, imageId: string) => void
    onCanvasDrop: (event: DragEvent) => Promise<void>
    onCanvasDragOver: (event: DragEvent) => void
    onFlowImageOpenPreview: (nodeId: string) => void
    onFlowImageAppendToCanvas: (nodeId: string) => void
    beginFlowNodeDragDuplicateTracking: (isDuplicateMode: boolean) => void
    finishFlowNodeDragDuplicateTracking: (isDuplicateMode: boolean) => void
    getFlowNodeUiStateCached: (nodeId: string) => NodeUiState | PromptNodeUiState
    createNodeInteractions: (
      options: CanvasGraphEditingNodeInteractionsOptions,
    ) => CanvasGraphEditingNodeInteractions
    grouping: CanvasGroupRuntimeCommands
  }
  imageWorkflow: CanvasImageWorkflow['commands']
}

export type CanvasWorkspaceRuntime = {
  viewModel: CanvasWorkspaceRuntimeViewModel
  commands: CanvasWorkspaceRuntimeCommands
}
