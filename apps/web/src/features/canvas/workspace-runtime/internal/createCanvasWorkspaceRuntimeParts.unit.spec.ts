import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { IMAGE_NODE_ACTION } from '../../../../shared/domain/imageActions'
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
  useCanvasGraphRuntime: vi.fn(),
  useCanvasViewportRuntime: vi.fn(),
  useCanvasTaskRuntime: vi.fn(),
  useCanvasImageTools: vi.fn(),
}))

vi.mock('../../feedback/useCanvasFeedback', () => ({
  useCanvasFeedback: () => mocks.feedback,
}))

vi.mock('../../composables/useCanvasBoardRuntime', () => ({
  useCanvasBoardRuntime: mocks.useCanvasBoardRuntime,
}))

vi.mock('../../composables/useCanvasGraphRuntime', () => ({
  useCanvasGraphRuntime: mocks.useCanvasGraphRuntime,
}))

vi.mock('../../composables/useCanvasViewportRuntime', () => ({
  useCanvasViewportRuntime: mocks.useCanvasViewportRuntime,
}))

vi.mock('../../composables/useCanvasTaskRuntime', () => ({
  useCanvasTaskRuntime: mocks.useCanvasTaskRuntime,
}))

vi.mock('../../image-tools', () => ({
  useCanvasImageTools: mocks.useCanvasImageTools,
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
    _onFlowNodeParamUpdate: vi.fn(),
    markFlowExecutionNodeDirty: vi.fn(),
    clearSelectedFlowElementsSync: vi.fn(),
    startFlowInteraction: vi.fn(),
    stopFlowInteraction: vi.fn(),
    getMediaInputImages: vi.fn(() => []),
    resolvePromptInputImageNodeIds: vi.fn(() => []),
  }
}

function createTaskRuntime() {
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
    createNodeKind: ref('prompt' as const),
    flowPaneAdapter,
    uploadApi: { upload: vi.fn() },
    openNodeImagePreview: vi.fn(),
    openImageUrlPreview: vi.fn(),
    logoutAndRedirect: vi.fn(),
    startNodeDragInteraction: vi.fn(),
    stopNodeDragInteraction: vi.fn(),
    refreshFlowHandleVisibility: vi.fn(),
    openRecharge: vi.fn(),
  }
}

function createImageToolsOptions() {
  return {
    activeTool: ref('select'),
    canvasWrapRef: ref(null),
    isCanvasInteractionLocked: ref(false),
    getImageStageElement: vi.fn(),
    clearActiveNodePanel: vi.fn(),
    hideStrengthControl: vi.fn(),
    isImageActionPanelVisible: vi.fn(),
    isImagePromptComposerVisible: vi.fn(),
    strengthControl: {
      activeAction: ref(null),
      getValue: vi.fn(),
      setRangeRef: vi.fn(),
      setTriggerRef: vi.fn(),
      onRangePointerDown: vi.fn(),
      onInput: vi.fn(),
      onChange: vi.fn(),
      onPointerUp: vi.fn(),
      onClick: vi.fn(),
    },
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
    mocks.useCanvasImageTools.mockReturnValue({ kind: 'image-tools' })
  })

  it('owns feedback selection for board, graph, task, and image tools', async () => {
    const board = createBoardRuntime()
    const graph = createGraphRuntime()
    const task = createTaskRuntime()
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
    mocks.useCanvasGraphRuntime.mockReturnValue(graph)
    mocks.useCanvasTaskRuntime.mockReturnValue(task)
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
    expect(runtime.persistence.reattachPendingImageResults).toBe(task.reattachPendingImageResults)
    expect(runtime.graphEditing.grouping).toBe(board.graphEditing.grouping)
    expect(runtime.imageWorkflow.runImageDirectAction).toBe(task.runImageDirectAction)
    expect(runtime.imageWorkflow.runCanvasExtendAction).toBe(task.runCanvasExtendAction)

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
    expect(mocks.useCanvasGraphRuntime).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: board.flowPane.nodes,
        edges: board.flowPane.edges,
        graphAdapter: board.graphEditing.adapter,
        screenToFlowCoordinate: runtimeOptions.flowPaneAdapter.screenToFlowCoordinate,
        getSelectedElements: runtimeOptions.flowPaneAdapter.getSelectedElements,
        feedback: mocks.feedback.graph,
      }),
    )
    expect(mocks.useCanvasTaskRuntime).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: board.flowPane.nodes,
        edges: board.flowPane.edges,
        graphAdapter: board.graphEditing.adapter,
        refreshNodeInternals: graph.scheduleFlowNodeInternalsRefresh,
        flushAutosave: runtime.persistence.flushAutosave,
        feedback: mocks.feedback.taskExecution,
      }),
    )

    runtime.imageWorkflow.createImageTools(createImageToolsOptions())

    expect(mocks.useCanvasImageTools).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: board.flowPane.nodes,
        currentBoardId: board.boardSession.currentBoardId,
        getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
        getImageNodeSnapshot: graph.getImageNodeSnapshot,
        connectionAdapter: graph.connectionAdapter,
        runInpaintAction: task.runImageInpaintAction,
        runPromptGeneration: task.runImagePromptGeneration,
        isGenerationAvailable: task.isGenerationAvailable,
        isDirectActionRunning: task.isNodeDirectActionRunning,
        clearSelectedFlowElements: graph.clearSelectedFlowElementsSync,
        feedback: mocks.feedback.imageTools,
      }),
    )
  })

  it('projects only the stable Canvas Workspace facade from internal parts', () => {
    const board = createBoardRuntime()
    const graph = createGraphRuntime()
    const task = createTaskRuntime()
    mocks.useCanvasBoardRuntime.mockReturnValue(board)
    mocks.useCanvasGraphRuntime.mockReturnValue(graph)
    mocks.useCanvasTaskRuntime.mockReturnValue(task)

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
      task.reattachPendingImageResults,
    )
    expect(runtime.commands.imageWorkflow.runImageDirectAction).toBe(task.runImageDirectAction)
    expect('isDirty' in runtime.viewModel.persistence).toBe(false)
    expect('connectNodes' in runtime.commands.flowPane).toBe(false)
    expect('nodeContextMenu' in runtime.viewModel.graphEditing).toBe(false)
    expect('canvasNodeMenuItems' in runtime.viewModel.graphEditing).toBe(false)
    expect('closeNodeContextMenu' in runtime.commands.graphEditing).toBe(false)
    expect('openNodeContextMenu' in runtime.commands.graphEditing).toBe(false)
  })

  it('keeps output count mutations in the workspace graph wiring', () => {
    const board = createBoardRuntime()
    const graph = createGraphRuntime()
    const task = createTaskRuntime()
    mocks.useCanvasBoardRuntime.mockReturnValue(board)
    mocks.useCanvasGraphRuntime.mockReturnValue(graph)
    mocks.useCanvasTaskRuntime.mockReturnValue(task)

    const runtime = createCanvasWorkspaceRuntimeParts(createWorkspaceOptions())
    runtime.imageWorkflow.createImageTools(createImageToolsOptions())
    const imageToolOptions = mocks.useCanvasImageTools.mock.calls[0]?.[0]
    const patch = { key: 'outputCount', value: 2 } as const

    imageToolOptions.updateOutputCount('node-a', patch)

    expect(graph._onFlowNodeParamUpdate).toHaveBeenCalledWith('node-a', patch)
    expect(graph.markFlowExecutionNodeDirty).toHaveBeenCalledWith('node-a', {
      includeSelf: true,
    })
  })

  it('keeps prompt generation workflow policy inside the workspace runtime', () => {
    const board = createBoardRuntime()
    const graph = createGraphRuntime()
    const task = createTaskRuntime()
    graph.getMediaInputImages.mockReturnValue([])
    mocks.useCanvasBoardRuntime.mockReturnValue(board)
    mocks.useCanvasGraphRuntime.mockReturnValue(graph)
    mocks.useCanvasTaskRuntime.mockReturnValue(task)

    const runtime = createCanvasWorkspaceRuntimeParts(createWorkspaceOptions())
    const cancelFlowExpandMode = vi.fn()
    const cancelFlowMaskEditorMode = vi.fn()
    const { onFlowPromptGenerate } = runtime.imageWorkflow.createPromptGenerationHandlers({
      cancelFlowExpandMode,
      cancelFlowMaskEditorMode,
    })

    onFlowPromptGenerate('prompt-a', 'Draw this', 2, 'auto')

    expect(cancelFlowExpandMode).toHaveBeenCalled()
    expect(cancelFlowMaskEditorMode).toHaveBeenCalled()
    expect(graph.getMediaInputImages).toHaveBeenCalledWith('prompt-a')
    expect(task.runTextPromptGeneration).toHaveBeenCalledWith('prompt-a', 'Draw this', 2, 'auto')
  })

  it('keeps fusion parameter dirty marking inside the workspace action wiring', () => {
    const board = createBoardRuntime()
    const graph = createGraphRuntime()
    const task = createTaskRuntime()
    mocks.useCanvasBoardRuntime.mockReturnValue(board)
    mocks.useCanvasGraphRuntime.mockReturnValue(graph)
    mocks.useCanvasTaskRuntime.mockReturnValue(task)

    const runtime = createCanvasWorkspaceRuntimeParts(createWorkspaceOptions())
    const { onFlowFusionPromptGenerate } = runtime.imageWorkflow.createImageActionHandlers({
      flowImageEditorNodeId: ref(null),
      consumeSuppressedStrengthClick: vi.fn(() => false),
      openActiveNodePanel: vi.fn(),
      clearActiveNodePanel: vi.fn(),
      hideStrengthControl: vi.fn(),
      selectFlowNodeForContextMenu: vi.fn(),
      cancelFlowExpandMode: vi.fn(),
      cancelFlowMaskEditorMode: vi.fn(),
      enterFlowExpandMode: vi.fn(),
      enterFlowImageEditorMode: vi.fn(),
      focusImageEditorAndShowChrome: vi.fn(),
      runImagePromptAction: vi.fn(),
      getActionStrength: vi.fn(),
    })

    onFlowFusionPromptGenerate('fusion-a', 'Blend', 3, '1:1')

    expect(graph._onFlowNodeParamUpdate).toHaveBeenCalledTimes(3)
    expect(graph.markFlowExecutionNodeDirty).toHaveBeenCalledTimes(3)
    expect(graph.markFlowExecutionNodeDirty).toHaveBeenCalledWith('fusion-a', {
      includeSelf: true,
    })
    expect(task.runFusionPromptGeneration).toHaveBeenCalledWith('fusion-a', 'Blend', 3, '1:1')
  })

  it('keeps image workflow entrypoints reachable from the workspace runtime surface', () => {
    const board = createBoardRuntime()
    const graph = createGraphRuntime()
    const task = createTaskRuntime()
    mocks.useCanvasBoardRuntime.mockReturnValue(board)
    mocks.useCanvasGraphRuntime.mockReturnValue(graph)
    mocks.useCanvasTaskRuntime.mockReturnValue(task)

    const runtime = createCanvasWorkspaceRuntimeParts(createWorkspaceOptions())
    const extendPayload = {
      nodeId: 'image-1',
      frameRect: { left: 0, top: 0, width: 100, height: 100 },
      originalImageRect: { left: 10, top: 10, width: 80, height: 80 },
      prompt: 'extend',
      outputCount: 1,
    }

    runtime.imageWorkflow.runImageDirectAction('image-1', IMAGE_NODE_ACTION.hdUpscale)
    runtime.imageWorkflow.runCanvasExtendAction('image-1', extendPayload)

    expect(task.runImageDirectAction).toHaveBeenCalledWith('image-1', IMAGE_NODE_ACTION.hdUpscale)
    expect(task.runCanvasExtendAction).toHaveBeenCalledWith('image-1', extendPayload)
  })
})
