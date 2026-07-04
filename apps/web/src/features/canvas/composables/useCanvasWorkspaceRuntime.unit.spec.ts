import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useCanvasWorkspaceRuntime } from './useCanvasWorkspaceRuntime'

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

vi.mock('../feedback/useCanvasFeedback', () => ({
  useCanvasFeedback: () => mocks.feedback,
}))

vi.mock('./useCanvasBoardRuntime', () => ({
  useCanvasBoardRuntime: mocks.useCanvasBoardRuntime,
}))

vi.mock('./useCanvasGraphRuntime', () => ({
  useCanvasGraphRuntime: mocks.useCanvasGraphRuntime,
}))

vi.mock('./useCanvasViewportRuntime', () => ({
  useCanvasViewportRuntime: mocks.useCanvasViewportRuntime,
}))

vi.mock('./useCanvasTaskRuntime', () => ({
  useCanvasTaskRuntime: mocks.useCanvasTaskRuntime,
}))

vi.mock('../image-tools', () => ({
  useCanvasImageTools: mocks.useCanvasImageTools,
}))

function createBoardRuntime() {
  return {
    nodes: ref([]),
    edges: ref([]),
    fullNodes: ref([]),
    fullEdges: ref([]),
    currentBoardId: ref('board-a'),
    flushAutosave: vi.fn(),
    graphRuntime: {
      captureHistory: vi.fn(),
      markDirty: vi.fn(),
      nodeDrag: {
        begin: vi.fn(),
        commit: vi.fn(),
      },
    },
    graphAdapter: {},
    loadInitialOrRouteBoard: vi.fn(),
    markBoardViewportChanged: vi.fn(),
    commitPendingBoardViewportChange: vi.fn(),
    closeBoardHeaderEditing: vi.fn(),
  }
}

function createGraphRuntime() {
  return {
    scheduleFlowNodeInternalsRefresh: vi.fn(),
    getImageNodeDisplayPayload: vi.fn(),
    getImageNodeSnapshot: vi.fn(),
    createImageNodeFromFile: vi.fn(),
    onConnect: vi.fn(),
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
    isNodeDirectActionRunning: vi.fn(),
    reattachPendingImageResults: vi.fn(),
  }
}

function createWorkspaceOptions() {
  return {
    route: { query: {} } as never,
    router: {} as never,
    getUserIdentity: () => ({ userId: 1, userUid: 'user-a' }),
    createNodeKind: ref('prompt' as const),
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
    uploadApi: { upload: vi.fn() },
    sanitizeApiErrorMessage: vi.fn(),
    openNodeImagePreview: vi.fn(),
    openImageUrlPreview: vi.fn(),
    logoutAndRedirect: vi.fn(),
    startNodeDragInteraction: vi.fn(),
    stopNodeDragInteraction: vi.fn(),
    refreshFlowHandleVisibility: vi.fn(),
  }
}

describe('useCanvasWorkspaceRuntime', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.useCanvasViewportRuntime.mockReturnValue({ zoomLevel: ref(100) })
    mocks.useCanvasImageTools.mockReturnValue({ kind: 'image-tools' })
  })

  it('owns feedback selection for board, graph, task, and image tools', () => {
    const board = createBoardRuntime()
    const graph = createGraphRuntime()
    const task = createTaskRuntime()
    mocks.useCanvasBoardRuntime.mockReturnValue(board)
    mocks.useCanvasGraphRuntime.mockReturnValue(graph)
    mocks.useCanvasTaskRuntime.mockReturnValue(task)

    const runtime = useCanvasWorkspaceRuntime(createWorkspaceOptions())

    expect(runtime.flowPane.nodes).toBe(board.nodes)
    expect(runtime.flowPane.nodeDragTransactions.begin).toBe(board.graphRuntime.nodeDrag.begin)
    expect(runtime.boardChrome.currentBoardId).toBe(board.currentBoardId)
    expect(runtime.persistence.reattachPendingImageResults).toBe(
      task.reattachPendingImageResults,
    )

    expect(mocks.useCanvasBoardRuntime).toHaveBeenCalledWith(
      expect.objectContaining({
        feedback: mocks.feedback.boardSession,
      }),
    )
    expect(mocks.useCanvasGraphRuntime).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: board.fullNodes,
        edges: board.fullEdges,
        graphAdapter: board.graphAdapter,
        feedback: mocks.feedback.graph,
      }),
    )
    expect(mocks.useCanvasTaskRuntime).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: board.fullNodes,
        edges: board.fullEdges,
        graphAdapter: board.graphAdapter,
        refreshNodeInternals: graph.scheduleFlowNodeInternalsRefresh,
        feedback: mocks.feedback.taskExecution,
      }),
    )

    runtime.createImageTools({
      activeTool: ref('select'),
      canvasWrapRef: ref(null),
      isCanvasInteractionLocked: ref(false),
      getImageStageElement: vi.fn(),
      clearActiveNodePanel: vi.fn(),
      hideStrengthControl: vi.fn(),
      isImageActionPanelVisible: vi.fn(),
      isImagePromptComposerVisible: vi.fn(),
      activeStrengthAction: ref(null),
      getActionStrength: vi.fn(),
      setStrengthRangeRef: vi.fn(),
      setStrengthTriggerRef: vi.fn(),
      onStrengthRangePointerDown: vi.fn(),
      onStrengthInput: vi.fn(),
      onStrengthChange: vi.fn(),
      onStrengthPointerUp: vi.fn(),
      onStrengthClick: vi.fn(),
    })

    expect(mocks.useCanvasImageTools).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: board.nodes,
        currentBoardId: board.currentBoardId,
        getImageNodeDisplayPayload: graph.getImageNodeDisplayPayload,
        getImageNodeSnapshot: graph.getImageNodeSnapshot,
        runInpaintAction: task.runImageInpaintAction,
        runPromptGeneration: task.runImagePromptGeneration,
        isDirectActionRunning: task.isNodeDirectActionRunning,
        clearSelectedFlowElements: graph.clearSelectedFlowElementsSync,
        feedback: mocks.feedback.imageTools,
      }),
    )
  })

  it('keeps output count mutations in the workspace graph wiring', () => {
    const board = createBoardRuntime()
    const graph = createGraphRuntime()
    const task = createTaskRuntime()
    mocks.useCanvasBoardRuntime.mockReturnValue(board)
    mocks.useCanvasGraphRuntime.mockReturnValue(graph)
    mocks.useCanvasTaskRuntime.mockReturnValue(task)

    const runtime = useCanvasWorkspaceRuntime(createWorkspaceOptions())
    runtime.createImageTools({
      activeTool: ref('select'),
      canvasWrapRef: ref(null),
      isCanvasInteractionLocked: ref(false),
      getImageStageElement: vi.fn(),
      clearActiveNodePanel: vi.fn(),
      hideStrengthControl: vi.fn(),
      isImageActionPanelVisible: vi.fn(),
      isImagePromptComposerVisible: vi.fn(),
      activeStrengthAction: ref(null),
      getActionStrength: vi.fn(),
      setStrengthRangeRef: vi.fn(),
      setStrengthTriggerRef: vi.fn(),
      onStrengthRangePointerDown: vi.fn(),
      onStrengthInput: vi.fn(),
      onStrengthChange: vi.fn(),
      onStrengthPointerUp: vi.fn(),
      onStrengthClick: vi.fn(),
    })
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

    const runtime = useCanvasWorkspaceRuntime(createWorkspaceOptions())
    const cancelFlowExpandMode = vi.fn()
    const cancelFlowMaskEditorMode = vi.fn()
    const { onFlowPromptGenerate } = runtime.createPromptGenerationHandlers({
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

    const runtime = useCanvasWorkspaceRuntime(createWorkspaceOptions())
    const { onFlowFusionPromptGenerate } = runtime.createImageActionHandlers({
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
})
