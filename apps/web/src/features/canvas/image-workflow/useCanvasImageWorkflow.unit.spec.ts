import { describe, expect, it, vi } from 'vitest'
import { useCanvasImageWorkflow } from './useCanvasImageWorkflow'

const mocks = vi.hoisted(() => ({
  useCanvasImageTools: vi.fn(),
}))

vi.mock('../image-tools', () => ({
  useCanvasImageTools: mocks.useCanvasImageTools,
}))

describe('useCanvasImageWorkflow', () => {
  it('passes the flow-pane connection adapter to image tools without a raw connection command', () => {
    const connectionAdapter = { connectIntent: vi.fn() }
    const workflow = useCanvasImageWorkflow({
      graph: {
        connectionAdapter,
      },
      task: {},
      flowPaneAdapter: {},
      feedback: {},
    } as never)

    workflow.createImageTools({} as never)

    expect(mocks.useCanvasImageTools).toHaveBeenCalledWith(
      expect.objectContaining({ connectionAdapter }),
    )
    expect(mocks.useCanvasImageTools.mock.calls[0]?.[0]).not.toHaveProperty('connectImageNodes')
  })
})
