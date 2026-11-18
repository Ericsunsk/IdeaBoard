import type { Edge, Node } from '@vue-flow/core'
import { describe, expect, it, vi } from 'vitest'
import { effectScope, shallowRef } from 'vue'
import type { FlowNodeData } from '../../../core/nodes/flowTypes'
import { useFlowCanvasNodeAdapter } from './useFlowCanvasNodeAdapter'
import type {
  CanvasGraphCommitPatch,
  CanvasGraphEdgeRecord,
  CanvasGraphNodeRecord,
} from '../../../core/graph'
import {
  createCanvasGraphCommitProjectionController,
  projectCanvasGraphCommit,
  projectCanvasNodeReplacement,
  type CanvasFlowGraphProjection,
} from './canvasGraphCommitProjection'

function imageRecord(id: number, sourceImageUrl: string): CanvasGraphNodeRecord {
  return {
    id: `image-${id}`,
    kind: 'image',
    isSelected: false,
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
    isSelected: false,
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

describe('projectCanvasNodeReplacement', () => {
  it('retains live previews without carrying other runtime data into a full replacement', () => {
    const localUpload = { previewUrl: 'blob:local-image' }
    const libraryImportPreviewUrl = 'blob:library-image'
    const snapshotNode = imageRecord(1, 'managed-image').snapshot
    const current = {
      ...flowNode('image-1'),
      data: {
        kind: 'image',
        localUpload,
        libraryImportPreviewUrl,
        runtimeOnly: 'old-runtime-data',
        snapshotNode: imageRecord(1, 'old-image').snapshot,
      },
    }
    const next = {
      ...flowNode('image-1'),
      data: { kind: 'image', title: 'new-title', snapshotNode },
    }

    const projected = projectCanvasNodeReplacement({ current: [current], next: [next] })[0]!

    expect(projected.data).toEqual({ ...next.data, localUpload, libraryImportPreviewUrl })
    expect(projected.data.snapshotNode).toBe(snapshotNode)
    expect(next.data).not.toHaveProperty('localUpload')
    expect(snapshotNode).not.toHaveProperty('localUpload')
    expect(current.data.snapshotNode).toMatchObject({ sourceImageUrl: 'old-image' })

    const incremental = projectCanvasGraphCommit({
      current: { nodes: [current], edges: [] },
      patch: patch({
        nodes: {
          records: [
            {
              id: 'image-1',
              before: imageRecord(1, 'old-image'),
              after: imageRecord(1, 'managed-image'),
            },
          ],
          order: [],
        },
      }),
    })
    expect(incremental.graph.nodes[0]!.data.runtimeOnly).toBe('old-runtime-data')
  })

  it.each(['detached', 'different-kind', 'removed', 'cleared'] as const)(
    'does not restore previews after %s',
    (state) => {
      const next = { ...flowNode('image-1'), data: { kind: 'image' } }
      const current = {
        ...flowNode('image-1'),
        data: {
          kind: state === 'different-kind' ? 'prompt' : 'image',
          ...(state === 'cleared'
            ? {}
            : {
                localUpload: { previewUrl: 'blob:local-image' },
                libraryImportPreviewUrl: 'blob:library-image',
              }),
        },
      }
      const projected = projectCanvasNodeReplacement({
        current: state === 'detached' ? null : [current],
        next: state === 'removed' ? [] : [next],
      })

      expect(projected).toEqual(state === 'removed' ? [] : [next])
    },
  )
})

describe('projectCanvasGraphCommit', () => {
  it('keeps a dropped local image visible when its position commits during upload', () => {
    const before = imageRecord(1, '')
    const localUpload = { previewUrl: 'blob:uploading-local-image' }
    const currentNode = shallowRef<Node>({
      ...flowNode('image-1'),
      data: { kind: 'image', snapshotNode: before.snapshot, localUpload },
    })
    const scope = effectScope()
    try {
      const presentation = scope.run(() =>
        useFlowCanvasNodeAdapter({
          data: () => currentNode.value.data as FlowNodeData,
          uiState: () => undefined,
          imageDirectGenerating: () => false,
          outpaintModeActive: () => false,
          isRetouchActive: () => false,
        }),
      )!
      expect(presentation.displayImage.value?.url).toBe(localUpload.previewUrl)
      const result = projectCanvasGraphCommit({
        current: { nodes: [currentNode.value], edges: [] },
        patch: patch({
          nodes: {
            records: [
              {
                id: 'image-1',
                before,
                after: { ...before, snapshot: { ...before.snapshot, x: 240, y: 160 } },
              },
            ],
            order: [],
          },
        }),
      })
      currentNode.value = result.graph.nodes[0]!
      expect(presentation.displayImage.value?.url).toBe(localUpload.previewUrl)
      expect(presentation.localUpload.value).toEqual(localUpload)
      expect(currentNode.value.position).toEqual({ x: 240, y: 160 })
    } finally {
      scope.stop()
    }
  })

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
    } satisfies Partial<import('@vue-flow/core').GraphNode> as Node
    const result = projectCanvasGraphCommit({
      current: { nodes: [currentNode], edges: [] },
      patch: patch({
        nodes: {
          records: [
            {
              id: 'image-1',
              before: { ...imageRecord(1, 'old'), isSelected: true },
              after: { ...imageRecord(1, 'new'), isSelected: false },
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
    } satisfies Partial<import('@vue-flow/core').GraphNode>)
  })

  it('keeps the renderer selection across a domain-only node update', () => {
    const currentNode = { ...flowNode('image-1', 'old'), selected: true } satisfies Partial<
      import('@vue-flow/core').GraphNode
    > as Node
    const result = projectCanvasGraphCommit({
      current: { nodes: [currentNode], edges: [] },
      patch: patch({
        nodes: {
          records: [{ id: 'image-1', before: imageRecord(1, 'old'), after: imageRecord(1, 'new') }],
          order: [],
        },
      }),
    })

    expect(result.graph.nodes[0]).toMatchObject({ selected: true } satisfies Partial<
      import('@vue-flow/core').GraphNode
    >)
  })

  it('commits a dragged node snapshot even when Vue Flow already rendered its new position', () => {
    const currentNode = {
      ...flowNode('image-1', 'old'),
      // Vue Flow has applied this movement before the drag transaction commits.
      position: { x: 240, y: 160 },
      dimensions: { width: 280, height: 180 },
      computedPosition: { x: 240, y: 160, z: 0 },
    } as Node
    const before = imageRecord(1, 'old')
    const after = {
      ...before,
      snapshot: { ...before.snapshot, x: 240, y: 160 },
    }

    const result = projectCanvasGraphCommit({
      current: { nodes: [currentNode], edges: [] },
      patch: patch({
        nodes: {
          records: [{ id: 'image-1', before, after }],
          order: [],
        },
      }),
    })

    expect(result.graph.nodes[0]).not.toBe(currentNode)
    expect(result.graph.nodes[0]).toMatchObject({
      position: { x: 240, y: 160 },
      data: { snapshotNode: { x: 240, y: 160 } },
      computedPosition: { x: 240, y: 160, z: 0 },
    })
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
          records: [
            { id: 'connection-1', before: connectionRecord(1), after: connectionRecord(1) },
          ],
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
    const renderer = vi.fn<(projection: { kind: string }) => void>().mockImplementationOnce(() => {
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
