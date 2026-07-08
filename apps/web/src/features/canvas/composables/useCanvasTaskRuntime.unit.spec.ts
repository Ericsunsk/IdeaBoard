import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useCanvasTaskRuntime } from './useCanvasTaskRuntime'

const mocks = vi.hoisted(() => {
  const plan = {
    kind: 'text-prompt',
    sourceNodeId: 'prompt-1',
    source: { url: '', ratio: 1 },
    prompt: 'draw this',
    outputCount: 1,
    aspectRatio: 'auto',
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
    workflow: {
      executeImageWorkflow: vi.fn(async () => undefined),
      isNodeDirectActionRunning: vi.fn(() => false),
      reattachPendingImageResults: vi.fn(),
    },
    executionOptions: null as null | { onWarning: (message: string) => void },
  }
})

vi.mock('../execution', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../execution')>()
  return {
    ...actual,
    createImageExecutionPlanner: vi.fn(() => mocks.planner),
    useFlowImageWorkflowExecution: vi.fn((options) => {
      mocks.executionOptions = options as { onWarning: (message: string) => void }
      return mocks.workflow
    }),
  }
})

function createRuntime() {
  const feedback = {
    succeeded: vi.fn<(message: string) => void>(),
    warning: vi.fn<(message: string) => void>(),
  }
  const runtimeOptions = {
    nodes: ref([]),
    edges: ref([]),
    graphAdapter: {},
    createCurrentCanvasGraphStore: vi.fn(() => ({})),
    flushAutosave: vi.fn(async () => undefined),
    currentBoardId: ref('board-a'),
    refreshNodeInternals: vi.fn(),
    feedback,
  }
  const runtime = useCanvasTaskRuntime(runtimeOptions as never)
  return { runtime, feedback }
}

describe('useCanvasTaskRuntime', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(0)
    vi.clearAllMocks()
    mocks.executionOptions = null
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('retries provider availability after the degraded generation cooldown expires', async () => {
    const { runtime, feedback } = createRuntime()

    mocks.executionOptions?.onWarning('provider unavailable')

    expect(runtime.isGenerationAvailable()).toBe(false)
    await runtime.runTextPromptGeneration('prompt-1', 'draw this', 1, 'auto')

    expect(mocks.workflow.executeImageWorkflow).not.toHaveBeenCalled()
    expect(feedback.warning).toHaveBeenCalledWith('provider unavailable')

    vi.setSystemTime(15_001)

    expect(runtime.isGenerationAvailable()).toBe(true)
    await runtime.runTextPromptGeneration('prompt-1', 'draw this', 1, 'auto')

    expect(mocks.planner.planTextPromptGeneration).toHaveBeenCalledWith(
      'prompt-1',
      'draw this',
      1,
      'auto',
    )
    expect(mocks.workflow.executeImageWorkflow).toHaveBeenCalledTimes(1)
  })
})
