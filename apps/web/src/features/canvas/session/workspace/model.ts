import type { Edge, EdgeChange, GraphNode, Node, NodeChange } from '@vue-flow/core'
import type { ComputedRef, Ref } from 'vue'
import type { CanvasCreatableNodeKind, CanvasNodeItemByKind } from '../../../board/ports/canvas'
import type { CanvasGroupRuntimeCommands, CanvasGroupRuntimeView } from '../graph-editing'
import type { CanvasImageQuality, CanvasVueFlowConnectionAdapter } from '../../session/flow-adapter'
import type { CanvasGraphNodeIdForKind } from '../../core/graph'
import type { CanvasImageWorkflow } from '../image-workflow'
import type {
  CanvasGraphEditingNodeInteractions,
  CanvasGraphEditingNodeInteractionsOptions,
} from '../graph-editing'
import type { NodeUiState, PromptNodeUiState } from '../../core/nodes/canvasNodeUi.ts'
import type { CanvasLibraryImportDescriptor } from '../../../library'

export type CanvasRendererSelectableElement = (Node | Edge) & Partial<Pick<GraphNode, 'selected'>>

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

type CanvasWorkspaceRuntimeViewModel = {
  boardChrome: {
    currentBoardId: ComputedRef<string | null>
    isBoardMenuOpen: Ref<boolean>
    isRenamingBoard: Ref<boolean>
    boardNameDraft: Ref<string>
    isRenamePending: Ref<boolean>
    catalogRecovery: Ref<CanvasWorkspaceCatalogRecoveryState>
    isBoardArchived: Ref<boolean>
    isRestoringArchivedBoard: Ref<boolean>
    currentBoardName: ComputedRef<string>
    boardOptions: ComputedRef<Array<{ id: string; name: string }>>
  }
  flowPane: {
    nodes: Ref<Node[]>
    edges: Ref<Edge[]>
    renderEdges: ComputedRef<Edge[]>
  }
  persistence: {
    history: CanvasWorkspaceHistory
  }
  viewportControls: {
    zoomLevel: ComputedRef<number>
    imageQuality: CanvasImageQuality
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

type CanvasWorkspaceRuntimeCommands = {
  boardSession: {
    setBoardNameInputRef: (element: HTMLElement | null) => void
    toggleBoardMenu: () => void
    startBoardRename: () => void
    cancelBoardRename: () => void
    closeBoardHeaderEditing: () => void
    onConfirmBoardRename: () => Promise<void>
    reloadCatalog: () => Promise<void>
    restoreArchivedBoard: () => Promise<boolean>
    onSelectBoard: (boardId: string) => void
    onNavigateRequest: (path: string) => Promise<void>
    onLogoutRequest: () => Promise<void>
  }
  flowPane: {
    onNodesChange: (changes: NodeChange[]) => void
    onEdgesChange: (changes: EdgeChange[]) => void
    reconcileRendererSelection: (elements: CanvasRendererSelectableElement[]) => void
    connectionAdapter: CanvasVueFlowConnectionAdapter
    nodeDragTransactions: {
      begin: (nodeIds: readonly string[]) => void
      commit: () => void
      /** Save-to-library outcome: ends the drag with no position commit. */
      finishAsLibrarySave: () => readonly string[] | null
      isActive: () => boolean
    }
  }
  persistence: {
    flushAutosave: (options?: { isSilent?: boolean }) => Promise<void>
    recoverBoardSession: () => Promise<void>
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
  }
  graphEditing: {
    createNodeAtPosition: <T extends CanvasCreatableNodeKind>(
      kind: T,
      x: number,
      y: number,
      initialSnapshotPatch?: Partial<CanvasNodeItemByKind[T]>,
    ) => CanvasGraphNodeIdForKind<T> | null
    libraryImport: {
      add: (
        candidate: CanvasLibraryImportDescriptor,
        center: { x: number; y: number },
      ) => Promise<CanvasGraphNodeIdForKind<'image'> | null>
    }
    imageImport: {
      retry: (nodeId: string) => Promise<boolean>
    }
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
