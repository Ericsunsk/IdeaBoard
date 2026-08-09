import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import {
  createCanvasWorkspaceRuntime,
  createCanvasWorkspaceRuntimeParts,
} from './createCanvasWorkspaceRuntimeParts'

const mocks = vi.hoisted(() => ({
  feedback: {
    boardSession: {
      warning: vi.fn(),
      saveSucceeded: vi.fn(),
      saveFailed: vi.fn(),
    },
    graph: {
      operationSucceeded: vi.fn(),
      operationWarning: vi.fn(),
      operationLoading: vi.fn(),
    },
    taskExecution: {
      succeeded: vi.fn(),
      warning: vi.fn(),
      insufficientCredits: vi.fn(),
    },
    imageTools: {
      succeeded: vi.fn(),
      warning: vi.fn(),
      actionUnavailable: vi.fn(),
    },
  },
  useCanvasBoardRuntime: vi.fn(),
  useCanvasGraphEditing: vi.fn(),
  useCanvasViewportRuntime: vi.fn(),
  useCanvasImageWorkflow: vi.fn(),
}))

vi.mock('../../feedback/useCanvasFeedback', () => ({
  useCanvasFeedback: () => mocks.feedback,
}))

vi.mock('../../composables/useCanvasBoardRuntime', () => ({
  useCanvasBoardRuntime: mocks.useCanvasBoardRuntime,
}))

vi.mock('../../graph-editing', () => ({
  useCanvasGraphEditing: mocks.useCanvasGraphEditing,
}))

vi.mock('../../composables/useCanvasViewportRuntime', () => ({
  useCanvasViewportRuntime: mocks.useCanvasViewportRuntime,
}))

vi.mock('../../image-workflow', () => ({
  useCanvasImageWorkflow: mocks.useCanvasImageWorkflow,
}))

function createBoardRuntime() {
  const nodes = ref([])
  const edges = ref([])
  const currentBoardId = ref('board-a')
  const flushAutosave = vi.fn()
  const nodeDragTransactions = {
    begin: vi.fn(),
    commit: vi.fn(),
  }
  const grouping = {
    groupFrames: ref([]),
  }
  return {
    boardSession: {
      boardBaseSnapshot: ref({ groups: [] }),
      currentBoardId,
      isDirty: ref(true),
      isSavingBoard: ref(false),
      saveStatus: ref('retry' as const),
      loadInitialOrRouteBoard: vi.fn(),
      flushAutosave,
      history: {},
    },
    flowPane: {
      nodes,
      edges,
      onNodesChange: vi.fn(),
      onEdgesChange: vi.fn(),
      nodeDragTransactions,
    },
    graphEditing: {
      adapter: {},
      createCurrentCanvasGraphStore: vi.fn(),
      deleteNode: vi.fn(),
      grouping,
    },
    boardChrome: {
      closeBoardHeaderEditing: vi.fn(),
      isRenamePending: ref(false),
      catalogRecovery: ref({ status: 'idle' as const }),
      reloadCatalog: vi.fn(),
      onNavigateRequest: vi.fn(),
      onLogoutRequest: vi.fn(),
    },
  }
}

function createGraphRuntime() {
  return {
    scheduleFlowNodeInternalsRefresh: vi.fn(),
    getImageNodeDisplayPayload: vi.fn(),
    getImageNodeSnapshot: vi.fn(),
    createImageNodeFromFile: vi.fn(),
    connectionAdapter: {
      vueFlowBindings: {
        isValidConnection: vi.fn(() => true),
        onConnect: vi.fn(),
        onEdgeUpdate: vi.fn(),
      },
      connectIntent: vi.fn(),
    },
    uploadImageToBoard: vi.fn(),
    updateNodeParams: vi.fn(),
    markFlowExecutionNodeDirty: vi.fn(),
    clearSelectedFlowElementsSync: vi.fn(),
    startFlowInteraction: vi.fn(),
    stopFlowInteraction: vi.fn(),
    getMediaInputImages: vi.fn(() => []),
    resolvePromptInputImageNodeIds: vi.fn(() => []),
    createNodeAtPosition: vi.fn(),
    createNodeInteractions: vi.fn(),
  }
}

function createGraphEditingRuntime(graph: ReturnType<typeof createGraphRuntime>) {
  return {
    flowPane: {
      connectionAdapter: graph.connectionAdapter,
    },
    commands: {
      scheduleNodeInternalsRefresh: graph.scheduleFlowNodeInternalsRefresh,
      createNodeAtPosition: graph.createNodeAtPosition,
      createNodeInteractions: graph.createNodeInteractions,
      mediaInputs: {
        drop: vi.fn(),
        open: vi.fn(),
        preview: vi.fn(),
        remove: vi.fn(),
      },
      canvasFiles: {
        drop: vi.fn(),
        dragOver: vi.fn(),
      },
      imageNodes: {
        openPreview: vi.fn(),
        appendToCanvas: vi.fn(),
      },
    },
    workflowPort: {
      getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
      getImageNodeSnapshot: graph.getImageNodeSnapshot,
      createImageNodeFromFile: graph.createImageNodeFromFile,
      uploadImageToBoard: graph.uploadImageToBoard,
      getMediaInputImages: graph.getMediaInputImages,
      resolvePromptInputImageNodeIds: graph.resolvePromptInputImageNodeIds,
      updateNodeParams: graph.updateNodeParams,
      markNodeExecutionDirty: graph.markFlowExecutionNodeDirty,
    },
  }
}

function createImageWorkflowRuntime() {
  return {
    runImageInpaintAction: vi.fn(),
    runImagePromptGeneration: vi.fn(),
    runImagePromptGenerationFromSource: vi.fn(),
    runTextPromptGeneration: vi.fn(),
    runFusionPromptGeneration: vi.fn(),
    runImageDirectAction: vi.fn(),
    runCanvasExtendAction: vi.fn(),
    isGenerationAvailable: vi.fn(() => true),
    isNodeDirectActionRunning: vi.fn(),
    reattachPendingImageResults: vi.fn(),
    createImageTools: vi.fn(),
    createImageActionHandlers: vi.fn(),
    createCanvasExtendOverlay: vi.fn(),
  }
}

function createWorkspaceOptions() {
  const flowPaneAdapter = {
    viewport: ref({ x: 0, y: 0, zoom: 1 }),
    getViewport: vi.fn(() => ({ x: 0, y: 0, zoom: 1 })),
    setViewport: vi.fn(),
    setCenter: vi.fn(),
    zoomIn: vi.fn(),
    zoomOut: vi.fn(),
    panBy: vi.fn(() => true),
    updateNodeInternals: vi.fn(),
    screenToFlowCoordinate: vi.fn((point: { x: number; y: number }) => point),
    getSelectedElements: ref([]),
    removeSelectedElements: vi.fn(),
    vueFlowGraphActions: {
      setNodes: vi.fn(),
    },
  }

  return {
    route: { query: {} } as never,
    router: {} as never,
    getUserIdentity: () => ({ userId: 1, userUid: 'user-a' }),
    flowPaneAdapter,
    uploadApi: { upload: vi.fn() },
    graphEditingBrowser: {
      pickImageFile: vi.fn(async () => null),
      readDroppedFile: vi.fn(() => null),
      readDropScreenPoint: vi.fn(() => ({ x: 0, y: 0 })),
      markDropAsCopy: vi.fn(),
    },
    openNodeImagePreview: vi.fn(),
    openImageUrlPreview: vi.fn(),
    logoutAndRedirect: vi.fn(),
    startNodeDragInteraction: vi.fn(),
    stopNodeDragInteraction: vi.fn(),
    refreshFlowHandleVisibility: vi.fn(),
    openRecharge: vi.fn(),
  }
}

describe('createCanvasWorkspaceRuntimeParts', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.useCanvasViewportRuntime.mockReturnValue({
      zoomLevel: ref(100),
      flushViewportPersistence: vi.fn(),
      loadCanvasBoardAndProbe: vi.fn(),
    })
  })

  it('owns feedback selection for board, graph, task, and image tools', async () => {
    const board = createBoardRuntime()
    const graph = createGraphRuntime()
    const imageWorkflow = createImageWorkflowRuntime()
    const events: string[] = []
    const viewportRuntime = {
      zoomLevel: ref(100),
      flushViewportPersistence: vi.fn(() => events.push('viewport')),
      loadCanvasBoardAndProbe: vi.fn(),
    }
    board.boardSession.flushAutosave.mockImplementation(async () => {
      events.push('autosave')
    })
    mocks.useCanvasBoardRuntime.mockReturnValue(board)
    mocks.useCanvasGraphEditing.mockReturnValue(createGraphEditingRuntime(graph))
    mocks.useCanvasImageWorkflow.mockReturnValue(imageWorkflow)
    mocks.useCanvasViewportRuntime.mockReturnValue(viewportRuntime)

    const runtimeOptions = createWorkspaceOptions()
    const runtime = createCanvasWorkspaceRuntimeParts(runtimeOptions)

    expect(runtime.flowPane.nodes).toBe(board.flowPane.nodes)
    expect(runtime.flowPane.edges).toBe(board.flowPane.edges)
    expect(runtime.flowPane.onNodesChange).toBe(board.flowPane.onNodesChange)
    expect(runtime.flowPane.onEdgesChange).toBe(board.flowPane.onEdgesChange)
    expect(runtime.flowPane.connectionAdapter).toBe(graph.connectionAdapter)
    expect('connectNodes' in runtime.flowPane).toBe(false)
    expect('vueFlowConnectionBindings' in runtime.flowPane).toBe(false)
    expect('onConnect' in runtime.flowPane).toBe(false)
    expect('onEdgeUpdate' in runtime.flowPane).toBe(false)
    expect('isValidConnection' in runtime.flowPane).toBe(false)
    expect(runtime.flowPane.nodeDragTransactions.begin).toBe(
      board.flowPane.nodeDragTransactions.begin,
    )
    expect(runtime.boardChrome.currentBoardId).toBe(board.boardSession.currentBoardId)
    expect(runtime.boardChrome.isRenamePending).toBe(board.boardChrome.isRenamePending)
    expect(runtime.boardChrome.catalogRecovery).toBe(board.boardChrome.catalogRecovery)
    expect(runtime.boardChrome.reloadCatalog).toBe(board.boardChrome.reloadCatalog)
    expect(runtime.boardChrome.onNavigateRequest).toBe(board.boardChrome.onNavigateRequest)
    expect(runtime.boardChrome.onLogoutRequest).toBe(board.boardChrome.onLogoutRequest)
    expect('isDirty' in runtime.persistence).toBe(false)
    expect('isSavingBoard' in runtime.persistence).toBe(false)
    expect('saveStatus' in runtime.persistence).toBe(false)
    expect(runtime.persistence.flushAutosave).not.toBe(board.boardSession.flushAutosave)
    expect(runtime.persistence.reattachPendingImageResults).toBe(
      imageWorkflow.reattachPendingImageResults,
    )
    expect(runtime.graphEditing.grouping).toBe(board.graphEditing.grouping)
    expect(runtime.imageWorkflow).toBe(imageWorkflow)

    await runtime.persistence.flushAutosave({ silent: false })

    expect(events).toEqual(['viewport', 'autosave'])
    expect(board.boardSession.flushAutosave).toHaveBeenCalledWith({ silent: false })

    expect(mocks.useCanvasBoardRuntime).toHaveBeenCalledWith(
      expect.objectContaining({
        feedback: mocks.feedback.boardSession,
        vueFlowGraphActions: runtimeOptions.flowPaneAdapter.vueFlowGraphActions,
      }),
    )
    expect(mocks.useCanvasViewportRuntime).toHaveBeenCalledWith(
      expect.objectContaining({
        getUserUid: expect.any(Function),
        getViewport: runtimeOptions.flowPaneAdapter.getViewport,
        setViewport: runtimeOptions.flowPaneAdapter.setViewport,
      }),
    )
    expect(mocks.useCanvasViewportRuntime.mock.calls[0]?.[0].getUserUid()).toBe('user-a')
    expect(mocks.useCanvasGraphEditing).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: board.flowPane.nodes,
        edges: board.flowPane.edges,
        graphAdapter: board.graphEditing.adapter,
        screenToFlowCoordinate: runtimeOptions.flowPaneAdapter.screenToFlowCoordinate,
        browser: runtimeOptions.graphEditingBrowser,
        upload: runtimeOptions.uploadApi,
        feedback: mocks.feedback.graph,
      }),
    )
    expect(mocks.useCanvasImageWorkflow).toHaveBeenCalledWith(
      expect.objectContaining({
        graph: expect.objectContaining({
          nodes: board.flowPane.nodes,
          currentBoardId: board.boardSession.currentBoardId,
          getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
          connectionAdapter: graph.connectionAdapter,
        }),
        execution: expect.objectContaining({
          edges: board.flowPane.edges,
          graphAdapter: board.graphEditing.adapter,
          refreshNodeInternals: graph.scheduleFlowNodeInternalsRefresh,
          flushAutosave: runtime.persistence.flushAutosave,
        }),
        feedback: {
          imageTools: mocks.feedback.imageTools,
          taskExecution: mocks.feedback.taskExecution,
        },
      }),
    )
  })

  it('projects only the stable Canvas Workspace facade from internal parts', () => {
    const board = createBoardRuntime()
    const graph = createGraphRuntime()
    const imageWorkflow = createImageWorkflowRuntime()
    mocks.useCanvasBoardRuntime.mockReturnValue(board)
    mocks.useCanvasGraphEditing.mockReturnValue(createGraphEditingRuntime(graph))
    mocks.useCanvasImageWorkflow.mockReturnValue(imageWorkflow)

    const runtime = createCanvasWorkspaceRuntime(createWorkspaceOptions())

    expect(Object.keys(runtime).sort()).toEqual(['commands', 'viewModel'])
    expect(runtime.viewModel.flowPane).toEqual({
      nodes: board.flowPane.nodes,
      edges: board.flowPane.edges,
    })
    expect(runtime.commands.flowPane).toEqual({
      onNodesChange: board.flowPane.onNodesChange,
      onEdgesChange: board.flowPane.onEdgesChange,
      connectionAdapter: graph.connectionAdapter,
      nodeDragTransactions: board.flowPane.nodeDragTransactions,
    })
    expect(runtime.commands.persistence.reattachPendingImageResults).toBe(
      imageWorkflow.reattachPendingImageResults,
    )
    expect(runtime.commands.imageWorkflow.runImageDirectAction).toBe(
      imageWorkflow.runImageDirectAction,
    )
    expect('isDirty' in runtime.viewModel.persistence).toBe(false)
    expect('connectNodes' in runtime.commands.flowPane).toBe(false)
    expect('nodeContextMenu' in runtime.viewModel.graphEditing).toBe(false)
    expect('canvasNodeMenuItems' in runtime.viewModel.graphEditing).toBe(false)
    expect('closeNodeContextMenu' in runtime.commands.graphEditing).toBe(false)
    expect('openNodeContextMenu' in runtime.commands.graphEditing).toBe(false)
  })
})
