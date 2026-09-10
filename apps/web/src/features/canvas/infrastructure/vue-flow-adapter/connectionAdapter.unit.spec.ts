import { describe, expect, it, vi } from 'vitest'
import { createCanvasVueFlowConnectionAdapter } from './index'
import { normalizeFlowConnection } from './flowConnectionAdapter.ts'
import type { FlowNodeData } from '../../domain/nodes/flowTypes.ts'

function createLookup(nodes: Record<string, FlowNodeData>) {
  return (nodeId: string) => nodes[nodeId]
}

describe('vue-flow-adapter connection public seam', () => {
  it('keeps Vue Flow bindings and programmatic intent behind one adapter', () => {
    const canConnect = vi.fn(() => true)
    const connect = vi.fn(() => true)
    const adapter = createCanvasVueFlowConnectionAdapter({ canConnect, connect })

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

  it('normalizes the aggregate input handle to the compatible semantic port', () => {
    const normalized = normalizeFlowConnection(
      {
        source: 'image-1',
        target: 'fusion-2',
        sourceHandle: 'out:image',
        targetHandle: 'in:__auto',
      },
      createLookup({
        'image-1': { kind: 'image', nodeType: 'image', title: 'Image' },
        'fusion-2': { kind: 'fusion', nodeType: 'fusion', title: 'Fusion' },
      }),
    )

    expect(normalized).toMatchObject({
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
    })
  })

  it('chooses the next compatible open port when an aggregate target is reused', () => {
    const normalized = normalizeFlowConnection(
      {
        source: 'prompt-2',
        target: 'fusion-1',
        sourceHandle: 'out:prompt',
        targetHandle: 'in:__auto',
      },
      createLookup({
        'prompt-2': { kind: 'prompt', nodeType: 'prompt', title: 'Prompt' },
        'fusion-1': { kind: 'fusion', nodeType: 'fusion', title: 'Fusion' },
      }),
      [
        {
          target: 'fusion-1',
          targetHandle: 'in:image',
        },
      ],
    )

    expect(normalized).toMatchObject({
      sourceHandle: 'out:prompt',
      targetHandle: 'in:prompt',
    })
  })
})
