import { i18n } from '../../../../shared/i18n'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useImageWorkflowExecution } from './useImageWorkflowExecution'
import { CanvasBillingPreflightError, createPendingImageExecutionCore } from '../execution'

const mocks = vi.hoisted(() => {
  const plan = {
    kind: 'generation',
    sourceNodeId: 'prompt-1',
    source: { url: '', ratio: 1 },
    referenceImages: [],
    prompt: 'draw this',
    outputCount: 1,
    aspectRatio: 'auto',
    resultOptions: {},
  }
  return {
    planner: {
      planImageDirectAction: vi.fn(() => plan),
      planOutpaintAction: vi.fn(() => plan),
      planImageInpaintAction: vi.fn(() => plan),
      planImageGeneration: vi.fn(() => plan),
      planImageGenerationFromSource: vi.fn(() => plan),
      planGeneration: vi.fn(() => plan),
      planFusionGeneration: vi.fn(() => plan),
    },
    workflow: {
      executeImageWorkflow: vi.fn(async () => undefined),
      isNodeExecutionActive: vi.fn(() => false),
      reattachPendingImageResults: vi.fn(),
    },
    executionOptions: null as null | {
      onWarning: (message: string) => void
      onError?: (error: unknown) => void
    },
  }
})

vi.mock('../execution', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../execution')>()
  return {
    ...actual,
    createImageExecutionPlanner: vi.fn(() => mocks.planner),
    useImageWorkflowExecution: vi.fn((options) => {
      mocks.executionOptions = options as {
        onWarning: (message: string) => void
        onError?: (error: unknown) => void
      }
      return mocks.workflow
    }),
  }
})

function createRuntime() {
  const feedback = {
    succeeded: vi.fn<(message: string) => void>(),
    warning: vi.fn<(message: string) => void>(),
    insufficientCredits: vi.fn<() => void>(),
  }
  const runtimeOptions = {
    nodes: ref([]),
    edges: ref([]),
    readCanvasGraphStore: vi.fn(() => ({})),
    flushAutosave: vi.fn(async () => undefined),
    refreshNodeInternals: vi.fn(),
    executionCore: createPendingImageExecutionCore(),
    cancelTasks: vi.fn(),
    feedback,
    // Workspace gate stubbed available: selection-store coverage lives in
    // the adapter spec; runtime specs stay hermetic.
    isWorkspaceAvailable: () => true,
  }
  const runtime = useImageWorkflowExecution(runtimeOptions as never)
  return { runtime, feedback }
}

describe('useImageWorkflowExecution', () => {
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

    mocks.executionOptions?.onWarning(i18n.global.t('canvas.execution.generationUnavailable'))

    expect(runtime.isGenerationAvailable()).toBe(false)
    await runtime.runGeneration('prompt-1', [], 'draw this', 1, 'auto')

    expect(mocks.workflow.executeImageWorkflow).not.toHaveBeenCalled()
    expect(feedback.warning).toHaveBeenCalledWith(
      i18n.global.t('canvas.execution.generationUnavailable'),
    )

    vi.setSystemTime(15_001)

    expect(runtime.isGenerationAvailable()).toBe(true)
    await runtime.runGeneration('prompt-1', [], 'draw this', 1, 'auto')

    expect(mocks.planner.planGeneration).toHaveBeenCalledWith(
      'prompt-1',
      [],
      'draw this',
      1,
      'auto',
    )
    expect(mocks.workflow.executeImageWorkflow).toHaveBeenCalledTimes(1)
  })

  it('routes typed wallet insufficiency to the actionable billing feedback intent', () => {
    const { feedback } = createRuntime()

    mocks.executionOptions?.onError?.(
      new CanvasBillingPreflightError('insufficient-credits', 'Idea不足'),
    )

    expect(feedback.insufficientCredits).toHaveBeenCalledOnce()
    expect(feedback.warning).not.toHaveBeenCalled()
  })

  it('disables task actions with guidance when no workspace is selected', async () => {
    const feedback = {
      succeeded: vi.fn<(message: string) => void>(),
      warning: vi.fn<(message: string) => void>(),
      insufficientCredits: vi.fn<() => void>(),
    }
    const gated = useImageWorkflowExecution({
      nodes: ref([]),
      edges: ref([]),
      readCanvasGraphStore: vi.fn(() => ({})),
      flushAutosave: vi.fn(async () => undefined),
      refreshNodeInternals: vi.fn(),
      executionCore: createPendingImageExecutionCore(),
      cancelTasks: vi.fn(),
      feedback,
      isWorkspaceAvailable: () => false,
    } as never)

    expect(gated.isGenerationAvailable()).toBe(false)
    await gated.runGeneration('prompt-1', [], 'draw this', 1, 'auto')

    expect(mocks.workflow.executeImageWorkflow).not.toHaveBeenCalled()
    expect(feedback.warning).toHaveBeenCalledWith(
      'Select a workspace before running image tasks.',
    )
  })
})
