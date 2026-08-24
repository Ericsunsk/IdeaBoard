import { describe, expect, it } from 'vitest'
import type { Edge, Node } from '@vue-flow/core'
import { projectVisibleCanvasGraph } from './canvasVisibleGraphProjection'

function node(id: string, x: number, y = 0): Node {
  return { id, position: { x, y }, data: {} }
}

function edge(id: string, source: string, target: string): Edge {
  return { id, source, target }
}

describe('projectVisibleCanvasGraph', () => {
  it('projects only the viewport window while retaining connected edges', () => {
    const graph = {
      nodes: [node('near', 0), node('near-target', 300), node('far', 10_000)],
      edges: [edge('near-edge', 'near', 'near-target'), edge('far-edge', 'near', 'far')],
    }

    const projection = projectVisibleCanvasGraph(graph, {
      viewport: { x: 0, y: 0, zoom: 1 },
      viewportSize: { width: 900, height: 600 },
      overscanPx: 0,
    })

    expect(projection.nodes.map(({ id }) => id)).toEqual(['near', 'near-target'])
    expect(projection.edges.map(({ id }) => id)).toEqual(['near-edge'])
  })

  it('includes an active execution node outside the viewport', () => {
    const far = node('running', 10_000)
    const projection = projectVisibleCanvasGraph(
      { nodes: [far], edges: [] },
      {
        viewport: { x: 0, y: 0, zoom: 1 },
        viewportSize: { width: 900, height: 600 },
        overscanPx: 0,
        alwaysIncludeNodeIds: new Set(['running']),
      },
    )

    expect(projection.nodes).toEqual([far])
  })
})

