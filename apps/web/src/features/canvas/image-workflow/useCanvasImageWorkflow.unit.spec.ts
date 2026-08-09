import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { IMAGE_NODE_ACTION } from '../config/imageActionRegistry'
import { useCanvasImageWorkflow } from './useCanvasImageWorkflow'

const mocks = vi.hoisted(() => {
  const plan = {
    kind: 'image-action',
    sourceNodeId: 'image-1',
    source: { url: 'https://example.com/source.png', ratio: 1 },
    action: 'remove-bg',
    resultOptions: {},
  }
  return {
    planner: {
      planImageDirectAction: vi.fn(() => plan),
      planCanvasExtendAction: vi.fn(() => plan),
      planImageInpaintAction: vi.fn(() => plan),
      planImagePromptGeneration: vi.fn(() => plan),
      planImagePromptGenerationFromSource: vi.fn(() => plan),
      planTextPromptGeneration: vi.fn(() => plan),
      planFusionPromptGeneration: vi.fn(() => plan),
    },
    execution: {
      executeImageWorkflow: vi.fn(async () => undefined),
      isNodeDirectActionRunning: vi.fn(() => false),
      reattachPendingImageResults: vi.fn(),
    },
    useCanvasImageTools: vi.fn(),
  }
})

vi.mock('../execution', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../execution')>()
  return {
    ...actual,
    createImageExecutionPlanner: vi.fn(() => mocks.planner),
    useFlowImageWorkflowExecution: vi.fn(() => mocks.execution),
  }
})

vi.mock('../image-tools', () => ({
  useCanvasImageTools: mocks.useCanvasImageTools,
}))

function createOptions() {
  const graph = {
    nodes: ref([]),
    currentBoardId: ref('board-a'),
    getImageNodeDisplayPayload: vi.fn(),
    getImageNodeSnapshot: vi.fn(),
    createImageNodeFromFile: vi.fn(),
    connectionAdapter: { connectIntent: vi.fn() },
    uploadImageToBoard: vi.fn(),
    clearSelectedFlowElements: vi.fn(),
    startCanvasInteraction: vi.fn(),
    stopCanvasInteraction: vi.fn(),
    getMediaInputImages: vi.fn(() => []),
    resolvePromptInputImageNodeIds: vi.fn(() => []),
    updateSnapshotNodeParam: vi.fn(),
    markNodeExecutionDirty: vi.fn(),
  }
  const feedback = {
    imageTools: {
      succeeded: vi.fn(),
      warning: vi.fn(),
      actionUnavailable: vi.fn(),
    },
    taskExecution: {
      succeeded: vi.fn(),
      warning: vi.fn(),
      insufficientCredits: vi.fn(),
    },
  }
  const options = {
    graph,
    execution: {
      edges: ref([]),
      graphAdapter: {},
      createCurrentCanvasGraphStore: vi.fn(() => ({})),
      boardBaseSnapshot: ref({ groups: [] }),
      flushAutosave: vi.fn(async () => undefined),
      refreshNodeInternals: vi.fn(),
    },
    flowPaneAdapter: {
      screenToFlowCoordinate: vi.fn((point) => point),
      getViewport: vi.fn(() => ({ x: 0, y: 0, zoom: 1 })),
      setViewport: vi.fn(),
      setCenter: vi.fn(),
    },
    openImageUrlPreview: vi.fn(),
    feedback,
  }
  return { options, graph, feedback }
}

function createActionOptions() {
  return {
    flowImageEditorNodeId: ref<string | null>(null),
    consumeSuppressedStrengthClick: vi.fn(() => false),
    openActiveNodePanel: vi.fn(),
    clearActiveNodePanel: vi.fn(),
    hideStrengthControl: vi.fn(),
    selectFlowNodeForContextMenu: vi.fn(),
    cancelFlowExpandMode: vi.fn(),
    cancelFlowMaskEditorMode: vi.fn(),
    enterFlowExpandMode: vi.fn(() => true),
    enterFlowImageEditorMode: vi.fn(() => true),
    focusImageEditorAndShowChrome: vi.fn(),
    runImagePromptAction: vi.fn(),
    getActionStrength: vi.fn(() => 72),
  }
}

describe('useCanvasImageWorkflow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.useCanvasImageTools.mockReturnValue({ kind: 'image-tools' })
  })

  it('owns direct action planning and execution behind the workflow intent seam', async () => {
    const { options } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const actionOptions = createActionOptions()
    const handlers = workflow.createImageActionHandlers(actionOptions)

    handlers.onFlowImageDirectAction('image-1', IMAGE_NODE_ACTION.removeBg)
    await vi.waitFor(() => expect(mocks.execution.executeImageWorkflow).toHaveBeenCalledOnce())

    expect(mocks.planner.planImageDirectAction).toHaveBeenCalledWith(
      'image-1',
      IMAGE_NODE_ACTION.removeBg,
      undefined,
    )
    expect(actionOptions.cancelFlowExpandMode).toHaveBeenCalledOnce()
    expect(actionOptions.cancelFlowMaskEditorMode).toHaveBeenCalledOnce()
    expect(actionOptions.clearActiveNodePanel).toHaveBeenCalledOnce()
    expect(actionOptions.hideStrengthControl).toHaveBeenCalledOnce()
  })

  it('passes strength intent through the same direct action workflow path', async () => {
    const { options } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const handlers = workflow.createImageActionHandlers(createActionOptions())

    handlers.onFlowImageDirectAction('image-1', IMAGE_NODE_ACTION.styleVariation)
    await vi.waitFor(() => expect(mocks.execution.executeImageWorkflow).toHaveBeenCalledOnce())

    expect(mocks.planner.planImageDirectAction).toHaveBeenCalledWith(
      'image-1',
      IMAGE_NODE_ACTION.styleVariation,
      { strength: 72 },
    )
  })

  it('keeps local editor and canvas-extend intents out of Task submission', () => {
    const { options } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const actionOptions = createActionOptions()
    const handlers = workflow.createImageActionHandlers(actionOptions)

    handlers.onFlowImageDirectAction('image-1', IMAGE_NODE_ACTION.localImageEditor)
    handlers.onFlowImageDirectAction('image-1', IMAGE_NODE_ACTION.imageExtend)

    expect(actionOptions.enterFlowImageEditorMode).toHaveBeenCalledWith('image-1', 'adjust')
    expect(actionOptions.focusImageEditorAndShowChrome).toHaveBeenCalledWith('image-1')
    expect(actionOptions.enterFlowExpandMode).toHaveBeenCalledWith('image-1')
    expect(mocks.planner.planImageDirectAction).not.toHaveBeenCalled()
    expect(mocks.execution.executeImageWorkflow).not.toHaveBeenCalled()
  })

  it('wires Image Tool Session and graph mutations without exposing execution internals', () => {
    const { options, graph, feedback } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)

    workflow.createImageTools({} as never)

    expect(mocks.useCanvasImageTools).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: graph.nodes,
        currentBoardId: graph.currentBoardId,
        connectionAdapter: graph.connectionAdapter,
        isGenerationAvailable: workflow.isGenerationAvailable,
        feedback: feedback.imageTools,
      }),
    )
    const imageToolOptions = mocks.useCanvasImageTools.mock.calls[0]?.[0]
    const patch = { key: 'outputCount', value: 2 } as const

    imageToolOptions.updateOutputCount('image-1', patch)

    expect(graph.updateSnapshotNodeParam).toHaveBeenCalledWith('image-1', patch)
    expect(graph.markNodeExecutionDirty).toHaveBeenCalledWith('image-1', {
      includeSelf: true,
    })
  })

  it('owns fusion mutations and pending-result reattachment on the workflow runtime', () => {
    const { options, graph } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const handlers = workflow.createImageActionHandlers(createActionOptions())

    handlers.onFlowFusionPromptGenerate('fusion-1', 'Blend', 3, '1:1')
    workflow.reattachPendingImageResults()

    expect(graph.updateSnapshotNodeParam).toHaveBeenCalledTimes(3)
    expect(graph.markNodeExecutionDirty).toHaveBeenCalledTimes(3)
    expect(mocks.planner.planFusionPromptGeneration).toHaveBeenCalledWith(
      'fusion-1',
      'Blend',
      3,
      '1:1',
    )
    expect(mocks.execution.reattachPendingImageResults).toHaveBeenCalledOnce()
  })
})
