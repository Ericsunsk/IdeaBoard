import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useFlowCanvasNodeHandlers } from './useFlowCanvasNodeHandlers'

function createHarness() {
  const options = {
    dismissTransientCanvasUi: vi.fn(),
    discardFlowImageToolNodeEdits: vi.fn(),
    deleteNode: vi.fn(),
    updatePromptText: vi.fn(),
    updateNodeParams: vi.fn(),
    resetImage: vi.fn(),
    markFlowExecutionNodeDirty: vi.fn(),
  }
  const handlers = useFlowCanvasNodeHandlers(options)
  return { handlers, options }
}

describe('useFlowCanvasNodeHandlers', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('keeps prompt text commits synchronous but debounces the trailing dirty mark', () => {
    const { handlers, options } = createHarness()

    handlers.onFlowPromptTextUpdate('prompt-1', 'a')
    handlers.onFlowPromptTextUpdate('prompt-1', 'ab')
    handlers.onFlowPromptTextUpdate('prompt-1', 'abc')

    // The controlled textarea still sees every keystroke immediately.
    expect(options.updatePromptText).toHaveBeenCalledTimes(3)
    expect(options.markFlowExecutionNodeDirty).not.toHaveBeenCalled()

    vi.advanceTimersByTime(300)

    expect(options.markFlowExecutionNodeDirty).toHaveBeenCalledTimes(1)
    expect(options.markFlowExecutionNodeDirty).toHaveBeenCalledWith('prompt-1')
  })

  it('debounces dirty marks per node', () => {
    const { handlers, options } = createHarness()

    handlers.onFlowPromptTextUpdate('prompt-1', 'a')
    handlers.onFlowPromptTextUpdate('prompt-2', 'b')

    vi.advanceTimersByTime(300)

    expect(options.markFlowExecutionNodeDirty).toHaveBeenCalledTimes(2)
    expect(options.markFlowExecutionNodeDirty).toHaveBeenCalledWith('prompt-1')
    expect(options.markFlowExecutionNodeDirty).toHaveBeenCalledWith('prompt-2')
  })

  it('keeps param updates on the immediate dirty path', () => {
    const { handlers, options } = createHarness()

    handlers.onFlowNodeParamUpdate('prompt-1', { key: 'outputCount', value: 2 })

    expect(options.markFlowExecutionNodeDirty).toHaveBeenCalledTimes(1)
    expect(options.markFlowExecutionNodeDirty).toHaveBeenCalledWith('prompt-1', {
      includeSelf: true,
    })
  })
})
