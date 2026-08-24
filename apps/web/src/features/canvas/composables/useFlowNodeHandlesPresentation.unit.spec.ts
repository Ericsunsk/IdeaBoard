import { ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

type ProximityOptions = {
  direction: 'input' | 'output'
  getAnchorRect: () => DOMRect | null
}

const proximityOptions: ProximityOptions[] = []

vi.mock('./useCanvasHandleProximity', () => ({
  useCanvasHandleProximity: (options: ProximityOptions) => {
    proximityOptions.push(options)
    return {
      isActive: ref(false),
      point: ref({ x: 0, y: 0 }),
    }
  },
}))

import { useFlowNodeHandlesPresentation } from './useFlowNodeHandlesPresentation'

describe('useFlowNodeHandlesPresentation benchmark metrics', () => {
  afterEach(() => {
    proximityOptions.length = 0
    delete window.__canvasInteractionBenchmarkCounters
    vi.restoreAllMocks()
  })

  it('counts an actual handle layout read but not a missing element', () => {
    window.__canvasInteractionBenchmarkCounters = {}
    const presentation = useFlowNodeHandlesPresentation({
      kind: () => 'image',
      showHandles: () => true,
      disabled: () => false,
    })
    const outputOptions = proximityOptions.find((options) => options.direction === 'output')

    expect(outputOptions?.getAnchorRect()).toBeNull()
    expect(
      window.__canvasInteractionBenchmarkCounters.handleGeometryLayoutReadCount,
    ).toBeUndefined()

    const handle = document.createElement('div')
    const bounds = handle.getBoundingClientRect()
    vi.spyOn(handle, 'getBoundingClientRect').mockReturnValue(bounds)
    presentation.value.aggregates.find((item) => item.direction === 'output')?.setElement(handle)

    expect(outputOptions?.getAnchorRect()).toBe(bounds)
    expect(window.__canvasInteractionBenchmarkCounters).toMatchObject({
      handleGeometryLayoutReadCount: 1,
      runtimeTimingMs: {
        'handle-proximity.layout-read': { count: 1 },
      },
    })
  })
})
