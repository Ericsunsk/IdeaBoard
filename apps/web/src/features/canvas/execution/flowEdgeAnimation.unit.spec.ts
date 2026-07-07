import type { Edge } from '@vue-flow/core'
import { describe, expect, it } from 'vitest'
import { resolveImageInputEdgeAnimation } from './flowEdgeAnimation'

function makeEdge(overrides: Partial<Edge>): Edge {
  return {
    id: 'edge-1',
    source: 'image-1',
    sourceHandle: 'out:image',
    target: 'image-2',
    targetHandle: 'in:image',
    animated: false,
    ...overrides,
  }
}

describe('flowEdgeAnimation', () => {
  it('projects image input edge animation for targeted nodes only', () => {
    const activeTarget = makeEdge({ id: 'edge-active', target: 'image-2' })
    const inactiveTarget = makeEdge({ id: 'edge-inactive', target: 'image-3' })
    const promptInput = makeEdge({
      id: 'edge-prompt',
      target: 'fusion-1',
      targetHandle: 'in:prompt',
    })

    const result = resolveImageInputEdgeAnimation(
      [activeTarget, inactiveTarget, promptInput],
      ['image-2'],
      true,
    )

    expect(result.didChange).toBe(true)
    expect(result.edges).toEqual([
      expect.objectContaining({ id: 'edge-active', animated: true }),
      inactiveTarget,
      promptInput,
    ])
  })

  it('preserves edge array identity when no animation changes are needed', () => {
    const edge = makeEdge({ animated: true })
    const edges = [edge]

    const result = resolveImageInputEdgeAnimation(edges, ['image-2'], true)

    expect(result.didChange).toBe(false)
    expect(result.edges).toBe(edges)
    expect(result.edges[0]).toBe(edge)
  })
})
