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
  useCanvasGraphInteractions: vi.fn(),
  useCanvasViewportRuntime: vi.fn(),
  useCanvasImageWorkflow: vi.fn(),
  cancelTasks: vi.fn(),
  createCanvasTaskLifecycle: vi.fn(),
}))

vi.mock('../../feedback/useCanvasFeedback', () => ({
  useCanvasFeedback: () => mocks.feedback,
}))

vi.mock('../../composables/useCanvasBoardRuntime', () => ({
  useCanvasBoardRuntime: mocks.useCanvasBoardRuntime,
}))

vi.mock('../../graph-editing', () => ({
  useCanvasGraphEditing: mocks.useCanvasGraphEditing,
  useCanvasGraphInteractions: mocks.useCanvasGraphInteractions,
}))

vi.mock('../../composables/useCanvasViewportRuntime', () => ({
  useCanvasViewportRuntime: mocks.useCanvasViewportRuntime,
}))

vi.mock('../../image-workflow', () => ({
  useCanvasImageWorkflow: mocks.useCanvasImageWorkflow,
}))

vi.mock('../../task-lifecycle', () => ({
  createCanvasTaskLifecycle: mocks.createCanvasTaskLifecycle,
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
    view: { groupFrames: ref([]) },
    commands: { groupSelectedNodes: vi.fn() },
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
    libraryImport: {
      add: vi.fn(),
      retry: vi.fn(() => false),
    },
    resumePendingLibraryImports: vi.fn(() => 0),
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
      libraryImport: graph.libraryImport,
    },
    recoveryPort: {
      resumePendingLibraryImports: graph.resumePendingLibraryImports,
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

function createGraphInteractionRuntime(graph: ReturnType<typeof createGraphRuntime>) {
  return {
    viewModel: {
      hasSelectedNodes: ref(false),
      isInteractionActive: ref(false),
      isViewportPanInteractionActive: ref(false),
      isInteractionLocked: ref(false),
    },
    commands: {
      clearSelection: graph.clearSelectedFlowElementsSync,
      exportSelectedImages: vi.fn(),
      openImagePreview: vi.fn(),
      appendImageToCanvas: vi.fn(),
      beginNodeDragDuplicateTracking: vi.fn(),
      finishNodeDragDuplicateTracking: vi.fn(),
      getNodeUiState: vi.fn(),
    },
    runtimePort: {
      startInteraction: graph.startFlowInteraction,
      stopInteraction: graph.stopFlowInteraction,
    },
  }
}

function createImageWorkflowRuntime() {
  return {
    viewModel: {
      tools: {},
      canvasExtend: { nodeId: ref(null), overlayStyle: ref(null), handleRadius: 4 },
    },
    commands: {
      isGenerationAvailable: vi.fn(() => true),
      tools: {},
      interactions: {
        onFlowImageContextMenu: vi.fn(),
        onFlowFusionContextMenu: vi.fn(),
        onFlowImageDirectAction: vi.fn(),
        onFlowImagePromptGenerate: vi.fn(),
        onFlowFusionPromptGenerate: vi.fn(),
        onFlowPromptGenerate: vi.fn(),
        onFlowImageExpandSubmit: vi.fn(),
        onFlowExpandHandlePointerDown: vi.fn(),
        onFlowExpandFramePointerDown: vi.fn(),
        onFlowExpandHandlePointerMove: vi.fn(),
        onFlowExpandHandlePointerUp: vi.fn(),
      },
      cancelCanvasExtend: vi.fn(),
      hideStrengthControl: vi.fn(),
    },
    reattachPendingImageResults: vi.fn(),
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
    selectOnlyNode: vi.fn(() => true),
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
    materializeLibraryAsset: vi.fn(async () => ({
      mediaAssetId: 43,
      url: 'https://example.com/materialized.png',
      width: 200,
      height: 100,
    })),
    graphEditingBrowser: {
      pickImageFile: vi.fn(async () => null),
      readDroppedFile: vi.fn(() => null),
      readDropScreenPoint: vi.fn(() => ({ x: 0, y: 0 })),
      markDropAsCopy: vi.fn(),
    },
    graphInteractionBrowser: {
      downloadImage: vi.fn(),
    },
    openNodeImagePreview: vi.fn(),
    openImageUrlPreview: vi.fn(),
    imageWorkflowUi: {
      activeTool: ref('select'),
      canvasWrapRef: ref<HTMLElement | null>(null),
      getImageStageElement: vi.fn(() => null),
      panels: {
        open: vi.fn(),
        clear: vi.fn(),
        isActive: vi.fn(() => false),
        getActiveNodeId: vi.fn(() => null),
      },
    },
    logoutAndRedirect: vi.fn(),
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
    mocks.createCanvasTaskLifecycle.mockReturnValue({
      cancelTasks: mocks.cancelTasks,
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
    mocks.useCanvasGraphInteractions.mockReturnValue(createGraphInteractionRuntime(graph))
    mocks.useCanvasImageWorkflow.mockReturnValue(imageWorkflow)
    mocks.useCanvasViewportRuntime.mockReturnValue(viewportRuntime)

    const runtimeOptions = createWorkspaceOptions()
    const runtime = createCanvasWorkspaceRuntimeParts(runtimeOptions)

    expect(Object.keys(runtime).sort()).toEqual([
      'board',
      'graph',
      'imageWorkflow',
      'persistence',
      'viewport',
    ])
    expect(runtime.board.flowPane.nodes).toBe(board.flowPane.nodes)
    expect(runtime.board.flowPane.edges).toBe(board.flowPane.edges)
    expect(runtime.board.flowPane.onNodesChange).toBe(board.flowPane.onNodesChange)
    expect(runtime.board.flowPane.onEdgesChange).toBe(board.flowPane.onEdgesChange)
    expect(runtime.graph.flowPane.connectionAdapter).toBe(graph.connectionAdapter)
    expect('connectNodes' in runtime.graph.flowPane).toBe(false)
    expect('vueFlowConnectionBindings' in runtime.graph.flowPane).toBe(false)
    expect('onConnect' in runtime.graph.flowPane).toBe(false)
    expect('isValidConnection' in runtime.graph.flowPane).toBe(false)
    expect(runtime.board.flowPane.nodeDragTransactions.begin).toBe(
      board.flowPane.nodeDragTransactions.begin,
    )
    expect(runtime.board.boardSession.currentBoardId).toBe(board.boardSession.currentBoardId)
    expect(runtime.board.boardChrome.isRenamePending).toBe(board.boardChrome.isRenamePending)
    expect(runtime.board.boardChrome.catalogRecovery).toBe(board.boardChrome.catalogRecovery)
    expect(runtime.board.boardChrome.reloadCatalog).toBe(board.boardChrome.reloadCatalog)
    expect(runtime.board.boardChrome.onNavigateRequest).toBe(board.boardChrome.onNavigateRequest)
    expect(runtime.board.boardChrome.onLogoutRequest).toBe(board.boardChrome.onLogoutRequest)
    expect('isDirty' in runtime.persistence).toBe(false)
    expect('isSavingBoard' in runtime.persistence).toBe(false)
    expect('saveStatus' in runtime.persistence).toBe(false)
    expect(runtime.persistence.flushAutosave).not.toBe(board.boardSession.flushAutosave)
    expect('reattachPendingImageResults' in runtime.persistence).toBe(false)
    expect('resumePendingLibraryImports' in runtime.persistence).toBe(false)
    expect(runtime.graph.viewModel.grouping).toBe(board.graphEditing.grouping.view)
    expect(runtime.graph.commands.grouping).toBe(board.graphEditing.grouping.commands)
    expect(runtime.imageWorkflow).toBe(imageWorkflow)

    await runtime.persistence.flushAutosave({ silent: false })
    await runtime.persistence.recoverBoardSession()

    expect(events).toEqual(['viewport', 'autosave'])
    expect(board.boardSession.flushAutosave).toHaveBeenCalledWith({ silent: false })
    expect(viewportRuntime.loadCanvasBoardAndProbe).toHaveBeenCalledTimes(1)
    expect(imageWorkflow.reattachPendingImageResults).toHaveBeenCalledTimes(1)
    expect(graph.resumePendingLibraryImports).toHaveBeenCalledTimes(1)

    expect(mocks.useCanvasBoardRuntime).toHaveBeenCalledWith(
      expect.objectContaining({
        feedback: mocks.feedback.boardSession,
        executionCore: expect.objectContaining({
          begin: expect.any(Function),
          hydratePendingLease: expect.any(Function),
          claimHydratedLease: expect.any(Function),
        }),
        vueFlowGraphActions: runtimeOptions.flowPaneAdapter.vueFlowGraphActions,
      }),
    )
    expect(mocks.createCanvasTaskLifecycle).toHaveBeenCalledWith({
      cancel: expect.any(Function),
    })
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
    expect(mocks.useCanvasGraphInteractions).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: board.flowPane.nodes,
        edges: board.flowPane.edges,
        graphAdapter: board.graphEditing.adapter,
        browser: runtimeOptions.graphInteractionBrowser,
        feedback: mocks.feedback.graph,
        isNodeExecutionActive: expect.any(Function),
      }),
    )
    expect(mocks.useCanvasImageWorkflow).toHaveBeenCalledWith(
      expect.objectContaining({
        graph: expect.objectContaining({
          nodes: board.flowPane.nodes,
          currentBoardId: board.boardSession.currentBoardId,
          getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
          connectionAdapter: graph.connectionAdapter,
          isCanvasInteractionLocked: expect.objectContaining({ value: false }),
        }),
        flowPaneAdapter: runtimeOptions.flowPaneAdapter,
        execution: expect.objectContaining({
          edges: board.flowPane.edges,
          graphAdapter: board.graphEditing.adapter,
          refreshNodeInternals: graph.scheduleFlowNodeInternalsRefresh,
          flushAutosave: runtime.persistence.flushAutosave,
          executionCore: expect.any(Object),
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
    mocks.useCanvasGraphInteractions.mockReturnValue(createGraphInteractionRuntime(graph))
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
    expect(runtime.commands.persistence.recoverBoardSession).toEqual(expect.any(Function))
    expect('reattachPendingImageResults' in runtime.commands.persistence).toBe(false)
    expect('resumePendingLibraryImports' in runtime.commands.persistence).toBe(false)
    expect(runtime.commands.imageWorkflow.isGenerationAvailable).toBe(
      imageWorkflow.commands.isGenerationAvailable,
    )
    expect('isDirty' in runtime.viewModel.persistence).toBe(false)
    expect('connectNodes' in runtime.commands.flowPane).toBe(false)
    expect('nodeContextMenu' in runtime.viewModel.graphEditing).toBe(false)
    expect('canvasNodeMenuItems' in runtime.viewModel.graphEditing).toBe(false)
    expect('closeNodeContextMenu' in runtime.commands.graphEditing).toBe(false)
    expect('openNodeContextMenu' in runtime.commands.graphEditing).toBe(false)
  })
})
