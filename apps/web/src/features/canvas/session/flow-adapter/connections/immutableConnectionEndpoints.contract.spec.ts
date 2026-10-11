import type { Edge, Node } from '@vue-flow/core'
import { shallowRef } from 'vue'
import { describe, expect, it } from 'vitest'
import {
  createEmptyBoardSnapshot,
  type BoardSnapshot,
  type CanvasConnection,
  type CanvasConnectionEndpoint,
  type ImageNodeItem,
  type PromptNodeItem,
} from '../../../../board'
import { CanvasGraphStore, toCanvasGraphEdgeId } from '../../../core/graph/canvasGraphStore'
import {
  commitCanvasGraphConnectIntent,
  isSameCanvasConnectionEndpoint,
  shouldReplaceCanvasConnection,
  toCanvasConnectionEndpointKey,
} from '../../../core/graph/canvasGraphMutationIntents'
import {
  flowConnectionToCanvasConnection,
  flowConnectionToCanvasConnectionEndpoint,
} from './flowConnectionAdapter.ts'
import { boardSnapshotToFlow, flowToBoardSnapshot } from '../projection/flowSnapshotAdapter.ts'
import { resolveConnectedPromptText } from '../projection/flowNodeSourceResolution'
import { useCanvasVueFlowGraphAdapter } from '../projection/useCanvasVueFlowGraphAdapter'

function imageNode(id: number): ImageNodeItem {
  return {
    id,
    type: 'image-node',
    x: id * 10,
    y: 0,
    width: 280,
    sourceImageUrl: `https://example.com/image-${id}.png`,
    sourceImageRatio: 1,
    outputCount: 1,
    outputStatus: 'ready',
    outputImages: [],
  }
}

function promptNode(id: number, text: string): PromptNodeItem {
  return {
    id,
    type: 'prompt',
    x: id * 10,
    y: 320,
    width: 280,
    title: `Prompt ${id}`,
    text,
    outputCount: 1,
    aspectRatio: 'auto',
    inputImages: [],
  }
}

function connection(
  id: number,
  fromNodeId: number,
  toNodeId: number,
  toPort: CanvasConnection['toPort'] = 'image',
): CanvasConnection {
  return {
    id,
    fromNodeType: 'image',
    fromNodeId,
    fromPort: 'image',
    toNodeType: 'prompt',
    toNodeId,
    toPort,
  }
}

function sampleSnapshot(): BoardSnapshot {
  const snapshot = createEmptyBoardSnapshot()
  snapshot.imageNodes = [imageNode(1), imageNode(2)]
  snapshot.promptNodes = [promptNode(1, 'alpha')]
  snapshot.connections = [connection(1, 1, 1)]
  return snapshot
}

describe('immutable connection endpoints', () => {
  const liveEndpoint: CanvasConnectionEndpoint = {
    fromNodeType: 'image',
    fromNodeId: 1,
    fromPort: 'image',
    toNodeType: 'prompt',
    toNodeId: 1,
    toPort: 'image',
  }

  it('derives endpoint identity verbatim from the live edge handles', () => {
    const endpoint = flowConnectionToCanvasConnectionEndpoint({
      source: 'image-1',
      target: 'prompt-1',
      sourceHandle: 'out:image',
      targetHandle: 'in:image',
    })

    expect(endpoint).toEqual(liveEndpoint)

    const movedTarget: CanvasConnectionEndpoint = { ...liveEndpoint, toNodeId: 2 }
    expect(isSameCanvasConnectionEndpoint(liveEndpoint, movedTarget)).toBe(false)
    expect(toCanvasConnectionEndpointKey(liveEndpoint)).not.toBe(
      toCanvasConnectionEndpointKey(movedTarget),
    )
  })

  it('refuses to infer endpoints without explicit in/out ports', () => {
    expect(
      flowConnectionToCanvasConnectionEndpoint({
        source: 'image-1',
        target: 'prompt-1',
        sourceHandle: 'out:__auto',
        targetHandle: 'in:image',
      }),
    ).toBeNull()
    expect(
      flowConnectionToCanvasConnectionEndpoint({
        source: 'image-1',
        target: 'prompt-1',
        sourceHandle: null,
        targetHandle: null,
      }),
    ).toBeNull()
  })

  it('carries parsed endpoints verbatim onto the canvas connection', () => {
    expect(
      flowConnectionToCanvasConnection(9, {
        source: 'image-1',
        target: 'prompt-1',
        sourceHandle: 'out:image',
        targetHandle: 'in:image',
      }),
    ).toEqual({ ...liveEndpoint, id: 9 })
  })

  it('treats a retargeted connection as a replacement, never a patch', () => {
    const current = connection(1, 1, 1)
    expect(
      shouldReplaceCanvasConnection(current, connection(2, 1, 1), {
        allowsMultipleTargetConnections: false,
      }),
    ).toBe(true)
    expect(
      shouldReplaceCanvasConnection(current, connection(2, 2, 1), {
        allowsMultipleTargetConnections: false,
      }),
    ).toBe(true)
    expect(
      shouldReplaceCanvasConnection(current, connection(2, 2, 1), {
        allowsMultipleTargetConnections: true,
      }),
    ).toBe(false)
    expect(
      shouldReplaceCanvasConnection(current, connection(2, 1, 2), {
        allowsMultipleTargetConnections: false,
      }),
    ).toBe(false)
  })

  it('commits a moved connection as delete plus recreate with a fresh identity', () => {
    const store = CanvasGraphStore.fromSnapshot(sampleSnapshot())
    const moved = connection(2, 2, 1)

    const summary = commitCanvasGraphConnectIntent(store, {
      connection: moved,
      allowsMultipleTargetConnections: false,
    })

    expect(summary).toMatchObject({
      reason: 'connect',
      changedEdgeIds: [toCanvasGraphEdgeId(2)],
      removedEdgeIds: [toCanvasGraphEdgeId(1)],
    })
    // The old identity is gone; it was never patched to the new endpoints.
    expect(store.getEdge(toCanvasGraphEdgeId(1))).toBeUndefined()
    expect(store.getEdge(toCanvasGraphEdgeId(2))?.connection).toEqual(moved)
    expect(store.exportSnapshot().connections).toEqual([moved])
  })

  it('projects snapshot edges with verbatim canonical endpoints', () => {
    const snapshot = sampleSnapshot()
    const flow = boardSnapshotToFlow(snapshot)

    expect(flow.edges).toEqual([
      expect.objectContaining({
        id: 'connection-1',
        source: 'image-1',
        sourceHandle: 'out:image',
        target: 'prompt-1',
        targetHandle: 'in:image',
      }),
    ])

    const restored = flowToBoardSnapshot(flow)
    expect(restored.connections).toEqual(snapshot.connections)
  })

  it('materializes moved edges from their current endpoints without stale endpoint leakage', () => {
    const base = sampleSnapshot()
    const flow = boardSnapshotToFlow(base)
    const movedEdges = flow.edges.map((edge) =>
      edge.id === 'connection-1'
        ? { ...edge, source: 'image-2', data: { connectionSnapshot: { id: 1 } } }
        : edge,
    )

    const materialized = flowToBoardSnapshot(
      { nodes: flow.nodes, edges: movedEdges },
      base,
    )

    expect(materialized.connections).toEqual([
      {
        fromNodeType: 'image',
        fromNodeId: 2,
        fromPort: 'image',
        toNodeType: 'prompt',
        toNodeId: 1,
        toPort: 'image',
        id: expect.any(Number),
      },
    ])
    expect(
      toCanvasConnectionEndpointKey({
        fromNodeType: 'image',
        fromNodeId: 2,
        fromPort: 'image',
        toNodeType: 'prompt',
        toNodeId: 1,
        toPort: 'image',
      }),
    ).toBe('image:2:image:prompt:1:image')
  })

  it('resolves consumers from the current endpoints only', () => {
    const options = {
      nodes: [
        {
          id: 'prompt-1',
          data: { kind: 'prompt', snapshotNode: { text: 'alpha' } },
        },
        {
          id: 'prompt-2',
          data: { kind: 'prompt', snapshotNode: { text: 'beta' } },
        },
        { id: 'image-9', data: { kind: 'image', snapshotNode: {} } },
      ],
      edges: [
        {
          source: 'prompt-1',
          sourceHandle: 'out:prompt',
          target: 'image-9',
          targetHandle: 'in:prompt',
        },
      ],
    }

    expect(resolveConnectedPromptText(options, 'image-9')).toBe('alpha')
    expect(
      resolveConnectedPromptText(
        {
          ...options,
          edges: [
            {
              source: 'prompt-2',
              sourceHandle: 'out:prompt',
              target: 'image-9',
              targetHandle: 'in:prompt',
            },
          ],
        },
        'image-9',
      ),
    ).toBe('beta')
    expect(resolveConnectedPromptText({ ...options, edges: [] }, 'image-9')).toBe('')
  })

  it('exposes no edge-endpoint mutation on the Vue Flow surface adapter', () => {
    const nodes = shallowRef<Node[]>([])
    const edges = shallowRef<Edge[]>([])
    const adapter = useCanvasVueFlowGraphAdapter({ nodes, edges })

    expect('updateEdge' in adapter).toBe(false)
    expect(typeof adapter.addEdges).toBe('function')
    expect(typeof adapter.removeEdges).toBe('function')
  })
})
