import { ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { IMAGE_NODE_ACTION } from '../../core/nodes/imageActionRegistry.ts'
import type { ImageActionIntentRouterOptions } from './imageActionIntentRouter'
import { createImageActionIntentRouter } from './imageActionIntentRouter'

function createOptions() {
  const options: ImageActionIntentRouterOptions = {
    state: { flowRetouchNodeId: ref(null) },
    strength: {
      consumeSuppressedClick: vi.fn(() => false),
      getValue: vi.fn(() => 64),
    },
    ui: {
      openPanel: vi.fn(),
      clearPanel: vi.fn(),
      hideStrengthControl: vi.fn(),
      warn: vi.fn(),
    },
    selection: { selectNode: vi.fn() },
    modes: {
      cancelOutpaint: vi.fn(),
      cancelInpaint: vi.fn(),
      enterOutpaint: vi.fn(() => true),
      enterRetouch: vi.fn(() => true),
      focusRetouch: vi.fn(),
    },
    execution: {
      runImageDirectAction: vi.fn(),
      runFusionGeneration: vi.fn(),
    },
    params: { updateNodeParam: vi.fn() },
  }
  return options
}

describe('createImageActionIntentRouter', () => {
  it('keeps strength actions on the direct execution path', () => {
    const options = createOptions()
    const router = createImageActionIntentRouter(options)

    router.onFlowImageDirectAction('image-1', IMAGE_NODE_ACTION.variations)

    expect(options.execution.runImageDirectAction).toHaveBeenCalledWith(
      'image-1',
      IMAGE_NODE_ACTION.variations,
      { prompt: '', outputCount: 1, strength: 64 },
    )
    expect(options.modes.cancelOutpaint).toHaveBeenCalledOnce()
    expect(options.modes.cancelInpaint).toHaveBeenCalledOnce()
  })

  it('isolates retouch mode transitions from task execution', () => {
    const options = createOptions()
    const router = createImageActionIntentRouter(options)

    router.onFlowImageDirectAction('image-1', IMAGE_NODE_ACTION.retouch)

    expect(options.modes.enterRetouch).toHaveBeenCalledWith('image-1', 'adjust')
    expect(options.modes.focusRetouch).toHaveBeenCalledWith('image-1')
    expect(options.execution.runImageDirectAction).not.toHaveBeenCalled()
  })

  it('persists fusion parameters before starting generation', () => {
    const options = createOptions()
    const router = createImageActionIntentRouter(options)

    router.onFlowFusionPromptGenerate('fusion-1', 'A study', 2, '16:9')

    expect(options.params.updateNodeParam).toHaveBeenNthCalledWith(1, 'fusion-1', {
      key: 'prompt',
      value: 'A study',
    })
    expect(options.params.updateNodeParam).toHaveBeenNthCalledWith(2, 'fusion-1', {
      key: 'outputCount',
      value: 2,
    })
    expect(options.params.updateNodeParam).toHaveBeenNthCalledWith(3, 'fusion-1', {
      key: 'aspectRatio',
      value: '16:9',
    })
    expect(options.execution.runFusionGeneration).toHaveBeenCalledWith(
      'fusion-1',
      'A study',
      2,
      '16:9',
    )
  })
})
