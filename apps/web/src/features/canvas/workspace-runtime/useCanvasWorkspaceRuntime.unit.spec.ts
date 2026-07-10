import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useCanvasWorkspaceRuntime } from './useCanvasWorkspaceRuntime'

const mocks = vi.hoisted(() => ({
  createCanvasWorkspaceRuntimeParts: vi.fn(),
}))

vi.mock('./internal/createCanvasWorkspaceRuntimeParts', () => ({
  createCanvasWorkspaceRuntimeParts: mocks.createCanvasWorkspaceRuntimeParts,
}))

function createRuntimeParts() {
  return {
    boardChrome: {
      currentBoardId: ref('board-a'),
      isBoardMenuOpen: ref(false),
      isRenamingBoard: ref(false),
      boardNameDraft: ref('Board A'),
      boardRenameRecovery: ref({ status: 'idle' as const }),
      currentBoardName: ref('Board A'),
      boardOptions: ref([]),
      setBoardNameInputRef: vi.fn(),
      refreshBoardMetas: vi.fn(),
      toggleBoardMenu: vi.fn(),
      startBoardRename: vi.fn(),
      cancelBoardRename: vi.fn(),
      closeBoardHeaderEditing: vi.fn(),
      onConfirmBoardRename: vi.fn(),
      retryBoardRename: vi.fn(),
      rollbackBoardRename: vi.fn(),
      onSelectBoard: vi.fn(),
      onNavigateRequest: vi.fn(),
      onLogoutRequest: vi.fn(),
    },
    flowPane: {
      nodes: ref([]),
      edges: ref([]),
      onNodesChange: vi.fn(),
      onEdgesChange: vi.fn(),
      connectNodes: vi.fn(),
      vueFlowConnectionBindings: {
        isValidConnection: vi.fn(() => true),
        onConnect: vi.fn(),
        onEdgeUpdate: vi.fn(),
      },
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
      flushViewportPersistence: vi.fn(),
      loadCanvasBoardAndProbe: vi.fn(),
    },
    graphEditing: {
      graphAdapter: {},
      scheduleFlowNodeInternalsRefresh: vi.fn(),
      createNodeAtPosition: vi.fn(),
      grouping: {
        view: {
          groupFrames: ref([]),
          selectionGroupAction: ref(null),
        },
        commands: {
          selectGroupAtPoint: vi.fn(),
          handlePaneClick: vi.fn(),
          handleNodeClick: vi.fn(),
          groupSelectedNodes: vi.fn(),
          ungroupSelectedGroup: vi.fn(),
          deleteSelectedGroup: vi.fn(),
          groupDrag: {
            begin: vi.fn(),
            move: vi.fn(),
            commit: vi.fn(),
            cancel: vi.fn(),
          },
          groupResize: {
            begin: vi.fn(),
            move: vi.fn(),
            commit: vi.fn(),
            cancel: vi.fn(),
          },
        },
        internal: {
          selectedGroupId: ref(null),
          selectedGroupCoveredExternalNodeIds: ref([]),
          projectCurrentGroupNodeLayers: vi.fn(),
          handleCommittedGroupsChanged: vi.fn(),
          resetTransientSelectionState: vi.fn(),
          clearPendingGroupJoin: vi.fn(),
          onNodeDragStart: vi.fn(),
          onBeforeNodesChange: vi.fn(),
          onBeforeEdgesChange: vi.fn(),
          resolveRemovedFlowNodeChanges: vi.fn(),
          applyDraggedNodeMembershipChanges: vi.fn(),
        },
      },
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
      isGenerationAvailable: vi.fn(() => true),
      runImageDirectAction: vi.fn(),
      runCanvasExtendAction: vi.fn(),
      createImageTools: vi.fn(),
      createImageActionHandlers: vi.fn(),
      createPromptGenerationHandlers: vi.fn(),
      createCanvasExtendOverlay: vi.fn(),
    },
    createNodeMediaInputs: vi.fn(),
    createNodeHandlers: vi.fn(),
  }
}

describe('workspace-runtime useCanvasWorkspaceRuntime', () => {
  it('exposes only viewModel and commands at the public seam', () => {
    const runtimeParts = createRuntimeParts()
    mocks.createCanvasWorkspaceRuntimeParts.mockReturnValue(runtimeParts)

    const workspace = useCanvasWorkspaceRuntime({} as never)

    expect(Object.keys(workspace).sort()).toEqual(['commands', 'viewModel'])
    expect(workspace.viewModel.flowPane).toEqual({
      nodes: runtimeParts.flowPane.nodes,
      edges: runtimeParts.flowPane.edges,
    })
    expect(workspace.commands.flowPane).toEqual({
      onNodesChange: runtimeParts.flowPane.onNodesChange,
      onEdgesChange: runtimeParts.flowPane.onEdgesChange,
      connectNodes: runtimeParts.flowPane.connectNodes,
      vueFlowConnectionBindings: runtimeParts.flowPane.vueFlowConnectionBindings,
      nodeDragTransactions: runtimeParts.flowPane.nodeDragTransactions,
    })
    expect('isValidConnection' in workspace.commands.flowPane).toBe(false)
    expect('onConnect' in workspace.commands.flowPane).toBe(false)
    expect('onEdgeUpdate' in workspace.commands.flowPane).toBe(false)
    expect(workspace.commands.boardSession.onNavigateRequest).toBe(
      runtimeParts.boardChrome.onNavigateRequest,
    )
    expect(workspace.viewModel.boardChrome.boardRenameRecovery).toBe(
      runtimeParts.boardChrome.boardRenameRecovery,
    )
    expect(workspace.commands.boardSession.retryBoardRename).toBe(
      runtimeParts.boardChrome.retryBoardRename,
    )
    expect(workspace.commands.boardSession.rollbackBoardRename).toBe(
      runtimeParts.boardChrome.rollbackBoardRename,
    )
    expect(workspace.viewModel.persistence).toEqual({
      history: runtimeParts.persistence.history,
    })
    expect('isDirty' in workspace.viewModel.persistence).toBe(false)
    expect('isSavingBoard' in workspace.viewModel.persistence).toBe(false)
    expect('saveStatus' in workspace.viewModel.persistence).toBe(false)
    expect(workspace.viewModel.graphEditing.grouping).toEqual({
      groupFrames: runtimeParts.graphEditing.grouping.view.groupFrames,
      selectionGroupAction: runtimeParts.graphEditing.grouping.view.selectionGroupAction,
    })
    expect(workspace.commands.graphEditing.grouping).toBe(runtimeParts.graphEditing.grouping.commands)
    expect('groupFrames' in workspace.viewModel.graphEditing).toBe(false)
    expect('groupSelectedNodes' in workspace.commands.graphEditing).toBe(false)
    expect('selectedGroupId' in workspace.viewModel.graphEditing.grouping).toBe(false)
    expect('clearPendingGroupJoin' in workspace.commands.graphEditing.grouping).toBe(false)
    expect('resolveRemovedFlowNodeChanges' in workspace.commands.graphEditing.grouping).toBe(false)
    expect(workspace.commands.graphEditing.scheduleFlowNodeInternalsRefresh).toBe(
      runtimeParts.graphEditing.scheduleFlowNodeInternalsRefresh,
    )
    expect(workspace.commands.imageWorkflow.runImageDirectAction).toBe(
      runtimeParts.imageWorkflow.runImageDirectAction,
    )
    expect(workspace.commands.imageWorkflow.isGenerationAvailable).toBe(
      runtimeParts.imageWorkflow.isGenerationAvailable,
    )
    expect(workspace.commands.imageWorkflow.createImageTools).toBe(
      runtimeParts.imageWorkflow.createImageTools,
    )
    expect('imageTools' in workspace.commands).toBe(false)
  })
})
