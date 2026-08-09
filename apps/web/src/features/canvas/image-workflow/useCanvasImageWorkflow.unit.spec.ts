import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { IMAGE_NODE_ACTION } from '../config/imageActionRegistry'
import type { MediaInputImage } from '../composables/useFlowMediaInputImages'
import { ImageExecutionPlanError } from '../execution/planning/ImageExecutionPlan'
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
    executionOptions: null as null | { onWarning: (message: string) => void },
    useCanvasImageTools: vi.fn(),
  }
})

vi.mock('../execution', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../execution')>()
  return {
    ...actual,
    createImageExecutionPlanner: vi.fn(() => mocks.planner),
    useFlowImageWorkflowExecution: vi.fn((options) => {
      mocks.executionOptions = options as { onWarning: (message: string) => void }
      return mocks.execution
    }),
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
    getMediaInputImages: vi.fn(() => [] as MediaInputImage[]),
    resolvePromptInputImageNodeIds: vi.fn(() => [] as string[]),
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
    mocks.executionOptions = null
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

  it('routes text Prompt intent through planning and execution with normalized inputs', async () => {
    const { options } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const actionOptions = createActionOptions()
    const handlers = workflow.createImageActionHandlers(actionOptions)

    handlers.onFlowPromptGenerate('prompt-1', 'Draw this', 2, 'unsupported' as never)
    await vi.waitFor(() => expect(mocks.execution.executeImageWorkflow).toHaveBeenCalledOnce())

    expect(mocks.planner.planTextPromptGeneration).toHaveBeenCalledWith(
      'prompt-1',
      'Draw this',
      2,
      'auto',
    )
    expect(actionOptions.cancelFlowExpandMode).toHaveBeenCalledOnce()
    expect(actionOptions.cancelFlowMaskEditorMode).toHaveBeenCalledOnce()
  })

  it('routes linked and uploaded Prompt inputs through their canonical image paths', async () => {
    const { options, graph } = createOptions()
    graph.getMediaInputImages.mockReturnValue([
      {
        id: 'linked-image-1',
        sourceType: 'node',
        sourceId: 'image-1',
        sourceNodeId: 'image-1',
        url: 'https://example.com/linked.png',
        ratio: 0.5,
      },
      {
        id: 'uploaded-1',
        sourceType: 'external',
        sourceId: 'uploaded-1',
        url: 'https://example.com/uploaded.png',
        ratio: 1.5,
      },
    ])
    graph.resolvePromptInputImageNodeIds.mockReturnValue(['image-1'])
    const workflow = useCanvasImageWorkflow(options as never)
    const handlers = workflow.createImageActionHandlers(createActionOptions())

    handlers.onFlowPromptGenerate('prompt-1', 'Remix', 3, '16:9')
    await vi.waitFor(() => expect(mocks.execution.executeImageWorkflow).toHaveBeenCalledTimes(2))

    expect(mocks.planner.planImagePromptGeneration).toHaveBeenCalledWith(
      'image-1',
      'Remix',
      3,
      '16:9',
      { outputAnchorNodeId: 'prompt-1' },
    )
    expect(mocks.planner.planImagePromptGenerationFromSource).toHaveBeenCalledWith(
      { url: 'https://example.com/uploaded.png', ratio: 1.5 },
      'uploaded-1',
      'Remix',
      3,
      '16:9',
      { outputAnchorNodeId: 'prompt-1' },
    )
  })

  it('surfaces Prompt planning failures through workflow feedback', async () => {
    const { options, feedback } = createOptions()
    mocks.planner.planTextPromptGeneration.mockImplementationOnce(() => {
      throw new ImageExecutionPlanError('请输入 Prompt')
    })
    const workflow = useCanvasImageWorkflow(options as never)
    const handlers = workflow.createImageActionHandlers(createActionOptions())

    handlers.onFlowPromptGenerate('prompt-1', '', 1, 'auto')
    await vi.waitFor(() =>
      expect(feedback.taskExecution.warning).toHaveBeenCalledWith('请输入 Prompt'),
    )

    expect(mocks.execution.executeImageWorkflow).not.toHaveBeenCalled()
  })

  it('keeps Prompt generation disabled during provider degradation', () => {
    vi.useFakeTimers()
    try {
      const { options, feedback } = createOptions()
      const workflow = useCanvasImageWorkflow(options as never)
      const handlers = workflow.createImageActionHandlers(createActionOptions())

      mocks.executionOptions?.onWarning('provider unavailable')
      handlers.onFlowPromptGenerate('prompt-1', 'Keep editing', 1, 'auto')

      expect(feedback.taskExecution.warning).toHaveBeenCalledWith('provider unavailable')
      expect(mocks.planner.planTextPromptGeneration).not.toHaveBeenCalled()
      expect(mocks.execution.executeImageWorkflow).not.toHaveBeenCalled()
    } finally {
      vi.clearAllTimers()
      vi.useRealTimers()
    }
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
