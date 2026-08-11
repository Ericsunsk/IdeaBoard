import { describe, expect, it, vi } from 'vitest'
import { createCanvasFlowPaneConnectionAdapter } from './index'

describe('flow-pane connection public seam', () => {
  it('keeps Vue Flow bindings and programmatic intent behind one adapter', () => {
    const canConnect = vi.fn(() => true)
    const connect = vi.fn(() => true)
    const adapter = createCanvasFlowPaneConnectionAdapter({ canConnect, connect })

    const connection = {
      source: 'prompt-1',
      target: 'image-2',
      sourceHandle: 'out:prompt',
      targetHandle: 'in:image',
    }
    const intent = {
      sourceNodeId: 'prompt-1' as const,
      targetNodeId: 'image-2' as const,
      sourcePort: 'prompt' as const,
      targetPort: 'image' as const,
    }

    expect(Object.keys(adapter.vueFlowBindings)).toEqual(['isValidConnection', 'onConnect'])
    expect(adapter.vueFlowBindings.isValidConnection(connection)).toBe(true)
    adapter.vueFlowBindings.onConnect(connection)
    expect(adapter.connectIntent(intent)).toBe(true)

    expect(canConnect).toHaveBeenCalledWith(connection)
    expect(connect).toHaveBeenNthCalledWith(1, connection)
    expect(connect).toHaveBeenNthCalledWith(2, {
      source: 'prompt-1',
      target: 'image-2',
      sourceHandle: 'out:prompt',
      targetHandle: 'in:image',
    })
  })
})
