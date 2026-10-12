import { i18n } from '../../../../core/i18n'
import { ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { IMAGE_NODE_ACTION } from '../../core/nodes/imageActionRegistry.ts'
import type { MediaInputImage } from '../graph-editing'
import { createPendingImageExecutionCore } from '../execution'
import { useCanvasImageWorkflow } from './useCanvasImageWorkflow'
import { getWorkspaceSelectionStore, resetWorkspaceSelectionStore } from '../../../workspace'
import { resetMediaWorkspaceIdResolver } from '../../../../api/mediaAssetDelivery'

const mocks = vi.hoisted(() => {
  const plan = {
    kind: 'image-action',
    sourceNodeId: 'image-1',
    source: { url: 'https://example.com/source.png', ratio: 1 },
    action: 'cutout',
    resultOptions: {},
  }
  return {
    planner: {
      planImageDirectAction: vi.fn(() => plan),
      planOutpaintAction: vi.fn(() => plan),
      planImageInpaintAction: vi.fn(() => plan),
      planGeneration: vi.fn(() => plan),
      planFusionGeneration: vi.fn(() => plan),
    },
    execution: {
      executeImageWorkflow: vi.fn(async () => undefined),
      isNodeExecutionActive: vi.fn(() => false),
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
    useImageWorkflowExecution: vi.fn((options) => {
      mocks.executionOptions = options as { onWarning: (message: string) => void }
      return mocks.execution
    }),
  }
})

vi.mock('../../tools/image-tools', () => ({
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
    isCanvasInteractionLocked: ref(false),
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
  const ui = {
    activeTool: ref('select'),
    canvasWrapRef: ref<HTMLElement | null>(null),
    getImageStageElement: vi.fn(() => null),
    panels: {
      open: vi.fn(),
      clear: vi.fn(),
      isActive: vi.fn(() => false),
      getActiveNodeId: vi.fn(() => null),
    },
  }
  const options = {
    graph,
    execution: {
      edges: ref([]),
      readCanvasGraphStore: vi.fn(() => ({})),
      boardBaseSnapshot: ref({ groups: [] }),
      flushAutosave: vi.fn(async () => undefined),
      refreshNodeInternals: vi.fn(),
      executionCore: createPendingImageExecutionCore(),
      cancelTasks: vi.fn(),
    },
    flowPaneAdapter: {
      screenToFlowCoordinate: vi.fn((point) => point),
      getViewport: vi.fn(() => ({ x: 0, y: 0, zoom: 1 })),
      setViewport: vi.fn(),
      setCenter: vi.fn(),
      selectOnlyNode: vi.fn(() => true),
    },
    openImageUrlPreview: vi.fn(),
    ui,
    feedback,
  }
  return { options, graph, feedback, ui }
}

describe('useCanvasImageWorkflow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.executionOptions = null
    resetWorkspaceSelectionStore()
    getWorkspaceSelectionStore().select('ws_test123')
    mocks.useCanvasImageTools.mockReturnValue({
      viewModel: {
        active: { nodeId: ref(null) },
        locks: { isChromeHidden: ref(false), isCanvasLocked: ref(false) },
        chrome: ref(null),
        retouch: { isApplying: ref(false) },
        nodeBindings: { stageFor: vi.fn(), actionFor: vi.fn() },
      },
      commands: {
        lifecycle: {
          enter: vi.fn(() => true),
          cancel: vi.fn(),
          commit: vi.fn(() => true),
          discardNodeEdits: vi.fn(),
        },
        editor: {
          apply: vi.fn(async () => undefined),
          onCanvasWheel: vi.fn(),
          focusAndShowChrome: vi.fn(),
          scheduleChromeAnchorUpdate: vi.fn(),
        },
        chrome: {},
      },
    })
  })

  afterEach(() => {
    resetWorkspaceSelectionStore()
    resetMediaWorkspaceIdResolver()
  })

  it('owns direct action planning and execution behind the workflow intent seam', async () => {
    const { options } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const handlers = workflow.commands.interactions

    handlers.onFlowImageDirectAction('image-1', IMAGE_NODE_ACTION.cutout)
    await vi.waitFor(() => expect(mocks.execution.executeImageWorkflow).toHaveBeenCalledOnce())

    expect(mocks.planner.planImageDirectAction).toHaveBeenCalledWith(
      'image-1',
      IMAGE_NODE_ACTION.cutout,
      {
        prompt: '',
        outputCount: 1,
      },
    )
    expect(options.ui.panels.clear).toHaveBeenCalledOnce()
  })

  it('passes strength intent through the same direct action workflow path', async () => {
    const { options } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const handlers = workflow.commands.interactions

    handlers.onFlowImageDirectAction('image-1', IMAGE_NODE_ACTION.variations)
    await vi.waitFor(() => expect(mocks.execution.executeImageWorkflow).toHaveBeenCalledOnce())

    expect(mocks.planner.planImageDirectAction).toHaveBeenCalledWith(
      'image-1',
      IMAGE_NODE_ACTION.variations,
      {
        strength: 50,
        prompt: '',
        outputCount: 1,
      },
    )
  })

  it('routes quick and Composer submissions through the same selected workflow', async () => {
    const { options } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const handlers = workflow.commands.interactions

    handlers.onFlowImageDirectAction(
      'image-1',
      IMAGE_NODE_ACTION.variations,
      'soft watercolor',
      4,
      72,
    )
    await vi.waitFor(() => expect(mocks.execution.executeImageWorkflow).toHaveBeenCalledOnce())
    expect(mocks.planner.planImageDirectAction).toHaveBeenLastCalledWith(
      'image-1',
      IMAGE_NODE_ACTION.variations,
      { prompt: 'soft watercolor', outputCount: 4, strength: 72 },
    )
  })

  it('routes text Prompt intent through planning and execution with normalized inputs', async () => {
    const { options } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const handlers = workflow.commands.interactions

    handlers.onFlowPromptGenerate('prompt-1', 'Draw this', 2, 'unsupported' as never)
    await vi.waitFor(() => expect(mocks.execution.executeImageWorkflow).toHaveBeenCalledOnce())

    expect(mocks.planner.planGeneration).toHaveBeenCalledWith(
      'prompt-1',
      [],
      'Draw this',
      2,
      'auto',
    )
  })

  it('submits linked and uploaded Prompt inputs as one ordered reference set', async () => {
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
    const handlers = workflow.commands.interactions

    handlers.onFlowPromptGenerate('prompt-1', 'Remix', 3, '16:9')
    await vi.waitFor(() => expect(mocks.execution.executeImageWorkflow).toHaveBeenCalledOnce())

    expect(mocks.planner.planGeneration).toHaveBeenCalledWith(
      'prompt-1',
      [
        { url: 'https://example.com/linked.png', ratio: 0.5 },
        { url: 'https://example.com/uploaded.png', ratio: 1.5 },
      ],
      'Remix',
      3,
      '16:9',
    )
  })

  it('allows an empty optional user prompt', async () => {
    const { options } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const handlers = workflow.commands.interactions

    handlers.onFlowPromptGenerate('prompt-1', '', 1, 'auto')
    await vi.waitFor(() => expect(mocks.execution.executeImageWorkflow).toHaveBeenCalledOnce())
    expect(mocks.planner.planGeneration).toHaveBeenCalledWith('prompt-1', [], '', 1, 'auto')
  })

  it('keeps Prompt generation disabled during provider degradation', () => {
    vi.useFakeTimers()
    try {
      const { options, feedback } = createOptions()
      const workflow = useCanvasImageWorkflow(options as never)
      const handlers = workflow.commands.interactions

      mocks.executionOptions?.onWarning(i18n.global.t('canvas.execution.generationUnavailable'))
      handlers.onFlowPromptGenerate('prompt-1', 'Keep editing', 1, 'auto')

      expect(feedback.taskExecution.warning).toHaveBeenCalledWith(
        i18n.global.t('canvas.execution.generationUnavailable'),
      )
      expect(mocks.planner.planGeneration).not.toHaveBeenCalled()
      expect(mocks.execution.executeImageWorkflow).not.toHaveBeenCalled()
    } finally {
      vi.clearAllTimers()
      vi.useRealTimers()
    }
  })

  it('keeps local editor and outpaint intents out of Task submission', () => {
    const { options } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const handlers = workflow.commands.interactions

    handlers.onFlowImageDirectAction('image-1', IMAGE_NODE_ACTION.retouch)
    handlers.onFlowImageDirectAction('image-1', IMAGE_NODE_ACTION.outpaint)

    const imageTools = mocks.useCanvasImageTools.mock.results[0]?.value
    expect(imageTools.commands.lifecycle.enter).toHaveBeenCalledWith('image-1', 'adjust')
    expect(imageTools.commands.editor.focusAndShowChrome).toHaveBeenCalledWith('image-1')
    expect(mocks.planner.planImageDirectAction).not.toHaveBeenCalled()
    expect(mocks.execution.executeImageWorkflow).not.toHaveBeenCalled()
  })

  it('wires Image Tool Session without exposing fill execution internals', () => {
    const { options, graph, feedback } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)

    expect(mocks.useCanvasImageTools).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: graph.nodes,
        currentBoardId: graph.currentBoardId,
        connectionAdapter: graph.connectionAdapter,
        isCanvasInteractionLocked: graph.isCanvasInteractionLocked,
        isGenerationAvailable: workflow.commands.isGenerationAvailable,
        feedback: feedback.imageTools,
      }),
    )
    workflow.commands.interactions.onFlowImageContextMenu('image-1', new MouseEvent('contextmenu'))
    expect(options.flowPaneAdapter.selectOnlyNode).toHaveBeenCalledWith('image-1')
    const imageToolOptions = mocks.useCanvasImageTools.mock.calls[0]?.[0]
    expect(imageToolOptions).not.toHaveProperty('updateOutputCount')
  })

  it('owns fusion mutations and pending-result reattachment on the workflow runtime', () => {
    const { options, graph } = createOptions()
    const workflow = useCanvasImageWorkflow(options as never)
    const handlers = workflow.commands.interactions

    handlers.onFlowFusionPromptGenerate('fusion-1', 'Blend', 3, '1:1')
    workflow.reattachPendingImageResults()

    expect(graph.updateSnapshotNodeParam).toHaveBeenCalledTimes(3)
    expect(graph.markNodeExecutionDirty).toHaveBeenCalledTimes(3)
    expect(mocks.planner.planFusionGeneration).toHaveBeenCalledWith('fusion-1', 'Blend', 3, '1:1')
    expect(mocks.execution.reattachPendingImageResults).toHaveBeenCalledOnce()
  })
})
