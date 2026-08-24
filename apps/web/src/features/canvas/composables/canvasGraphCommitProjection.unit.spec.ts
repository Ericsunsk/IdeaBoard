import type { Edge, Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import type {
  CanvasGraphCommitPatch,
  CanvasGraphEdgeRecord,
  CanvasGraphNodeRecord,
} from '../graph-runtime'
import {
  createCanvasGraphCommitProjectionController,
  projectCanvasGraphCommit,
  type CanvasFlowGraphProjection,
} from './canvasGraphCommitProjection'

function imageRecord(id: number, sourceImageUrl: string): CanvasGraphNodeRecord {
  return {
    id: `image-${id}`,
    kind: 'image',
    selected: false,
    snapshot: {
      id,
      type: 'image-node',
      x: id * 100,
      y: 40,
      width: 280,
      sourceImageUrl,
      sourceImageRatio: 1,
      outputStatus: 'idle',
      outputImages: [],
      outputCount: 1,
      outputResolution: '1k',
    },
  }
}

function connectionRecord(id: number): CanvasGraphEdgeRecord {
  return {
    id: `connection-${id}`,
    selected: false,
    connection: {
      id,
      fromNodeType: 'image',
      fromNodeId: 1,
      fromPort: 'image',
      toNodeType: 'image',
      toNodeId: 2,
      toPort: 'image',
    },
  }
}

function patch(input: Partial<CanvasGraphCommitPatch> = {}): CanvasGraphCommitPatch {
  return {
    nodes: { records: [], order: [] },
    edges: { records: [], order: [] },
    groups: { records: [], order: [] },
    ...input,
  }
}

function flowNode(id: string, url = ''): Node {
  return {
    id,
    type: 'canvas',
    position: { x: 0, y: 0 },
    data: { snapshotNode: { sourceImageUrl: url } },
  }
}

describe('projectCanvasGraphCommit', () => {
  it('replaces only the changed node and preserves all other Flow identities', () => {
    const unchangedNode = flowNode('image-2', 'unchanged')
    const changedNode = {
      ...flowNode('image-1', 'old'),
      dimensions: { width: 280, height: 180 },
      handleBounds: { source: [], target: [] },
    } as Node
    const current: CanvasFlowGraphProjection = {
      nodes: [changedNode, unchangedNode],
      edges: [{ id: 'connection-1', source: 'image-1', target: 'image-2' }],
    }
    const result = projectCanvasGraphCommit({
      current,
      patch: patch({
        nodes: {
          records: [{ id: 'image-1', before: imageRecord(1, 'old'), after: imageRecord(1, 'new') }],
          order: [],
        },
      }),
    })

    expect(result.kind).toBe('incremental')
    expect(result.needsRecovery).toBe(false)
    expect(result.graph.nodes).not.toBe(current.nodes)
    expect(result.graph.nodes[0]).not.toBe(changedNode)
    expect(result.graph.nodes[0]).toMatchObject({
      id: 'image-1',
      position: { x: 100, y: 40 },
      dimensions: { width: 280 },
      handleBounds: { source: [], target: [] },
      data: { snapshotNode: { sourceImageUrl: 'new' } },
    })
    expect(result.graph.nodes[1]).toBe(unchangedNode)
    expect(result.graph.edges).toBe(current.edges)
    expect(result.changedNodeIds).toEqual(['image-1'])
  })

  it('clears renderer selection only when the canonical selection changes', () => {
    const currentNode = {
      ...flowNode('image-1', 'old'),
      selected: true,
      computedPosition: { x: 100, y: 40, z: 0 },
      handleBounds: { source: [], target: [] },
    } as Node
    const result = projectCanvasGraphCommit({
      current: { nodes: [currentNode], edges: [] },
      patch: patch({
        nodes: {
          records: [
            {
              id: 'image-1',
              before: { ...imageRecord(1, 'old'), selected: true },
              after: { ...imageRecord(1, 'new'), selected: false },
            },
          ],
          order: [],
        },
      }),
    })

    expect(result.graph.nodes[0]).toMatchObject({
      selected: false,
      computedPosition: { x: 100, y: 40, z: 0 },
      handleBounds: { source: [], target: [] },
    })
  })

  it('keeps the renderer selection across a domain-only node update', () => {
    const currentNode = { ...flowNode('image-1', 'old'), selected: true } as Node
    const result = projectCanvasGraphCommit({
      current: { nodes: [currentNode], edges: [] },
      patch: patch({
        nodes: {
          records: [{ id: 'image-1', before: imageRecord(1, 'old'), after: imageRecord(1, 'new') }],
          order: [],
        },
      }),
    })

    expect(result.graph.nodes[0]).toMatchObject({ selected: true })
  })

  it('keeps execution animation out of canonical edge projection', () => {
    const currentEdge = {
      id: 'connection-1',
      source: 'image-1',
      target: 'image-2',
      animated: true,
    } as Edge
    const result = projectCanvasGraphCommit({
      current: { nodes: [], edges: [currentEdge] },
      patch: patch({
        edges: {
          records: [{ id: 'connection-1', before: connectionRecord(1), after: connectionRecord(1) }],
          order: [],
        },
      }),
    })

    expect(result.graph.edges[0]).not.toHaveProperty('animated')
  })

  it('marks a missing current record for a recoverable full projection', () => {
    const result = projectCanvasGraphCommit({
      current: { nodes: [], edges: [] },
      patch: patch({
        nodes: {
          records: [{ id: 'image-1', before: imageRecord(1, 'old'), after: imageRecord(1, 'new') }],
          order: [],
        },
      }),
    })

    expect(result.needsRecovery).toBe(true)
  })

  it('applies changed-ID order patches without rebuilding unchanged node records', () => {
    const first = flowNode('image-1')
    const second = flowNode('image-2')
    const result = projectCanvasGraphCommit({
      current: { nodes: [first, second], edges: [] },
      patch: patch({
        nodes: {
          records: [],
          order: [{ id: 'image-2', after: { nextId: 'image-1' } }],
        },
      }),
    })

    expect(result.graph.nodes).toEqual([second, first])
    expect(result.graph.nodes[0]).toBe(second)
    expect(result.graph.nodes[1]).toBe(first)
  })
})

describe('Canvas Graph commit projection recovery', () => {
  it('retains canonical facts when rendering fails and recovers with a full projection', () => {
    const canonicalGraph: CanvasFlowGraphProjection = {
      nodes: [flowNode('image-1', 'canonical')],
      edges: [],
    }
    const renderer = vi
      .fn<(projection: { kind: string }) => void>()
      .mockImplementationOnce(() => {
        throw new Error('renderer failed')
      })
    const onFailure = vi.fn()
    let scheduledRecovery: (() => void) | undefined
    const controller = createCanvasGraphCommitProjectionController({
      projectFull: () => canonicalGraph,
      render: renderer,
      onFailure,
      scheduleMicrotask: (task) => {
        scheduledRecovery = task
      },
    })
    const current: CanvasFlowGraphProjection = { nodes: [flowNode('image-1', 'old')], edges: [] }

    expect(() =>
      controller.apply({
        current,
        patch: patch({
          nodes: {
            records: [
              { id: 'image-1', before: imageRecord(1, 'old'), after: imageRecord(1, 'canonical') },
            ],
            order: [],
          },
        }),
      }),
    ).toThrow('renderer failed')

    expect(controller.needsRecovery()).toBe(true)
    expect(onFailure).toHaveBeenCalledOnce()
    expect(canonicalGraph.nodes[0]).toMatchObject({ id: 'image-1' })

    scheduledRecovery?.()
    expect(renderer).toHaveBeenLastCalledWith(expect.objectContaining({ kind: 'full' }))
    expect(controller.needsRecovery()).toBe(false)
  })
})
