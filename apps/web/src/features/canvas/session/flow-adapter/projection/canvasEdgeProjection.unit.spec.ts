import type { Edge } from '@vue-flow/core'
import { describe, expect, it } from 'vitest'
import {
  projectCanvasRenderEdges,
  toCanvasSemanticEdge,
  toCanvasSemanticEdges,
} from './canvasEdgeProjection'

describe('canvas edge projection', () => {
  it('keeps semantic edges stable while removing render-only animation', () => {
    const semanticEdge: Edge = { id: 'semantic', source: 'a', target: 'b' }
    const renderEdge: Edge = { ...semanticEdge, animated: true }

    expect(toCanvasSemanticEdge(semanticEdge)).toBe(semanticEdge)
    expect(toCanvasSemanticEdges([semanticEdge])).toEqual([semanticEdge])
    expect(toCanvasSemanticEdge(renderEdge)).toEqual(semanticEdge)
    expect(toCanvasSemanticEdge(renderEdge)).not.toBe(renderEdge)
  })

  it('projects animation without mutating or replacing unrelated edges', () => {
    const imageEdge: Edge = {
      id: 'image-input',
      source: 'prompt-1',
      target: 'image-1',
      targetHandle: 'in:image',
    }
    const promptEdge: Edge = {
      id: 'prompt-input',
      source: 'prompt-1',
      target: 'prompt-2',
      targetHandle: 'in:prompt',
    }

    const renderEdges = projectCanvasRenderEdges(
      [imageEdge, promptEdge],
      (edge) => edge.target === 'image-1',
    )

    expect(renderEdges).toEqual([{ ...imageEdge, animated: true }, promptEdge])
    expect(imageEdge).not.toHaveProperty('animated')
    expect(renderEdges[1]).toBe(promptEdge)
    expect(projectCanvasRenderEdges(renderEdges, (edge) => edge.target === 'image-1')).toBe(
      renderEdges,
    )
    expect(projectCanvasRenderEdges([imageEdge], () => false)).toEqual([
      { ...imageEdge, animated: false } satisfies Partial<import('@vue-flow/core').Edge>,
    ])
  })
})
