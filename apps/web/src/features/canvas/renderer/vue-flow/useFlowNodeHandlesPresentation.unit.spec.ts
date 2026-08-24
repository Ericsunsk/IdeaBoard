import { ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

type ProximityOptions = {
  nodeId: string
  direction: 'input' | 'output'
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

describe('useFlowNodeHandlesPresentation', () => {
  afterEach(() => {
    proximityOptions.length = 0
    vi.restoreAllMocks()
  })

  it('registers only node identity and direction with the proximity coordinator', () => {
    const presentation = useFlowNodeHandlesPresentation({
      nodeId: () => 'image-1',
      kind: () => 'image',
      showHandles: () => true,
      disabled: () => false,
    })

    expect(proximityOptions).toMatchObject([
      { nodeId: 'image-1', direction: 'input' },
      { nodeId: 'image-1', direction: 'output' },
    ])
    expect(presentation.value.aggregates.every((aggregate) => !('setElement' in aggregate))).toBe(
      true,
    )
  })
})
