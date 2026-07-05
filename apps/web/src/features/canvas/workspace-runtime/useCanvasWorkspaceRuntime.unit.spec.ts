import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useCanvasWorkspaceRuntime } from './useCanvasWorkspaceRuntime'

const mocks = vi.hoisted(() => ({
  useLegacyCanvasWorkspaceRuntime: vi.fn(),
}))

vi.mock('../composables/useCanvasWorkspaceRuntime', () => ({
  useCanvasWorkspaceRuntime: mocks.useLegacyCanvasWorkspaceRuntime,
}))

function createLegacyRuntime() {
  return {
    boardChrome: {
      currentBoardId: ref('board-a'),
      isBoardMenuOpen: ref(false),
      isRenamingBoard: ref(false),
      boardNameDraft: ref('Board A'),
      currentBoardName: ref('Board A'),
      boardOptions: ref([]),
      setBoardNameInputRef: vi.fn(),
      refreshBoardMetas: vi.fn(),
      toggleBoardMenu: vi.fn(),
      startBoardRename: vi.fn(),
      cancelBoardRename: vi.fn(),
      closeBoardHeaderEditing: vi.fn(),
      onConfirmBoardRename: vi.fn(),
      onSelectBoard: vi.fn(),
      onNavigateRequest: vi.fn(),
      onLogoutRequest: vi.fn(),
    },
    flowPane: {
      nodes: ref([]),
      edges: ref([]),
      onNodesChange: vi.fn(),
      onEdgesChange: vi.fn(),
      isValidConnection: vi.fn(() => true),
      onConnect: vi.fn(),
      onEdgeUpdate: vi.fn(),
      nodeDragTransactions: {
        begin: vi.fn(),
        commit: vi.fn(),
      },
    },
    persistence: {
      flushAutosave: vi.fn(),
      history: {},
      loadCanvasBoardAndProbe: vi.fn(),
      reattachPendingImageResults: vi.fn(),
    },
    viewportControls: {
      zoomLevel: ref(100),
      onZoomIn: vi.fn(),
      onZoomOut: vi.fn(),
      onPanViewport: vi.fn(),
      onMiniMapCenterRequested: vi.fn(),
      onMiniMapDragStart: vi.fn(),
      onMiniMapDragMove: vi.fn(),
      onMiniMapDragEnd: vi.fn(),
      onViewportChangeEnd: vi.fn(),
      onFlowMoveStart: vi.fn(),
      onFlowMove: vi.fn(),
      onFlowMoveEnd: vi.fn(),
      onNodeDragStart: vi.fn(),
      onNodeDragStop: vi.fn(),
      loadCanvasBoardAndProbe: vi.fn(),
    },
    graphEditing: {
      graphAdapter: {},
      scheduleFlowNodeInternalsRefresh: vi.fn(),
      createNodeAtPosition: vi.fn(),
      hasSelectedFlowNodes: ref(false),
      onExportSelectedImages: vi.fn(),
      isFlowInteractionActive: ref(false),
      isFlowViewportPanInteractionActive: ref(false),
      isFlowInteractionLocked: ref(false),
      nodeContextMenu: ref({ visible: false }),
      canvasNodeMenuItems: ref([]),
      closeNodeContextMenu: vi.fn(),
      openNodeContextMenu: vi.fn(),
      onFlowMediaInputImageDrop: vi.fn(),
      onFlowMediaInputImageOpen: vi.fn(),
      onFlowMediaInputImagePreview: vi.fn(),
      onFlowMediaInputImageRemove: vi.fn(),
      onCanvasDrop: vi.fn(),
      onCanvasDragOver: vi.fn(),
      onFlowImageOpenPreview: vi.fn(),
      onFlowImageAppendToCanvas: vi.fn(),
      beginFlowNodeDragDuplicateTracking: vi.fn(),
      finishFlowNodeDragDuplicateTracking: vi.fn(),
      getFlowNodeUiStateCached: vi.fn(),
    },
    imageWorkflow: {
      runImageDirectAction: vi.fn(),
      runCanvasExtendAction: vi.fn(),
    },
    createImageTools: vi.fn(),
    createImageActionHandlers: vi.fn(),
    createPromptGenerationHandlers: vi.fn(),
    createCanvasExtendOverlay: vi.fn(),
    createNodeMediaInputs: vi.fn(),
    createNodeHandlers: vi.fn(),
  }
}

describe('workspace-runtime useCanvasWorkspaceRuntime', () => {
  it('exposes only viewModel and commands at the public seam', () => {
    const legacyRuntime = createLegacyRuntime()
    mocks.useLegacyCanvasWorkspaceRuntime.mockReturnValue(legacyRuntime)

    const workspace = useCanvasWorkspaceRuntime({} as never)

    expect(Object.keys(workspace).sort()).toEqual(['commands', 'viewModel'])
    expect(workspace.viewModel.flowPane).toEqual({
      nodes: legacyRuntime.flowPane.nodes,
      edges: legacyRuntime.flowPane.edges,
    })
    expect(workspace.commands.flowPane).toEqual({
      onNodesChange: legacyRuntime.flowPane.onNodesChange,
      onEdgesChange: legacyRuntime.flowPane.onEdgesChange,
      isValidConnection: legacyRuntime.flowPane.isValidConnection,
      onConnect: legacyRuntime.flowPane.onConnect,
      onEdgeUpdate: legacyRuntime.flowPane.onEdgeUpdate,
      nodeDragTransactions: legacyRuntime.flowPane.nodeDragTransactions,
    })
    expect(workspace.commands.boardSession.onNavigateRequest).toBe(
      legacyRuntime.boardChrome.onNavigateRequest,
    )
    expect(workspace.commands.imageWorkflow.runImageDirectAction).toBe(
      legacyRuntime.imageWorkflow.runImageDirectAction,
    )
    expect(workspace.commands.imageTools.createImageTools).toBe(
      legacyRuntime.createImageTools,
    )
  })
})
