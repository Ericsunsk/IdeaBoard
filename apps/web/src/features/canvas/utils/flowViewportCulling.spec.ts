import { afterEach, describe, expect, it, vi } from 'vitest'
import { cullFlowGraphForViewport, isCanvasFlowCullingEnabled } from './flowViewportCulling'

type TestNode = {
  id: string
  position: { x: number; y: number }
  data: { snapshotNode: { width: number; height: number } }
}

type TestEdge = {
  id: string
  source: string
  target: string
}

function makeNode(id: string, x: number, y: number): TestNode {
  return {
    id,
    position: { x, y },
    data: { snapshotNode: { width: 100, height: 100 } },
  }
}

describe('flowViewportCulling', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('mounts only nodes that intersect the viewport plus overscan', () => {
    const graph = {
      nodes: [
        makeNode('inside', 20, 20),
        makeNode('partial-left', -80, 40),
        makeNode('outside', 520, 20),
      ],
      edges: [] as TestEdge[],
    }

    const result = cullFlowGraphForViewport(graph, {
      viewport: { x: 0, y: 0, zoom: 1 },
      viewportSize: { width: 500, height: 300 },
      overscanPx: 0,
    })

    expect(result.nodes.map((node) => node.id)).toEqual(['inside', 'partial-left'])
    expect(result.visibleNodeIds.has('outside')).toBe(false)
  })

  it('filters edges unless both endpoints are visible', () => {
    const graph = {
      nodes: [makeNode('source', 20, 20), makeNode('target', 180, 20), makeNode('hidden', 700, 20)],
      edges: [
        { id: 'visible-edge', source: 'source', target: 'target' },
        { id: 'hidden-source-edge', source: 'hidden', target: 'target' },
        { id: 'hidden-target-edge', source: 'source', target: 'hidden' },
      ],
    }

    const result = cullFlowGraphForViewport(graph, {
      viewport: { x: 0, y: 0, zoom: 1 },
      viewportSize: { width: 500, height: 300 },
      overscanPx: 0,
    })

    expect(result.edges.map((edge) => edge.id)).toEqual(['visible-edge'])
  })

  it('treats VITE_CANVAS_FLOW_ENABLE_CULLING=false as disabled', () => {
    vi.stubEnv('VITE_CANVAS_FLOW_ENABLE_CULLING', 'true')

    expect(isCanvasFlowCullingEnabled()).toBe(true)

    vi.stubEnv('VITE_CANVAS_FLOW_ENABLE_CULLING', 'false')

    expect(isCanvasFlowCullingEnabled()).toBe(false)
  })
})
